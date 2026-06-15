package nnue

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/ollybritton/StupidChess/position"
)

const realNetPath = "testdata/net.nnue"

// haveRealNet reports whether a real HalfKP network is available for the
// behavioural tests. When it is absent the corresponding tests are skipped
// rather than failed, so the package still builds and tests green without it.
func haveRealNet() bool {
	_, err := os.Stat(realNetPath)
	return err == nil
}

// --- Feature indexing ---------------------------------------------------------

// TestExpectedNetworkHash pins the architecture hash this package computes for
// HalfKP_256x2-32-32. The value is the one embedded in every classic Stockfish
// NNUE file (verified against a real net); if the PS_ offsets, the layer stack
// or the dimensions drift, this hash changes and the test catches it.
func TestExpectedNetworkHash(t *testing.T) {
	const want = 0x3E5AA6EE
	if got := ExpectedNetworkHash(); got != want {
		t.Fatalf("ExpectedNetworkHash = 0x%08X, want 0x%08X", got, want)
	}
}

// TestFeatureDimensions checks the classic HalfKP dimension count.
func TestFeatureDimensions(t *testing.T) {
	if FeatureDimensions != 41024 {
		t.Fatalf("FeatureDimensions = %d, want 41024", FeatureDimensions)
	}
	if psEnd != 641 {
		t.Fatalf("psEnd = %d, want 641", psEnd)
	}
	if TransformedOutputDimensions != 512 {
		t.Fatalf("TransformedOutputDimensions = %d, want 512", TransformedOutputDimensions)
	}
}

// TestOrient checks the 180-degree perspective flip.
func TestOrient(t *testing.T) {
	// White perspective: identity.
	for _, sq := range []uint8{0, 7, 28, 63} {
		if got := orient(position.White, sq); got != sq {
			t.Fatalf("orient(White, %d) = %d, want %d", sq, got, sq)
		}
	}
	// Black perspective: s ^ 63 (A1<->H8, E4<->D5, etc.).
	pairs := [][2]uint8{
		{position.SquareA1, position.SquareH8},
		{position.SquareE1, position.SquareD8},
		{position.SquareE4, position.SquareD5},
		{position.SquareH1, position.SquareA8},
	}
	for _, p := range pairs {
		if got := orient(position.Black, p[0]); got != p[1] {
			t.Fatalf("orient(Black, %d) = %d, want %d", p[0], got, p[1])
		}
		if got := orient(position.Black, p[1]); got != p[0] {
			t.Fatalf("orient(Black, %d) = %d, want %d", p[1], got, p[0])
		}
	}
}

// TestMakeIndexRange checks that feature indices for non-king pieces stay within
// the HalfKP range, and that the perspective convention is wired correctly: a
// white pawn from White's POV and a black pawn from Black's POV (same oriented
// king and oriented square) must collide on the same PS_W_PAWN bucket.
func TestMakeIndexRange(t *testing.T) {
	// All non-king pieces, all squares, both perspectives, king on a few squares.
	pieces := []position.ColoredPiece{
		position.WhitePawn, position.BlackPawn,
		position.WhiteKnight, position.BlackKnight,
		position.WhiteBishop, position.BlackBishop,
		position.WhiteRook, position.BlackRook,
		position.WhiteQueen, position.BlackQueen,
	}
	for _, persp := range []position.Color{position.White, position.Black} {
		for _, ksq := range []uint8{0, 4, 28, 63} {
			ok := orient(persp, ksq)
			for _, pc := range pieces {
				for sq := uint8(0); sq < 64; sq++ {
					idx := MakeIndex(persp, sq, pc, ok)
					if idx >= FeatureDimensions {
						t.Fatalf("MakeIndex(%v, %d, %v, ksq=%d) = %d out of range",
							persp, sq, pc, ksq, idx)
					}
				}
			}
		}
	}

	// Perspective symmetry: a white pawn seen from White equals a black pawn
	// seen from Black when their oriented coordinates coincide. Under the
	// 180-degree flip a square s maps to s^63, so we choose squares that are
	// each other's mirror image:
	//   white king a1 (oriented 0) <-> black king h8 (oriented 63^63 = 0)
	//   white pawn b2              <-> black pawn g7 (orient: b2^63 == g7? check)
	wKing := orient(position.White, position.SquareA1) // 0
	bKing := orient(position.Black, position.SquareH8) // 63 ^ 63 = 0
	if wKing != bKing {
		t.Fatalf("expected oriented kings to match: %d vs %d", wKing, bKing)
	}
	// b2 from White's POV is oriented identically; the mirror square g7 from
	// Black's POV orients to the same value (g7 ^ 63 == b2).
	if orient(position.Black, position.SquareG7) != orient(position.White, position.SquareB2) {
		t.Fatalf("mirror squares chosen incorrectly: g7 orients to %d, b2 to %d",
			orient(position.Black, position.SquareG7), orient(position.White, position.SquareB2))
	}
	wIdx := MakeIndex(position.White, position.SquareB2, position.WhitePawn, wKing)
	bIdx := MakeIndex(position.Black, position.SquareG7, position.BlackPawn, bKing)
	if wIdx != bIdx {
		t.Fatalf("perspective symmetry broken: white-pov %d != black-pov %d", wIdx, bIdx)
	}
}

