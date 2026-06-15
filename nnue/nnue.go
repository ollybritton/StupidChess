// Package nnue implements an Efficiently Updatable Neural Network (NNUE)
// evaluator for the classic Stockfish HalfKP architecture
// (HalfKP_256x2-32-32).
//
// It is a faithful port of the HalfKP-era NNUE code from Stockfish, the
// reference C++ for which lives under reference/nnue/ in this repository.
// The intent is to reproduce the same arithmetic, the same feature indexing
// and the same serialized file layout, so that a network file produced by the
// classic Stockfish trainer/serializer loads and evaluates identically here.
//
// # Architecture
//
//	HalfKP feature set      : 41024 input features per perspective
//	Feature transformer     : 41024 -> 256, int16 weights and biases
//	Both perspectives        : 256 * 2 = 512 transformed inputs (clipped to uint8)
//	Hidden layer 1 (affine) : 512 -> 32, int8 weights, int32 biases
//	Clipped ReLU            : 32 -> 32
//	Hidden layer 2 (affine) : 32 -> 32, int8 weights, int32 biases
//	Clipped ReLU            : 32 -> 32
//	Output layer (affine)   : 32 -> 1, int8 weights, int32 bias
//	Output is divided by FV_SCALE (=16) to give centipawns, side-to-move POV.
//
// # File format (little-endian throughout)
//
// The loader implements the standard Stockfish NNUE serialization as found in
// evaluate_nnue.cpp (ReadHeader / ReadParameters) and the per-component
// ReadParameters methods:
//
//	Network header:
//	  uint32  version           (== 0x7AF32F16)
//	  uint32  hashValue         (architecture hash; for HalfKP_256x2-32-32 this is kHashValue)
//	  uint32  archStringLen     (length of the following description string)
//	  bytes   archString        (archStringLen bytes of ASCII, e.g. the layer description)
//
//	Feature transformer block:
//	  uint32  ftHash            (== RawFeatures::kHashValue ^ (256*2))
//	  int16   biases[256]
//	  int16   weights[256 * 41024]   (row-major: feature index outer, dimension inner)
//
//	Network (dense stack) block:
//	  uint32  netHash           (combined hash of the affine/relu stack)
//	  -- output layer is written outermost because Stockfish nests layers, so the
//	     stream order produced by ReadParameters recursion is:
//	     input-slice (no params) -> affine1 -> relu1 -> affine2 -> relu2 -> affine_out
//	  int32   affine1.biases[32]
//	  int8    affine1.weights[32 * 512]       (padded input dim = 512, already a multiple of 32)
//	  int32   affine2.biases[32]
//	  int8    affine2.weights[32 * 32]        (padded input dim = 32)
//	  int32   out.biases[1]
//	  int8    out.weights[1 * 32]             (padded input dim = 32)
//
// Note on ordering: in Stockfish the dense stack is built as
// AffineOut<ClippedReLU<AffineMid<ClippedReLU<AffineIn<InputSlice>>>>> and
// ReadParameters recurses into previous_layer_ FIRST, so the *innermost* layer
// (affine1, closest to the input) is serialized first and the output layer
// last. That is the order this loader reads them in.
//
// The single network hash check is intentionally lenient: a mismatch is
// reported via Network.HashOK but does not by itself abort the load, because
// the architecture is fixed by this package. The version word, however, must
// match.
package nnue

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/ollybritton/StupidChess/position"
)

// ---------------------------------------------------------------------------
// Constants (mirroring reference/nnue/nnue_common.h and the architecture)
// ---------------------------------------------------------------------------

