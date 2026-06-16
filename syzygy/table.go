package syzygy

import (
	"encoding/binary"
	"fmt"
)

// This file holds the in-memory representation of a loaded .rtbw table and the
// material-key helper. The binary parsing (init_table_wdl, setup_pairs) and the
// probing (encode_*, decompress_pairs) live in init.go and probe.go.
//
// We model only what the WDL path needs. A "bucket" is one (side, file) slice
// of the table. Non-pawn tables have files=1; pawn tables have files=1 or 4.

// pairsData mirrors struct PairsData in tbcore.h: the canonical-Huffman /
// recursive-pairing description for one compressed stream.
type pairsData struct {
	indextable []byte   // raw[idxStart:] (6 bytes per main index entry)
	sizetable  []uint16 // block sizes
	data       []byte   // raw[dataStart:] block-aligned compressed stream
	offset     []uint16 // offset[] read from header (len h); indexed [l-minLen]
	symlen     []uint8  // expanded length minus 1 for each symbol
	sympat     []byte   // 3 bytes per symbol
	blocksize  uint32   // log2 of block byte size
	idxbits    uint32
	minLen     int32
	base       []uint64 // base[] (len h), already shifted; indexed [l-minLen]

	// Single-valued table (data[0]&0x80): every position has the same value.
	isConst    bool
	constValue uint8
}

// bucket is one (side, file) decoded slice: a pairsData plus the piece order
// (pieces), grouping (norm) and placement factors (factor).
type bucket struct {
	precomp *pairsData
	factor  [tbPieces]uint64
	pieces  [tbPieces]uint8
	norm    [tbPieces]uint8
}

// dtzBucket is the single-sided analogue of bucket used by DTZ tables. A .rtbz
// stores only one side (recovered at probe time from flags&1), so there is no
// [side] dimension. The shape otherwise matches bucket so the same encodePiece /
// encodePawn / decompressPairs machinery applies unchanged.
type dtzBucket struct {
	precomp *pairsData
	factor  [tbPieces]uint64
	pieces  [tbPieces]uint8
	norm    [tbPieces]uint8
}

// tbTable is a single loaded .rtbw material configuration.
type tbTable struct {
	ready     int32 // 1 once initTableWDL has parsed this table; read atomically on the probe hot path
	key       uint64
	symmetric bool
	hasPawns  bool
	num       int
	encType   int      // enc_type (0,1,2) for piece tables
	pawns     [2]uint8 // pawns[0], pawns[1] for pawn tables

	split bool // data[4]&1: separate black-to-move tables
	files int  // 1 or 4

	// buckets indexed by [file][side]; side 0 = white-to-move bucket,
	// side 1 = black-to-move (== side 0 when not split).
	buckets [4][2]*bucket

	raw  []byte
	name string

	// DTZ (.rtbz) parsing. dtzReady mirrors `ready` but gates initTableDTZ
	// independently: a table may be WDL-ready but not DTZ-ready. dtzRaw holds
	// the .rtbz bytes (loaded eagerly in Load, parsed lazily in ensureReadyDTZ).
	// dtzFiles is 1 (non-pawn, or single-file pawn) or 4. A DTZ table is
	// single-sided, so a dtzBucket per file (file 0 for non-pawn) suffices.
	dtzReady  int32
	dtzRaw    []byte
	dtzFiles  int
	dtzBucket [4]*dtzBucket
	dtzFlags  [4]uint8     // per-file flags returned by setupPairs (bit0=side, bit1=mapped, ...)
	dtzMap    []byte       // raw[mapBase:]; the flat map[] re-encoding array (when any flag&2)
	dtzMapIdx [4][4]uint16 // [file][wdl class]: byte offset into dtzMap of that sub-table's first entry
}

func le16(b []byte) uint32 { return uint32(binary.LittleEndian.Uint16(b)) }
func le32(b []byte) uint32 { return binary.LittleEndian.Uint32(b) }

