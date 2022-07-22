package position

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// Position represents a chess position.
type Position struct {
	Squares  [64]ColoredPiece // Squares stores an array of pieces, represented by integers starting at 0 for A1 and ending at 63 for H8.
	Occupied [2]Bitboard      // Occupied holds two bitboards, one for white and one for black, indicating pieces of that color in a certain square.
	Pieces   [6]Bitboard      // Pieces holds 6 bitboards, one for each type of piece (pawn, knight, bishop, rook, queen, king).

	KingLocation [2]uint8 // KingLocation holds the position in the Squares array for each side's king.
	EnPassant    uint8    // EnPassant holds the position in the Squares array for the en passant target square (i.e. the square that a pawn passed over while moving two squares. It is equal to 255 when there is no target.

	Castling CastlingAvailability // Castling holds castling availability for each side.

	SideToMove Color

	HalfmoveClock uint // HalfmoveClock stores the number of halfmoves since the last capture or pawn advance.
	FullMoves     uint // FullMoves stores the number of full moves.
}

const StartingPosition string = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"

// NewPositionFromFEN converts a valid FEN string into a Board struct.
// FEN strings look like so:
//   rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1
// In general:
// 	 <rank 1>/<rank 2>/<rank 3>/<rank 4>/<rank 5>/<rank 6>/<rank 7>/<rank 8> <side to move> <castling rights> <en passant target> <halfmove clock> <full moves>
func NewPositionFromFEN(input string) (*Position, error) {
	sections := strings.Split(input, " ")

	if len(sections) != 6 {
		return nil, fmt.Errorf("fen string %q does not have the required 6 sections", input)
	}

	ranks := strings.Split(sections[0], "/")

	if len(ranks) != 8 {
		return nil, fmt.Errorf("fen string %q does not have 8 ranks", input)
	}

	rawSideToMove := sections[1]
	castlingRights := sections[2]
	rawEnPassantTarget := sections[3]
	rawHalfmoveClock := sections[4]
	rawFullMoves := sections[5]

	squares := [64]ColoredPiece{}
	kingLocation := [2]uint8{}

	// Iterate through the given ranks, writing to the squares array and saving the location of the king.
	// Ranks are given from the top down, starting with the black pieces.
	occupied := [2]Bitboard{}
	pieces := [6]Bitboard{}

	for y := 0; y < 8; y++ {
		rank := ranks[y]
		var x int = (7 - y) * 8
		var i int

		if len(rank) <= 0 || len(rank) > 8 {
			return nil, fmt.Errorf("invalid FEN, rank %q is too long", rank)
		}

		for i != len(rank) {
			char := rank[i]

			switch char {
			case '1', '2', '3', '4', '5', '6', '7', '8':
				blanks, _ := strconv.Atoi(string(char))

				for t := x; t < x+blanks; t++ {
					squares[t] = Empty
				}

				x += blanks
				i++
				continue

			case 'P':
				squares[x] = WhitePawn
				occupied[White].On(uint8(x))
				pieces[Pawn].On(uint8(x))
			case 'p':
				squares[x] = BlackPawn
				occupied[Black].On(uint8(x))
				pieces[Pawn].On(uint8(x))
			case 'N':
				squares[x] = WhiteKnight
				occupied[White].On(uint8(x))
				pieces[Knight].On(uint8(x))
			case 'n':
				squares[x] = BlackKnight
				occupied[Black].On(uint8(x))
				pieces[Knight].On(uint8(x))
			case 'B':
				squares[x] = WhiteBishop
				occupied[White].On(uint8(x))
				pieces[Bishop].On(uint8(x))
			case 'b':
				squares[x] = BlackBishop
				occupied[Black].On(uint8(x))
				pieces[Bishop].On(uint8(x))
			case 'R':
				squares[x] = WhiteRook
				occupied[White].On(uint8(x))
				pieces[Rook].On(uint8(x))
			case 'r':
				squares[x] = BlackRook
				occupied[Black].On(uint8(x))
				pieces[Rook].On(uint8(x))
			case 'Q':
				squares[x] = WhiteQueen
				occupied[White].On(uint8(x))
				pieces[Queen].On(uint8(x))
			case 'q':
				squares[x] = BlackQueen
				occupied[Black].On(uint8(x))
				pieces[Queen].On(uint8(x))
			case 'K':
				squares[x] = WhiteKing
				occupied[White].On(uint8(x))
				pieces[King].On(uint8(x))
				kingLocation[White] = uint8(x)
			case 'k':
				squares[x] = BlackKing
				occupied[Black].On(uint8(x))
				pieces[King].On(uint8(x))
				kingLocation[Black] = uint8(x)
			}

			i++
			x++

			// if x > (y*8)+8 {
			// 	return nil, fmt.Errorf("invalid FEN string %v, rank is too long", input)
			// }
		}
	}

	var enPassantTarget uint8
	if rawEnPassantTarget == "-" {
		enPassantTarget = 255
	} else {
		enPassantTarget = StringToSquare(rawEnPassantTarget)
	}

	var sideToMove Color

	if rawSideToMove == "w" {
		sideToMove = White
	} else if rawSideToMove == "b" {
		sideToMove = Black
	} else {
		return nil, fmt.Errorf("side to move is %q, not 'b' or 'w' as expected in FEN string %v", rawSideToMove, input)
	}

	halfmoveClock, err := strconv.Atoi(rawHalfmoveClock)
	if err != nil {
		return nil, fmt.Errorf("invalid halfmove clock %q in fen string %q", rawHalfmoveClock, input)
	}

	fullMoves, err := strconv.Atoi(rawFullMoves)
	if err != nil {
		return nil, fmt.Errorf("invalid full moves %q in fen string %q", rawFullMoves, input)
	}

	if castlingRights == "" {
		return nil, fmt.Errorf("invalid FEN string %v, castling rights are omitted", input)
	}

	return &Position{
		Squares:       squares,
		Occupied:      occupied,
		Pieces:        pieces,
		KingLocation:  kingLocation,
		EnPassant:     enPassantTarget,
		Castling:      castlingAvailabilityFromString(castlingRights),
		SideToMove:    sideToMove,
		HalfmoveClock: uint(halfmoveClock),
		FullMoves:     uint(fullMoves),
	}, nil
}

