package match

import (
	"context"
	"math"
	"math/rand"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ollybritton/StupidChess/search"
)

// SPSAConfig parameterises a tuning run. The engine's tunable search options (search.TunableParams) are
// optimised by Simultaneous Perturbation Stochastic Approximation: every iteration perturbs all
// parameters at once by +/-step, plays a colour-swapped game pair between the + and - settings, and
// steps the parameter vector along the resulting one-sample gradient. The learning rate decays so the
// vector settles. Many noisy iterations beat few precise ones, which is exactly what SPSA exploits.
type SPSAConfig struct {
	Engine      EngineSpec  // the binary/engine to tune; per-iteration params are set via setoption
	TC          TimeControl // per-move search budget for the tuning games
	Iterations  int         // number of perturbation iterations (each plays a 2-game pair)
	Concurrency int         // iterations in flight at once (default: NumCPU)
	Rate0       float64     // initial learning-rate factor (default 0.3)
	HalfLife    int         // iterations over which the learning rate halves (default Iterations/2)
	Openings    []string    // book lines (default: defaultOpenings)
	Seed        int64
	OnUpdate    func(iter int, theta map[string]int) // called periodically with the current vector
}

// RunSPSA tunes the engine's search parameters and returns the final vector. The returned map is keyed by
// the UCI option names from search.TunableParams.
func RunSPSA(ctx context.Context, cfg SPSAConfig) (map[string]int, error) {
	specs := search.TunableParams()
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = runtime.NumCPU()
	}
	if len(cfg.Openings) == 0 {
		cfg.Openings = defaultOpenings
	}
	if cfg.Rate0 <= 0 {
		cfg.Rate0 = 0.3
	}
	if cfg.HalfLife <= 0 {
		cfg.HalfLife = cfg.Iterations/2 + 1
	}

	// Pre-flight one session to surface a bad binary/option before the pool starts.
	if s, err := newSession(cfg.Engine); err != nil {
		return nil, err
	} else {
		s.close()
	}

	theta := map[string]int{}
	for _, p := range specs {
		theta[p.Name] = p.Default
	}
	var mu sync.Mutex

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var iter int64
	var wg sync.WaitGroup
	for w := 0; w < cfg.Concurrency; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			spsaWorker(ctx, cfg, specs, theta, &mu, &iter, wid)
		}(w)
	}
	wg.Wait()

	return snapshotTheta(theta, &mu), nil
}

func spsaWorker(ctx context.Context, cfg SPSAConfig, specs []search.ParamSpec, theta map[string]int, mu *sync.Mutex, iter *int64, wid int) {
	a, err := newSession(cfg.Engine)
	if err != nil {
		return
	}
	defer a.close()
	b, err := newSession(cfg.Engine)
	if err != nil {
		return
	}
	defer b.close()

	rng := rand.New(rand.NewSource(cfg.Seed + int64(wid)*7919 + 1))

	for {
		k := int(atomic.AddInt64(iter, 1))
		if k > cfg.Iterations {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Read the current vector and perturb every parameter by +/-step in a random direction.
		mu.Lock()
		cur := make(map[string]int, len(theta))
		for n, v := range theta {
			cur[n] = v
		}
		mu.Unlock()

		delta := make(map[string]float64, len(specs))
		for _, p := range specs {
			d := 1.0
			if rng.Intn(2) == 0 {
				d = -1.0
			}
			delta[p.Name] = d
			up := clampInt(cur[p.Name]+int(math.Round(d*p.Step)), p.Min, p.Max)
			dn := clampInt(cur[p.Name]-int(math.Round(d*p.Step)), p.Min, p.Max)
			_ = a.gui.SetOption(p.Name, strconv.Itoa(up))
			_ = b.gui.SetOption(p.Name, strconv.Itoa(dn))
		}

		// Play the same opening twice with colours reversed, so theta+ (engine a) plays both sides.
		opening := openingMoves(cfg.Openings[k%len(cfg.Openings)])
		_ = a.gui.NewGame()
		_ = b.gui.NewGame()
		o1, e1 := playGame(a, b, opening, cfg.TC, 400) // a (theta+) is White
		_ = a.gui.NewGame()
		_ = b.gui.NewGame()
		o2, e2 := playGame(b, a, opening, cfg.TC, 400) // a (theta+) is Black

		if e1 != nil || e2 != nil {
			// A broken session can't be trusted; rebuild both and skip this iteration.
			a.close()
			b.close()
			if a, err = newSession(cfg.Engine); err != nil {
				return
			}
			if b, err = newSession(cfg.Engine); err != nil {
				return
			}
			continue
		}

		// theta+'s score over the pair, in [0,1], turned into a centred gradient sign in [-1,1].
		sPlus := outcomeScore(o1, true) + outcomeScore(o2, false)
		r := sPlus - 1 // (sPlus/2)*2 - 1

		rate := cfg.Rate0 / (1 + float64(k)/float64(cfg.HalfLife))

		mu.Lock()
		for _, p := range specs {
			step := int(math.Round(rate * r * delta[p.Name] * p.Step))
			theta[p.Name] = clampInt(theta[p.Name]+step, p.Min, p.Max)
		}
		snap := make(map[string]int, len(theta))
		for n, v := range theta {
			snap[n] = v
		}
		mu.Unlock()

		if cfg.OnUpdate != nil && k%20 == 0 {
			cfg.OnUpdate(k, snap)
		}
	}
}

// outcomeScore returns engine a's score for one game (1 win, 0.5 draw, 0 loss), given whether a was White.
func outcomeScore(o Outcome, aIsWhite bool) float64 {
	switch o {
	case Draw:
		return 0.5
	case WhiteWins:
		if aIsWhite {
			return 1
		}
		return 0
	default: // BlackWins
		if aIsWhite {
			return 0
		}
		return 1
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func snapshotTheta(theta map[string]int, mu *sync.Mutex) map[string]int {
	mu.Lock()
	defer mu.Unlock()
	out := make(map[string]int, len(theta))
	for n, v := range theta {
		out[n] = v
	}
	return out
}
