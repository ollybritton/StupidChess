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
	options   SearchOptions
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
		s.options = request.options

		alpha, beta := position.MinEval, position.MaxEval
		bestMove, bestScore := position.NoMove, position.NoEval
		depth := request.options.Depth

		legalMoves := pos.MovesLegal()
		slice := legalMoves.AsSlice()

		for _, move := range slice {
			childPV.clear()
			pos.MakeMove(move)

			score := -s.search(-beta, -alpha, depth-1, 1, &childPV, pos)
			s.responses <- fmt.Sprintf("info currmove %s score %d", move.String(), score)

			pos.UndoMove(move)

			move.SetEval(position.ScoreFromPerspective(score, pos.SideToMove))

			if score > bestScore {
				bestScore = score
				pv.clear()
				pv.catenate(move, &childPV)

				bestMove = move
				alpha = score

				s.responses <- fmt.Sprintf("info score cp %v depth %v nodes %v pv %s", bestScore, depth, s.nodeCount, pv.String())
			}
		}

		s.responses <- fmt.Sprintf("info score cp %v depth %v nodes %v pv %s", bestScore, depth, s.nodeCount, pv.String())
		s.responses <- fmt.Sprintf("bestmove %s", bestMove.String())
	}

	return nil
}

func (s *AlphaBetaSearch) search(alpha int16, beta int16, depth uint, ply int, pv *pvList, pos *position.Position) int16 {
	s.nodeCount++

	if depth <= 0 {
		return position.ScoreFromPerspective(position.EvalSimple(pos), pos.SideToMove) // TODO: make more customisable
	}

	pv.clear()

	legalMoves := pos.MovesLegal()
	slice := legalMoves.AsSlice()

	bestMove, bestScore := position.NoMove, position.MinEval

	// BUG: There's issues around forced mates since there's no legal moves, and so the "bestMove" ends up being
	// the null move. How to fix?
	// TODO: doesn't understand draw by threefold repetition

	var childPV pvList

	for _, move := range slice {
		childPV.clear()

		pos.MakeMove(move)
		score := -s.search(-beta, -alpha, depth-1, ply+1, &childPV, pos)
		pos.UndoMove(move)

		if score > bestScore {
			bestScore = score
			bestMove = move
			_ = bestMove

			pv.catenate(move, &childPV)
		}

		if score >= beta {
			break
		}

		if score > alpha {
			alpha = score
		}

		if time.Since(s.nextTime) >= time.Second {
			diff := time.Since(s.startTime)
			s.responses <- fmt.Sprintf("info time %v ndes %v nps %v score %d pv %s", diff.Milliseconds(), s.nodeCount, s.nodeCount/int(diff.Seconds()), bestScore, pv)
			s.nextTime = time.Now()
		}

		if s.stop || time.Since(s.startTime) > s.options.MoveTime {
			return alpha
		}

	}

	if len(slice) == 0 {
		if pos.KingInCheck(pos.SideToMove) {
			return -30000 + int16(ply) + 1
		}

		return 0 // TODO: return contempt value instead?
	}

	return bestScore
}
