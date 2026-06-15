package search

import (
	"fmt"
	"math/bits"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/syzygy"
)

// shared holds the state common to every worker of one parallel (Lazy SMP) search: the stop flag, the
// clock deadlines, and the aggregate node count. All of it is touched atomically, since several search
// goroutines and the controller goroutine (ponderhit/stop) access it concurrently.
//
// pondering is 1 during a "go ponder" search: there is no deadline until the pondered move is actually
// played (PonderHit), at which point the clock starts. hardDeadline/softDeadline are unix nanoseconds;
// 0 means "no deadline" (pondering or infinite analysis). The search stops at the hard deadline and
// refuses to start a new iteration past the soft deadline. plannedMoveTime is the move time computed at
// the start of the search, used to install the deadlines on a ponderhit. nodes is the running total
// across all workers (flushed in batches to avoid per-node contention).
type shared struct {
	stop            int32
	pondering       int32
	hardDeadline    int64
	softDeadline    int64
	plannedMoveTime int64
	nodes           int64
}

// AlphaBetaSearch holds one engine's search. The fields below `sh` are per-worker: Root copies the
// struct for each Lazy-SMP thread, so each gets its own ordering tables, path stack and local node
// counter, while the transposition table (a pointer) and `sh` (a pointer) are shared between them.
type AlphaBetaSearch struct {
	requests  chan Request
	responses chan string

	threads int32   // number of search threads to run (Lazy SMP); accessed atomically
	sh      *shared // coordination shared across this search's workers

	// tt caches results across the search tree (and across moves in a game) so transposed positions
	// aren't re-searched and the best move from a prior search is tried first. It is shared by all
	// workers (a pointer, so copying the struct shares it). nil disables it.
	tt *transpositionTable

	// tb, if non-nil, is a Syzygy endgame tablebase: positions with few enough pieces are probed for an
	// exact win/draw/loss, truncating the search with perfect endgame knowledge. Shared by all workers.
	tb *syzygy.Tablebases

	us       position.Color
	evalUs   position.Evaluator
	evalThem position.Evaluator

	startTime time.Time
	nodeCount int // this worker's local node count (flushed in batches into sh.nodes)

	options SearchOptions

	// Draw detection. gameHistory holds the Zobrist hashes of the positions played in the actual game
	// before the root (so a move that repeats one of them can be recognised). pathHashes[ply] holds the
	// hash of the node currently being searched at that ply, so a repetition back up the search line can
	// be recognised too. Together with the fifty-move counter this lets the engine see (and, when ahead,
	// avoid) draws.
	gameHistory []uint64
	pathHashes  []uint64

	// Move-ordering memory, reset per search. killers[ply] are two quiet moves that recently caused a
	// beta cutoff at that ply (tried early in sibling nodes); history[color][from][to] accumulates how
	// often a quiet move caused a cutoff (used to order the remaining quiet moves). Good ordering is what
	// makes alpha-beta (and the pruning below) actually pay off.
	killers [maxPlies][2]position.Move
	history [2][64][64]int32

	rootDepth uint // the depth of the current iterative-deepening iteration (bounds extensions)
}

// drawScore is the value of a draw (by repetition or the fifty-move rule). Scoring it 0 means a winning
// engine (eval > 0) steers away from draws and a losing one steers toward them.
const drawScore int16 = 0

// maxPlies bounds ply-indexed tables. It is larger than maxSearchDepth to leave room for extensions.
const maxPlies = 128

// maxMoves is a safe upper bound on legal moves in a position (the real maximum is 218), used to size a
// per-node ordering scratch array on the stack.
const maxMoves = 256

// Pruning margins, in centipawns. These trade a small amount of accuracy for a large reduction in nodes
// near the leaves. Conservative values so tactics still surface.
const (
	rfpMaxDepth      = 6   // reverse futility pruning only at shallow depth
	rfpMargin        = 80  // per ply of depth
	futilityMaxDepth = 6   // futility pruning of quiet moves only at shallow depth
	futilityMargin   = 100 // per ply of depth
	lmpMaxDepth      = 6   // late move pruning only at shallow depth
	deltaMargin      = 200 // quiescence delta pruning safety margin
)

