package board

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
	occupied := [2]Bitboard{}
	pieces := [6]Bitboard{}
	for y, rank := range ranks {
		var x int = y * 8
		var i int

		if len(rank) <= 0 || len(rank) > 8 {
			return nil, fmt.Errorf("invalid FEN, rank %v is too long", rank)
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
				occupied[White].Set(uint(x))
				pieces[Pawn].Set(uint(x))
			case 'p':
				squares[x] = BlackPawn
				occupied[Black].Set(uint(x))
				pieces[Pawn].Set(uint(x))
			case 'N':
				squares[x] = WhiteKnight
				occupied[White].Set(uint(x))
				pieces[Knight].Set(uint(x))
			case 'n':
				squares[x] = BlackKnight
				occupied[Black].Set(uint(x))
				pieces[Knight].Set(uint(x))
			case 'B':
				squares[x] = WhiteBishop
				occupied[White].Set(uint(x))
				pieces[Bishop].Set(uint(x))
			case 'b':
				squares[x] = BlackBishop
				occupied[Black].Set(uint(x))
				pieces[Bishop].Set(uint(x))
			case 'R':
				squares[x] = WhiteRook
				occupied[White].Set(uint(x))
				pieces[Rook].Set(uint(x))
			case 'r':
				squares[x] = BlackRook
				occupied[Black].Set(uint(x))
				pieces[Rook].Set(uint(x))
			case 'Q':
				squares[x] = WhiteQueen
				occupied[White].Set(uint(x))
				pieces[Queen].Set(uint(x))
			case 'q':
				squares[x] = BlackQueen
				occupied[Black].Set(uint(x))
				pieces[Queen].Set(uint(x))
			case 'K':
				squares[x] = WhiteKing
				occupied[White].Set(uint(x))
				pieces[King].Set(uint(x))
				kingLocation[White] = uint8(x)
			case 'k':
				squares[x] = BlackKing
				occupied[Black].Set(uint(x))
				pieces[King].Set(uint(x))
				kingLocation[Black] = uint8(x)
			}

			i++
			x++

			if x > (y*8)+8 {
				return nil, fmt.Errorf("invalid FEN string %v, rank is too long", input)
			}
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
	for y := 0; y < 8; y++ {
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

		if y != 7 {
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

	for y := 7; y >= 0; y-- {
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
	out.WriteString("     ")
	out.WriteString(divider)

	for i, rank := range ranks {
		out.WriteString("  ")
		out.WriteString(fmt.Sprint(8 - i))
		out.WriteString("  ")
		out.WriteString(rank)
		out.WriteString("\n     ")
		out.WriteString(divider)
	}

	out.WriteString("\n")
	out.WriteString("       A   B   C   D   E   F   G   H")

	return out.String()
}

// HasEnPassant returns true if the current player has a valid en passant move.
func (p *Position) HasEnPassant() bool {
	return p.EnPassant != 255
}
