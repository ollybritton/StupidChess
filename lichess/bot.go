package lichess

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
)

// reconnectDelay is how long to wait before reopening the event stream after it drops.
const reconnectDelay = 5 * time.Second

// Matchmaking pacing: how often to consider seeking a game, and how long to wait after sending a
// challenge before sending another (Lichess expires unaccepted real-time challenges after ~20s).
const (
	matchmakeInterval = 10 * time.Second
	challengeCooldown = 25 * time.Second
)

// staleGameTimeout is how long a game may go with no move by either side before the bot abandons it.
// It is generous enough never to fire during a legitimately slow move in the fast time controls the bot
// plays, but it rescues the bot from a game whose opponent has vanished: such a game otherwise occupies
// a game slot forever and, once MaxGames is reached, stops the bot from seeking any new games.
const staleGameTimeout = 4 * time.Minute

// errGameOver is a sentinel used to stop reading a game stream once the game has finished.
var errGameOver = errors.New("game over")

// Bot connects an engine to Lichess: it streams events, accepts challenges per its policy, and plays
// each game by driving a fresh Mover.
type Bot struct {
	client   *Client
	newMover func(gameID string) (Mover, error)

	// Accept decides whether to take an incoming challenge. Defaults to AcceptStandard.
	Accept func(Challenge) bool
	// Greeting, if non-empty, is sent once in the player chat at the start of each game.
	Greeting string
	// Logf logs progress. Defaults to a no-op.
	Logf func(format string, args ...interface{})

	// Seek, when true, makes the bot challenge online bots whenever it has fewer than MaxGames games in
	// progress, so it is almost always playing. Challenge is the time control / rating it offers.
	Seek      bool
	MaxGames  int
	Challenge ChallengeParams

	me string // lowercased account id, learned at startup

	mu       sync.Mutex      // guards games
	games    map[string]bool // game ids currently being played
	lastSeek int64           // unix nanos of the last challenge we sent (atomic)
}

// NewBot builds a bot that drives games with movers from newMover (one per game).
func NewBot(client *Client, newMover func(gameID string) (Mover, error)) *Bot {
	return &Bot{
		client:   client,
		newMover: newMover,
		Accept:   AcceptStandard,
		Logf:     func(string, ...interface{}) {},
		MaxGames: 1,
		Challenge: ChallengeParams{
			ClockLimit:     180 * time.Second,
			ClockIncrement: 2 * time.Second,
			Color:          "random",
			Variant:        "standard",
		},
		games: map[string]bool{},
	}
}

// AcceptStandard accepts standard-chess challenges and declines variants the engines cannot play.
func AcceptStandard(ch Challenge) bool {
	return ch.Variant.Key == "" || ch.Variant.Key == "standard"
}

// Run learns the account identity and then streams events until ctx is cancelled, reconnecting if the
// stream drops.
func (b *Bot) Run(ctx context.Context) error {
	acct, err := b.client.Account(ctx)
	if err != nil {
		return err
	}
	b.me = strings.ToLower(acct.ID)
	b.Logf("connected to Lichess as %s", acct.Username)

	if b.Seek {
		go b.matchmake(ctx)
	}

	for {
		err := b.streamEvents(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		b.Logf("event stream ended (%v); reconnecting in %s", err, reconnectDelay)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(reconnectDelay):
		}
	}
}

func (b *Bot) streamEvents(ctx context.Context) error {
	body, err := b.client.StreamEvents(ctx)
	if err != nil {
		return err
	}
	defer body.Close()

	return streamNDJSON(body, func(line []byte) error {
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			b.Logf("skipping malformed event: %v", err)
			return nil
		}

		switch ev.Type {
		case "challenge":
			b.handleChallenge(ctx, ev.Challenge)
		case "gameStart":
			b.startGame(ctx, ev.Game.ID)
		}
		return nil
	})
}

// startGame begins playing a game on its own goroutine, unless one is already in progress for it (the
// event stream re-sends gameStart for ongoing games when it reconnects, which would otherwise spawn a
// duplicate engine for the same game).
func (b *Bot) startGame(ctx context.Context, gameID string) {
	b.mu.Lock()
	if b.games[gameID] {
		b.mu.Unlock()
		return
	}
	b.games[gameID] = true
	b.mu.Unlock()

	go func() {
		defer func() {
			b.mu.Lock()
			delete(b.games, gameID)
			b.mu.Unlock()
		}()
		b.playGame(ctx, gameID)
	}()
}

// gameCount reports how many games are currently in progress.
func (b *Bot) gameCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.games)
}

// matchmake periodically challenges an online bot whenever the bot has room for another game, so it is
// almost always playing.
func (b *Bot) matchmake(ctx context.Context) {
	ticker := time.NewTicker(matchmakeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		if b.gameCount() >= b.MaxGames {
			continue
		}
		// Don't pile up challenges: wait for the last one to be accepted or to expire first.
		if last := atomic.LoadInt64(&b.lastSeek); last != 0 && time.Since(time.Unix(0, last)) < challengeCooldown {
			continue
		}
		b.seekGame(ctx)
	}
}