// stopped reports whether the search has been asked to abort.
func (s *AlphaBetaSearch) stopped() bool {
	return atomic.LoadInt32(&s.sh.stop) == 1
}

// countNode records a searched node. Each worker keeps a local counter and flushes it into the shared
// total in batches, so the running node total is accurate without per-node atomic contention.
func (s *AlphaBetaSearch) countNode() {
	s.nodeCount++
	if s.nodeCount&1023 == 0 {
		atomic.AddInt64(&s.sh.nodes, 1024)
	}
}

// SetThreads sets how many search threads to run (Lazy SMP). Clamped to at least 1.
func (s *AlphaBetaSearch) SetThreads(n int) {
	if n < 1 {
		n = 1
	}
	atomic.StoreInt32(&s.threads, int32(n))
}

// SetEvaluator swaps the evaluation functions (e.g. to switch from the hand-crafted eval to an NNUE
// network, or back). Call it between searches; the workers copy the evaluators at the start of a search.
func (s *AlphaBetaSearch) SetEvaluator(evalUs, evalThem position.Evaluator) {
	s.evalUs, s.evalThem = evalUs, evalThem
}

// SetTablebases installs (or clears, with nil) the Syzygy tablebases used to truncate the search in
// endgames with perfect knowledge.
func (s *AlphaBetaSearch) SetTablebases(tb *syzygy.Tablebases) {
	s.tb = tb
}

// tbWinScore is the value of a tablebase win. It sits above any ordinary evaluation (evalLimit, 20000)
// but below a real forced mate (mateScoreBound, 29000), so a found mate is still preferred, and the
// per-ply decrement nudges the engine toward reaching the won position sooner.
const tbWinScore = 28000

// maxSearchDepth caps iterative deepening. It is far beyond what this engine reaches in practice; it
// exists so a ponder/infinite search (which has no clock) cannot loop on the uint depth counter.
const maxSearchDepth = 64

func (s *AlphaBetaSearch) setDeadlines(start time.Time, moveTime time.Duration) {
	atomic.StoreInt64(&s.sh.hardDeadline, start.Add(moveTime).UnixNano())
	// Don't begin an iteration we almost certainly can't finish: the next depth typically costs several
	// times the last, and an interrupted depth is discarded entirely, so starting one past ~60% of the
	// budget is wasted time.
	atomic.StoreInt64(&s.sh.softDeadline, start.Add(moveTime*3/5).UnixNano())
}

func (s *AlphaBetaSearch) clearDeadlines() {
	atomic.StoreInt64(&s.sh.hardDeadline, 0)
	atomic.StoreInt64(&s.sh.softDeadline, 0)
}

func (s *AlphaBetaSearch) hardTimeUp() bool {
	d := atomic.LoadInt64(&s.sh.hardDeadline)
	return d != 0 && time.Now().UnixNano() >= d
}

func (s *AlphaBetaSearch) softTimeUp() bool {
	d := atomic.LoadInt64(&s.sh.softDeadline)
	return d != 0 && time.Now().UnixNano() >= d
}

// PonderHit is called when the move the engine was pondering on is actually played: the opponent's
// turn is over and our clock starts now, so we switch from open-ended pondering to a timed search.
func (s *AlphaBetaSearch) PonderHit() {
	if atomic.CompareAndSwapInt32(&s.sh.pondering, 1, 0) {
		moveTime := time.Duration(atomic.LoadInt64(&s.sh.plannedMoveTime))
		s.setDeadlines(time.Now(), moveTime)
	}
}

func NewAlphaBetaSearch(requests chan Request, responses chan string, evalUs position.Evaluator, evalThem position.Evaluator) *AlphaBetaSearch {
	return &AlphaBetaSearch{
		requests:   requests,
		responses:  responses,
		threads:    1,
		sh:         &shared{},
		evalUs:     evalUs,
		evalThem:   evalThem,
		tt:         newTranspositionTable(),
		pathHashes: make([]uint64, maxPlies),
	}
}

func (s *AlphaBetaSearch) Requests() chan Request {
	return s.requests
}

func (s *AlphaBetaSearch) Responses() chan string {
	return s.responses
}

func (s *AlphaBetaSearch) Stop() {
	atomic.StoreInt32(&s.sh.stop, 1)
}

