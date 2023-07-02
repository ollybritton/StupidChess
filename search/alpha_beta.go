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
	stop      bool
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
	s.stop = true
}

func (s *AlphaBetaSearch) Listen() error {
	var pv pvList
	var childPV pvList

	childPV.new()

	for request := range s.requests {
		pos := request.pos
		s.stop = false

		s.startTime = time.Now()
		s.nextTime = time.Now()
		s.nodeCount = 0
		s.stop = request.options.Stop

		alpha, beta := int16(-10000), int16(10000) // TODO: replace with more sensible defaults
		bestMove, bestScore := position.Move(0), int16(-10001)
		depth := request.options.Depth

		legalMoves := pos.MovesLegal()

		slice := legalMoves.AsSlice()

		for _, move := range slice {
			childPV.clear()
			pos.MakeMove(move)

			score := -s.search(-beta, -alpha, depth-1, 1, &childPV, pos)
			s.responses <- fmt.Sprintf("info currmove %s score %d", move.String(), score)

			pos.UndoMove(move)

			move.SetEval(score)

			if score > bestScore {
				bestScore = score
				pv.clear()
				pv.catenate(move, &childPV)

				bestMove = move
				alpha = score

				s.responses <- fmt.Sprintf("info score cp %v depth %v nodes %v pv %s", bestScore, depth, s.nodeCount, pv.String())
			}
		}

		legalMoves.Sort()
		s.responses <- fmt.Sprintf("info score cp %v depth %v nodes %v pv %s", bestScore, depth, s.nodeCount, pv.String())
		s.responses <- fmt.Sprintf("bestmove %s", bestMove.String())
	}

	return nil
}

func (s *AlphaBetaSearch) search(alpha int16, beta int16, depth uint, ply int, pv *pvList, pos *position.Position) int16 {
	s.nodeCount++

	if depth <= 0 {
		if pos.SideToMove == position.Black {
			return -position.EvalSimple(pos) // TODO: make more customisable
		} else {
			return position.EvalSimple(pos)
		}
	}

	pv.clear()

	legalMoves := pos.MovesLegal()
	slice := legalMoves.AsSlice()

	bestMove, bestScore := position.Move(0), int16(-10010)
	// BUG: There's issues around forced mates since there's no legal moves, and so the "bestMove" ends up being
	// the null move. How to fix?

	var childPV pvList

	for _, move := range slice {
		childPV.clear()
		pos.MakeMove(move)
		score := -s.search(-beta, -alpha, depth-1, ply+1, &childPV, pos)
		pos.UndoMove(move)

		if score > bestScore {
			bestScore = score
			pv.catenate(move, &childPV)

			if score >= beta {
				return score
			}

			if score > alpha {
				bestMove = move
				_ = bestMove
				alpha = score
			}
		}

		if time.Since(s.nextTime) >= time.Second {
			diff := time.Since(s.startTime)
			s.responses <- fmt.Sprintf("info time %v ndes %v nps %v score %d pv %s", diff.Milliseconds(), s.nodeCount, s.nodeCount/int(diff.Seconds()), bestScore, pv)
			s.nextTime = time.Now()
		}

		if s.stop {
			return alpha
		}
	}

	return bestScore
}