const (
	// Version is the magic version word at the head of every classic NNUE file.
	Version uint32 = 0x7AF32F16

	// FVScale is the final output divisor turning the network's raw int32
	// output into centipawns.
	FVScale = 16

	// WeightScaleBits is the right-shift applied after each affine layer
	// before clipped ReLU (the quantization scale of the dense weights).
	WeightScaleBits = 6

	// TransformedFeatureDimensions is the per-perspective accumulator size.
	TransformedFeatureDimensions = 256

	// HalfDimensions is an alias used in the transformer.
	HalfDimensions = TransformedFeatureDimensions

	// TransformedOutputDimensions is the combined size fed to the dense stack
	// (both perspectives concatenated).
	TransformedOutputDimensions = TransformedFeatureDimensions * 2 // 512
)

// HalfKP feature-set dimensions (reference/nnue/nnue_common.h PS_ offsets and
// reference/nnue/features/half_kp.cpp).
const (
	squareNB = 64

	// Piece-square offsets. These are the PS_ values; PS_NONE marks "no
	// feature" (kings are excluded from HalfKP).
	psNone    = 0
	psWPawn   = 1
	psBPawn   = 1*squareNB + 1 // 65
	psWKnight = 2*squareNB + 1 // 129
	psBKnight = 3*squareNB + 1 // 193
	psWBishop = 4*squareNB + 1 // 257
	psBBishop = 5*squareNB + 1 // 321
	psWRook   = 6*squareNB + 1 // 385
	psBRook   = 7*squareNB + 1 // 449
	psWQueen  = 8*squareNB + 1 // 513
	psBQueen  = 9*squareNB + 1 // 577

	// psEnd is the per-king stride. Kings are not encoded, so the largest
	// piece offset used is for queens; psEnd = 10*64+1 = 641.
	psEnd = 10*squareNB + 1 // 641

	// FeatureDimensions is the classic HalfKP count: 64 king squares * 641.
	FeatureDimensions = squareNB * psEnd // 41024
)

// kppBoardIndex maps this repository's position.ColoredPiece to the PS_ offset
// for each of the two perspectives ([0] = white-to-move/"us" view as White,
// [1] = the mirrored view). It is the analogue of Stockfish's
// kpp_board_index[PIECE_NB][COLOR_NB], re-keyed to the local piece enum.
//
// In Stockfish the table is indexed by Piece and Color(perspective), with the
// convention "W = us, B = them; viewed from the other side W and B are
// reversed". Concretely, for a given coloured piece the [perspective] entry is
// the PS_ offset to use when building that perspective's feature index.
//
// Local ColoredPiece enum (from position/pieces.go):
//
//	WhitePawn=0  BlackPawn=1  WhiteKnight=2 BlackKnight=3
//	WhiteBishop=4 BlackBishop=5 WhiteRook=6  BlackRook=7
//	WhiteQueen=8  BlackQueen=9  WhiteKing=10 BlackKing=11  Empty=12
//
// Perspective index: 0 = White's own POV, 1 = Black's own POV. From White's
// POV a white pawn is "our pawn" (PS_W_PAWN); from Black's POV that same white
// pawn is "their pawn" (PS_B_PAWN). Kings map to PS_NONE because HalfKP omits
// them from the encoded features.
var kppBoardIndex = [13][2]uint32{
	//                       {White POV, Black POV}
	position.WhitePawn:   {psWPawn, psBPawn},
	position.BlackPawn:   {psBPawn, psWPawn},
	position.WhiteKnight: {psWKnight, psBKnight},
	position.BlackKnight: {psBKnight, psWKnight},
	position.WhiteBishop: {psWBishop, psBBishop},
	position.BlackBishop: {psBBishop, psWBishop},
	position.WhiteRook:   {psWRook, psBRook},
	position.BlackRook:   {psBRook, psWRook},
	position.WhiteQueen:  {psWQueen, psBQueen},
	position.BlackQueen:  {psBQueen, psWQueen},
	position.WhiteKing:   {psNone, psNone},
	position.BlackKing:   {psNone, psNone},
	position.Empty:       {psNone, psNone},
}