func (s *AlphaBetaSearch) Root() error {
	for request := range s.requests {
		pos := request.pos

		s.startTime = time.Now()
		s.options = request.options
		s.gameHistory = s.options.History
		atomic.StoreInt32(&s.sh.stop, 0)
		atomic.StoreInt64(&s.sh.nodes, 0)

		var timeRemaining, increment time.Duration
		if pos.SideToMove == position.White {
			timeRemaining, increment = s.options.WhiteTimeRemaining, s.options.WhiteIncrement
			s.us = position.White
		} else {
			timeRemaining, increment = s.options.BlackTimeRemaining, s.options.BlackIncrement
			s.us = position.Black
		}

		if s.options.MoveTime == 0 {
			s.options.MoveTime = DefaultTimeManager(timeRemaining, increment, s.options.MovesToGo)
		}
		atomic.StoreInt64(&s.sh.plannedMoveTime, int64(s.options.MoveTime))

		// Install the time controls. Pondering and infinite analysis run without a deadline (ended only
		// by ponderhit or stop); a normal search gets soft/hard deadlines from the planned move time.
		switch {
		case s.options.Ponder:
			atomic.StoreInt32(&s.sh.pondering, 1)
			s.clearDeadlines()
		case s.options.Infinite:
			atomic.StoreInt32(&s.sh.pondering, 0)
			s.clearDeadlines()
		default:
			atomic.StoreInt32(&s.sh.pondering, 0)
			s.setDeadlines(s.startTime, s.options.MoveTime)
		}

		s.responses <- fmt.Sprintf("info string searching for %s/%s (inc %s)", s.options.MoveTime, timeRemaining, increment)

		// Lazy SMP: run several worker goroutines that share the transposition table. The main worker
		// (id 0) reports info, applies the soft-time cutoff and produces the move; the others just help
		// fill the table so the main worker gets more cutoffs and reaches greater depth. Each worker is a
		// copy of this struct with its own ordering tables, path stack and board.
		threads := int(atomic.LoadInt32(&s.threads))
		if threads < 1 {
			threads = 1
		}

		var wg sync.WaitGroup
		var mainBest, mainPonder position.Move

		for id := 0; id < threads; id++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				w := *s // shares sh and tt (pointers); copies the per-worker fields below
				w.pathHashes = make([]uint64, maxPlies)
				w.pathHashes[0] = pos.ZobristHash()
				w.killers = [maxPlies][2]position.Move{}
				w.history = [2][64][64]int32{}
				w.nodeCount = 0

				best, ponder := w.iterativeDeepen(pos.Clone(), id == 0)
				if id == 0 {
					mainBest, mainPonder = best, ponder
				}
			}(id)
		}
		wg.Wait()

		// Report the best move, plus the move we expect in reply so the controller can ponder on it.
		if mainPonder != position.NoMove {
			s.responses <- fmt.Sprintf("bestmove %s ponder %s", mainBest.String(), mainPonder.String())
		} else {
			s.responses <- fmt.Sprintf("bestmove %s", mainBest.String())
		}
	}

	return nil
}

