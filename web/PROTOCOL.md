# StupidChess web protocol

The web UI talks to the Go backend over plain HTTP. There are two halves:

- **Server → client**: a Server-Sent Events (SSE) stream at `GET /api/events`. The server pushes
  JSON events as they happen (board state, engine thinking info, raw UCI traffic).
- **Client → server**: ordinary `POST` requests with a JSON body to `/api/*` endpoints (the user
  makes a move, starts a game, etc).

This avoids any websocket dependency. The frontend uses `EventSource` for the stream and `fetch()`
for actions. There is a single shared game on the server (this is a local debugging tool, not a
multi-user service); every connected client sees the same game and any client can drive it.

## Server → client events (SSE)

Each SSE message is a single line `data: <json>\n\n`. Every JSON object has a `type` field.

On a new connection the server immediately sends one `engines` event and one `state` event.

### `engines`
The available engines, for populating the player dropdowns. Each has a stable `id` (used in
`new_game`), a human-friendly `name`, and a `description`. The frontend adds its own "Human" option.
```json
{ "type": "engines", "list": [
  { "id": "tryhard", "name": "Try Hard", "description": "A genuine engine: alpha-beta search ..." },
  { "id": "worstfish", "name": "Worstfish", "description": "Asks Stockfish to rank every move ..." }
] }
```

### `state`
The full current game state. Sent after every change (a move, a new game, a FEN load). The frontend
should treat this as the single source of truth and re-render from it.
```json
{
  "type": "state",
  "fen": "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1",
  "turn": "w",                       // "w" or "b" — side to move
  "dests": { "e2": ["e3", "e4"], "g1": ["f3", "h3"] },  // legal moves: from-square -> [to-squares]
  "lastMove": ["e2", "e4"],          // or null
  "check": false,                    // is the side to move in check
  "inProgress": true,                // false once the game is over
  "result": null,                    // null | "1-0" | "0-1" | "1/2-1/2"
  "reason": null,                    // null | "checkmate" | "stalemate" | "fifty-move" | "insufficient material"
  "moves": [                         // full move history, in order
    { "uci": "e2e4", "color": "w", "number": 1 }
  ],
  "players": { "white": "human", "black": "tryhard" },  // "human" or an engine name
  "thinking": null,                  // "w" | "b" | null — which side's engine is currently searching
  "mode": "auto",                    // "auto" | "manual" — engine-vs-engine autoplay mode
  "ownBook": true,                   // whether book-capable engines use their opening book
  "flipped": false                   // hint only; the frontend owns board orientation
}
```
`dests` only contains entries for the side to move, and is empty when an engine is thinking or the
game is over. Squares are lowercase algebraic (`a1`..`h8`). Promotions appear as several entries with
the same to-square; the frontend prompts for the promotion piece and sends it back.

### `info`
Streamed live while an engine searches. One per UCI `info` line that carries search data. Fields are
best-effort (absent fields are omitted), so guard for missing keys.
```json
{
  "type": "info",
  "color": "w",                      // which side's engine produced this
  "depth": 5,
  "score": { "type": "cp", "value": 35 },   // type "cp" (centipawns) or "mate" (moves to mate)
  "nodes": 19384,
  "nps": 412000,
  "timeMs": 47,
  "pv": ["e2e4", "e7e5", "g1f3"]
}
```

### `uci`
Raw UCI protocol traffic, for the debug console. `dir` is `"send"` (server → engine) or `"recv"`
(engine → server).
```json
{ "type": "uci", "engine": "tryhard", "color": "w", "dir": "recv", "line": "bestmove e2e4" }
```

### `error`
```json
{ "type": "error", "message": "engine tryhard exited unexpectedly" }
```

## Client → server actions (POST JSON)

All return `200` with `{ "ok": true }` or `4xx`/`5xx` with `{ "ok": false, "error": "..." }`. Side
effects (board changes, engine moves) are reported back asynchronously via the SSE `state`/`info`
events, NOT in the POST response. So after a successful POST the frontend just waits for the next
`state` event.

| Endpoint | Body | Meaning |
|---|---|---|
| `POST /api/new_game` | `{ "white": "human", "black": "tryhard", "fen": null }` | Start a new game. `white`/`black` are `"human"` or an engine name. `fen` optional (defaults to the standard start). Spawns engine subprocesses as needed. |
| `POST /api/move` | `{ "from": "e2", "to": "e4", "promotion": "q" }` | Apply a human move. `promotion` is `"q"`/`"r"`/`"b"`/`"n"` or omitted. Rejected if it is not a legal move or it is not a human's turn. After a human move, if the opponent is an engine, the server triggers it automatically. |
| `POST /api/control` | `{ "mode": "auto" }` | Set engine-vs-engine autoplay mode: `"auto"` plays both engines through to the end; `"manual"` waits for `step`. |
| `POST /api/step` | `{}` | In an engine-vs-engine game, make the side-to-move engine play exactly one move. |
| `POST /api/go` | `{}` | Force the side-to-move engine (if any) to think and move now. Works for human-vs-engine too (hint/“move now”). |
| `POST /api/set_options` | `{ "movetime": 1000, "depth": 6, "ownBook": true }` | Set options sent to engines. All fields optional. `movetime` in ms (`0`/absent = let the engine manage its own time); `ownBook` toggles the opening book on book-capable engines (e.g. tryhard) via UCI `setoption`. |
| `POST /api/set_fen` | `{ "fen": "..." }` | Load a position from FEN, keeping the current players. |

## UI requirements (summary for the frontend)

A clean, Lichess-style single page. Centre: the board. Around it: controls and debugging panels.
See the agent brief for full detail. Board orientation (flip) is owned entirely by the frontend; it
never needs the server. The move list shows UCI long-algebraic moves (e.g. `e2e4`) since this is an
engine-debugging tool and UCI is what the engines actually speak.
