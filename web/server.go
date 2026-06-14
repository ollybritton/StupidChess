package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"sort"
	"sync"
)

//go:embed static
var staticFiles embed.FS

// EngineSpec describes how to launch one selectable engine, plus how to present it. Name is the stable
// id used in the protocol; DisplayName and Description are for the UI. Built-in engines run this same
// binary (`stupidchess uci -e <name>`); external UCI engines like Stockfish point at their own binary.
type EngineSpec struct {
	Name        string
	DisplayName string
	Description string
	Path        string
	Args        []string
}

// Server bridges the browser to a single shared Game. Browsers receive events over a Server-Sent
// Events stream and send actions as JSON POSTs; there is no websocket dependency.
type Server struct {
	game       *Game
	engineList []engineDescriptor

	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

// NewServer builds the server from the set of selectable engines.
func NewServer(specs []EngineSpec) *Server {
	specMap := make(map[string]EngineSpec, len(specs))
	descriptors := make([]engineDescriptor, 0, len(specs))
	for _, sp := range specs {
		specMap[sp.Name] = sp
		name := sp.DisplayName
		if name == "" {
			name = sp.Name
		}
		descriptors = append(descriptors, engineDescriptor{ID: sp.Name, Name: name, Description: sp.Description})
	}
	sort.Slice(descriptors, func(i, j int) bool { return descriptors[i].Name < descriptors[j].Name })

	s := &Server{
		engineList: descriptors,
		clients:    map[chan []byte]struct{}{},
	}
	s.game = NewGame(specMap, s.broadcast)
	return s
}

// Handler returns the HTTP handler serving both the static UI and the API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/events", s.handleEvents)
	mux.HandleFunc("/api/new_game", s.action(s.handleNewGame))
	mux.HandleFunc("/api/move", s.action(s.handleMove))
	mux.HandleFunc("/api/control", s.action(s.handleControl))
	mux.HandleFunc("/api/step", s.action(s.handleStep))
	mux.HandleFunc("/api/go", s.action(s.handleGo))
	mux.HandleFunc("/api/set_fen", s.action(s.handleSetFEN))
	mux.HandleFunc("/api/set_options", s.action(s.handleSetOptions))

	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err) // the embed path is a compile-time constant; this can't happen
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))

	return mux
}

// ---- SSE broadcast ---------------------------------------------------------

func (s *Server) subscribe() chan []byte {
	ch := make(chan []byte, 64)
	s.mu.Lock()
	s.clients[ch] = struct{}{}
	s.mu.Unlock()
	return ch
}

func (s *Server) unsubscribe(ch chan []byte) {
	s.mu.Lock()
	delete(s.clients, ch)
	s.mu.Unlock()
}

// broadcast serializes an event and sends it to every connected client. Sends are non-blocking: a
// client that has fallen behind drops events rather than stalling the game (it will resync on the
// next state event).
func (s *Server) broadcast(ev any) {
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	msg := []byte("data: " + string(data) + "\n\n")

	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.subscribe()
	defer s.unsubscribe(ch)

	// Prime this client with the engine list and the current state.
	s.sendTo(w, flusher, enginesEvent{Type: "engines", List: s.engineList})
	s.sendTo(w, flusher, s.game.Snapshot())

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			if _, err := w.Write(msg); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) sendTo(w http.ResponseWriter, flusher http.Flusher, ev any) {
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

// ---- action endpoints ------------------------------------------------------

// action wraps a handler that returns an error into an HTTP handler that enforces POST and renders a
// uniform JSON result. Side effects are reported asynchronously over SSE, not in this response.
func (s *Server) action(fn func(r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "POST required"})
			return
		}
		if err := fn(r); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func (s *Server) handleNewGame(r *http.Request) error {
	var body struct {
		White string `json:"white"`
		Black string `json:"black"`
		FEN   string `json:"fen"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	if body.White == "" {
		body.White = "human"
	}
	if body.Black == "" {
		body.Black = "human"
	}
	return s.game.NewGameAction(body.White, body.Black, body.FEN)
}

func (s *Server) handleMove(r *http.Request) error {
	var body struct {
		From      string `json:"from"`
		To        string `json:"to"`
		Promotion string `json:"promotion"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	if len(body.From) != 2 || len(body.To) != 2 {
		return fmt.Errorf("from/to must be squares like e2")
	}
	return s.game.UserMove(body.From, body.To, body.Promotion)
}

func (s *Server) handleControl(r *http.Request) error {
	var body struct {
		Mode string `json:"mode"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	return s.game.SetControl(body.Mode)
}

func (s *Server) handleStep(r *http.Request) error { return s.game.StepOrGo() }
func (s *Server) handleGo(r *http.Request) error   { return s.game.StepOrGo() }

func (s *Server) handleSetFEN(r *http.Request) error {
	var body struct {
		FEN string `json:"fen"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	if body.FEN == "" {
		return fmt.Errorf("fen is required")
	}
	return s.game.SetFEN(body.FEN)
}

func (s *Server) handleSetOptions(r *http.Request) error {
	var body struct {
		MoveTime *int `json:"movetime"`
		Depth    *int `json:"depth"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	return s.game.SetOptions(body.MoveTime, body.Depth)
}

// ---- small helpers ---------------------------------------------------------

func decode(r *http.Request, v any) error {
	if r.Body == nil {
		return nil
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil && err.Error() != "EOF" {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