// StringFEN returns the current position's FEN string.
// FEN strings look like so:
//   rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1
// In general:
// 	 <rank 1>/<rank 2>/<rank 3>/<rank 4>/<rank 5>/<rank 6>/<rank 7>/<rank 8> <side to move> <castling rights> <en passant target> <halfmove clock> <full moves>
func (b *Position) StringFEN() string {
	var out bytes.Buffer

	// Ranks
	for y := 7; y >= 0; y-- {
		empty := 0

		for x := 0; x < 8; x++ {
			i := x + (y * 8)
			curr := b.Squares[i]

			if curr == Empty {
				empty += 1
				continue
			} else if empty != 0 {
				out.WriteString(fmt.Sprint(empty))
			}

			empty = 0
			out.WriteString(curr.String())
		}

		if empty != 0 {
			out.WriteString(fmt.Sprint(empty))
		}

		if y != 0 {
			out.WriteString("/")
		}
	}

	out.WriteString(" ")

	// Side to move
	if b.SideToMove == White {
		out.WriteString("w")
	} else {
		out.WriteString("b")
	}

	out.WriteString(" ")

	// Castling rights
	out.WriteString(b.Castling.String())
	out.WriteString(" ")

	// En passant target
	if b.HasEnPassant() {
		out.WriteString(SquareToString(b.EnPassant))
	} else {
		out.WriteString("-")
	}

	out.WriteString(" ")

	// Halfmove clock
	out.WriteString(fmt.Sprint(b.HalfmoveClock))
	out.WriteString(" ")

	// Full moves
	out.WriteString(fmt.Sprint(b.FullMoves))

	return out.String()
}

