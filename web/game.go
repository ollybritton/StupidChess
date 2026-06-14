package web

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
	"github.com/ollybritton/StupidChess/uciclient"
)

// defaultMoveTime bounds how long an engine thinks per move in the UI. Without it the engines fall
// back to their time manager with a one-hour default clock, which is minutes per move.
const defaultMoveTime = 1000 * time.Millisecond

// histMove is one played move, recorded for the move list and for replaying to engines.
type histMove struct {
	uci    string
	color  position.Color
	number int
}

// playerSlot is one side's player: either a human or a running engine subprocess.
type playerSlot struct {
	name string // "human" or an engine name
	sess *uciclient.GUISession
}

func (p playerSlot) isEngine() bool { return p.name != "" && p.name != "human" }

// Game owns all game state and runs as a single goroutine (an actor): every mutation is a closure sent
// on cmds and executed serially, so the state needs no locks. Engine searches are slow, so they run on
// their own goroutines and post their results back as closures; a monotonic epoch lets the actor
// discard results from searches that a new game or position change has superseded.
type Game struct {
	cmds chan func()
	emit func(ev any)

	specs map[string]EngineSpec

	pos      *position.Position
	startFEN string
	history  []histMove

	white playerSlot
	black playerSlot

	mode    string // "auto" | "manual" (engine-vs-engine autoplay)
	options search.SearchOptions

	epoch        int
	searching    bool
	thinkingSide position.Color
}

// NewGame constructs the game actor. specs maps each selectable engine name to how it is launched;
// emit broadcasts an event to all connected clients.
func NewGame(specs map[string]EngineSpec, emit func(ev any)) *Game {
	pos, _ := position.NewPositionFromFEN(position.StartingPosition)

	options := search.NewDeafultOptions()
	options.MoveTime = defaultMoveTime

	g := &Game{
		cmds:     make(chan func(), 64),
		emit:     emit,
		specs:    specs,
		pos:      pos,
		startFEN: position.StartingPosition,
		mode:     "auto",
		options:  options,
		white:    playerSlot{name: "human"},
		black:    playerSlot{name: "human"},
	}

	go func() {
		for fn := range g.cmds {
			fn()
		}
	}()

	return g
}

// do runs fn on the actor goroutine.
func (g *Game) do(fn func()) { g.cmds <- fn }

// doSync runs fn on the actor goroutine and waits for it to return an error.
func (g *Game) doSync(fn func() error) error {
	reply := make(chan error, 1)
	g.do(func() { reply <- fn() })
	return <-reply
}

func (g *Game) slot(c position.Color) playerSlot {
	if c == position.White {
		return g.white
	}
	return g.black
}

// ---- queries ---------------------------------------------------------------

func (g *Game) result() (inProgress bool, result *string, reason *string) {
	if g.pos.MovesLegal().Len() == 0 {
		if g.pos.KingInCheck(g.pos.SideToMove) {
			r := "1-0"
			if g.pos.SideToMove == position.White {
				r = "0-1" // the side to move is checkmated, so the other side wins
			}
			reasonStr := "checkmate"
			return false, &r, &reasonStr
		}
		r := "1/2-1/2"
		reasonStr := "stalemate"
		return false, &r, &reasonStr
	}

	if g.pos.HalfmoveClock >= 100 {
		r := "1/2-1/2"
		reasonStr := "fifty-move"
		return false, &r, &reasonStr
	}

	return true, nil, nil
}

// dests maps each from-square to the squares the side to move may legally move it to.
func (g *Game) dests() map[string][]string {
	d := map[string][]string{}
	for _, m := range g.pos.MovesLegal().AsSlice() {
		s := m.String()
		from, to := s[0:2], s[2:4]
		dup := false
		for _, existing := range d[from] {
			if existing == to {
				dup = true
				break
			}
		}
		if !dup {
			d[from] = append(d[from], to)
		}
	}
	return d
}

func (g *Game) historyUCI() []string {
	out := make([]string, len(g.history))
	for i, h := range g.history {
		out[i] = h.uci
	}
	return out
}

func (g *Game) stateEvent() stateEvent {
	inProgress, result, reason := g.result()

	dests := map[string][]string{}
	if inProgress && !g.searching && !g.slot(g.pos.SideToMove).isEngine() {
		dests = g.dests()
	}

	var lastMove []string
	if len(g.history) > 0 {
		u := g.history[len(g.history)-1].uci
		lastMove = []string{u[0:2], u[2:4]}
	}

	var thinking *string
	if g.searching {
		t := colorString(g.thinkingSide)
		thinking = &t
	}

	moves := make([]moveEntry, len(g.history))
	for i, h := range g.history {
		moves[i] = moveEntry{UCI: h.uci, Color: colorString(h.color), Number: h.number}
	}

	return stateEvent{
		Type:       "state",
		FEN:        g.pos.StringFEN(),
		Turn:       colorString(g.pos.SideToMove),
		Dests:      dests,
		LastMove:   lastMove,
		Check:      g.pos.KingInCheck(g.pos.SideToMove),
		InProgress: inProgress,
		Result:     result,
		Reason:     reason,
		Moves:      moves,
		Players:    playersInfo{White: g.white.name, Black: g.black.name},
		Thinking:   thinking,
		Mode:       g.mode,
		Flipped:    false,
	}
}

