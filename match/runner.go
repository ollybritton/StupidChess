package match

import (
	"context"
	"runtime"
	"sync"
)

// Config parameterises a match run.
type Config struct {
	A, B        EngineSpec   // the two engines; results are reported from A's perspective
	TC          TimeControl  // per-move search budget
	MaxGames    int          // hard cap on games played (the SPRT may stop sooner)
	Concurrency int          // games in flight at once (default: NumCPU)
	MaxPlies    int          // game-length cap, scored as a draw (default: 400)
	SPRT        *SPRT        // sequential test; if nil, the match runs to MaxGames
	Openings    []string     // book lines (default: defaultOpenings)
	OnProgress  func(Stats)  // called after each completed game, for live reporting
}

// Stats is the running tally of a match, from engine A's perspective.
type Stats struct {
	Wins, Draws, Losses int
	Errors              int
	LLR                 float64
	Lower, Upper        float64
	Elo, EloMargin      float64
	Verdict             Verdict
}

// Games returns the number of decided games counted so far.
func (s Stats) Games() int { return s.Wins + s.Draws + s.Losses }

type gameSpec struct {
	opening []string
	swap    bool // if true, engine B plays White
}

type gameResult struct {
	outcome Outcome
	swap    bool
	err     error
}

// Run plays the match described by cfg, updating the SPRT after each game and
// stopping early once a hypothesis is accepted. It returns the final tally.
func Run(ctx context.Context, cfg Config) (Stats, error) {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = runtime.NumCPU()
	}
	if cfg.MaxPlies <= 0 {
		cfg.MaxPlies = 400
	}
	if len(cfg.Openings) == 0 {
		cfg.Openings = defaultOpenings
	}

	// Pre-flight: build one pair of sessions to surface a bad binary/option before
	// spinning up the whole pool, then close them.
	if a, err := newSession(cfg.A); err != nil {
		return Stats{}, err
	} else {
		a.close()
	}
	if b, err := newSession(cfg.B); err != nil {
		return Stats{}, err
	} else {
		b.close()
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	feed := make(chan gameSpec)
	results := make(chan gameResult, cfg.Concurrency)

	var wg sync.WaitGroup
	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runWorker(ctx, cfg, feed, results)
		}()
	}

	// Feeder: emit each opening twice (colours reversed) in an endless cycle until
	// the context is cancelled (decision reached or MaxGames hit).
	go func() {
		defer close(feed)
		for round := 0; ; round++ {
			opening := openingMoves(cfg.Openings[round%len(cfg.Openings)])
			for _, swap := range []bool{false, true} {
				select {
				case <-ctx.Done():
					return
				case feed <- gameSpec{opening: opening, swap: swap}:
				}
			}
		}
	}()

	go func() { wg.Wait(); close(results) }()

	var stats Stats
	if cfg.SPRT != nil {
		stats.Lower, stats.Upper = cfg.SPRT.Lower(), cfg.SPRT.Upper()
	}
	// minGames stops a lucky early streak from deciding before there is real
	// evidence; the regularised LLR keeps it finite, this keeps it honest.
	const minGames = 16
	stopped := false
	var final Stats // latched at the moment of decision, so in-flight games don't overwrite it

	for res := range results {
		switch {
		case res.err != nil:
			stats.Errors++
		default:
			countResult(&stats, res)
		}

		if cfg.SPRT != nil {
			stats.LLR = cfg.SPRT.LLR(stats.Wins, stats.Draws, stats.Losses)
			stats.Verdict = cfg.SPRT.Decide(stats.Wins, stats.Draws, stats.Losses)
		}
		stats.Elo, stats.EloMargin = EloWithError(stats.Wins, stats.Draws, stats.Losses)

		if cfg.OnProgress != nil {
			cfg.OnProgress(stats)
		}

		if !stopped {
			decided := cfg.SPRT != nil && stats.Verdict != Continue && stats.Games() >= minGames
			if decided || (cfg.MaxGames > 0 && stats.Games() >= cfg.MaxGames) {
				stopped = true
				final = stats   // report the result as it stood when we decided to stop
				cancel()        // stop the feeder; workers drain and exit, then results closes
			}
		}
	}

	if stopped {
		return final, nil
	}
	return stats, nil
}

// countResult folds one game's outcome into the tally from A's perspective,
// accounting for the colour swap.
func countResult(stats *Stats, res gameResult) {
	if res.outcome == Draw {
		stats.Draws++
		return
	}
	whiteWon := res.outcome == WhiteWins
	aWasWhite := !res.swap
	if whiteWon == aWasWhite {
		stats.Wins++
	} else {
		stats.Losses++
	}
}

// runWorker owns one pair of engine sessions and plays games until the feed is
// closed. A game that errors (engine crash or protocol failure) is reported and
// the sessions are rebuilt before continuing.
func runWorker(ctx context.Context, cfg Config, feed <-chan gameSpec, results chan<- gameResult) {
	a, err := newSession(cfg.A)
	if err != nil {
		return
	}
	defer a.close()
	b, err := newSession(cfg.B)
	if err != nil {
		return
	}
	defer b.close()

	for spec := range feed {
		_ = a.gui.NewGame()
		_ = b.gui.NewGame()

		white, black := a, b
		if spec.swap {
			white, black = b, a
		}

		outcome, gerr := playGame(white, black, spec.opening, cfg.TC, cfg.MaxPlies)

		select {
		case results <- gameResult{outcome: outcome, swap: spec.swap, err: gerr}:
		case <-ctx.Done():
			return
		}

		if gerr != nil {
			// A broken session can't be trusted; rebuild both before the next game.
			a.close()
			b.close()
			if a, err = newSession(cfg.A); err != nil {
				return
			}
			if b, err = newSession(cfg.B); err != nil {
				return
			}
		}
	}
}