// PrettyPrint returns a string containing the current position as if it were on a chess board.
func (p *Position) PrettyPrint() string {
	ranks := []string{}

	for y := 0; y < 8; y++ {
		pieces := []string{}

		for i := (y * 8); i < (y*8)+8; i++ {
			char := p.Squares[i].String()

			if char == "?" {
				char = " "
			}

			pieces = append(pieces, char)
		}

		ranks = append(ranks, "| "+strings.Join(pieces, " | ")+" |")
	}

	var out bytes.Buffer

	divider := strings.Repeat("-", 33) + "\n"

	out.WriteString(fmt.Sprintf("To move: %s, Castling status: %s, En passant target: %s", p.SideToMove, p.Castling, SquareToString(p.EnPassant)))

	out.WriteString("\n     ")
	out.WriteString(divider)

	for i := 7; i >= 0; i-- {
		rank := ranks[i]
		out.WriteString("  ")
		out.WriteString(fmt.Sprint(i + 1))
		out.WriteString("  ")
		out.WriteString(rank)
		out.WriteString("\n     ")
		out.WriteString(divider)
	}

	out.WriteString("\n")
	out.WriteString("       A   B   C   D   E   F   G   H")

	return out.String()
}

// MakeMove makes a move on the chess board, or returns an error if it is invalid.
// Bitboards for occupation and piece locations are updated through the setSquare function.
func (p *Position) MakeMove(m Move) bool {
	// Need to handle 6 special cases:
	// - White king moving
	// - Black king moving
	// (These two cases can move two pieces at once and disable castling)

	// - White rook moving
	// - Black rook moving
	// (These two cases mean some castling privileges may be lost)

	// - White pawn moving forward two squares onto an empty square
	// - Black pawn moving forward two squares onto an empty square
	// (These two cases either perform an en passant capture or set the en passant target square)

	movingPiece := p.Squares[m.From]
	var newEnPassantTarget uint8 = 255

	switch {
	case movingPiece == WhiteKing:
		// Disable any type of castling for the white king as they have moved.
		p.Castling.off(longW | shortW)

		// The king has moved two squares and so it is known they have castled.
		// Here we only need to worry about moving the rook, as the king is moved by the general code outside of the switch.
		if abs(int(m.From)-int(m.To)) == 2 {
			// Determine if they have castled long or short.
			if m.To == SquareG1 { // Short castle
				p.setSquare(SquareF1, WhiteRook)
				p.setSquare(SquareH1, Empty)
			} else { // Long castle
				p.setSquare(SquareD1, WhiteRook)
				p.setSquare(SquareA1, Empty)
			}
		}
	case movingPiece == BlackKing:
		// Disable any type of castling for the black king as they have moved.
		p.Castling.off(longB | shortB)

		// The king has moved two squares and so it is known they have castled.
		// Here we only need to worry about moving the rook, as the king is moved by the general code outside of the switch.
		if abs(int(m.From)-int(m.To)) == 2 {
			// Determine if they have castled long or short.
			if m.To == SquareG8 { // Short castle
				p.setSquare(SquareF8, BlackRook)
				p.setSquare(SquareH8, Empty)
			} else { // Long castle
				p.setSquare(SquareD8, BlackRook)
				p.setSquare(SquareA8, Empty)
			}
		}
	case movingPiece == WhiteRook:
		// Disable castling for the king on the side where the rook has moved.
		if m.From == SquareA1 {
			p.Castling.off(longW)
		} else if m.From == SquareH1 {
			p.Castling.off(shortW)
		}
	case movingPiece == BlackRook:
		// Disable castling for the king on the side where the rook has moved.
		if m.From == SquareA8 {
			p.Castling.off(longB)
		} else if m.From == SquareH8 {
			p.Castling.off(shortB)
		}
	case p.Squares[m.To] == WhiteRook:
		// Disable castling when a white rook is taken.
		if m.To == SquareA1 {
			p.Castling.off(longW)
		} else if m.To == SquareH1 {
			p.Castling.off(shortW)
		}
	case p.Squares[m.To] == BlackRook:
		// Disable castling when a black rook is taken.
		if m.To == SquareA8 {
			p.Castling.off(longB)
		} else if m.To == SquareH8 {
			p.Castling.off(shortB)
		}
	case movingPiece == WhitePawn && p.Squares[m.To] == Empty:
		if m.To-m.From == 16 {
			// The pawn has moved two full squares onto an empty square.
			// Therefore there is a new en passant target on the rank between its original position and its new position.
			newEnPassantTarget = m.From + 8
		} else if m.To-m.From == 7 || m.To-m.From == 9 {
			// The pawn has moved diagonally onto an empty square -- this must be an en passant capture.
			p.setSquare(p.EnPassant-8, Empty)
		}
	case movingPiece == BlackPawn && p.Squares[m.To] == Empty:
		if m.From-m.To == 16 {
			// The pawn has moved two full squares onto an empty square.
			// Therefore there is a new en passant target on the rank between its original position and its new position.
			newEnPassantTarget = m.To + 8
		} else if m.From-m.To == 7 || m.From-m.To == 9 {
			// The pawn has moved diagonally onto an empty square -- this must be an en passant capture.
			p.setSquare(p.EnPassant+8, Empty)
		}
	}

	p.EnPassant = newEnPassantTarget
	p.setSquare(m.From, Empty)

	if m.Promotion == None {
		p.setSquare(m.To, movingPiece)
	} else {
		p.setSquare(m.To, m.Promotion.OfColor(p.SideToMove))
	}

	// TODO: this whole thing of inverting the side to move and then checking if the original side to move is okay seems a little convoluted
	// is there a better way?
	p.SideToMove = p.SideToMove.Invert()

	if p.KingInCheck(p.SideToMove.Invert()) {
		p.UndoMove(m)
		return false
	}

	// If the side to move is now white, we can update the fullmove clock.
	if p.SideToMove == White {
		p.FullMoves += 1
	}

	// If there has been no capture and it's not a pawn move, then we need to increment the halfmove clock.
	if movingPiece != WhitePawn && movingPiece != BlackPawn && p.Squares[m.To] != Empty {
		p.HalfmoveClock += 1
	} else {
		p.HalfmoveClock = 0
	}

	return true
}