func (g *Game) broadcast() { g.emit(g.stateEvent()) }

func (g *Game) emitError(format string, a ...interface{}) {
	g.emit(errorEvent{Type: "error", Message: fmt.Sprintf(format, a...)})
}

// ---- mutations (all run on the actor goroutine) ----------------------------

// findLegal returns the fully-described legal move matching a UCI string, if any.
func (g *Game) findLegal(uciMove string) (position.Move, bool) {
	for _, m := range g.pos.MovesLegal().AsSlice() {
		if m.String() == uciMove {
			return m, true
		}
	}
	return position.NoMove, false
}

func (g *Game) applyMove(uciMove string) bool {
	m, ok := g.findLegal(uciMove)
	if !ok {
		return false
	}
	color := g.pos.SideToMove
	number := int(g.pos.FullMoves)
	g.pos.MakeMove(m)
	g.history = append(g.history, histMove{uci: uciMove, color: color, number: number})
	return true
}

// reset tears down any running engines and starts a fresh game with the given players and FEN. It is
// the single path used by both "new game" and "set fen", so an in-flight search is always cleanly
// abandoned (the old subprocesses are closed; their results are discarded by the epoch check).
func (g *Game) reset(white, black, fen string) error {
	for _, name := range []string{white, black} {
		if name != "human" && !g.knownEngine(name) {
			return fmt.Errorf("unknown engine %q", name)
		}
	}

	if fen == "" {
		fen = position.StartingPosition
	}
	pos, err := position.NewPositionFromFEN(fen)
	if err != nil {
		return fmt.Errorf("invalid FEN: %w", err)
	}

	g.teardownEngines()

	g.epoch++
	g.searching = false
	g.pos = pos
	g.startFEN = fen
	g.history = nil

	white2, err := g.makeSlot(white, position.White)
	if err != nil {
		return err
	}
	black2, err := g.makeSlot(black, position.Black)
	if err != nil {
		g.closeSlot(white2)
		return err
	}
	g.white = white2
	g.black = black2

	g.broadcast()
	g.maybeTriggerEngine()
	return nil
}

func (g *Game) knownEngine(name string) bool {
	_, ok := g.specs[name]
	return ok
}

func (g *Game) makeSlot(name string, color position.Color) (playerSlot, error) {
	if name == "human" {
		return playerSlot{name: "human"}, nil
	}

	spec, ok := g.specs[name]
	if !ok {
		return playerSlot{}, fmt.Errorf("unknown engine %q", name)
	}

	sess, err := uciclient.NewGUISessionFromBinary(spec.Path, spec.Args...)
	if err != nil {
		return playerSlot{}, fmt.Errorf("couldn't start engine %q: %w", name, err)
	}

	colorStr := colorString(color)
	sess.Trace = func(dir, line string) {
		g.emit(uciEvent{Type: "uci", Engine: name, Color: colorStr, Dir: dir, Line: line})
	}

	if err := sess.Open(); err != nil {
		sess.Close()
		return playerSlot{}, fmt.Errorf("couldn't initialise engine %q: %w", name, err)
	}
	if err := sess.NewGame(); err != nil {
		sess.Close()
		return playerSlot{}, fmt.Errorf("engine %q not ready: %w", name, err)
	}

	return playerSlot{name: name, sess: sess}, nil
}

func (g *Game) closeSlot(p playerSlot) {
	if p.sess != nil {
		p.sess.Close()
	}
}

func (g *Game) teardownEngines() {
	g.closeSlot(g.white)
	g.closeSlot(g.black)
	g.white = playerSlot{name: "human"}
	g.black = playerSlot{name: "human"}
}

// maybeTriggerEngine starts the side-to-move engine searching if it should move now.
func (g *Game) maybeTriggerEngine() {
	if g.searching {
		return
	}
	if inProgress, _, _ := g.result(); !inProgress {
		return
	}
	if !g.slot(g.pos.SideToMove).isEngine() {
		return
	}
	// In an engine-vs-engine game, manual mode waits for an explicit step/go.
	if g.white.isEngine() && g.black.isEngine() && g.mode == "manual" {
		return
	}
	g.startSearch()
}

func (g *Game) startSearch() {
	slot := g.slot(g.pos.SideToMove)
	if slot.sess == nil {
		return
	}

	g.searching = true
	g.thinkingSide = g.pos.SideToMove

	epoch := g.epoch
	color := g.pos.SideToMove
	sess := slot.sess
	fen := g.startFEN
	moves := g.historyUCI()
	opts := g.options

	g.broadcast() // reflect the "thinking" state immediately

	go func() {
		if err := sess.SetPosition(fen, moves); err != nil {
			g.do(func() { g.finishSearch(epoch, position.NoMove, err) })
			return
		}
		colorStr := colorString(color)
		move, err := sess.Search(opts, func(line string) {
			if ev := parseInfo(line, colorStr); ev != nil {
				g.emit(ev)
			}
		})
		g.do(func() { g.finishSearch(epoch, move, err) })
	}()
}

