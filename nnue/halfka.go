// This file implements the modern Stockfish 15.1 "big" NNUE evaluator,
// HalfKAv2_hm with a 1024x2 feature transformer, eight PSQT buckets and eight
// per-material layer stacks. It is a PARALLEL implementation to the classic
// HalfKP code in nnue.go: it introduces its own types (KANetwork,
// KAAccumulator) and its own constants/tables (all KA-prefixed) and does not
// disturb the HalfKP path. It reuses the package-level little-endian readers
// (byteReader) and the clip8 / perspectiveIndex helpers from nnue.go.
//
// It is a faithful port of reference/sf15/src/nnue/: nnue_architecture.h,
// nnue_feature_transformer.h, features/half_ka_v2_hm.{h,cpp} and the three
// layer headers (affine_transform.h, clipped_relu.h, sqr_clipped_relu.h). The
// intent is bit-exact agreement with the Stockfish 15.1 NNUE evaluation of the
// distributed net nn-ad9b42354671.nnue.
//
// # Architecture
//
//	HalfKAv2_hm feature set : 22528 input features per perspective
//	                          (king-bucketed + horizontally mirrored; kings
//	                          ARE encoded as features here, unlike HalfKP)
//	Feature transformer     : 22528 -> 1024, int16 weights/biases, plus an
//	                          int32 PSQT side-channel of 8 buckets per feature
//	Pairwise transform      : the 1024-wide accumulator of each perspective is
//	                          split into two 512 halves; corresponding clipped
//	                          elements are multiplied (sum0*sum1/128) to give a
//	                          512-byte uint8 vector per perspective, 1024 total
//	Layer stack (per bucket): fc_0 1024->16 (15 real + 1 forwarded)
//	                          ac_sqr_0 / ac_0 activations, concatenated to 30
//	                          fc_1 30->32, ac_1, fc_2 32->1
//	Output blend            : fc_2_out[0] + fc_0_out[15]*9600/8128 = positional
//	Final value             : (psqt + positional) / OutputScale, stm POV
//
// # Unit conversion
//
// The internal value (psqt + positional)/OutputScale is in Stockfish's internal
// units, side-to-move POV. Stockfish's "eval" command prints the White-POV value
// divided by NormalizeToPawnValue (=361) as pawns. So:
//
//	internalWhitePOV = (psqt + positional) / 16   (flip sign if Black to move)
//	pawns            = float64(internalWhitePOV) / 361.0
//	centipawns       = internalWhitePOV * 100 / 361
//
// EvalKA returns centipawns side-to-move-relative, matching how the HalfKP Eval
// is consumed by the engine; KANetwork.EvalInternal returns the raw internal
// (psqt+positional)/16 stm-POV value for exact oracle comparison in tests.
package nnue

import (
	"math/bits"

	"github.com/ollybritton/StupidChess/position"
)

// ---------------------------------------------------------------------------
// Constants (reference/sf15/src/nnue/nnue_architecture.h, nnue_common.h)
// ---------------------------------------------------------------------------

const (
	// KAVersion is the magic version word at the head of a HalfKAv2_hm file.
	// Note: distinct from the HalfKP Version (0x7AF32F16).
	KAVersion uint32 = 0x7AF32F20

	// KAHalfDims is the per-perspective accumulator size (TransformedFeatureDimensions).
	KAHalfDims = 1024

	// KAPSQTBuckets is the number of PSQT side-channel buckets per feature.
	KAPSQTBuckets = 8

	// KALayerStacks is the number of per-material layer stacks (one selected by bucket).
	KALayerStacks = 8

	// KAOutputScale and KAWeightScaleBits are the quantization constants from nnue_common.h.
	KAOutputScale     = 16
	KAWeightScaleBits = 6

	// KAFeatureDims is the per-perspective input feature count: 64 * 704 / 2.
	KAFeatureDims = 22528

	// psNB is the per-king-bucket stride: 11 * 64.
	psNB = 704

	// kaFC0Outputs is the number of "real" fc_0 outputs; one further output
	// (index 15) is forwarded into the final blend, so 16 are computed.
	kaFC0Outputs = 15
	kaFC0Total   = kaFC0Outputs + 1 // 16
	// kaFC1Inputs is the logical fc_1 input width (FC0Outputs*2 = 30); on disk
	// the weight matrix is padded to 32 columns.
	kaFC1Inputs       = 30
	kaFC1InputsPadded = 32
	kaFC1Outputs      = 32
	kaFC2InputsPadded = 32
	kaFC0InputsPadded = 1024 // ceil_to_multiple(1024, 32)

	// kaFeatureHash is FeatureSet::HashValue (HalfKAv2_hm).
	kaFeatureHash uint32 = 0x7f234cb8

	// KANormalizeToPawn is uci.h's NormalizeToPawnValue: divide the internal
	// White-POV value by this to obtain pawns as printed by "eval".
	KANormalizeToPawn = 361
)