// iterativeDeepen runs iterative deepening for one Lazy-SMP worker on its own board. The main worker
// reports info, applies the soft-time cutoff, and (when pondering) waits for ponderhit/stop before
// returning, then halts the helpers; helper workers just deepen, sharing the transposition table, until
// the search is stopped.
func (s *AlphaBetaSearch) iterativeDeepen(pos *position.Position, isMain bool) (bestMove, ponderMove position.Move) {
	var pv, childPV, bestLine pvList
	childPV.new()
	bestLine.new()

	// Root moves: ordered by a quick static eval for the first iteration, then re-sorted by the real
	// search scores on subsequent iterations (so the best move is searched first).
	legalMoves := pos.MovesLegalWithEvaluation(position.EvalSimple)
	if moves := legalMoves.AsSlice(); len(moves) > 0 {
		bestMove = moves[0] // fallback so we always have a legal move, even if depth 1 is interrupted
	}

	for depth := uint(1); depth <= s.options.Depth && depth <= maxSearchDepth; depth++ {
		s.rootDepth = depth
		legalMoves.Sort()

		bestScore := position.NoEval
		alpha, beta := position.MinEval, position.MaxEval
		depthBestMove := position.NoMove
		interrupted := false

		for i, move := range legalMoves.AsSlice() {
			if s.stopped() {
				interrupted = true
				break
			}

			childPV.clear()
			pos.MakeMove(move)
			score := -s.search(-beta, -alpha, depth-1, 1, &childPV, pos)
			pos.UndoMove(move)

			if s.stopped() {
				interrupted = true
				break
			}

			move.SetEval(score)
			legalMoves.Moves[i] = move

			if score > bestScore {
				bestScore = score
				depthBestMove = move
				pv.clear()
				pv.catenate(move, &childPV)
				alpha = score
			}

			if isMain {
				s.responses <- fmt.Sprintf(
					"info currmove %s currmovenumber %d nodes %d depth %d score %s",
					move.String(), i+1, atomic.LoadInt64(&s.sh.nodes), depth, formatScore(score),
				)
			}
		}

		// Discard an interrupted depth and keep the previous completed depth's best move.
		if interrupted {
			break
		}
		bestMove = depthBestMove
		bestLine = append(bestLine[:0], pv...)

		if isMain {
			diff := time.Since(s.startTime)
			nodes := atomic.LoadInt64(&s.sh.nodes)
			if diff.Seconds() < 1 {
				s.responses <- fmt.Sprintf("info depth %d score %s nodes %d time %d pv %s",
					depth, formatScore(bestScore), nodes, diff.Milliseconds(), pv.String())
			} else {
				s.responses <- fmt.Sprintf("info depth %d score %s nodes %d nps %.0f time %d pv %s",
					depth, formatScore(bestScore), nodes,
					1000*(float64(nodes)/float64(diff.Milliseconds())), diff.Milliseconds(), pv.String())
			}

			// Don't start a new iteration we are unlikely to finish.
			if s.softTimeUp() {
				break
			}
		}
	}

	if isMain {
		// While pondering, the main worker must not return until the opponent has moved (ponderhit clears
		// `pondering`) or we are stopped. Then halt the helper workers now that we have our answer.
		for atomic.LoadInt32(&s.sh.pondering) == 1 && !s.stopped() {
			time.Sleep(2 * time.Millisecond)
		}
		atomic.StoreInt32(&s.sh.stop, 1)
	}

	ponderMove = position.NoMove
	if len(bestLine) >= 2 {
		ponderMove = bestLine[1]
	}
	return bestMove, ponderMove
}

// isRepetition reports whether the current position (hash) has already occurred along the current
// search line or earlier in the actual game, within the fifty-move window (only moves since the last
// irreversible one can repeat). A single prior occurrence is treated as a draw: it is the simplest rule
// that makes a winning engine refuse to repeat, which is exactly what we want. Positions with the wrong
// side to move have a different hash and so never match, so we can scan every ply.
func (s *AlphaBetaSearch) isRepetition(hash uint64, ply, halfmoveClock int) bool {
	scanned := 0

	// Back up the current search line (ply-1, ply-2, ... toward the root).
	for i := ply - 1; i >= 0 && scanned < halfmoveClock; i-- {
		if s.pathHashes[i] == hash {
			return true
		}
		scanned++
	}

	// Then into the game's history before the root.
	for i := len(s.gameHistory) - 1; i >= 0 && scanned < halfmoveClock; i-- {
		if s.gameHistory[i] == hash {
			return true
		}
		scanned++
	}

	return false
}

// maxQuiescencePly caps quiescence recursion as a safety valve against pathological capture/check
// sequences. Captures alone are self-terminating (material is finite), but check chains may not be.
const maxQuiescencePly = 64

// pieceOrderValue is a small centipawn-ish table for move ordering (MVV-LVA), indexed by Piece.
var pieceOrderValue = [7]int{
	position.Pawn: 100, position.Knight: 320, position.Bishop: 330,
	position.Rook: 500, position.Queen: 900, position.King: 0,
}

// Move-ordering score bands, from best to worst. Captures and promotions are ordered above the killers,
// which are above quiet moves ordered by history.
const (
	scoreTT      = 1 << 24
	scoreCapture = 1 << 20
	scorePromo   = 1 << 19
	scoreKiller1 = (1 << 18) + 1
	scoreKiller2 = 1 << 18
)