// orient rotates a square 180 degrees for the Black perspective, matching
// Stockfish's orient(): s ^ (perspective * 63). Square encoding is identical
// to this repo's (A1 = 0 ... H8 = 63), so s ^ 63 maps A1<->H8 i.e. a 180-degree
// rotation of the board.
func orient(perspective position.Color, sq uint8) uint8 {
	if perspective == position.Black {
		return sq ^ 63
	}
	return sq
}

// MakeIndex returns the HalfKP feature index for a piece of type pc sitting on
// square sq, given the (already oriented) friendly king square orientedKing,
// from the given perspective. This is the direct analogue of
// HalfKP::MakeIndex in reference/nnue/features/half_kp.cpp:
//
//	orient(perspective, s) + kpp_board_index[pc][perspective] + PS_END * ksq
//
// where ksq is the oriented king square. The caller is responsible for not
// calling this on kings (their PS_ offset is PS_NONE, which would alias index 0
// of every king bucket).
func MakeIndex(perspective position.Color, sq uint8, pc position.ColoredPiece, orientedKing uint8) uint32 {
	return uint32(orient(perspective, sq)) +
		kppBoardIndex[pc][perspectiveIndex(perspective)] +
		psEnd*uint32(orientedKing)
}

// perspectiveIndex maps a Color to the 0/1 index used by kppBoardIndex and the
// accumulator. White -> 0, Black -> 1.
func perspectiveIndex(c position.Color) int {
	if c == position.White {
		return 0
	}
	return 1
}

// appendActiveIndices fills out with the active HalfKP feature indices for the
// given perspective, mirroring HalfKP::AppendActiveIndices. Kings are skipped.
func appendActiveIndices(pos *position.Position, perspective position.Color, out *[]uint32) {
	king := pos.KingLocation[perspectiveIndex(perspective)]
	orientedKing := orient(perspective, king)
	for sq := uint8(0); sq < 64; sq++ {
		pc := pos.Squares[sq]
		if pc == position.Empty {
			continue
		}
		if pc == position.WhiteKing || pc == position.BlackKing {
			continue
		}
		*out = append(*out, MakeIndex(perspective, sq, pc, orientedKing))
	}
}

// ---------------------------------------------------------------------------
// Accumulator
// ---------------------------------------------------------------------------

// Accumulator holds the feature-transformer output for both perspectives. Each
// perspective is a vector of TransformedFeatureDimensions int16 values. It can
// be updated incrementally (Add/Remove a single feature) or recomputed from
// scratch (Refresh).
//
// Indexing: accumulation[0] is White's perspective, accumulation[1] is Black's.
// This matches Stockfish's accumulation[perspective][0].
type Accumulator struct {
	accumulation [2][TransformedFeatureDimensions]int16
	computed     bool
}

// Refresh recomputes the accumulator for both perspectives from scratch, using
// the piece placement in pos. This is the analogue of
// FeatureTransformer::RefreshAccumulator: start from the biases, then add the
// weight column for every active feature.
func (a *Accumulator) Refresh(n *Network, pos *position.Position) {
	a.refreshPerspective(n, pos, position.White)
	a.refreshPerspective(n, pos, position.Black)
	a.computed = true
}

// refreshPerspective recomputes a single perspective's accumulation from scratch
// (biases plus the weight column of every active feature for that perspective).
// A full Refresh is just this done for both perspectives. It is also the path
// taken incrementally when the side's own king moves: under HalfKP every feature
// of a perspective is keyed by that perspective's king square, so a king move
// invalidates the whole perspective and there is nothing to update incrementally.
func (a *Accumulator) refreshPerspective(n *Network, pos *position.Position, perspective position.Color) {
	p := perspectiveIndex(perspective)
	acc := &a.accumulation[p]
	copy(acc[:], n.ftBiases[:])

	orientedKing := orient(perspective, pos.KingLocation[p])
	for sq := uint8(0); sq < 64; sq++ {
		pc := pos.Squares[sq]
		if pc == position.Empty || pc == position.WhiteKing || pc == position.BlackKing {
			continue
		}
		col := n.ftWeightColumn(MakeIndex(perspective, sq, pc, orientedKing))
		for j := 0; j < TransformedFeatureDimensions; j++ {
			acc[j] += col[j]
		}
	}
}

