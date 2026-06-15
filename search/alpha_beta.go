package search

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/ollybritton/StupidChess/position"
)

type AlphaBetaSearch struct {
	requests  chan Request
	responses chan string

	us       position.Color
	evalUs   position.Evaluator
	evalThem position.Evaluator

	startTime time.Time
	nextTime  time.Time
	nodeCount int

	options SearchOptions

	// tt caches results across the search tree (and across moves in a game) so transposed positions
	// aren't re-searched and the best move from a prior search is tried first. nil disables it.
	tt *transpositionTable

	// stop is set from another goroutine (via Stop) to abort the current search. It is accessed
	// atomically because the search runs on its own goroutine; the previous code used a plain bool on
	// the shared options struct, which was a data race.
	stop int32

	// Time control is communicated to the search goroutine through atomics so the controller goroutine
	// (which handles ponderhit/stop) never races on shared time.Time values.
	//
	// pondering is 1 during a "go ponder" search: there is no deadline until the pondered move is
	// actually played (PonderHit), at which point the clock starts. hardDeadline/softDeadline are unix
	// nanoseconds; 0 means "no deadline" (pondering or infinite analysis). The search stops at the hard
	// deadline and refuses to start a new iteration past the soft deadline. plannedMoveTime is the move
	// time computed at the start of the search, used to install the deadlines on a ponderhit.
	pondering       int32
	hardDeadline    int64
	softDeadline    int64
	plannedMoveTime int64

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
}

// drawScore is the value of a draw (by repetition or the fifty-move rule). Scoring it 0 means a winning
// engine (eval > 0) steers away from draws and a losing one steers toward them.
const drawScore int16 = 0

// maxPlies bounds ply-indexed tables. It is larger than maxSearchDepth to leave room for extensions.
const maxPlies = 128

// maxMoves is a safe upper bound on legal moves in a position (the real maximum is 218), used to size a
// per-node ordering scratch array on the stack.
const maxMoves = 256

// stopped reports whether the search has been asked to abort.
func (s *AlphaBetaSearch) stopped() bool {
	return atomic.LoadInt32(&s.stop) == 1
}

// maxSearchDepth caps iterative deepening. It is far beyond what this engine reaches in practice; it
// exists so a ponder/infinite search (which has no clock) cannot loop on the uint depth counter.
const maxSearchDepth = 64

func (s *AlphaBetaSearch) setDeadlines(start time.Time, moveTime time.Duration) {
	atomic.StoreInt64(&s.hardDeadline, start.Add(moveTime).UnixNano())
	// Don't begin an iteration we almost certainly can't finish: the next depth typically costs several
	// times the last, and an interrupted depth is discarded entirely, so starting one past ~60% of the
	// budget is wasted time.
	atomic.StoreInt64(&s.softDeadline, start.Add(moveTime*3/5).UnixNano())
}

func (s *AlphaBetaSearch) clearDeadlines() {
	atomic.StoreInt64(&s.hardDeadline, 0)
	atomic.StoreInt64(&s.softDeadline, 0)
}

func (s *AlphaBetaSearch) hardTimeUp() bool {
	d := atomic.LoadInt64(&s.hardDeadline)
	return d != 0 && time.Now().UnixNano() >= d
}

func (s *AlphaBetaSearch) softTimeUp() bool {
	d := atomic.LoadInt64(&s.softDeadline)
	return d != 0 && time.Now().UnixNano() >= d
}

// PonderHit is called when the move the engine was pondering on is actually played: the opponent's
// turn is over and our clock starts now, so we switch from open-ended pondering to a timed search.
func (s *AlphaBetaSearch) PonderHit() {
	if atomic.CompareAndSwapInt32(&s.pondering, 1, 0) {
		moveTime := time.Duration(atomic.LoadInt64(&s.plannedMoveTime))
		s.setDeadlines(time.Now(), moveTime)
	}
}

