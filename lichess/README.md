# Playing on Lichess

This package connects a StupidChess engine to the [Lichess Bot API](https://lichess.org/api#tag/Bot):
it streams events, accepts challenges, and plays each game by driving an engine subprocess.

## One-time setup

1. Create a **fresh** Lichess account for the bot (a BOT account cannot have played any games as a
   human, so don't use your personal account).
2. Generate an API token with the **`bot:play`** scope at
   <https://lichess.org/account/oauth/token>.
3. Upgrade the account to a BOT account (irreversible):

   ```
   stupidchess lichess upgrade --token <token>
   ```

You can also export the token instead of passing `--token` each time:

```
export LICHESS_TOKEN=<token>
```

## Running the bot

Pick the engine with `-e` and start playing:

```
stupidchess lichess play -e fortress
stupidchess lichess play -e soloist
stupidchess lichess play -e tryhard --greeting "gl hf 🤖"
```

The bot then:

- accepts standard-chess challenges (and declines variants it can't play),
- plays every game it's in, one engine subprocess per game (so it can play several at once),
- passes each game's clock through to the engine, so time-managing engines (tryhard) pace themselves
  while the instant personality engines just move.

Challenge the bot by visiting its Lichess profile and clicking **Challenge to a game**, or send it an
open challenge. Stop the bot with ctrl-c.

## How it works

- `client.go` — a thin authenticated wrapper over the Lichess HTTP API, including NDJSON streaming.
- `engine.go` — `EngineMover`, which drives a `stupidchess uci -e <name>` subprocess via the
  `uciclient` package and returns its best move.
- `bot.go` — the event loop: account identity, challenge handling, and one `playGame` goroutine per
  game that detects whose turn it is and posts moves.

The engine choice is the only "personality" knob; see the engine descriptions (`stupidchess serve`
lists them) for what each one does.