// Add activates a single feature in the given perspective's accumulation by
// adding its weight column. This is the incremental "added feature" path of
// FeatureTransformer::UpdateAccumulator.
func (a *Accumulator) Add(n *Network, perspective position.Color, index uint32) {
	acc := &a.accumulation[perspectiveIndex(perspective)]
	col := n.ftWeightColumn(index)
	for j := 0; j < TransformedFeatureDimensions; j++ {
		acc[j] += col[j]
	}
}

// Remove deactivates a single feature in the given perspective's accumulation
// by subtracting its weight column. This is the incremental "removed feature"
// path of FeatureTransformer::UpdateAccumulator.
func (a *Accumulator) Remove(n *Network, perspective position.Color, index uint32) {
	acc := &a.accumulation[perspectiveIndex(perspective)]
	col := n.ftWeightColumn(index)
	for j := 0; j < TransformedFeatureDimensions; j++ {
		acc[j] -= col[j]
	}
}

// featureChange is a single non-king piece placement (a piece appearing on, or
// leaving, a square) caused by a move. Kings are never recorded because HalfKP
// does not encode them as features.
type featureChange struct {
	piece position.ColoredPiece
	sq    uint8
}

// Update computes this accumulator as the result of applying move m to the
// position that prev describes, leaving pos as the board AFTER the move. It is
// the incremental counterpart of Refresh: instead of rebuilding from scratch it
// copies prev and adds/removes only the handful of feature columns the move
// touched.
//
// Under HalfKP each perspective's features are keyed by that perspective's own
// king square, so when the side to move moves its king (including castling) that
// whole perspective is rebuilt with refreshPerspective; the opponent's
// perspective is still updated incrementally (the king is not a feature there).
// All the other cases - captures (including en passant, whose captured pawn is
// not on the destination square), promotions, and the castling rook - reduce to
// a small list of removed and added (piece, square) features applied to both
// perspectives. prev and a may not alias.
func (a *Accumulator) Update(n *Network, prev *Accumulator, pos *position.Position, m position.Move) {
	mover := m.Moved().Color()
	moved := m.Moved()
	from, to := m.From(), m.To()
	captured := m.Captured()
	promo := m.Promotion()
	movedKing := moved.Colorless() == position.King

	var removed, added [3]featureChange
	nr, na := 0, 0
	remove := func(pc position.ColoredPiece, sq uint8) {
		if pc.Colorless() == position.King {
			return
		}
		removed[nr] = featureChange{pc, sq}
		nr++
	}
	add := func(pc position.ColoredPiece, sq uint8) {
		if pc.Colorless() == position.King {
			return
		}
		added[na] = featureChange{pc, sq}
		na++
	}

	// The moving piece leaves its origin square.
	remove(moved, from)

	// The captured piece, if any. En passant captures a pawn that is not on the
	// destination square but one rank behind it (relative to the mover).
	enPassant := moved.Colorless() == position.Pawn &&
		m.PriorEnPassantTarget() != position.NoEnPassant && to == m.PriorEnPassantTarget()
	switch {
	case enPassant:
		capSq := to - 8
		if mover == position.Black {
			capSq = to + 8
		}
		remove(captured, capSq)
	case captured != position.Empty:
		remove(captured, to)
	}

	// The moving piece arrives on its destination, becoming the promoted piece if
	// this is a promotion.
	if promo != position.None {
		add(promo.OfColor(mover), to)
	} else {
		add(moved, to)
	}

	// Castling additionally relocates the rook (the king is handled by refreshing
	// the mover's perspective below).
	if movedKing && (int(from)-int(to) == 2 || int(to)-int(from) == 2) {
		rook := position.Rook.OfColor(mover)
		var rookFrom, rookTo uint8
		switch to {
		case position.SquareG1:
			rookFrom, rookTo = position.SquareH1, position.SquareF1
		case position.SquareC1:
			rookFrom, rookTo = position.SquareA1, position.SquareD1
		case position.SquareG8:
			rookFrom, rookTo = position.SquareH8, position.SquareF8
		case position.SquareC8:
			rookFrom, rookTo = position.SquareA8, position.SquareD8
		}
		remove(rook, rookFrom)
		add(rook, rookTo)
	}

	for p := 0; p < 2; p++ {
		perspective := position.Color(p)

		// The mover's own king moved: this perspective's king bucket changed, so
		// nothing can be carried over - rebuild it from the board.
		if movedKing && perspective == mover {
			a.refreshPerspective(n, pos, perspective)
			continue
		}

		a.accumulation[p] = prev.accumulation[p]
		orientedKing := orient(perspective, pos.KingLocation[p])
		acc := &a.accumulation[p]
		for i := 0; i < nr; i++ {
			col := n.ftWeightColumn(MakeIndex(perspective, removed[i].sq, removed[i].piece, orientedKing))
			for j := 0; j < TransformedFeatureDimensions; j++ {
				acc[j] -= col[j]
			}
		}
		for i := 0; i < na; i++ {
			col := n.ftWeightColumn(MakeIndex(perspective, added[i].sq, added[i].piece, orientedKing))
			for j := 0; j < TransformedFeatureDimensions; j++ {
				acc[j] += col[j]
			}
		}
	}
	a.computed = true
}