// ---------------------------------------------------------------------------
// Feature indexing (reference/sf15/src/nnue/features/half_ka_v2_hm.{h,cpp})
// ---------------------------------------------------------------------------

// PS_ block offsets, each a multiple of SQUARE_NB=64. Unlike HalfKP, kings are
// encoded (PS_KING), and both colours' kings share the single PS_KING block.
const (
	psWPawnKA   = 0
	psBPawnKA   = 1 * 64
	psWKnightKA = 2 * 64
	psBKnightKA = 3 * 64
	psWBishopKA = 4 * 64
	psBBishopKA = 5 * 64
	psWRookKA   = 6 * 64
	psBRookKA   = 7 * 64
	psWQueenKA  = 8 * 64
	psBQueenKA  = 9 * 64
	psKingKA    = 10 * 64
)

// kaPieceSquareIndex[perspective][localColoredPiece] is the PS_ block offset to
// add for that piece from that perspective. It is the local-enum re-keying of
// Stockfish's PieceSquareIndex[COLOR_NB][PIECE_NB] (half_ka_v2_hm.h):
// "W = us, B = them; viewed from the other side W and B are reversed". Kings map
// to PS_KING in BOTH perspectives (there is no white/black king block).
//
// Perspective index: 0 = White own POV, 1 = Black own POV.
var kaPieceSquareIndex = [2][13]uint32{
	0: { // White's own POV
		position.WhitePawn:   psWPawnKA,
		position.BlackPawn:   psBPawnKA,
		position.WhiteKnight: psWKnightKA,
		position.BlackKnight: psBKnightKA,
		position.WhiteBishop: psWBishopKA,
		position.BlackBishop: psBBishopKA,
		position.WhiteRook:   psWRookKA,
		position.BlackRook:   psBRookKA,
		position.WhiteQueen:  psWQueenKA,
		position.BlackQueen:  psBQueenKA,
		position.WhiteKing:   psKingKA,
		position.BlackKing:   psKingKA,
		position.Empty:       0,
	},
	1: { // Black's own POV (W and B reversed)
		position.WhitePawn:   psBPawnKA,
		position.BlackPawn:   psWPawnKA,
		position.WhiteKnight: psBKnightKA,
		position.BlackKnight: psWKnightKA,
		position.WhiteBishop: psBBishopKA,
		position.BlackBishop: psWBishopKA,
		position.WhiteRook:   psBRookKA,
		position.BlackRook:   psWRookKA,
		position.WhiteQueen:  psBQueenKA,
		position.BlackQueen:  psWQueenKA,
		position.WhiteKing:   psKingKA,
		position.BlackKing:   psKingKA,
		position.Empty:       0,
	},
}

// kaOrientTBL[perspective][ksq] is the value XORed into a piece square in
// make_index. It is transcribed literally from OrientTBL in half_ka_v2_hm.h
// (square 0 = A1 is the first element). Decomposed: the entry depends only on
// the king's FILE. For White, king on files a..d => SQ_H1(7) (horizontal
// mirror), files e..h => SQ_A1(0) (identity). For Black the baseline is a rank
// flip: files a..d => SQ_H8(63) (full 180 rotation), files e..h => SQ_A8(56).
// This realizes "king always mirrored onto the e..h files".
var kaOrientTBL = func() [2][64]uint8 {
	const (
		sqA1, sqH1, sqA8, sqH8 = 0, 7, 56, 63
	)
	var t [2][64]uint8
	for sq := 0; sq < 64; sq++ {
		file := sq & 7
		if file < 4 { // a..d (queenside): mirror to kingside
			t[0][sq] = sqH1
			t[1][sq] = sqH8
		} else { // e..h (kingside): identity baseline
			t[0][sq] = sqA1
			t[1][sq] = sqA8
		}
	}
	return t
}()