// UndoMove undoes the last move.
func (p *Position) UndoMove(m Move) {
	p.EnPassant = m.PriorEnPassantTarget
	p.Castling = m.PriorCastling

	p.setSquare(m.To, m.Captured)
	p.setSquare(m.From, m.Moved)

	if m.Moved.Colorless() == Pawn {
		if m.To == p.EnPassant {
			// This was an en passant move, so we need to undo setting m.To to the captured piece above and instead place
			// the pawns properly.
			p.setSquare(m.To, Empty)

			switch int(m.To) - int(m.From) {
			case 7, 9: // I think this code disagrees with the source of GoBit, if there's a bug related to undoing moves it might be here.
				p.setSquare(m.To-8, BlackPawn)
			case -7, -9:
				p.setSquare(m.To+8, WhitePawn)
			}
		}
	} else if m.Moved.Colorless() == King {
		sideMoving := m.Moved.Color()

		switch int(m.To) - int(m.From) {
		case 2: // Short castling for white, long castling for black
			if sideMoving == White {
				p.setSquare(SquareH1, WhiteRook)
				p.setSquare(SquareF1, Empty)
			} else if sideMoving == Black {
				p.setSquare(SquareH8, BlackRook)
				p.setSquare(SquareF8, Empty)
			}

		case -2: // Long castling for white, short castling for black
			if sideMoving == White {
				p.setSquare(SquareA1, WhiteRook)
				p.setSquare(SquareD1, Empty)
			} else if sideMoving == Black {
				p.setSquare(SquareA8, BlackRook)
				p.setSquare(SquareD8, Empty)
			}
		}
	}

	p.SideToMove = p.SideToMove.Invert()

	// If the side to move is now black, we need to subtract one from the fullmove clock.
	if p.SideToMove == Black {
		p.FullMoves -= 1
	}

	// TODO: update proper recovery of the fullmove and halfmove clock

}