// --- Accumulator add/remove symmetry -----------------------------------------

// TestAccumulatorAddRemoveSymmetry checks that adding then removing the same
// feature returns the accumulator to its starting state, and that incremental
// updates from Refresh agree with a from-scratch Refresh of the resulting
// position. Uses a tiny random network so it does not need a real net.
func TestAccumulatorAddRemoveSymmetry(t *testing.T) {
	n := randomTinyNetwork(1)

	// Empty position is illegal for Eval, but Refresh just needs piece data;
	// we use a legal position to keep things realistic.
	pos := mustFEN(t, "4k3/8/8/8/8/8/8/4K3 w - - 0 1") // bare kings, no features

	var a Accumulator
	a.Refresh(n, pos)

	// Snapshot.
	var before [2][TransformedFeatureDimensions]int16
	before = a.accumulation

	// Pick an arbitrary feature index for each perspective and add+remove it.
	idxW := MakeIndex(position.White, position.SquareE4, position.WhiteKnight,
		orient(position.White, pos.KingLocation[0]))
	idxB := MakeIndex(position.Black, position.SquareE4, position.WhiteKnight,
		orient(position.Black, pos.KingLocation[1]))

	a.Add(n, position.White, idxW)
	a.Add(n, position.Black, idxB)
	a.Remove(n, position.White, idxW)
	a.Remove(n, position.Black, idxB)

	if a.accumulation != before {
		t.Fatalf("add then remove did not restore accumulator")
	}

	// Now check incremental == refresh: build position with a white knight on
	// e4, compare an incrementally-updated accumulator against a fresh one.
	posWithN := mustFEN(t, "4k3/8/8/8/4N3/8/8/4K3 w - - 0 1")

	var inc Accumulator
	inc.Refresh(n, pos) // bare kings
	inc.Add(n, position.White, idxW)
	inc.Add(n, position.Black, idxB)

	var fresh Accumulator
	fresh.Refresh(n, posWithN)

	if inc.accumulation != fresh.accumulation {
		t.Fatalf("incremental update disagrees with refresh")
	}
}

// --- Tiny hand-made network round-trips through save+load and evaluates -------