// sameMove compares two moves by their from/to/promotion only, ignoring the prior-state bits packed
// into a Move (which differ between positions), so a move remembered in one node matches the same move
// generated in another.
func sameMove(a, b position.Move) bool {
	return a.From() == b.From() && a.To() == b.To() && a.Promotion() == b.Promotion()
}

// isQuiet reports whether a move is neither a capture nor a promotion (only quiet moves feed the killer
// and history tables).
func isQuiet(m position.Move) bool {
	return m.Captured() == position.Empty && m.Promotion() == position.None
}

// scoreMove assigns a move its ordering key.
func (s *AlphaBetaSearch) scoreMove(m position.Move, ttMove compactMove, ply int) int {
	if ttMove.matches(m) {
		return scoreTT
	}
	if captured := m.Captured(); captured != position.Empty {
		return scoreCapture + pieceOrderValue[captured.Colorless()]*16 - pieceOrderValue[m.Moved().Colorless()]
	}
	if m.Promotion() != position.None {
		return scorePromo
	}
	if ply < len(s.killers) {
		if sameMove(s.killers[ply][0], m) {
			return scoreKiller1
		}
		if sameMove(s.killers[ply][1], m) {
			return scoreKiller2
		}
	}
	h := int(s.history[m.Moved().Color()][m.From()][m.To()])
	if h >= scoreKiller2 { // keep quiet moves ordered below the killers
		h = scoreKiller2 - 1
	}
	return h
}

// hasNonPawnMaterial reports whether the side has any piece beyond pawns and the king. Null-move
// pruning is unsafe without it (pawn-and-king endgames are full of zugzwang, where passing would lose).
func hasNonPawnMaterial(pos *position.Position, c position.Color) bool {
	pieces := pos.Pieces[position.Knight] | pos.Pieces[position.Bishop] | pos.Pieces[position.Rook] | pos.Pieces[position.Queen]
	return pos.Occupied[c]&pieces != 0
}

// lmrReduction is how many plies to shave off a late, quiet move's search. Later moves and deeper nodes
// are reduced more; the move is re-searched at full depth if the reduced search unexpectedly beats alpha.
func lmrReduction(depth uint, moveCount int) uint {
	r := uint(1)
	if depth >= 6 {
		r++
	}
	if moveCount >= 10 {
		r++
	}
	return r
}

// recordCutoff rewards a quiet move that caused a beta cutoff: it becomes a killer for this ply and its
// history score grows with the depth (deeper cutoffs are more valuable).
func (s *AlphaBetaSearch) recordCutoff(m position.Move, ply int, depth uint) {
	if ply < len(s.killers) && !sameMove(s.killers[ply][0], m) {
		s.killers[ply][1] = s.killers[ply][0]
		s.killers[ply][0] = m
	}
	s.history[m.Moved().Color()][m.From()][m.To()] += int32(depth * depth)
}