func NewAlphaBetaSearch(requests chan Request, responses chan string, evalUs position.Evaluator, evalThem position.Evaluator) *AlphaBetaSearch {
	return &AlphaBetaSearch{
		requests:   requests,
		responses:  responses,
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
	atomic.StoreInt32(&s.stop, 1)
}

func (s *AlphaBetaSearch) Root() error {
	var pv pvList      // Holds the principle variation
	var childPV pvList // Holds the principle variation of the position after the first move is made

	childPV.new()

	for request := range s.requests {
		pos := request.pos // Position we are searching

		s.startTime = time.Now()      // Record start time so we know to stop if time is up
		s.nextTime = time.Now()       // Record next time as a counter so we can periodically print information
		s.nodeCount = 0               // Record number of nodes so we can stop after searching a certain number of nodes
		s.options = request.options   // Store options in the search struct so we don't have to explicitly pass around.
		atomic.StoreInt32(&s.stop, 0) // Make sure we don't stop straight away if we were told to stop previously

		// Seed draw detection: the prior game positions, and the root itself at ply 0 (the search proper
		// starts at the root's children, ply 1, so a child that returns to the root must see it here).
		s.gameHistory = s.options.History
		s.pathHashes[0] = pos.ZobristHash()

		// Fresh move-ordering memory for this move (it carries over across deepening iterations, which is
		// the point, but not across moves where it would be stale).
		for i := range s.killers {
			s.killers[i][0], s.killers[i][1] = position.NoMove, position.NoMove
		}
		s.history = [2][64][64]int32{}

		var timeRemaining, increment time.Duration

		if pos.SideToMove == position.White {
			timeRemaining = s.options.WhiteTimeRemaining
			increment = s.options.WhiteIncrement
			s.us = position.White
		} else {
			timeRemaining = s.options.BlackTimeRemaining
			increment = s.options.BlackIncrement
			s.us = position.Black
		}

		if s.options.MoveTime == 0 {
			s.options.MoveTime = DefaultTimeManager(timeRemaining, increment, s.options.MovesToGo)
		}
		atomic.StoreInt64(&s.plannedMoveTime, int64(s.options.MoveTime))

		// Install the time controls. Pondering and infinite analysis run without a deadline (ended only
		// by ponderhit or stop); a normal search gets soft/hard deadlines from the planned move time.
		switch {
		case s.options.Ponder:
			atomic.StoreInt32(&s.pondering, 1)
			s.clearDeadlines()
		case s.options.Infinite:
			atomic.StoreInt32(&s.pondering, 0)
			s.clearDeadlines()
		default:
			atomic.StoreInt32(&s.pondering, 0)
			s.setDeadlines(s.startTime, s.options.MoveTime)
		}

		s.responses <- fmt.Sprintf("info string searching for %s/%s (inc %s)", s.options.MoveTime, timeRemaining, increment)

		// Best move/PV from the last FULLY COMPLETED depth. A depth interrupted by the clock is
		// discarded, so the engine never commits to a half-searched (and possibly blundering) move.
		bestMove := position.NoMove
		// bestLine is the principal variation of the last completed depth; its second move is what we
		// expect the opponent to reply, and is reported as the ponder move.
		var bestLine pvList
		bestLine.new()

		// Root moves: ordered by a quick static eval for the first iteration, then re-sorted by the
		// real search scores on subsequent iterations (so the best move is searched first).
		legalMoves := pos.MovesLegalWithEvaluation(position.EvalSimple)
		if moves := legalMoves.AsSlice(); len(moves) > 0 {
			bestMove = moves[0] // fallback so we always have a legal move, even if depth 1 is interrupted
		}

		// Iterative deepening.
		for depth := uint(1); depth <= s.options.Depth && depth <= maxSearchDepth; depth++ {
			legalMoves.Sort()

			bestScore := position.NoEval
			// Alpha and beta bound the search window; initialised once per depth so alpha rises as
			// better root moves are found and later moves are searched with a narrowing window.
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

				// Remember the score so the next iteration searches the best moves first.
				move.SetEval(score)
				legalMoves.Moves[i] = move

				if score > bestScore {
					bestScore = score
					depthBestMove = move
					pv.clear()
					pv.catenate(move, &childPV)
					alpha = score
				}

				s.responses <- fmt.Sprintf(
					"info currmove %s currmovenumber %d nodes %d depth %d score %s",
					move.String(),
					i+1,
					s.nodeCount,
					depth,
					formatScore(score),
				)
			}

			// Discard an interrupted depth and keep the previous completed depth's best move.
			if interrupted {
				break
			}
			bestMove = depthBestMove
			bestLine = append(bestLine[:0], pv...) // remember the completed PV for the ponder move

			diff := time.Since(s.startTime)
			if diff.Seconds() < 1 {
				s.responses <- fmt.Sprintf(
					"info depth %d score %s nodes %d time %d pv %s",
					depth, formatScore(bestScore), s.nodeCount, diff.Milliseconds(), pv.String(),
				)
			} else {
				s.responses <- fmt.Sprintf(
					"info depth %d score %s nodes %d nps %.0f time %d pv %s",
					depth, formatScore(bestScore), s.nodeCount,
					1000*(float64(s.nodeCount)/float64(diff.Milliseconds())),
					diff.Milliseconds(), pv.String(),
				)
			}

			// Don't start a new iteration we are unlikely to finish (no effect while pondering / infinite,
			// which have no deadline).
			if s.softTimeUp() {
				break
			}
		}

		// While pondering we must not return a move until the opponent has actually moved: wait for a
		// ponderhit (which clears `pondering` and lets us fall through) or a stop. This only matters in
		// the rare case the depth cap is reached before either arrives.
		for atomic.LoadInt32(&s.pondering) == 1 && !s.stopped() {
			time.Sleep(2 * time.Millisecond)
		}

		// Report the best move, plus the move we expect in reply so the controller can ponder on it.
		ponderMove := position.NoMove
		if len(bestLine) >= 2 {
			ponderMove = bestLine[1]
		}
		if ponderMove != position.NoMove {
			s.responses <- fmt.Sprintf("bestmove %s ponder %s", bestMove.String(), ponderMove.String())
		} else {
			s.responses <- fmt.Sprintf("bestmove %s", bestMove.String())
		}
	}

	return nil
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
func (s *AlphaBetaSearch) scoreMove(m, ttMove position.Move, ply int) int {
	if ttMove != position.NoMove && sameMove(m, ttMove) {
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
	s.nodeCount++

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

	// At the horizon, resolve outstanding captures with a quiescence search before evaluating, so the
	// engine never judges a position mid-exchange (which is what made it hang pieces).
	if depth <= 0 {
		return s.quiesce(alpha, beta, ply, pos)
	}

	pv.clear()

	alphaOrig := alpha

	// Probe the transposition table. A stored result searched at least as deep can cut this node off
	// immediately; otherwise its best move still improves our move ordering.
	var ttMove position.Move = position.NoMove
	if s.tt != nil {
		if e, ok := s.tt.probe(hash); ok {
			ttMove = e.move
			if uint(e.depth) >= depth {
				ttScore := scoreFromTT(e.score, ply)
				switch {
				case e.bound == boundExact:
					return ttScore
				case e.bound == boundLower && ttScore >= beta:
					return ttScore
				case e.bound == boundUpper && ttScore <= alpha:
					return ttScore
				}
			}
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

		childPV.clear()
		score := -s.search(-beta, -alpha, depth-1, ply+1, &childPV, pos)
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
			atomic.StoreInt32(&s.stop, 1)
		}
		// Honour an explicit node limit (`go nodes N`); the default is math.MaxUint, so it never fires
		// unless a limit was actually requested.
		if uint(s.nodeCount) >= s.options.Nodes {
			atomic.StoreInt32(&s.stop, 1)
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
	s.nodeCount++

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