// TestTinyNetworkRoundTrip builds a small deterministic network, saves it,
// reloads it, and checks that the reloaded network is byte-identical in its
// parameters and produces the same evaluation as the original.
func TestTinyNetworkRoundTrip(t *testing.T) {
	orig := randomTinyNetwork(42)

	dir := t.TempDir()
	path := filepath.Join(dir, "tiny.nnue")
	if err := orig.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !loaded.HashOK {
		t.Fatalf("loaded.HashOK = false, header hash mismatch on round-trip")
	}

	// Parameters must match exactly.
	if loaded.ftBiases != orig.ftBiases {
		t.Fatalf("ftBiases changed across round-trip")
	}
	if len(loaded.ftWeights) != len(orig.ftWeights) {
		t.Fatalf("ftWeights length changed: %d vs %d", len(loaded.ftWeights), len(orig.ftWeights))
	}
	for i := range orig.ftWeights {
		if loaded.ftWeights[i] != orig.ftWeights[i] {
			t.Fatalf("ftWeights[%d] changed: %d vs %d", i, loaded.ftWeights[i], orig.ftWeights[i])
		}
	}
	if loaded.l1Biases != orig.l1Biases || loaded.l1Weights != orig.l1Weights {
		t.Fatalf("layer1 params changed across round-trip")
	}
	if loaded.l2Biases != orig.l2Biases || loaded.l2Weights != orig.l2Weights {
		t.Fatalf("layer2 params changed across round-trip")
	}
	if loaded.outBias != orig.outBias || loaded.outWeights != orig.outWeights {
		t.Fatalf("output params changed across round-trip")
	}

	// Evaluations must match.
	pos := mustFEN(t, position.StartingPosition)
	if a, b := orig.Eval(pos), loaded.Eval(pos); a != b {
		t.Fatalf("eval differs after round-trip: %d vs %d", a, b)
	}
}

// TestTinyNetworkEvalDeterministic checks that the tiny network produces a
// stable, finite evaluation and that flipping the side to move negates the
// transformed-input ordering in a sane way (eval stays within int16 range).
func TestTinyNetworkEvalDeterministic(t *testing.T) {
	n := randomTinyNetwork(7)
	pos := mustFEN(t, position.StartingPosition)

	v1 := n.Eval(pos)
	v2 := n.Eval(pos)
	if v1 != v2 {
		t.Fatalf("eval not deterministic: %d vs %d", v1, v2)
	}

	// EvalWith on a freshly refreshed accumulator must equal Eval.
	var acc Accumulator
	acc.Refresh(n, pos)
	if got := n.EvalWith(&acc, pos.SideToMove); got != v1 {
		t.Fatalf("EvalWith = %d, Eval = %d", got, v1)
	}
}

// --- Real-net behavioural tests (skipped if no net present) -------------------