func (s *AlphaBetaSearch) search(alpha int16, beta int16, depth uint, ply int, pv *pvList, pos *position.Position) int16 {
	s.countNode()

	// Safety valve: extensions can push ply past the nominal depth; never run off the end of the
	// ply-indexed tables. Quiescence (capped separately) resolves and evaluates the position.
	if ply >= maxPlies-1 {
		return s.quiesce(alpha, beta, ply, pos)
	}

	hash := pos.ZobristHash()

	// A repeated position, or an expired fifty-move counter, is a draw (but never at the root, where a
	// move must still be chosen). Scoring it as a draw lets a winning engine steer away from it and a
	// losing one steer toward it.
	if ply > 0 && (pos.HalfmoveClock >= 100 || s.isRepetition(hash, ply, int(pos.HalfmoveClock))) {
		return drawScore
	}
	if ply < len(s.pathHashes) {
		s.pathHashes[ply] = hash // record this node so deeper nodes can detect a repetition back to it
	}

	// Syzygy tablebases: in an endgame with few enough pieces the exact result is known, so we can stop
	// here with perfect knowledge instead of searching on. A win/loss is scored just under a real mate
	// (and nearer the root scores higher, to make progress); cursed wins / blessed losses depend on the
	// fifty-move counter, so they are treated conservatively as draws.
	if s.tb != nil && ply > 0 {
		if bits.OnesCount64(uint64(pos.Occupied[position.White]|pos.Occupied[position.Black])) <= s.tb.MaxPieces() {
			if wdl, ok := s.tb.ProbeWDL(pos); ok {
				switch {
				case wdl >= 2:
					return tbWinScore - int16(ply)
				case wdl <= -2:
					return -tbWinScore + int16(ply)
				default:
					return drawScore
				}
			}
		}
	}

	// At the horizon, resolve outstanding captures with a quiescence search before evaluating, so the
	// engine never judges a position mid-exchange (which is what made it hang pieces).
	if depth <= 0 {
		return s.quiesce(alpha, beta, ply, pos)
	}

	pv.clear()

	alphaOrig := alpha

	// Probe the transposition table. A stored result searched at least as deep can cut this node off
	// immediately; otherwise its best move still improves our move ordering.
	var ttMove compactMove
	if s.tt != nil {
		if move, score, ttDepth, bound, ok := s.tt.probe(hash); ok {
			ttMove = move
			if uint(ttDepth) >= depth {
				ttScore := scoreFromTT(score, ply)
				switch {
				case bound == boundExact:
					return ttScore
				case bound == boundLower && ttScore >= beta:
					return ttScore
				case bound == boundUpper && ttScore <= alpha:
					return ttScore
				}
			}
		}
	}

	inCheck := pos.KingInCheck(pos.SideToMove)
	isPV := beta-alpha > 1 // a full window means this is a principal-variation node

	// Static evaluation of this node, used by the pruning heuristics below (meaningless in check, where
	// the side to move may be losing material it is forced to address).
	staticEval := position.NoEval
	if !inCheck {
		staticEval = s.leafEval(pos)
	}

	// Reverse futility pruning (a.k.a. static null move): if our static eval is so far above beta that
	// even handing back a depth-scaled margin keeps us above beta, assume the search would confirm it
	// and prune. Only at shallow non-PV nodes, never near a mate.
	if !isPV && !inCheck && depth <= rfpMaxDepth && beta < mateScoreBound &&
		int(staticEval)-rfpMargin*int(depth) >= int(beta) {
		return staticEval
	}

	// Null-move pruning: if we can give the opponent a free move and still reach beta with a shallower
	// search, the position is so good that the real moves will surely beat beta too, so we can prune.
	// Skipped in check (passing is illegal) and without pieces (pawn endgames are full of zugzwang,
	// where passing is actually best and this would prune a winning line).
	if !isPV && !inCheck && depth >= 3 && hasNonPawnMaterial(pos, pos.SideToMove) && staticEval >= beta {
		r := uint(2)
		if depth >= 6 {
			r = 3
		}

		savedEP := pos.EnPassant
		pos.EnPassant = position.NoEnPassant
		pos.SideToMove = pos.SideToMove.Invert()

		var nullPV pvList
		nullScore := -s.search(-beta, -beta+1, depth-1-r, ply+1, &nullPV, pos)

		pos.SideToMove = pos.SideToMove.Invert()
		pos.EnPassant = savedEP

		if nullScore >= beta {
			if nullScore >= mateScoreBound {
				nullScore = beta // don't trust a mate claimed by the reduced null search
			}
			return nullScore
		}
	}

	bestScore := position.NoEval
	bestMove := position.NoMove
	legalCount := 0

	// Generate, then order: the transposition-table move first, then captures (MVV-LVA), then the two
	// killer moves, then quiet moves by history score. Legality is checked lazily — MakeMove returns
	// false when a move leaves our own king in check, which is cheaper than filtering up front.
	moves := pos.MovesPseudolegal().AsSlice()
	var scores [maxMoves]int
	for i := range moves {
		scores[i] = s.scoreMove(moves[i], ttMove, ply)
	}

	var childPV pvList

	for i := 0; i < len(moves); i++ {
		// Selection sort: pull the best-scored remaining move to the front. This is cheaper than fully
		// sorting because most nodes cut off after a few moves.
		bestIdx := i
		for j := i + 1; j < len(moves); j++ {
			if scores[j] > scores[bestIdx] {
				bestIdx = j
			}
		}
		moves[i], moves[bestIdx] = moves[bestIdx], moves[i]
		scores[i], scores[bestIdx] = scores[bestIdx], scores[i]
		move := moves[i]

		if !pos.MakeMove(move) {
			continue // illegal: this move left our king in check
		}
		legalCount++

		// Check extension: a move that gives check usually starts something forcing, so search it a ply
		// deeper. Bounded by 2x the root depth so a string of checks can't explode the tree.
		givesCheck := pos.KingInCheck(pos.SideToMove)
		extension := uint(0)
		if givesCheck && ply < 2*int(s.rootDepth) {
			extension = 1
		}
		newDepth := depth - 1 + extension

		// Prune late, quiet, non-checking moves at shallow non-PV nodes (we keep the first move so there
		// is always a result):
		//   - late move pruning: once enough moves have been tried, skip the rest entirely.
		//   - futility pruning: if the static eval plus a depth-scaled margin still can't reach alpha, a
		//     quiet move is very unlikely to, so skip it.
		if !isPV && !inCheck && extension == 0 && legalCount > 1 && isQuiet(move) && !givesCheck {
			if depth <= lmpMaxDepth && legalCount > 3+int(depth*depth) {
				pos.UndoMove(move)
				continue
			}
			if depth <= futilityMaxDepth && int(staticEval)+futilityMargin*int(depth) <= int(alpha) {
				pos.UndoMove(move)
				continue
			}
		}

		childPV.clear()
		var score int16
		if legalCount == 1 {
			// First (best-ordered) move: search it with the full window to establish the PV.
			score = -s.search(-beta, -alpha, newDepth, ply+1, &childPV, pos)
		} else {
			// Late move reductions: search a late, quiet, non-checking move shallower first, on a null
			// window. If it beats alpha we re-search at full depth.
			reduction := uint(0)
			if extension == 0 && depth >= 3 && legalCount >= 4 && isQuiet(move) && !inCheck && !givesCheck {
				reduction = lmrReduction(depth, legalCount)
				if reduction > newDepth-1 {
					reduction = newDepth - 1
				}
			}

			// Null-window scout (Principal Variation Search): we only expect to confirm this move is not
			// better than the PV move, which a zero-width window decides faster.
			score = -s.search(-alpha-1, -alpha, newDepth-reduction, ply+1, &childPV, pos)
			if reduction > 0 && score > alpha {
				score = -s.search(-alpha-1, -alpha, newDepth, ply+1, &childPV, pos) // reduced search surprised us
			}
			if score > alpha && score < beta {
				score = -s.search(-beta, -alpha, newDepth, ply+1, &childPV, pos) // a real new PV: full window
			}
		}
		pos.UndoMove(move)

		if score > bestScore {
			bestScore = score
			bestMove = move
			pv.catenate(move, &childPV)
		}
		if score > alpha {
			alpha = score
		}
		if alpha >= beta {
			// Beta cutoff. If it was a quiet move, remember it (killer + history) so it is tried earlier
			// in sibling and future nodes.
			if isQuiet(move) {
				s.recordCutoff(move, ply, depth)
			}
			break
		}

		if s.hardTimeUp() {
			atomic.StoreInt32(&s.sh.stop, 1)
		}
		// Honour an explicit node limit (`go nodes N`) against the shared total; the default is
		// math.MaxUint, so this never fires unless a limit was actually requested.
		if uint64(atomic.LoadInt64(&s.sh.nodes)) >= uint64(s.options.Nodes) {
			atomic.StoreInt32(&s.sh.stop, 1)
		}
		if s.stopped() {
			return alpha // aborted: don't store a partial result
		}
	}

	// No legal move: checkmate if in check, otherwise stalemate.
	if legalCount == 0 {
		if pos.KingInCheck(pos.SideToMove) {
			return position.MinEval + int16(ply) + 1
		}
		return 0 // stalemate; TODO: return a contempt value instead
	}

	// Store the result. Mate scores are rewritten to be relative to this node (scoreToTT) so they remain
	// correct when the position is transposed to at a different ply.
	if s.tt != nil {
		bound := boundExact
		if bestScore <= alphaOrig {
			bound = boundUpper
		} else if bestScore >= beta {
			bound = boundLower
		}
		s.tt.store(hash, depth, scoreToTT(bestScore, ply), bound, bestMove)
	}

	return bestScore
}

