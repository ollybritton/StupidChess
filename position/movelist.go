package position

import "sort"

// MoveList represents a list of moves that can be sorted and filtered.
type MoveList struct {
	Moves []Move
}

// NewMoveList returns a new list of moves of a given size.
func NewMoveList(size uint) *MoveList {
	return &MoveList{Moves: make([]Move, 0, size)}
}

// Append add a move to the move list.
func (l *MoveList) Append(move Move) {
	l.Moves = append(l.Moves, move)
}

// AppendMany adds multiple moves to the moves list.
func (l *MoveList) AppendMany(moves []Move) {
	l.Moves = append(l.Moves, moves...)
}

// AppendFromBitboard
func (l *MoveList) AppendFromBitboard(piece ColoredPiece,
	fromFunc func(uint8) uint8,
	bitboard Bitboard,
	squares []ColoredPiece,
	castling CastlingAvailability,
	enPassantTarget uint8,
) {
	for to := bitboard.FirstOn(); to <= bitboard.LastOn() && to != 64; to++ {
		if bitboard.IsOn(to) {
			l.Moves = append(l.Moves, (NewMove(fromFunc(to), to, piece, squares[to], None, castling, enPassantTarget)))
		}
	}
}

// AsSlice returns the moves in the move list as a slice.
func (l *MoveList) AsSlice() []Move {
	return l.Moves
}

// Len returns the number of moves in the move list.
func (l *MoveList) Len() int {
	return len(l.Moves)
}

// Copy returns a copied version of the move list.
func (l *MoveList) Copy() *MoveList {
	newMoves := append([]Move{}, l.Moves...)
	return &MoveList{newMoves}
}

// Less returns whether two different moves have a larger or smaller evaluation than the other.
// This is used to implement sort.Interface so that moves can be sorted by their evaluation.
func (l *MoveList) Less(i, j int) bool {
	return l.Moves[i].Eval() < l.Moves[j].Eval()
}

// Swap swaps the moves with indicies i and j.
// This is used to implement sort.Interface so that moves can be sorted by their evaluaiton.
func (l *MoveList) Swap(i, j int) {
	l.Moves[i], l.Moves[j] = l.Moves[j], l.Moves[i]
}

// Sort sorts the moves in the move list by their evaluation. Here moves with higher evaluation come first.
// It is a shorthand for sort.Sort(moveList).
func (l *MoveList) Sort() {
	sort.Sort(sort.Reverse(l))
}

// Filter removes moves in the move list according to a function that evaluates a move and says whether it is allowed in the
// list or not.
func (l *MoveList) Filter(allowedFunc func(Move) bool) {
	out := make([]Move, 0, len(l.Moves))

	for _, move := range l.Moves {
		if allowedFunc(move) {
			out = append(out, move)
		}
	}

	l.Moves = out
}

// FilterMap removes moves in the move list according to a function that evaluates a move and says whether it is allowed in the
// list or not, and also returns a potentially modified move if it is allowed in the list.
func (l *MoveList) FilterMap(allowedFunc func(Move) (bool, Move)) {
	out := make([]Move, 0, len(l.Moves))

	for _, move := range l.Moves {
		allowed, newMove := allowedFunc(move)
		if allowed {
			out = append(out, newMove)
		}

	}

	l.Moves = out
}
