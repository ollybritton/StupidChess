package lichess

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSideToMove(t *testing.T) {
	const blackToMove = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR b KQkq - 0 1"

	assert.Equal(t, position.White, sideToMove(position.StartingPosition, 0))
	assert.Equal(t, position.Black, sideToMove(position.StartingPosition, 1))
	assert.Equal(t, position.White, sideToMove(position.StartingPosition, 2))

	// When the starting FEN says black moves first, the parities flip.
	assert.Equal(t, position.Black, sideToMove(blackToMove, 0))
	assert.Equal(t, position.White, sideToMove(blackToMove, 1))
}

func TestSplitMoves(t *testing.T) {
	assert.Empty(t, splitMoves(""))
	assert.Equal(t, []string{"e2e4"}, splitMoves("e2e4"))
	assert.Equal(t, []string{"e2e4", "e7e5", "g1f3"}, splitMoves("e2e4 e7e5 g1f3"))
}

func TestAcceptStandard(t *testing.T) {
	assert.True(t, AcceptStandard(Challenge{Variant: Variant{Key: "standard"}}))
	assert.True(t, AcceptStandard(Challenge{})) // missing key defaults to standard
	assert.False(t, AcceptStandard(Challenge{Variant: Variant{Key: "chess960"}}))
	assert.False(t, AcceptStandard(Challenge{Variant: Variant{Key: "atomic"}}))
}

func TestResolveFEN(t *testing.T) {
	assert.Equal(t, position.StartingPosition, resolveFEN("startpos"))
	assert.Equal(t, position.StartingPosition, resolveFEN(""))
	custom := "8/8/8/8/8/8/4k3/4K3 w - - 0 1"
	assert.Equal(t, custom, resolveFEN(custom))
}

// stubMover is a Mover that always returns the same move and records how it was called.
type stubMover struct {
	move       string
	lastFEN    string
	lastMoves  []string
	closeCalls int
}

func (m *stubMover) Move(fen string, moves []string, _ search.SearchOptions) (string, error) {
	m.lastFEN, m.lastMoves = fen, moves
	return m.move, nil
}

func (m *stubMover) Close() error {
	m.closeCalls++
	return nil
}

func writeLine(t *testing.T, w http.ResponseWriter, line string) {
	t.Helper()
	_, err := fmt.Fprint(w, line+"\n")
	require.NoError(t, err)
	w.(http.Flusher).Flush()
}

// TestBotPlaysOurTurn exercises the whole loop against a fake Lichess: a gameStart event leads to a
// game stream where it is our turn, and the bot should POST the stub engine's move.
func TestBotPlaysOurTurn(t *testing.T) {
	played := make(chan string, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/account", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":"stupidchess","username":"StupidChess"}`)
	})
	mux.HandleFunc("/api/stream/event", func(w http.ResponseWriter, r *http.Request) {
		writeLine(t, w, `{"type":"gameStart","game":{"id":"abc123"}}`)
		<-r.Context().Done() // hold the stream open until the client disconnects
	})
	mux.HandleFunc("/api/bot/game/stream/", func(w http.ResponseWriter, r *http.Request) {
		// We are White, standard start, and it is move one: our turn.
		writeLine(t, w, `{"type":"gameFull","id":"abc123","initialFen":"startpos",`+
			`"white":{"id":"stupidchess"},"black":{"id":"rival"},`+
			`"state":{"type":"gameState","moves":"","status":"started","wtime":60000,"btime":60000,"winc":0,"binc":0}}`)
		<-r.Context().Done()
	})
	mux.HandleFunc("/api/bot/game/", func(w http.ResponseWriter, r *http.Request) {
		if i := strings.Index(r.URL.Path, "/move/"); i >= 0 {
			select {
			case played <- strings.TrimPrefix(r.URL.Path[i:], "/move/"):
			default:
			}
		}
		fmt.Fprint(w, `{"ok":true}`)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("test-token")
	client.baseURL = server.URL

	mover := &stubMover{move: "e2e4"}
	bot := NewBot(client, func(string) (Mover, error) { return mover, nil })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = bot.Run(ctx) }()

	select {
	case move := <-played:
		assert.Equal(t, "e2e4", move)
	case <-time.After(3 * time.Second):
		t.Fatal("bot did not play a move in time")
	}

	assert.Equal(t, position.StartingPosition, mover.lastFEN)
	assert.Empty(t, mover.lastMoves)
}

// TestBotWaitsWhenNotOurTurn: when the game state shows the opponent to move, the bot must not play.
func TestBotWaitsWhenNotOurTurn(t *testing.T) {
	played := make(chan string, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/account", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":"stupidchess","username":"StupidChess"}`)
	})
	mux.HandleFunc("/api/stream/event", func(w http.ResponseWriter, r *http.Request) {
		writeLine(t, w, `{"type":"gameStart","game":{"id":"g2"}}`)
		<-r.Context().Done()
	})
	mux.HandleFunc("/api/bot/game/stream/", func(w http.ResponseWriter, r *http.Request) {
		// We are Black, but no moves have been played, so it is White's (the opponent's) turn.
		writeLine(t, w, `{"type":"gameFull","id":"g2","initialFen":"startpos",`+
			`"white":{"id":"rival"},"black":{"id":"stupidchess"},`+
			`"state":{"type":"gameState","moves":"","status":"started"}}`)
		<-r.Context().Done()
	})
	mux.HandleFunc("/api/bot/game/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/move/") {
			played <- r.URL.Path
		}
		fmt.Fprint(w, `{"ok":true}`)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("test-token")
	client.baseURL = server.URL

	bot := NewBot(client, func(string) (Mover, error) { return &stubMover{move: "e7e5"}, nil })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = bot.Run(ctx) }()

	select {
	case <-played:
		t.Fatal("bot played a move when it was not its turn")
	case <-time.After(500 * time.Millisecond):
		// good: no move was played
	}
}