// seekGame challenges a random online bot (other than ourselves).
func (b *Bot) seekGame(ctx context.Context) {
	bots, err := b.client.OnlineBots(ctx, 50)
	if err != nil {
		b.Logf("matchmaking: could not list online bots: %v", err)
		return
	}

	candidates := bots[:0:0]
	for _, name := range bots {
		if strings.ToLower(name) != b.me {
			candidates = append(candidates, name)
		}
	}
	if len(candidates) == 0 {
		return
	}

	target := candidates[rand.Intn(len(candidates))]
	atomic.StoreInt64(&b.lastSeek, time.Now().UnixNano())

	if err := b.client.Challenge(ctx, target, b.Challenge); err != nil {
		b.Logf("matchmaking: could not challenge %s: %v", target, err)
		return
	}
	b.Logf("matchmaking: challenged %s", target)
}

func (b *Bot) handleChallenge(ctx context.Context, ch Challenge) {
	// Ignore our own outgoing challenges echoed back to us.
	if strings.ToLower(ch.Challenger.ID) == b.me {
		return
	}

	if b.Accept != nil && b.Accept(ch) {
		if err := b.client.AcceptChallenge(ctx, ch.ID); err != nil {
			b.Logf("could not accept challenge %s: %v", ch.ID, err)
			return
		}
		b.Logf("accepted challenge %s from %s (%s %s)", ch.ID, ch.Challenger.Name, ch.Speed, ch.Variant.Key)
		return
	}

	if err := b.client.DeclineChallenge(ctx, ch.ID, "generic"); err != nil {
		b.Logf("could not decline challenge %s: %v", ch.ID, err)
		return
	}
	b.Logf("declined challenge %s from %s (%s)", ch.ID, ch.Challenger.Name, ch.Variant.Key)
}

// playGame drives one game from start to finish on its own goroutine and engine.
func (b *Bot) playGame(ctx context.Context, gameID string) {
	mover, err := b.newMover(gameID)
	if err != nil {
		b.Logf("game %s: could not start engine: %v", gameID, err)
		return
	}
	defer mover.Close()

	body, err := b.client.StreamGame(ctx, gameID)
	if err != nil {
		b.Logf("game %s: could not open stream: %v", gameID, err)
		return
	}
	defer body.Close()

	// Stall watchdog: abandon the game if neither side moves for staleGameTimeout, so a vanished opponent
	// cannot wedge the bot forever. lastActivity is the time of the last real game event (moves arrive as
	// gameState; keep-alive pings are stripped before the handler, so they don't reset it).
	gameCtx, cancelGame := context.WithCancel(ctx)
	defer cancelGame()
	var lastActivity int64
	atomic.StoreInt64(&lastActivity, time.Now().UnixNano())
	go b.watchGameStall(gameCtx, gameID, body, &lastActivity)

	var (
		myColor    position.Color
		initialFEN = position.StartingPosition
		greeted    bool
		ponder     = &ponderState{}
	)

	err = streamNDJSON(body, func(line []byte) error {
		atomic.StoreInt64(&lastActivity, time.Now().UnixNano())
		var env envelope
		if err := json.Unmarshal(line, &env); err != nil {
			return nil
		}

		switch env.Type {
		case "gameFull":
			var gf GameFull
			if err := json.Unmarshal(line, &gf); err != nil {
				return nil
			}
			initialFEN = resolveFEN(gf.InitialFEN)
			myColor = b.colorIn(gf)
			b.Logf("game %s: playing as %s", gameID, myColor)

			if b.Greeting != "" && !greeted {
				greeted = true
				if err := b.client.Chat(ctx, gameID, "player", b.Greeting); err != nil {
					b.Logf("game %s: could not send greeting: %v", gameID, err)
				}
			}
			return b.onState(ctx, gameID, mover, initialFEN, myColor, gf.State, ponder)

		case "gameState":
			var st GameState
			if err := json.Unmarshal(line, &st); err != nil {
				return nil
			}
			return b.onState(ctx, gameID, mover, initialFEN, myColor, st, ponder)
		}
		return nil
	})

	if err != nil && !errors.Is(err, errGameOver) {
		b.Logf("game %s: stream error: %v", gameID, err)
	}
	b.Logf("game %s: finished", gameID)
}