// quiesce is a quiescence search. At the search horizon it keeps searching only captures and
// promotions until the position is quiet, so the static evaluation is never applied in the middle of
// an exchange. When the side to move is in check it instead searches every evasion, so a checkmate at
// the horizon is not mistaken for a quiet, equal-material position.
func (s *AlphaBetaSearch) quiesce(alpha, beta int16, ply int, pos *position.Position) int16 {
	s.countNode()

	if s.stopped() {
		return alpha
	}
	if ply >= maxQuiescencePly {
		return s.leafEval(pos)
	}

	inCheck := pos.KingInCheck(pos.SideToMove)

	bestScore := position.NoEval
	if !inCheck {
		// Stand pat: we are never obliged to capture, so the static eval is a lower bound on our score.
		bestScore = s.leafEval(pos)
		if bestScore >= beta {
			return bestScore
		}
		if bestScore > alpha {
			alpha = bestScore
		}
	}

	// Out of check, generate only captures and promotions (what quiescence needs); in check, every
	// move must be considered as a possible evasion.
	var moves *position.MoveList
	if inCheck {
		moves = pos.MovesPseudolegal()
	} else {
		moves = pos.MovesCaptures()
	}
	moves.OrderMVVLVA()
	legalCount := 0

	for _, move := range moves.AsSlice() {
		if !inCheck && move.Promotion() == position.None {
			// Delta pruning: skip a capture that, even if it won the captured piece for free plus a
			// margin, still couldn't reach alpha. (Promotions are exempt: they win more.)
			if int(bestScore)+pieceOrderValue[move.Captured().Colorless()]+deltaMargin < int(alpha) {
				continue
			}
			// SEE pruning: skip a capture that loses material once the recaptures are played out. These
			// are noise in quiescence; the static exchange answers it without searching.
			if pos.SEE(move) < 0 {
				continue
			}
		}

		if !pos.MakeMove(move) {
			continue
		}
		legalCount++

		score := -s.quiesce(-beta, -alpha, ply+1, pos)
		pos.UndoMove(move)

		if score > bestScore {
			bestScore = score
		}
		if score > alpha {
			alpha = score
		}
		if alpha >= beta {
			return bestScore
		}
	}

	// In check with no legal move is checkmate.
	if inCheck && legalCount == 0 {
		return position.MinEval + int16(ply) + 1
	}

	return bestScore
}

