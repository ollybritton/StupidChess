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
}

// stopped reports whether the search has been asked to abort.
func (s *AlphaBetaSearch) stopped() bool {
	return atomic.LoadInt32(&s.stop) == 1
}

func NewAlphaBetaSearch(requests chan Request, responses chan string, evalUs position.Evaluator, evalThem position.Evaluator) *AlphaBetaSearch {
	return &AlphaBetaSearch{
		requests:  requests,
		responses: responses,
		evalUs:    evalUs,
		evalThem:  evalThem,
		tt:        newTranspositionTable(),
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
			s.options.MoveTime = DefaultTimeManager(timeRemaining, increment)
		}

		s.responses <- fmt.Sprintf("info string searching for %s/%s (inc %s)", s.options.MoveTime, timeRemaining, increment)

		// Best move/PV from the last FULLY COMPLETED depth. A depth interrupted by the clock is
		// discarded, so the engine never commits to a half-searched (and possibly blundering) move.
		bestMove := position.NoMove

		// Root moves: ordered by a quick static eval for the first iteration, then re-sorted by the
		// real search scores on subsequent iterations (so the best move is searched first).
		legalMoves := pos.MovesLegalWithEvaluation(position.EvalSimple)
		if moves := legalMoves.AsSlice(); len(moves) > 0 {
			bestMove = moves[0] // fallback so we always have a legal move, even if depth 1 is interrupted
		}

		// Iterative deepening.
		for depth := uint(1); depth <= s.options.Depth; depth++ {
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

			// Stop if we are out of time for this move.
			if time.Since(s.startTime) > s.options.MoveTime {
				break
			}
		}

		s.responses <- fmt.Sprintf("bestmove %s", bestMove.String())
	}

	return nil
}

// maxQuiescencePly caps quiescence recursion as a safety valve against pathological capture/check
// sequences. Captures alone are self-terminating (material is finite), but check chains may not be.
const maxQuiescencePly = 64

func (s *AlphaBetaSearch) search(alpha int16, beta int16, depth uint, ply int, pv *pvList, pos *position.Position) int16 {
	s.nodeCount++

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
	var hash uint64
	if s.tt != nil {
		hash = pos.ZobristHash()
		if e, ok := s.tt.probe(hash); ok {
			ttMove = e.move
			if uint(e.depth) >= depth {
				switch {
				case e.bound == boundExact:
					return e.score
				case e.bound == boundLower && e.score >= beta:
					return e.score
				case e.bound == boundUpper && e.score <= alpha:
					return e.score
				}
			}
		}
	}

	bestScore := position.NoEval
	bestMove := position.NoMove
	legalCount := 0
	cutoff := false

	// TODO: doesn't yet understand draw by threefold repetition

	var childPV pvList

	// Stage 1: search the transposition-table move before generating anything. During iterative
	// deepening it is frequently the best move and produces an immediate cutoff, in which case the
	// whole move list never has to be generated or ordered. Equal hashes guarantee the stored move's
	// prior-state bits are valid here, so MakeMove/UndoMove round-trip correctly.
	if ttMove != position.NoMove && pos.MakeMove(ttMove) {
		legalCount++
		childPV.clear()
		score := -s.search(-beta, -alpha, depth-1, ply+1, &childPV, pos)
		pos.UndoMove(ttMove)

		bestScore = score
		bestMove = ttMove
		pv.catenate(ttMove, &childPV)
		if score > alpha {
			alpha = score
		}
		if alpha >= beta {
			cutoff = true
		}
	}

	if !cutoff && s.stopped() {
		return alpha
	}

	// Stage 2: generate, order (captures first by MVV-LVA) and search the remaining moves. Legality is
	// checked lazily — MakeMove returns false when a move leaves our own king in check, which is
	// cheaper than fully filtering the list up front.
	if !cutoff {
		moves := pos.MovesPseudolegal()
		moves.OrderMVVLVA()

		for _, move := range moves.AsSlice() {
			if ttMove != position.NoMove &&
				move.From() == ttMove.From() && move.To() == ttMove.To() && move.Promotion() == ttMove.Promotion() {
				continue // already searched in stage 1
			}
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
				break // beta cutoff: the opponent won't allow this line
			}

			if time.Since(s.startTime) > s.options.MoveTime {
				atomic.StoreInt32(&s.stop, 1)
			}
			// Honour an explicit node limit (`go nodes N`); the default is math.MaxUint, so it never
			// fires unless a limit was actually requested.
			if uint(s.nodeCount) >= s.options.Nodes {
				atomic.StoreInt32(&s.stop, 1)
			}
			if s.stopped() {
				return alpha // aborted: don't store a partial result
			}
		}
	}

	// No legal move: checkmate if in check, otherwise stalemate.
	if legalCount == 0 {
		if pos.KingInCheck(pos.SideToMove) {
			return position.MinEval + int16(ply) + 1
		}
		return 0 // stalemate; TODO: return a contempt value instead
	}

	// Store the result. Mate scores are ply-relative, so they are kept out of the ply-agnostic table.
	if s.tt != nil && !isMateScore(bestScore) {
		bound := boundExact
		if bestScore <= alphaOrig {
			bound = boundUpper
		} else if bestScore >= beta {
			bound = boundLower
		}
		s.tt.store(hash, depth, bestScore, bound, bestMove)
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