// kaKingBuckets[perspective][ksq] is the king-position bucket value v (0..31),
// transcribed from KingBuckets in half_ka_v2_hm.h (where the source pre-multiplies
// by PS_NB via the B(v) macro; here we keep the raw v and multiply by psNB at
// use). Closed form verified against the literal table:
//
//	White: v = 4*(7-rank) + min(file, 7-file)
//	Black: v = 4*rank     + min(file, 7-file)
//
// where rank = ksq>>3, file = ksq&7.
var kaKingBuckets = func() [2][64]uint32 {
	var t [2][64]uint32
	for sq := 0; sq < 64; sq++ {
		rank := sq >> 3
		file := sq & 7
		m := file
		if 7-file < m {
			m = 7 - file
		}
		t[0][sq] = uint32(4*(7-rank) + m) // White
		t[1][sq] = uint32(4*rank + m)     // Black
	}
	return t
}()

// kaMakeIndex returns the HalfKAv2_hm feature index for a piece pc on raw square
// s, given the raw friendly king square ksq, for the given perspective. Mirrors
// HalfKAv2_hm::make_index:
//
//	(s ^ OrientTBL[persp][ksq]) + PieceSquareIndex[persp][pc] + KingBuckets[persp][ksq]
//
// where the king bucket here carries the *psNB multiply. Both the orient and the
// bucket are looked up by the RAW king square; s is the raw piece square.
func kaMakeIndex(perspective position.Color, s uint8, pc position.ColoredPiece, ksq uint8) uint32 {
	p := perspectiveIndex(perspective)
	return uint32(s^kaOrientTBL[p][ksq]) +
		kaPieceSquareIndex[p][pc] +
		kaKingBuckets[p][ksq]*psNB
}

// kaAppendActiveIndices fills out with the active feature indices for the given
// perspective. Unlike HalfKP it iterates ALL occupied squares including both
// kings (HalfKAv2_hm::append_active_indices). At most 32 features are active.
func kaAppendActiveIndices(pos *position.Position, perspective position.Color, out *[]uint32) {
	ksq := pos.KingLocation[perspectiveIndex(perspective)]
	for sq := uint8(0); sq < 64; sq++ {
		pc := pos.Squares[sq]
		if pc == position.Empty {
			continue
		}
		*out = append(*out, kaMakeIndex(perspective, sq, pc, ksq))
	}
}

// ---------------------------------------------------------------------------
// Accumulator
// ---------------------------------------------------------------------------

// KAAccumulator holds the feature-transformer state for both perspectives: the
// 1024-wide int16 accumulation and the 8-wide int32 PSQT accumulation. It is the
// analogue of Stockfish's Accumulator (the big-net one). Indexing:
// accumulation[0] is White's perspective, accumulation[1] is Black's.
type KAAccumulator struct {
	accumulation [2][KAHalfDims]int16
	psqt         [2][KAPSQTBuckets]int32
	computed     bool
}

// Refresh recomputes both perspectives from scratch (the scalar refresh path of
// FeatureTransformer::update_accumulator): start the FT accumulation from the
// biases and the PSQT accumulation from zero, then add the FT weight column and
// PSQT column of every active feature. Kings are included.
func (a *KAAccumulator) Refresh(n *KANetwork, pos *position.Position) {
	a.refreshPerspective(n, pos, position.White)
	a.refreshPerspective(n, pos, position.Black)
	a.computed = true
}

func (a *KAAccumulator) refreshPerspective(n *KANetwork, pos *position.Position, perspective position.Color) {
	p := perspectiveIndex(perspective)
	acc := &a.accumulation[p]
	copy(acc[:], n.ftBiases[:])
	psqt := &a.psqt[p]
	for k := range psqt {
		psqt[k] = 0
	}

	ksq := pos.KingLocation[p]
	for sq := uint8(0); sq < 64; sq++ {
		pc := pos.Squares[sq]
		if pc == position.Empty {
			continue
		}
		index := kaMakeIndex(perspective, sq, pc, ksq)
		col := n.ftWeightColumn(index)
		for j := 0; j < KAHalfDims; j++ {
			acc[j] += col[j]
		}
		pcol := n.psqtColumn(index)
		for k := 0; k < KAPSQTBuckets; k++ {
			psqt[k] += pcol[k]
		}
	}
}