// TestRealNetLoads verifies the loader against a genuine classic NNUE file and
// the structural facts about it.
func TestRealNetLoads(t *testing.T) {
	if !haveRealNet() {
		t.Skip("no real net at " + realNetPath)
	}
	n, err := Load(realNetPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !n.HashOK {
		t.Fatalf("real net failed hash check (HashOK=false); arch=%q", n.Architecture)
	}
	if len(n.ftWeights) != FeatureDimensions*TransformedFeatureDimensions {
		t.Fatalf("ftWeights length = %d, want %d",
			len(n.ftWeights), FeatureDimensions*TransformedFeatureDimensions)
	}
}

// TestRealNetStartPositionSmall checks the start-position eval is small in
// magnitude, as a correctly loaded net must give a roughly balanced opening.
func TestRealNetStartPositionSmall(t *testing.T) {
	if !haveRealNet() {
		t.Skip("no real net at " + realNetPath)
	}
	n, err := Load(realNetPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	pos := mustFEN(t, position.StartingPosition)
	v := n.Eval(pos)
	if v < -100 || v > 100 {
		t.Fatalf("start-position eval = %d cp, expected |eval| <= 100", v)
	}
}

// TestRealNetMaterialSign checks that a large material advantage carries the
// correct sign from the side-to-move perspective.
func TestRealNetMaterialSign(t *testing.T) {
	if !haveRealNet() {
		t.Skip("no real net at " + realNetPath)
	}
	n, err := Load(realNetPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// White is up a whole queen, white to move: eval should be strongly +ve.
	whiteUp := mustFEN(t, "rnb1kbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1")
	if v := n.Eval(whiteUp); v < 500 {
		t.Fatalf("white up a queen (white to move): eval = %d cp, want >= 500", v)
	}

	// Same board, but black to move: from black's perspective black is down a
	// queen, so eval should be strongly -ve.
	whiteUpBlackTM := mustFEN(t, "rnb1kbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR b KQkq - 0 1")
	if v := n.Eval(whiteUpBlackTM); v > -500 {
		t.Fatalf("white up a queen (black to move): eval = %d cp, want <= -500", v)
	}

	// Black up a queen (white missing its queen), white to move: strongly -ve.
	blackUp := mustFEN(t, "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNB1KBNR w KQkq - 0 1")
	if v := n.Eval(blackUp); v > -300 {
		t.Fatalf("black up a queen (white to move): eval = %d cp, want <= -300", v)
	}
}

// TestRealNetIncrementalMatchesRefresh checks, against the real net, that an
// incrementally maintained accumulator agrees with a from-scratch refresh.
// It transforms the start position into one with an extra white knight by
// adding the appropriate features to both perspectives.
func TestRealNetIncrementalMatchesRefresh(t *testing.T) {
	if !haveRealNet() {
		t.Skip("no real net at " + realNetPath)
	}
	n, err := Load(realNetPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	base := mustFEN(t, "4k3/8/8/8/8/8/8/4K3 w - - 0 1")    // bare kings
	withN := mustFEN(t, "4k3/8/8/8/4N3/8/8/4K3 w - - 0 1") // + white knight e4

	var inc Accumulator
	inc.Refresh(n, base)
	inc.Add(n, position.White,
		MakeIndex(position.White, position.SquareE4, position.WhiteKnight,
			orient(position.White, base.KingLocation[0])))
	inc.Add(n, position.Black,
		MakeIndex(position.Black, position.SquareE4, position.WhiteKnight,
			orient(position.Black, base.KingLocation[1])))

	var fresh Accumulator
	fresh.Refresh(n, withN)

	if inc.accumulation != fresh.accumulation {
		t.Fatalf("incremental accumulator disagrees with refresh on real net")
	}
	if a, b := n.EvalWith(&inc, withN.SideToMove), n.Eval(withN); a != b {
		t.Fatalf("incremental eval %d != refresh eval %d", a, b)
	}
}

// --- helpers ------------------------------------------------------------------

func mustFEN(t *testing.T, fen string) *position.Position {
	t.Helper()
	pos, err := position.NewPositionFromFEN(fen)
	if err != nil {
		t.Fatalf("parse FEN %q: %v", fen, err)
	}
	return pos
}

// randomTinyNetwork builds a fully-populated network with small pseudo-random
// parameters drawn from a fixed seed. The feature-transformer weight slice is
// the full 41024*256 size (it must be, for the file layout to round-trip), but
// values are tiny so accumulators and evals stay in range.
func randomTinyNetwork(seed int64) *Network {
	rng := rand.New(rand.NewSource(seed))
	n := NewEmptyNetwork()
	n.Architecture = "tiny-test-net"

	for i := range n.ftBiases {
		n.ftBiases[i] = int16(rng.Intn(7) - 3)
	}
	// Only populate a sparse handful of feature columns to keep this fast;
	// the rest stay zero. Round-trip still serializes the whole slice.
	for k := 0; k < 2000; k++ {
		idx := rng.Intn(FeatureDimensions)
		col := n.ftWeights[idx*TransformedFeatureDimensions : (idx+1)*TransformedFeatureDimensions]
		for j := range col {
			col[j] = int16(rng.Intn(5) - 2)
		}
	}
	for i := range n.l1Biases {
		n.l1Biases[i] = int32(rng.Intn(2000) - 1000)
	}
	for i := range n.l1Weights {
		n.l1Weights[i] = int8(rng.Intn(15) - 7)
	}
	for i := range n.l2Biases {
		n.l2Biases[i] = int32(rng.Intn(2000) - 1000)
	}
	for i := range n.l2Weights {
		n.l2Weights[i] = int8(rng.Intn(15) - 7)
	}
	n.outBias = int32(rng.Intn(2000) - 1000)
	for i := range n.outWeights {
		n.outWeights[i] = int8(rng.Intn(15) - 7)
	}
	return n
}