// calcKeyFromPcs mirrors calc_key_from_pcs in tbprobe.c. pcs is indexed by the
// tbcore piece codes (white 1..6, black 9..14); mirror swaps colours (xor 8).
func calcKeyFromPcs(pcs []int, mirror bool) uint64 {
	m := 0
	if mirror {
		m = 8
	}
	return uint64(pcs[tbWhiteQueenCode^m])*primeWhiteQueen +
		uint64(pcs[tbWhiteRookCode^m])*primeWhiteRook +
		uint64(pcs[tbWhiteBishopCode^m])*primeWhiteBishop +
		uint64(pcs[tbWhiteKnightCode^m])*primeWhiteKnight +
		uint64(pcs[tbWhitePawnCode^m])*primeWhitePawn +
		uint64(pcs[tbBlackQueenCode^m])*primeBlackQueen +
		uint64(pcs[tbBlackRookCode^m])*primeBlackRook +
		uint64(pcs[tbBlackBishopCode^m])*primeBlackBishop +
		uint64(pcs[tbBlackKnightCode^m])*primeBlackKnight +
		uint64(pcs[tbBlackPawnCode^m])*primeBlackPawn
}

// Piece codes used by calc_key_from_pcs (white 1..6, black ORed with 8).
const (
	tbWhitePawnCode   = tbPawn       // 1
	tbWhiteKnightCode = tbKnight     // 2
	tbWhiteBishopCode = tbBishop     // 3
	tbWhiteRookCode   = tbRook       // 4
	tbWhiteQueenCode  = tbQueen      // 5
	tbBlackPawnCode   = tbPawn | 8   // 9
	tbBlackKnightCode = tbKnight | 8 // 10
	tbBlackBishopCode = tbBishop | 8 // 11
	tbBlackRookCode   = tbRook | 8   // 12
	tbBlackQueenCode  = tbQueen | 8  // 13
)

// pchr maps a piece index 0..5 to its name letter.
var pchr = [6]byte{'K', 'Q', 'R', 'B', 'N', 'P'}

// parseName parses a name like "KQvK" or "KRPvKP" into per-code counts (indexed
// 0..15 by tbcore code), the two material keys, and the basic flags. It mirrors
// the body of init_tb in tbcore.c.
func parseName(name string) (pcs [16]int, key, key2 uint64, hasPawns bool, num int, encType int, pawns [2]uint8, err error) {
	color := 0
	for i := 0; i < len(name); i++ {
		switch name[i] {
		case 'P':
			pcs[tbPawn|color]++
		case 'N':
			pcs[tbKnight|color]++
		case 'B':
			pcs[tbBishop|color]++
		case 'R':
			pcs[tbRook|color]++
		case 'Q':
			pcs[tbQueen|color]++
		case 'K':
			pcs[tbKing|color]++
		case 'v':
			color = 8
		default:
			err = fmt.Errorf("syzygy: bad character %q in table name %q", name[i], name)
			return
		}
	}
	key = calcKeyFromPcs(pcs[:], false)
	key2 = calcKeyFromPcs(pcs[:], true)
	for i := 0; i < 16; i++ {
		num += pcs[i]
	}
	hasPawns = pcs[tbWPawn]+pcs[tbBPawn] > 0

	if hasPawns {
		pawns[0] = uint8(pcs[tbWPawn])
		pawns[1] = uint8(pcs[tbBPawn])
		if pcs[tbBPawn] > 0 && (pcs[tbWPawn] == 0 || pcs[tbBPawn] < pcs[tbWPawn]) {
			pawns[0] = uint8(pcs[tbBPawn])
			pawns[1] = uint8(pcs[tbWPawn])
		}
	} else {
		// enc_type: count singletons.
		j := 0
		for i := 0; i < 16; i++ {
			if pcs[i] == 1 {
				j++
			}
		}
		if j >= 3 {
			encType = 0
		} else if j == 2 {
			encType = 2
		} else {
			// Only for suicide variants; not expected for standard tables.
			j = 16
			for i := 0; i < 16; i++ {
				if pcs[i] < j && pcs[i] > 1 {
					j = pcs[i]
				}
				encType = 1 + j
			}
		}
	}
	return
}