// ---------------------------------------------------------------------------
// Network
// ---------------------------------------------------------------------------

// Network holds a loaded HalfKP_256x2-32-32 NNUE network: the feature
// transformer plus the three affine layers of the dense stack.
type Network struct {
	// Architecture description string read from the file header (informational).
	Architecture string

	// HashOK reports whether the network hash in the file matched the hash this
	// package computes for HalfKP_256x2-32-32. A false value does not prevent
	// use (the architecture is fixed here) but flags a likely mismatch.
	HashOK bool

	// Feature transformer: 41024 -> 256, int16.
	// ftWeights is laid out feature-major: ftWeights[index*256 + j].
	ftBiases  [TransformedFeatureDimensions]int16
	ftWeights []int16 // length FeatureDimensions * TransformedFeatureDimensions

	// Dense stack. Affine weights are stored row-major with padded input
	// dimension equal to the (already SIMD-aligned) input size.
	// layer1: 512 -> 32
	l1Biases  [32]int32
	l1Weights [32 * TransformedOutputDimensions]int8 // [out*512 + in]

	// layer2: 32 -> 32
	l2Biases  [32]int32
	l2Weights [32 * 32]int8 // [out*32 + in]

	// output: 32 -> 1
	outBias    int32
	outWeights [32]int8 // [in]
}

// ftWeightColumn returns the 256-wide weight column for feature index, i.e. the
// slice ftWeights[index*256 : index*256+256]. Mirrors offset = kHalfDimensions
// * index in the reference.
func (n *Network) ftWeightColumn(index uint32) []int16 {
	off := int(index) * TransformedFeatureDimensions
	return n.ftWeights[off : off+TransformedFeatureDimensions]
}

// Load reads a serialized HalfKP network from path. See the package doc for the
// exact byte layout. It returns an error if the file cannot be read or the
// version word does not match; a network-hash mismatch is recorded in
// Network.HashOK but is not fatal.
func Load(path string) (*Network, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadNetwork(f)
}