// KingInCheck returns true if the given side to move has their king in check.
// TODO: actually make this work
func (p *Position) KingInCheck(side Color) bool {
	return p.IsAttacked(p.KingLocation[side], side.Invert())
}

// HasEnPassant returns true if the current player has a valid en passant move.
func (p *Position) HasEnPassant() bool {
	return p.EnPassant != 255
}

// IsEmpty returns true if the specified square is empty.
func (p *Position) IsEmpty(square uint8) bool {
	return p.Squares[square] == Empty
}

// IsValid returns true if the specified square is within the confines of the board.
func (p *Position) IsValid(square uint8) bool {
	return square <= 64
}

// OnRank returns true if the specified square is on the given rank.
// TODO: write tests for seeing if a particular square is on a given rank or file
func (p *Position) OnRank(square uint8, rank uint8) bool {
	rank -= 1
	return square >= 8*rank && square <= 8*rank+7
}

// OnFileA returns true if the specified square is on the A-file.
func (p *Position) OnFileA(square uint8) bool {
	return square%8 == 0
}

// OnFileH returns true if the specified square is on the H-file.
// TODO: why is this stuff a method on the struct?
func (p *Position) OnFileH(square uint8) bool {
	return square%8 == 7
}

// IsAttacked returns true if the square given is attacked by the color given.
func (p *Position) IsAttacked(square uint8, color Color) bool {
	if p.HasEnPassant() {
		if p.Squares[square] == WhitePawn {
			if p.EnPassant == square-8 {
				return true
			}
		}

		if p.Squares[square] == BlackPawn {
			if p.EnPassant == square+8 {
				return true
			}
		}
	}

	// Pawn attacks
	if !p.OnFileA(square) {
		if color == Black && !p.OnRank(square, 8) && p.Squares[square+7] == BlackPawn {
			return true
		} else if color == White && !p.OnRank(square, 1) && p.Squares[square-9] == WhitePawn {
			return true
		}
	}

	if !p.OnFileH(square) {
		if color == Black && !p.OnRank(square, 8) && p.Squares[square+9] == BlackPawn {
			return true
		} else if color == White && !p.OnRank(square, 1) && p.Squares[square-7] == WhitePawn {
			return true
		}
	}

	// Knight attacks
	if knightMoves[square]&p.Pieces[Knight]&p.Occupied[color] != 0 {
		return true
	}

	// King attacks
	if kingMoves[square]&p.Pieces[King]&p.Occupied[color] != 0 {
		return true
	}

	// Rook/queen attacks
	rookBlockers := (p.Occupied[color.Invert()] | p.Occupied[color]) & rookMasks[square]
	rookKey := (uint64(rookBlockers) * rookMagics[square].multiplier) >> uint64(rookMagics[square].shift)
	if (rookMoves[square][rookKey] & p.Occupied[color] & (p.Pieces[Rook] | p.Pieces[Queen])) != 0 {
		return true
	}

	// Bishop/queen attacks
	bishopBlockers := (p.Occupied[color.Invert()] | p.Occupied[color]) & bishopMasks[square]
	bishopKey := (uint64(bishopBlockers) * bishopMagics[square].multiplier) >> uint64(bishopMagics[square].shift)
	return (bishopMoves[square][bishopKey] & p.Occupied[color] & (p.Pieces[Bishop] | p.Pieces[Queen])) != 0
}

// setSquare sets a specific square on the board to a empty or to a certain piece.
// This updates the bitboards as well as modifying Squares.
func (p *Position) setSquare(square uint8, newPiece ColoredPiece) {
	oldPiece := p.Squares[square]

	if oldPiece == newPiece {
		return
	}

	p.Squares[square] = newPiece

	if oldPiece != Empty {
		p.Occupied[oldPiece.Color()].Off(square)
		p.Pieces[oldPiece.Colorless()].Off(square)
	}

	if newPiece != Empty {
		p.Occupied[newPiece.Color()].On(square)
		p.Pieces[newPiece.Colorless()].On(square)
	}

	if newPiece == WhiteKing || newPiece == BlackKing {
		p.KingLocation[newPiece.Color()] = square
	}
}