// ---------------------------------------------------------------------------
// Network
// ---------------------------------------------------------------------------

// kaLayerStack is one per-material network: fc_0 (1024->16), fc_1 (30->32 with
// padded input 32) and fc_2 (32->1). Affine weights are stored de-scrambled into
// plain [out*paddedIn + in] row-major layout ready for a naive dot product.
type kaLayerStack struct {
	fc0Bias    [kaFC0Total]int32
	fc0Weights [kaFC0Total * kaFC0InputsPadded]int8 // [out*1024 + in]
	fc1Bias    [kaFC1Outputs]int32
	fc1Weights [kaFC1Outputs * kaFC1InputsPadded]int8 // [out*32 + in]
	fc2Bias    int32
	fc2Weights [kaFC2InputsPadded]int8 // [in]
}

// KANetwork holds a loaded HalfKAv2_hm network: the feature transformer (FT
// weights/biases plus the PSQT side-channel) and the eight layer stacks.
type KANetwork struct {
	// Architecture is the description string from the file header (informational).
	Architecture string

	// HashOK reports whether the file's header hash matched the architecture
	// hash this package computes. A mismatch is non-fatal (the architecture is
	// fixed here) but flags a likely format drift; the version word is enforced.
	HashOK bool

	ftBiases    [KAHalfDims]int16
	ftWeights   []int16 // KAFeatureDims * KAHalfDims, layout [index*1024 + j]
	psqtWeights []int32 // KAFeatureDims * KAPSQTBuckets, layout [index*8 + bucket]

	stacks [KALayerStacks]kaLayerStack
}

// ftWeightColumn returns the 1024-wide FT weight column for feature index
// (offset = HalfDimensions * index).
func (n *KANetwork) ftWeightColumn(index uint32) []int16 {
	off := int(index) * KAHalfDims
	return n.ftWeights[off : off+KAHalfDims]
}

// psqtColumn returns the 8-wide PSQT column for feature index
// (psqtWeights[index*PSQTBuckets + k]).
func (n *KANetwork) psqtColumn(index uint32) []int32 {
	off := int(index) * KAPSQTBuckets
	return n.psqtWeights[off : off+KAPSQTBuckets]
}

// ---------------------------------------------------------------------------
// Evaluation (reference/sf15/src/nnue/nnue_architecture.h, evaluate_nnue.cpp)
// ---------------------------------------------------------------------------

// EvalKA computes the NNUE evaluation of pos, returning centipawns from the
// side-to-move's perspective (matching the units the HalfKP Eval returns). It
// performs a full refresh of the accumulator and then the forward pass.
func (n *KANetwork) EvalKA(pos *position.Position) int32 {
	v := n.EvalInternal(pos)           // internal stm-POV value
	return v * 100 / KANormalizeToPawn // centipawns, stm POV
}

// EvalInternal performs a full refresh then returns the raw internal value
// (psqt + positional)/OutputScale from the side-to-move's perspective. Divide by
// 361.0 (after flipping to White POV) to reproduce the oracle's printed pawns.
func (n *KANetwork) EvalInternal(pos *position.Position) int32 {
	var acc KAAccumulator
	acc.Refresh(n, pos)
	return n.EvalWith(&acc, pos.SideToMove, KAPieceCount(pos))
}

// KAPieceCount returns the number of pieces on the board, which selects the PSQT
// bucket and layer stack via bucket = (pieceCount-1)/4. It counts set bits in the
// combined occupancy bitboard (kings included), matching pos.count<ALL_PIECES>().
func KAPieceCount(pos *position.Position) int {
	return bits.OnesCount64(uint64(pos.Occupied[0] | pos.Occupied[1]))
}