// ReadNetwork reads a serialized network from r. It is the stream-based core of
// Load and is exported to allow loading from embedded data or other sources.
func ReadNetwork(r io.Reader) (*Network, error) {
	br := newByteReader(r)
	n := &Network{}

	// --- Network header (evaluate_nnue.cpp ReadHeader) ---
	version, err := br.uint32()
	if err != nil {
		return nil, fmt.Errorf("nnue: reading version: %w", err)
	}
	if version != Version {
		return nil, fmt.Errorf("nnue: bad version 0x%08X, want 0x%08X", version, Version)
	}
	fileHash, err := br.uint32()
	if err != nil {
		return nil, fmt.Errorf("nnue: reading hash: %w", err)
	}
	archLen, err := br.uint32()
	if err != nil {
		return nil, fmt.Errorf("nnue: reading arch length: %w", err)
	}
	if archLen > 1<<20 {
		return nil, fmt.Errorf("nnue: implausible architecture string length %d", archLen)
	}
	archBytes := make([]byte, archLen)
	if _, err := io.ReadFull(br, archBytes); err != nil {
		return nil, fmt.Errorf("nnue: reading arch string: %w", err)
	}
	n.Architecture = string(archBytes)
	n.HashOK = fileHash == ExpectedNetworkHash()

	// --- Feature transformer block (nnue_feature_transformer.h ReadParameters) ---
	ftHash, err := br.uint32()
	if err != nil {
		return nil, fmt.Errorf("nnue: reading FT hash: %w", err)
	}
	_ = ftHash // hash is verified loosely via HashOK; do not abort on it.

	for i := 0; i < TransformedFeatureDimensions; i++ {
		v, err := br.int16()
		if err != nil {
			return nil, fmt.Errorf("nnue: reading FT bias %d: %w", i, err)
		}
		n.ftBiases[i] = v
	}
	n.ftWeights = make([]int16, FeatureDimensions*TransformedFeatureDimensions)
	if err := br.int16Slice(n.ftWeights); err != nil {
		return nil, fmt.Errorf("nnue: reading FT weights: %w", err)
	}

	// --- Dense network block (evaluate_nnue.cpp Network::ReadParameters) ---
	netHash, err := br.uint32()
	if err != nil {
		return nil, fmt.Errorf("nnue: reading net hash: %w", err)
	}
	_ = netHash

	// Layer order follows Stockfish's recursion: innermost (affine1) first,
	// then affine2, then the output affine. The InputSlice and ClippedReLU
	// layers carry no parameters.
	if err := readAffine(br, n.l1Biases[:], n.l1Weights[:]); err != nil {
		return nil, fmt.Errorf("nnue: reading layer1: %w", err)
	}
	if err := readAffine(br, n.l2Biases[:], n.l2Weights[:]); err != nil {
		return nil, fmt.Errorf("nnue: reading layer2: %w", err)
	}
	outBias := make([]int32, 1)
	if err := readAffine(br, outBias, n.outWeights[:]); err != nil {
		return nil, fmt.Errorf("nnue: reading output layer: %w", err)
	}
	n.outBias = outBias[0]

	return n, nil
}

// readAffine reads one affine layer: first the int32 biases (one per output),
// then the int8 weights (outputs * inputs, row-major). The number of biases and
// weights is taken from the slice lengths supplied by the caller.
func readAffine(br *byteReader, biases []int32, weights []int8) error {
	for i := range biases {
		v, err := br.int32()
		if err != nil {
			return fmt.Errorf("bias %d: %w", i, err)
		}
		biases[i] = v
	}
	return br.int8Slice(weights)
}

// ---------------------------------------------------------------------------
// Evaluation
// ---------------------------------------------------------------------------

// Eval computes the NNUE evaluation of pos from the side-to-move's perspective,
// returning a value in centipawns. It performs a full (refresh) calculation: it
// rebuilds the accumulator from the position, transforms it through the dense
// stack and divides by FVScale, exactly as ComputeScore(pos, refresh=true) does
// in evaluate_nnue.cpp.
func (n *Network) Eval(pos *position.Position) int16 {
	var acc Accumulator
	acc.Refresh(n, pos)
	return n.EvalWith(&acc, pos.SideToMove)
}

