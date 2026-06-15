// Package match plays StupidChess engines against each other and measures the
// result with a Sequential Probability Ratio Test (SPRT), so a change can be
// accepted or rejected with controlled error rather than guessed at. It drives
// two engines as UCI subprocesses over an opening book, at a fixed node count
// per move (hardware-load independent, unlike a wall-clock control), and runs
// many games in parallel.
package match

import "math"

// SPRT is a sequential probability ratio test on the score of a match, deciding
// between two Elo hypotheses as results arrive. It uses the generalised SPRT
// (GSPRT) approximation on the trinomial win/draw/loss model: the per-game score
// (1 / 0.5 / 0) has an empirical mean and variance, and the log-likelihood ratio
// is accumulated until it crosses one of the Wald bounds derived from the target
// error rates.
//
// H0: the Elo difference is Elo0 (typically 0, "no improvement").
// H1: the Elo difference is Elo1 (typically a small positive number).
// Elo is measured for the first engine (A) relative to the second (B), so a
// positive result means A is the stronger engine.
type SPRT struct {
	Elo0, Elo1  float64
	Alpha, Beta float64

	lower, upper float64
}

// NewSPRT builds a test for the hypotheses [elo0, elo1] at error rates alpha
// (false accept of H1) and beta (false accept of H0).
func NewSPRT(elo0, elo1, alpha, beta float64) *SPRT {
	return &SPRT{
		Elo0:  elo0,
		Elo1:  elo1,
		Alpha: alpha,
		Beta:  beta,
		lower: math.Log(beta / (1 - alpha)),
		upper: math.Log((1 - beta) / alpha),
	}
}

// Lower and Upper are the Wald decision bounds: cross below Lower to accept H0,
// above Upper to accept H1.
func (s *SPRT) Lower() float64 { return s.lower }
func (s *SPRT) Upper() float64 { return s.upper }

// LLR returns the current log-likelihood ratio from win/draw/loss counts (from
// A's perspective). It is the GSPRT statistic: with per-game mean mu and
// variance v over n games, and target means mu0, mu1 for the two Elo
// hypotheses, LLR = n*(mu1-mu0)/v * (mu - (mu0+mu1)/2).
func (s *SPRT) LLR(w, d, l int) float64 {
	n := w + d + l
	if n == 0 {
		return 0
	}
	fw, fd, fl := float64(w), float64(d), float64(l)
	fn := float64(n)

	mu := (fw + 0.5*fd) / fn
	// Variance of the per-game score: E[x^2] - mu^2, with x in {0, 0.5, 1}.
	variance := (fw+0.25*fd)/fn - mu*mu
	// Guard the degenerate early case (all decisive one way, or all draws) so the
	// ratio stays finite; the test simply continues until variance is meaningful.
	if variance < 1e-9 {
		variance = 1e-9
	}

	mu0 := eloToScore(s.Elo0)
	mu1 := eloToScore(s.Elo1)
	_ = fl
	return fn * (mu1 - mu0) / variance * (mu - (mu0+mu1)/2)
}

// Verdict is the outcome of a (possibly still-running) SPRT.
type Verdict int

const (
	// Continue means neither bound has been crossed yet.
	Continue Verdict = iota
	// AcceptH1 means the first engine is stronger by at least the H1 margin.
	AcceptH1
	// AcceptH0 means the improvement is below the H1 margin (change rejected).
	AcceptH0
)

func (v Verdict) String() string {
	switch v {
	case AcceptH1:
		return "H1 accepted (A is stronger)"
	case AcceptH0:
		return "H0 accepted (no improvement)"
	default:
		return "inconclusive"
	}
}

// Decide maps the current LLR to a verdict.
func (s *SPRT) Decide(w, d, l int) Verdict {
	llr := s.LLR(w, d, l)
	switch {
	case llr >= s.upper:
		return AcceptH1
	case llr <= s.lower:
		return AcceptH0
	default:
		return Continue
	}
}

// eloToScore converts an Elo difference to an expected score in (0,1) under the
// standard logistic model: 1 / (1 + 10^(-elo/400)).
func eloToScore(elo float64) float64 {
	return 1 / (1 + math.Pow(10, -elo/400))
}

// EloWithError returns the Elo estimate for A (relative to B) from the
// win/draw/loss counts, together with the half-width of its 95% confidence
// interval. The estimate is the inverse logistic of the observed score; the
// margin is propagated from the score's standard error through that mapping.
func EloWithError(w, d, l int) (elo, margin float64) {
	n := w + d + l
	if n == 0 {
		return 0, math.Inf(1)
	}
	fw, fd := float64(w), float64(d)
	fn := float64(n)

	mu := (fw + 0.5*fd) / fn
	// Clamp away from 0 and 1 so the inverse logistic stays finite before enough
	// decisive games have been seen.
	const eps = 1e-4
	mu = math.Max(eps, math.Min(1-eps, mu))

	elo = -400 * math.Log10(1/mu-1)

	variance := (fw+0.25*fd)/fn - mu*mu
	if variance < 0 {
		variance = 0
	}
	stdErr := math.Sqrt(variance / fn)
	// d(elo)/d(mu) = 400 / (ln(10) * mu * (1-mu)).
	slope := 400 / (math.Ln10 * mu * (1 - mu))
	margin = 1.96 * slope * stdErr
	return elo, margin
}