// EvalWith computes the internal stm-POV value from an already-populated
// accumulator. bucket = (pieceCount-1)/4 selects both the PSQT extraction and the
// layer stack. The return value is (psqt + positional)/OutputScale, side-to-move
// POV. See the package doc for converting to pawns/centipawns.
func (n *KANetwork) EvalWith(acc *KAAccumulator, sideToMove position.Color, pieceCount int) int32 {
	bucket := (pieceCount - 1) / 4

	// --- Feature transformer: pairwise-multiply transform (1024 bytes) ---
	// perspectives = {stm, ~stm}; stm half occupies output[0:512].
	var input [KAHalfDims]uint8
	perspectives := [2]int{
		perspectiveIndex(sideToMove),
		perspectiveIndex(sideToMove.Invert()),
	}
	const half = KAHalfDims / 2 // 512
	for p := 0; p < 2; p++ {
		offset := half * p
		src := &acc.accumulation[perspectives[p]]
		for j := 0; j < half; j++ {
			sum0 := int32(src[j])
			sum1 := int32(src[j+half])
			if sum0 < 0 {
				sum0 = 0
			} else if sum0 > 127 {
				sum0 = 127
			}
			if sum1 < 0 {
				sum1 = 0
			} else if sum1 > 127 {
				sum1 = 127
			}
			input[offset+j] = uint8(sum0 * sum1 / 128)
		}
	}

	// PSQT scalar for the selected bucket (already carries the /2).
	psqt := (acc.psqt[perspectives[0]][bucket] - acc.psqt[perspectives[1]][bucket]) / 2

	stack := &n.stacks[bucket]

	// --- fc_0: 1024 -> 16 ---
	var fc0Out [kaFC0Total]int32
	kaAffine(input[:], stack.fc0Weights[:], stack.fc0Bias[:], fc0Out[:], kaFC0InputsPadded, KAHalfDims)

	// --- ac_sqr_0 (squared) and ac_0 (clipped), concatenated into 30 inputs ---
	// fc1in[0:15]  = SqrClippedReLU(fc0Out[0:15])
	// fc1in[15:30] = ClippedReLU(fc0Out[0:15])
	var fc1in [kaFC1InputsPadded]uint8 // padded to 32; cols 30,31 stay zero
	for i := 0; i < kaFC0Outputs; i++ {
		fc1in[i] = kaSqrClippedReLU(fc0Out[i])
		fc1in[kaFC0Outputs+i] = kaClippedReLU(fc0Out[i])
	}

	// --- fc_1: 30 -> 32 (dot over the logical 30 inputs) ---
	var fc1Out [kaFC1Outputs]int32
	kaAffine(fc1in[:kaFC1Inputs], stack.fc1Weights[:], stack.fc1Bias[:], fc1Out[:], kaFC1InputsPadded, kaFC1Inputs)

	// --- ac_1: ClippedReLU 32 -> 32 ---
	var ac1 [kaFC1Outputs]uint8
	for i := 0; i < kaFC1Outputs; i++ {
		ac1[i] = kaClippedReLU(fc1Out[i])
	}

	// --- fc_2: 32 -> 1 ---
	fc2Out := stack.fc2Bias
	for in := 0; in < kaFC1Outputs; in++ {
		fc2Out += int32(stack.fc2Weights[in]) * int32(ac1[in])
	}

	// --- output blend ---
	fwd := int32(int64(fc0Out[kaFC0Outputs]) * (600 * KAOutputScale) / (127 * (1 << KAWeightScaleBits)))
	positional := fc2Out + fwd

	// --- final internal value, side-to-move POV ---
	return (psqt + positional) / KAOutputScale
}

// kaAffine performs a plain affine transform with int8 weights laid out
// [out*paddedIn + in], uint8 inputs, int32 biases, accumulating into int32. Only
// the first len(in) (logical) inputs are summed, matching
// affine_transform_non_ssse3 which loops j < InputDimensions; the padded columns
// beyond that are not referenced.
func kaAffine(in []uint8, weights []int8, biases []int32, out []int32, paddedIn, inDims int) {
	for i := range out {
		sum := biases[i]
		row := weights[i*paddedIn : i*paddedIn+inDims]
		for j := 0; j < inDims; j++ {
			sum += int32(row[j]) * int32(in[j])
		}
		out[i] = sum
	}
}

// kaClippedReLU is the dense-stack activation: arithmetic right-shift by
// WeightScaleBits then clamp to [0,127] (ClippedReLU scalar path).
func kaClippedReLU(x int32) uint8 {
	return clip8(x >> KAWeightScaleBits)
}

// kaSqrClippedReLU is the squared activation: clamp((x*x >> 12)/128, 0, 127)
// (SqrClippedReLU scalar path). int64 is used for the square to avoid overflow.
func kaSqrClippedReLU(x int32) uint8 {
	v := (int64(x) * int64(x) >> (2 * KAWeightScaleBits)) / 128
	if v < 0 {
		return 0
	}
	if v > 127 {
		return 127
	}
	return uint8(v)
}
