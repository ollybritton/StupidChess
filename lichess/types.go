package lichess

// These mirror the subset of the Lichess Bot API JSON we care about. See
// https://lichess.org/api#tag/Bot for the full schemas; absent fields are simply ignored.

// Account is the response from GET /api/account. The id is the lowercased username and is what
// appears as white.id / black.id in game streams.
type Account struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Title    string `json:"title"`
}

// Event is one entry from the GET /api/stream/event NDJSON stream.
type Event struct {
	Type      string    `json:"type"` // "challenge", "gameStart", "gameFinish", "challengeCanceled", ...
	Challenge Challenge `json:"challenge"`
	Game      EventGame `json:"game"`
}

// EventGame is the lightweight game reference carried by gameStart / gameFinish events.
type EventGame struct {
	ID string `json:"id"`
}

// Challenge describes an incoming (or outgoing) challenge.
type Challenge struct {
	ID         string       `json:"id"`
	Status     string       `json:"status"`
	Challenger ChallengeUser `json:"challenger"`
	DestUser   ChallengeUser `json:"destUser"`
	Variant    Variant      `json:"variant"`
	Rated      bool         `json:"rated"`
	Speed      string       `json:"speed"`
}

type ChallengeUser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Variant struct {
	Key  string `json:"key"` // "standard", "chess960", "atomic", ...
	Name string `json:"name"`
}

// GameFull is the first message on a game stream: the full game, including its initial position and an
// embedded first GameState.
type GameFull struct {
	Type       string    `json:"type"` // "gameFull"
	ID         string    `json:"id"`
	Variant    Variant   `json:"variant"`
	InitialFEN string    `json:"initialFen"` // "startpos" or a FEN
	White      GamePlayer `json:"white"`
	Black      GamePlayer `json:"black"`
	State      GameState `json:"state"`
}

type GamePlayer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GameState is a snapshot of the moves played and the clocks. It arrives embedded in GameFull and then
// again on every subsequent move.
type GameState struct {
	Type   string `json:"type"` // "gameState"
	Moves  string `json:"moves"` // space-separated UCI moves, e.g. "e2e4 e7e5"
	WTime  int64  `json:"wtime"` // white clock, milliseconds
	BTime  int64  `json:"btime"` // black clock, milliseconds
	WInc   int64  `json:"winc"`  // white increment, milliseconds
	BInc   int64  `json:"binc"`  // black increment, milliseconds
	Status string `json:"status"` // "started", "mate", "resign", "outoftime", ...
}

// envelope is used to peek at a stream message's type before unmarshalling it fully.
type envelope struct {
	Type string `json:"type"`
}
