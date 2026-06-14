package web

// These structs are the JSON events pushed to the browser over SSE. See web/PROTOCOL.md for the
// full contract. Pointer and slice fields marshal to null/absent when empty, which the frontend
// treats as "unknown".

type moveEntry struct {
	UCI    string `json:"uci"`
	Color  string `json:"color"`
	Number int    `json:"number"`
}

type playersInfo struct {
	White string `json:"white"`
	Black string `json:"black"`
}

type stateEvent struct {
	Type       string              `json:"type"`
	FEN        string              `json:"fen"`
	Turn       string              `json:"turn"`
	Dests      map[string][]string `json:"dests"`
	LastMove   []string            `json:"lastMove"`
	Check      bool                `json:"check"`
	InProgress bool                `json:"inProgress"`
	Result     *string             `json:"result"`
	Reason     *string             `json:"reason"`
	Moves      []moveEntry         `json:"moves"`
	Players    playersInfo         `json:"players"`
	Thinking   *string             `json:"thinking"`
	Mode       string              `json:"mode"`
	OwnBook    bool                `json:"ownBook"`
	Flipped    bool                `json:"flipped"`
}

type scoreInfo struct {
	Type  string `json:"type"` // "cp" or "mate"
	Value int    `json:"value"`
}

type infoEvent struct {
	Type   string     `json:"type"`
	Color  string     `json:"color"`
	Depth  *int       `json:"depth,omitempty"`
	Score  *scoreInfo `json:"score,omitempty"`
	Nodes  *int       `json:"nodes,omitempty"`
	NPS    *int       `json:"nps,omitempty"`
	TimeMs *int       `json:"timeMs,omitempty"`
	PV     []string   `json:"pv,omitempty"`
}

type uciEvent struct {
	Type   string `json:"type"`
	Engine string `json:"engine"`
	Color  string `json:"color"`
	Dir    string `json:"dir"`
	Line   string `json:"line"`
}

// engineDescriptor presents one selectable engine: a stable id, a human-friendly name, and a blurb.
type engineDescriptor struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type enginesEvent struct {
	Type string             `json:"type"`
	List []engineDescriptor `json:"list"`
}

type errorEvent struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}