// EvalWith computes the evaluation from an already-populated accumulator, given
// whose turn it is to move. This is the incremental-friendly entry point: keep
// an Accumulator up to date with Add/Remove and call EvalWith to avoid the
// full Refresh. The return value is centipawns from sideToMove's perspective.
func (n *Network) EvalWith(acc *Accumulator, sideToMove position.Color) int16 {
	// Transform: clip each accumulator value to [0,127] into a uint8 buffer.
	// The side-to-move perspective occupies the first half, the opponent the
	// second half. (FeatureTransformer::Transform, perspectives = {stm, ~stm}.)
	var input [TransformedOutputDimensions]uint8
	perspectives := [2]int{perspectiveIndex(sideToMove), perspectiveIndex(sideToMove.Invert())}
	for p := 0; p < 2; p++ {
		offset := HalfDimensions * p
		src := &acc.accumulation[perspectives[p]]
		for j := 0; j < HalfDimensions; j++ {
			input[offset+j] = clip8(int32(src[j]))
		}
	}

	// Dense stack: affine1 -> clippedReLU -> affine2 -> clippedReLU -> output.
	var l1Out [32]int32
	affine(input[:], n.l1Weights[:], n.l1Biases[:], l1Out[:])
	var l1Clipped [32]uint8
	clippedReLU(l1Out[:], l1Clipped[:])

	var l2Out [32]int32
	affine(l1Clipped[:], n.l2Weights[:], n.l2Biases[:], l2Out[:])
	var l2Clipped [32]uint8
	clippedReLU(l2Out[:], l2Clipped[:])

	// Output layer: 32 -> 1.
	out := n.outBias
	for j := 0; j < 32; j++ {
		out += int32(n.outWeights[j]) * int32(l2Clipped[j])
	}

	// Scale to centipawns. Stockfish: Value(output[0] / FV_SCALE).
	return int16(out / FVScale)
}

// affine performs a single affine transform: out[i] = bias[i] + sum_j w[i][j] *
// in[j], with int8 weights (row-major, stride len(in)) and uint8 inputs,
// accumulating into int32. This is the scalar reference path from
// AffineTransform::Propagate. It is the most expensive part of an evaluation;
// a real speedup here needs SIMD (pmaddubsw), which would have to be assembly.
func affine(in []uint8, weights []int8, biases []int32, out []int32) {
	inDim := len(in)
	for i := range out {
		sum := biases[i]
		row := weights[i*inDim : i*inDim+inDim]
		for j := 0; j < inDim; j++ {
			sum += int32(row[j]) * int32(in[j])
		}
		out[i] = sum
	}
}

// clippedReLU applies the dense-stack activation: arithmetic right-shift by
// WeightScaleBits then clamp to [0,127], producing a uint8. Mirrors
// ClippedReLU::Propagate's scalar path: max(0, min(127, x >> kWeightScaleBits)).
func clippedReLU(in []int32, out []uint8) {
	for i, v := range in {
		out[i] = clip8(v >> WeightScaleBits)
	}
}

// clip8 clamps an int32 to the [0,127] range and returns it as a uint8. This is
// the saturating conversion used by both the feature transformer output and the
// clipped ReLU layers.
func clip8(v int32) uint8 {
	if v < 0 {
		return 0
	}
	if v > 127 {
		return 127
	}
	return uint8(v)
}

// ---------------------------------------------------------------------------
// Hash computation (for validation only)
// ---------------------------------------------------------------------------

// rawFeaturesHashValue is RawFeatures::kHashValue for the HalfKP(Friend) feature
// set. HalfKP::kHashValue = 0x5D69D5B9 ^ (AssociatedKing == kFriend), and for
// the Friend variant that bit is 1, giving 0x5D69D5B8. FeatureSet wraps a
// single feature so its kHashValue equals the feature's.
const rawFeaturesHashValue uint32 = 0x5D69D5B9 ^ 1 // 0x5D69D5B8