func (g *Game) finishSearch(epoch int, move position.Move, err error) {
	if epoch != g.epoch {
		return // superseded by a new game / position; ignore this result
	}

	g.searching = false

	if err != nil {
		g.emitError("engine error: %s", err)
		g.broadcast()
		return
	}

	if !g.applyMove(move.String()) {
		g.emitError("engine played illegal move %q", move.String())
		g.broadcast()
		return
	}

	g.broadcast()
	g.maybeTriggerEngine()
}

// ---- actions invoked from HTTP handlers ------------------------------------

func (g *Game) NewGameAction(white, black, fen string) error {
	return g.doSync(func() error { return g.reset(white, black, fen) })
}

func (g *Game) SetFEN(fen string) error {
	return g.doSync(func() error { return g.reset(g.white.name, g.black.name, fen) })
}

func (g *Game) UserMove(from, to, promotion string) error {
	return g.doSync(func() error {
		if inProgress, _, _ := g.result(); !inProgress {
			return errors.New("the game is over")
		}
		if g.searching {
			return errors.New("an engine is currently thinking")
		}
		if g.slot(g.pos.SideToMove).isEngine() {
			return errors.New("it is not a human's turn")
		}

		uciMove := strings.ToLower(from + to + promotion)
		if !g.applyMove(uciMove) {
			return fmt.Errorf("illegal move %q", uciMove)
		}

		g.broadcast()
		g.maybeTriggerEngine()
		return nil
	})
}

func (g *Game) SetControl(mode string) error {
	return g.doSync(func() error {
		if mode != "auto" && mode != "manual" {
			return fmt.Errorf("invalid mode %q", mode)
		}
		g.mode = mode
		g.broadcast()
		if mode == "auto" {
			g.maybeTriggerEngine()
		}
		return nil
	})
}

// StepOrGo makes the side-to-move engine play exactly one move. It backs both /api/step and /api/go.
func (g *Game) StepOrGo() error {
	return g.doSync(func() error {
		if g.searching {
			return errors.New("an engine is already thinking")
		}
		if inProgress, _, _ := g.result(); !inProgress {
			return errors.New("the game is over")
		}
		if !g.slot(g.pos.SideToMove).isEngine() {
			return errors.New("the side to move is not an engine")
		}
		g.startSearch()
		return nil
	})
}

func (g *Game) SetOptions(movetime, depth *int) error {
	return g.doSync(func() error {
		if movetime != nil {
			if *movetime > 0 {
				g.options.MoveTime = time.Duration(*movetime) * time.Millisecond
			} else {
				g.options.MoveTime = 0 // 0 lets the engine manage its own time
			}
		}
		if depth != nil && *depth > 0 {
			g.options.Depth = uint(*depth)
		}
		return nil
	})
}

// Snapshot returns the current state event (used to prime a newly connected client).
func (g *Game) Snapshot() stateEvent {
	reply := make(chan stateEvent, 1)
	g.do(func() { reply <- g.stateEvent() })
	return <-reply
}

// ---- helpers ---------------------------------------------------------------

func colorString(c position.Color) string {
	if c == position.White {
		return "w"
	}
	return "b"
}

func atoiAt(fields []string, i int) (int, bool) {
	if i < 0 || i >= len(fields) {
		return 0, false
	}
	v, err := strconv.Atoi(fields[i])
	if err != nil {
		return 0, false
	}
	return v, true
}

// parseInfo turns a UCI "info" line into an info event, or nil if it carries no search data (e.g.
// "info string ..." status lines).
func parseInfo(line, color string) *infoEvent {
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] != "info" || fields[1] == "string" {
		return nil
	}

	ev := &infoEvent{Type: "info", Color: color}
	got := false

	i := 1
	for i < len(fields) {
		switch fields[i] {
		case "depth":
			if v, ok := atoiAt(fields, i+1); ok {
				ev.Depth = &v
				got = true
			}
			i += 2
		case "nodes":
			if v, ok := atoiAt(fields, i+1); ok {
				ev.Nodes = &v
				got = true
			}
			i += 2
		case "nps":
			if v, ok := atoiAt(fields, i+1); ok {
				ev.NPS = &v
				got = true
			}
			i += 2
		case "time":
			if v, ok := atoiAt(fields, i+1); ok {
				ev.TimeMs = &v
				got = true
			}
			i += 2
		case "score":
			if i+2 < len(fields) {
				typ := fields[i+1]
				if v, ok := atoiAt(fields, i+2); ok && (typ == "cp" || typ == "mate") {
					ev.Score = &scoreInfo{Type: typ, Value: v}
					got = true
				}
			}
			i += 3
		case "pv":
			ev.PV = append([]string{}, fields[i+1:]...)
			got = true
			i = len(fields)
		case "currmove", "currmovenumber", "seldepth", "multipv", "hashfull", "tbhits":
			i += 2 // keyword plus its value
		default:
			i++
		}
	}

	if !got {
		return nil
	}
	return ev
}