// watchGameStall abandons a game that has made no progress for staleGameTimeout: it asks Lichess to abort
// the game (falling back to resign if there are too many moves to abort) and closes the stream, which
// unblocks playGame's read so the game slot is released. It exits when the game ends normally (gameCtx
// cancelled).
func (b *Bot) watchGameStall(ctx context.Context, gameID string, body io.Closer, lastActivity *int64) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			idle := time.Since(time.Unix(0, atomic.LoadInt64(lastActivity)))
			if idle < staleGameTimeout {
				continue
			}
			b.Logf("game %s: no progress for %s, abandoning", gameID, idle.Round(time.Second))
			if err := b.client.Abort(ctx, gameID); err != nil {
				_ = b.client.Resign(ctx, gameID)
			}
			_ = body.Close() // unblock playGame's stream read so it returns and frees the slot
			return
		}
	}
}

// ponderState tracks an in-progress "think on the opponent's clock" search for one game.
type ponderState struct {
	active bool   // a ponder search is running
	move   string // the opponent reply we predicted and are pondering on
}

// onState reacts to a game snapshot: if the game is over it stops; if it is our turn it computes and
// plays a move (reusing any pondering) and then starts pondering the expected continuation.
func (b *Bot) onState(ctx context.Context, gameID string, mover Mover, initialFEN string, myColor position.Color, st GameState, ps *ponderState) error {
	if st.Status != "" && st.Status != "started" {
		b.Logf("game %s: over (%s)", gameID, st.Status)
		return errGameOver
	}

	moves := splitMoves(st.Moves)
	if sideToMove(initialFEN, len(moves)) != myColor {
		return nil // opponent's turn: keep pondering (if we are) and wait
	}

	opts := clockOptions(st)
	best, ponder, err := b.think(mover, initialFEN, moves, opts, ps)
	if err != nil {
		b.Logf("game %s: engine error: %v", gameID, err)
		return nil
	}
	if best == "" || best == "0000" {
		return nil // no move (game already decided)
	}

	if err := b.client.MakeMove(ctx, gameID, best); err != nil {
		b.Logf("game %s: could not play %s: %v", gameID, best, err)
		return nil
	}
	b.Logf("game %s: played %s", gameID, best)

	b.startPonder(mover, initialFEN, moves, best, ponder, opts, ps)
	return nil
}

// think produces our move. If we were pondering and the opponent played exactly the move we predicted,
// the ponder search is already a search of the real position, so a ponderhit hands us its result for
// free; otherwise we abandon it and search the actual position.
func (b *Bot) think(mover Mover, initialFEN string, moves []string, opts search.SearchOptions, ps *ponderState) (best, ponder string, err error) {
	if ps.active {
		ps.active = false

		lastMove := ""
		if len(moves) > 0 {
			lastMove = moves[len(moves)-1]
		}
		if ps.move != "" && lastMove == ps.move {
			return mover.PonderHit()
		}
		_ = mover.StopPonder()
	}
	return mover.MoveWithPonder(initialFEN, moves, opts)
}

// startPonder begins thinking about the position after our move and the reply we expect, so that a
// correct prediction turns into a head start. It is a no-op when the engine offered no prediction.
func (b *Bot) startPonder(mover Mover, initialFEN string, moves []string, best, ponder string, opts search.SearchOptions, ps *ponderState) {
	ps.active = false
	if ponder == "" {
		return
	}

	predicted := append(append([]string{}, moves...), best, ponder)
	if err := mover.StartPonder(initialFEN, predicted, opts); err != nil {
		return
	}
	ps.active = true
	ps.move = ponder
}

// colorIn reports which colour we are playing in a game.
func (b *Bot) colorIn(gf GameFull) position.Color {
	if strings.ToLower(gf.White.ID) == b.me {
		return position.White
	}
	return position.Black
}

// resolveFEN turns Lichess's initialFen ("startpos" or a FEN) into a concrete FEN.
func resolveFEN(initialFEN string) string {
	if initialFEN == "" || initialFEN == "startpos" {
		return position.StartingPosition
	}
	return initialFEN
}

// splitMoves splits the space-separated UCI move list, returning an empty slice for an empty string.
func splitMoves(moves string) []string {
	return strings.Fields(moves)
}

// sideToMove returns whose turn it is after moveCount half-moves from a position whose FEN names the
// side that starts.
func sideToMove(initialFEN string, moveCount int) position.Color {
	start := position.White
	if fields := strings.Fields(initialFEN); len(fields) >= 2 && fields[1] == "b" {
		start = position.Black
	}
	if moveCount%2 == 1 {
		return start.Invert()
	}
	return start
}

// clockOptions builds search options from a game state's clocks, so time-managing engines (tryhard)
// can pace themselves. Engines that ignore time are unaffected.
func clockOptions(st GameState) search.SearchOptions {
	opts := search.NewDeafultOptions()
	if st.WTime > 0 {
		opts.WhiteTimeRemaining = time.Duration(st.WTime) * time.Millisecond
	}
	if st.BTime > 0 {
		opts.BlackTimeRemaining = time.Duration(st.BTime) * time.Millisecond
	}
	opts.WhiteIncrement = time.Duration(st.WInc) * time.Millisecond
	opts.BlackIncrement = time.Duration(st.BInc) * time.Millisecond
	return opts
}