// featureTransformerHash is GetHashValue() of the FeatureTransformer:
// RawFeatures::kHashValue ^ kOutputDimensions, with kOutputDimensions = 512.
func featureTransformerHash() uint32 {
	return rawFeaturesHashValue ^ uint32(TransformedOutputDimensions)
}

// inputSliceHash mirrors InputSlice<512,0>::GetHashValue():
// 0xEC42E90D ^ kOutputDimensions ^ (Offset<<10), Offset = 0.
func inputSliceHash(outputDims uint32) uint32 {
	return 0xEC42E90D ^ outputDims ^ (0 << 10)
}

// affineHash mirrors AffineTransform::GetHashValue():
// 0xCC03DAE4 + outputDims; ^= prev>>1; ^= prev<<31.
func affineHash(outputDims, prev uint32) uint32 {
	h := uint32(0xCC03DAE4)
	h += outputDims
	h ^= prev >> 1
	h ^= prev << 31
	return h
}

// clippedReLUHash mirrors ClippedReLU::GetHashValue(): 0x538D24C7 + prev.
func clippedReLUHash(prev uint32) uint32 {
	return 0x538D24C7 + prev
}

// networkHash computes the dense-stack hash for HalfKP_256x2-32-32. The stack
// is AffineOut<ClippedReLU<AffineMid<ClippedReLU<AffineIn<InputSlice>>>>> with
// dimensions 512 -> 32 -> 32 -> 1.
func networkHash() uint32 {
	h := inputSliceHash(TransformedOutputDimensions) // InputSlice<512>
	h = affineHash(32, h)                            // 512 -> 32
	h = clippedReLUHash(h)                           // 32
	h = affineHash(32, h)                            // 32 -> 32
	h = clippedReLUHash(h)                           // 32
	h = affineHash(1, h)                             // 32 -> 1
	return h
}

// ExpectedNetworkHash returns the architecture hash this package expects in the
// file header for HalfKP_256x2-32-32. It is the XOR-combination used by
// Stockfish to derive kHashValue from the feature transformer and the dense
// network hashes.
func ExpectedNetworkHash() uint32 {
	return featureTransformerHash() ^ networkHash()
}

// ---------------------------------------------------------------------------
// Little-endian reader helpers
// ---------------------------------------------------------------------------

// byteReader is a small buffered little-endian reader. It mirrors
// read_little_endian from nnue_common.h.
type byteReader struct {
	r   io.Reader
	buf [4]byte
}

func newByteReader(r io.Reader) *byteReader { return &byteReader{r: r} }

// Read implements io.Reader so byteReader can be passed to io.ReadFull.
func (b *byteReader) Read(p []byte) (int, error) { return b.r.Read(p) }

func (b *byteReader) uint32() (uint32, error) {
	if _, err := io.ReadFull(b.r, b.buf[:4]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b.buf[:4]), nil
}

func (b *byteReader) int16() (int16, error) {
	if _, err := io.ReadFull(b.r, b.buf[:2]); err != nil {
		return 0, err
	}
	return int16(binary.LittleEndian.Uint16(b.buf[:2])), nil
}

func (b *byteReader) int32() (int32, error) {
	v, err := b.uint32()
	return int32(v), err
}

// int16Slice fills dst with little-endian int16 values, reading in bulk.
func (b *byteReader) int16Slice(dst []int16) error {
	raw := make([]byte, len(dst)*2)
	if _, err := io.ReadFull(b.r, raw); err != nil {
		return err
	}
	for i := range dst {
		dst[i] = int16(binary.LittleEndian.Uint16(raw[i*2:]))
	}
	return nil
}

// int8Slice fills dst with int8 values read directly from the stream.
func (b *byteReader) int8Slice(dst []int8) error {
	raw := make([]byte, len(dst))
	if _, err := io.ReadFull(b.r, raw); err != nil {
		return err
	}
	for i := range dst {
		dst[i] = int8(raw[i])
	}
	return nil
}