// leafEval returns the static evaluation of a leaf from the side-to-move's perspective, using the
// "us"/"them" evaluator depending on whose turn it is (identical for symmetric engines like tryhard).
func (s *AlphaBetaSearch) leafEval(pos *position.Position) int16 {
	if pos.SideToMove == s.us {
		return position.ScoreFromPerspective(s.evalUs(pos), pos.SideToMove)
	}
	return position.ScoreFromPerspective(s.evalThem(pos), pos.SideToMove)
}

// formatScore renders an internal evaluation as a UCI "score" token. Near-extreme scores are forced
// mates (checkmate returns roughly ±MaxEval offset by the ply at which it occurs), so they are
// reported as "mate N" (N negative when we are the side being mated). Everything else is reported in
// centipawns. The mate check is done in ints because the evaluation itself is int16 and mate scores
// near ±MaxEval would otherwise be mishandled. The eval is already in centipawns (see simpleEvalTable).
func formatScore(score int16) string {
	const mateThreshold = position.MaxEval - 1000
	s := int(score)

	switch {
	case s > int(mateThreshold):
		// We are delivering mate. Distance in plies is MaxEval - score; convert to full moves.
		return fmt.Sprintf("mate %d", (int(position.MaxEval)-s+1)/2)
	case s < -int(mateThreshold):
		// We are being mated.
		return fmt.Sprintf("mate -%d", (int(position.MaxEval)+s+1)/2)
	default:
		// The evaluation is already in centipawns (one pawn = 100), so report it directly.
		return fmt.Sprintf("cp %d", s)
	}
}
