package search

import (
	"fmt"
	"time"

	"github.com/ollybritton/StupidChess/position"
)

type AlphaBetaSearch struct {
	requests  chan Request
	responses chan string

	startTime time.Time
	nextTime  time.Time
	nodeCount int

	options SearchOptions
}

func NewAlphaBetaSearch(requests chan Request, responses chan string) *AlphaBetaSearch {
	return &AlphaBetaSearch{
		requests:  requests,
		responses: responses,
	}
}

func (s *AlphaBetaSearch) Requests() chan Request {
	return s.requests
}

func (s *AlphaBetaSearch) Responses() chan string {
	return s.responses
}

func (s *AlphaBetaSearch) Stop() {
	s.options.Stop = true
}

func (s *AlphaBetaSearch) Root() error {
	var pv pvList      // Holds the principle variation
	var childPV pvList // Holds the principle variation of the position after the first move is made

	childPV.new()

	for request := range s.requests {
		pos := request.pos // Position we are searching

		s.startTime = time.Now()    // Record start time so we know to stop if time is up
		s.nextTime = time.Now()     // Record next time as a counter so we can periodically print information
		s.nodeCount = 0             // Record number of nodes so we can stop after searching a certain number of nodes
		s.options = request.options // Store options in the search struct so we don't have to explicitly pass around.
		s.options.Stop = false      // Make sure we don't stop straight away if we were told to stop previously

		var timeRemaining, increment time.Duration

		if pos.SideToMove == position.White {
			timeRemaining = s.options.WhiteTimeRemaining
			increment = s.options.WhiteIncrement
		} else {
			timeRemaining = s.options.BlackTimeRemaining
			increment = s.options.BlackTimeRemaining
		}

		if s.options.MoveTime == 0 {
			s.options.MoveTime = DefaultTimeManager(timeRemaining, increment)
		}

		s.responses <- fmt.Sprintf("info string searching for %s/%s", s.options.MoveTime, timeRemaining)

		// Keep track of the best move found so far. This is outside the loop so that we can return the best move found
		// if we are asked to stop searching at a particular depth.
		bestMove := position.NoMove

		// Generate legal moves and annotate them with the evaluation after they've taken place so we can improve
		// move ordering in the search.
		legalMoves := pos.MovesLegalWithEvaluation(position.EvalSimple)

		// For loop for iterative deepening
		for depth := uint(1); depth <= s.options.Depth; depth++ {
			// Sort legal moves by the evaluation calculated above
			legalMoves.Sort()

			// Best score for a move found so far
			bestScore := position.NoEval

			for i, move := range legalMoves.AsSlice() {
				// Alpha and beta
				// Alpha here is the best score we can be guaranteed to achieve
				// Beta here is the best score the opposing player can achieve
				alpha, beta := position.MinEval, position.MaxEval

				if s.options.Stop {
					break
				}

				// Clear the child PV so it can be used again for this move
				childPV.clear()

				// Make move, evaluate score of this position, and then undo move.
				pos.MakeMove(move)
				score := -s.search(-beta, -alpha, depth-1, 1, &childPV, pos)
				pos.UndoMove(move)

				if s.options.Stop {
					break
				}

				// Store evaluation of this move so that on the next iteration the move ordering is more effective
				move.SetEval(score)
				legalMoves.Moves[i] = move

				// If this is the best move we've seen so far...
				if score > bestScore {
					// Update bestScore to reflect this
					bestScore = score

					// Update the principle variation to use this move instead
					pv.clear()
					pv.catenate(move, &childPV)

					// Record this as the best move
					bestMove = move

					// Set alpha to this score
					alpha = score
				}

				s.responses <- fmt.Sprintf(
					"info currmove %s currmovenumber %d nodes %d depth %d score cp %d",
					move.String(),
					i+1,
					s.nodeCount,
					depth,
					score*100,
				)
			}

			if time.Since(s.startTime) > s.options.MoveTime {
				break
			}

			diff := time.Since(s.startTime)

			s.responses <- fmt.Sprintf(
				"info depth %d score cp %d nodes %d nps %.0f time %d pv %s",
				depth,
				bestScore*100,
				s.nodeCount,
				1000*(float64(s.nodeCount)/float64(diff.Milliseconds())),
				diff.Milliseconds(),
				pv.String(),
			)
		}

		s.responses <- fmt.Sprintf("bestmove %s", bestMove.String())
	}

	return nil
}

func (s *AlphaBetaSearch) search(alpha int16, beta int16, depth uint, ply int, pv *pvList, pos *position.Position) int16 {
	s.nodeCount++

	// If we're at depth 0, stop recursing and instead return a static evaluation of this position.
	if depth <= 0 {
		return position.ScoreFromPerspective(position.EvalSimple(pos), pos.SideToMove) // TODO: make more customisable
	}

	// Clear the principle variation
	pv.clear()

	// Generate all legal moves in this position
	legalMoves := pos.MovesLegalWithEvaluation(position.EvalSimple)
	legalMoves.Sort()

	// Initialise bestMove and bestScore to hold the best move found so far.
	bestMove, bestScore := position.NoMove, position.NoEval

	// TODO: doesn't yet understand draw by threefold repetition

	var childPV pvList

	for _, move := range legalMoves.AsSlice() {
		childPV.clear()

		pos.MakeMove(move)
		score := -s.search(-beta, -alpha, depth-1, ply+1, &childPV, pos)
		pos.UndoMove(move)

		// If this is the best score we've found so far...
		if score > bestScore {
			// Update bestScore and bestMove to track this (might not need bestMove)
			bestScore = score
			bestMove = move
			_ = bestMove

			// Add this to the principle variation
			pv.catenate(move, &childPV)

		}

		// If this is better than the best score we can guarantee so far, then update alpha to reflect this
		if score > alpha {
			alpha = score
		}

		// Beta cutoff:
		// The opposing player can guarantee a better position for themselves, so there's no point pursuing this position.
		if alpha >= beta {
			break
		}

		// Print info if required
		if time.Since(s.nextTime) >= time.Second {
			//diff := time.Since(s.startTime)
			//s.responses <- fmt.Sprintf("info time %v ndes %v nps %v", diff.Milliseconds(), s.nodeCount, s.nodeCount/int(diff.Seconds()))
			s.nextTime = time.Now()
		}

		if time.Since(s.startTime) > s.options.MoveTime {
			s.options.Stop = true
		}

		// If required to stop early, return alpha since this is the best we can do.
		if s.options.Stop {
			return alpha
		}

	}

	// If we have no moves available, it's either checkmate or stalemate, so return values
	// that reflect this.
	if legalMoves.Len() == 0 {
		if pos.KingInCheck(pos.SideToMove) {
			// Checkmate
			return -30000 + int16(ply) + 1
		}

		// Stalemate
		return 0 // TODO: return contempt value instead?
	}

	return bestScore
}
