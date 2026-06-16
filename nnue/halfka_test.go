package nnue

import (
	"math/rand"
	"os"
	"testing"

	"github.com/ollybritton/StupidChess/position"
)

const realKANetPath = "testdata/nn-ad9b42354671.nnue"

// haveRealKANet reports whether the genuine HalfKAv2_hm net is available. The
// behavioural tests skip rather than fail when it is absent, so the package still
// builds and tests green without the (gitignored) net file.
func haveRealKANet() bool {
	_, err := os.Stat(realKANetPath)
	return err == nil
}

// NOTE: these tests are intentionally self-contained: they validate the
// architecture hash and the HalfKAv2_hm feature-indexing tables (orient mirror,
// king buckets, dimensions) using only constants in this package. They do NOT
// load the real net or invoke the Stockfish oracle.
//
// The full bit-exact verification against Stockfish 15.1's NNUE eval (the
// "NNUE evaluation <x.xx> (white side)" value for nn-ad9b42354671.nnue) was run
// ad hoc over several hundred varied positions and is not kept as a committed
// test, because it depends on the gitignored net file and an external oracle
// binary. To reproduce: load the net with LoadKA, take KANetwork.EvalInternal,
// negate it when side-to-move is Black to get the White-POV value, and divide
// by KANormalizeToPawn (361) to obtain the pawn value Stockfish prints.

// TestExpectedKANetworkHash pins the architecture hash this package computes for
// HalfKAv2_hm (1024x2, 8 stacks). It is the hash word embedded in the file
// header of nn-ad9b42354671.nnue.
func TestExpectedKANetworkHash(t *testing.T) {
	const want = 0x1C102EF2
	if got := ExpectedKANetworkHash(); got != want {
		t.Fatalf("ExpectedKANetworkHash = 0x%08X, want 0x%08X", got, want)
	}
}

// TestKADimensions pins the per-perspective feature count and key constants.
func TestKADimensions(t *testing.T) {
	if KAFeatureDims != 22528 {
		t.Fatalf("KAFeatureDims = %d, want 22528", KAFeatureDims)
	}
	if psNB != 704 {
		t.Fatalf("psNB = %d, want 704", psNB)
	}
	if KAHalfDims != 1024 {
		t.Fatalf("KAHalfDims = %d, want 1024", KAHalfDims)
	}
	// Max feature index must stay in range: 31*704 + (10*64 + 63) = 22527.
	maxIdx := uint32(31)*psNB + psKingKA + 63
	if maxIdx != KAFeatureDims-1 {
		t.Fatalf("max feature index = %d, want %d", maxIdx, KAFeatureDims-1)
	}
}

// TestKAOrient checks the file-conditional orient XOR transcribed from OrientTBL.
// White king on files a..d mirrors horizontally (^7); on e..h it is identity.
// Black has a rank-flip baseline: a..d => ^63, e..h => ^56.
func TestKAOrient(t *testing.T) {
	cases := []struct {
		persp position.Color
		ksq   uint8
		want  uint8
	}{
		{position.White, position.SquareA1, 7},  // file a -> mirror
		{position.White, position.SquareD1, 7},  // file d -> mirror
		{position.White, position.SquareE1, 0},  // file e -> identity
		{position.White, position.SquareH1, 0},  // file h -> identity
		{position.White, position.SquareC4, 7},  // file c -> mirror
		{position.White, position.SquareG6, 0},  // file g -> identity
		{position.Black, position.SquareA8, 63}, // file a -> 180
		{position.Black, position.SquareD2, 63}, // file d -> 180
		{position.Black, position.SquareE8, 56}, // file e -> rank flip
		{position.Black, position.SquareH1, 56}, // file h -> rank flip
	}
	for _, c := range cases {
		if got := kaOrientTBL[perspectiveIndex(c.persp)][c.ksq]; got != c.want {
			t.Errorf("orient(%v, %d) = %d, want %d", c.persp, c.ksq, got, c.want)
		}
	}
}

// TestKAKingBuckets checks the king-bucket closed form against the literal
// KingBuckets table corners, and that the two perspectives are vertical mirrors.
func TestKAKingBuckets(t *testing.T) {
	w := perspectiveIndex(position.White)
	b := perspectiveIndex(position.Black)
	// White: v = 4*(7-rank) + min(file,7-file). A1(rank0,fileA)=28; H8(rank7,fileH)=0.
	cases := []struct {
		sq uint8
		wW uint32
		wB uint32
	}{
		{position.SquareA1, 28, 0},
		{position.SquareD1, 31, 3},
		{position.SquareE1, 31, 3},
		{position.SquareH1, 28, 0},
		{position.SquareA8, 0, 28},
		{position.SquareH8, 0, 28},
		// E4 = square 28, rank3, fileE(4), min(4,3)=3: W=4*(7-3)+3=19, B=4*3+3=15.
		{position.SquareE4, 19, 15},
	}
	for _, c := range cases {
		if got := kaKingBuckets[w][c.sq]; got != c.wW {
			t.Errorf("kingBucket[White][%d] = %d, want %d", c.sq, got, c.wW)
		}
		if got := kaKingBuckets[b][c.sq]; got != c.wB {
			t.Errorf("kingBucket[Black][%d] = %d, want %d", c.sq, got, c.wB)
		}
	}
}

// --- Incremental Update vs Refresh over random games --------------------------

// TestKAUpdateMatchesRefreshRandomGames is the core correctness check for the
// HalfKAv2_hm incremental accumulator. It plays many random legal games with a
// densely-randomised feature transformer (every FT and PSQT column populated with
// distinct values, so any wrong feature index cannot cancel) and, after every
// move, asserts that an accumulator carried forward with Update is bit-identical
// to one rebuilt with Refresh - in BOTH the int16 FT accumulation and the int32
// PSQT side-channel - and that the resulting internal eval agrees. This exercises
// the king-move refresh (each perspective re-buckets and mirrors on a king move),
// the king-as-feature handling on the opponent perspective, captures (including
// en passant), promotions and the castling rook.
func TestKAUpdateMatchesRefreshRandomGames(t *testing.T) {
	n := denseKATestNetwork(1)
	for seed := int64(0); seed < 30; seed++ {
		playRandomKAGameChecking(t, n, seed, 120)
	}
}

// TestRealKANetUpdateMatchesRefreshRandomGames runs the same random-game check
// against the genuine HalfKAv2_hm net, validating the incremental path on the
// exact weights the engine plays with (skipped if the net is absent).
func TestRealKANetUpdateMatchesRefreshRandomGames(t *testing.T) {
	if !haveRealKANet() {
		t.Skip("no real KA net at " + realKANetPath)
	}
	n, err := LoadKA(realKANetPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for seed := int64(0); seed < 8; seed++ {
		playRandomKAGameChecking(t, n, seed, 120)
	}
}

// playRandomKAGameChecking plays up to maxMoves random legal moves from the start
// position, maintaining a KAAccumulator incrementally with Update and comparing
// it (FT accumulation, PSQT side-channel, and internal eval) to a from-scratch
// Refresh after each move.
func playRandomKAGameChecking(t *testing.T, n *KANetwork, seed int64, maxMoves int) {
	t.Helper()
	rng := rand.New(rand.NewSource(seed + 1))
	pos := mustFEN(t, position.StartingPosition)

	var inc KAAccumulator
	inc.Refresh(n, pos)

	for ply := 0; ply < maxMoves; ply++ {
		moves := pos.MovesPseudolegal().AsSlice()
		rng.Shuffle(len(moves), func(i, j int) { moves[i], moves[j] = moves[j], moves[i] })

		var move position.Move
		made := false
		for _, mv := range moves {
			if pos.MakeMove(mv) {
				move, made = mv, true
				break
			}
		}
		if !made {
			return // checkmate or stalemate: game over
		}

		var child, fresh KAAccumulator
		child.Update(n, &inc, pos, move)
		fresh.Refresh(n, pos)
		if child.accumulation != fresh.accumulation {
			t.Fatalf("seed %d ply %d: Update FT accumulation disagrees with Refresh after %s (fen %s)",
				seed, ply, move.String(), pos.StringFEN())
		}
		if child.psqt != fresh.psqt {
			t.Fatalf("seed %d ply %d: Update PSQT accumulation disagrees with Refresh after %s (fen %s)",
				seed, ply, move.String(), pos.StringFEN())
		}
		pc := KAPieceCount(pos)
		if a, b := n.EvalWith(&child, pos.SideToMove, pc), n.EvalWith(&fresh, pos.SideToMove, pc); a != b {
			t.Fatalf("seed %d ply %d: incremental eval %d != refresh eval %d after %s",
				seed, ply, a, b, move.String())
		}
		inc = child
	}
}

// denseKATestNetwork builds a HalfKAv2_hm network whose entire feature
// transformer (both the int16 FT weight matrix and the int32 PSQT side-channel)
// is filled with distinct small pseudo-random values, so the incremental-vs-
// refresh test is sensitive to any wrong feature index. The dense-stack layers
// are filled too, so EvalWith exercises a non-trivial forward pass; their exact
// values do not matter for the Update==Refresh property.
func denseKATestNetwork(seed int64) *KANetwork {
	rng := rand.New(rand.NewSource(seed*2654435761 + 1))
	n := &KANetwork{Architecture: "dense-ka-test-net"}

	for i := range n.ftBiases {
		n.ftBiases[i] = int16(rng.Intn(64) - 32)
	}
	n.ftWeights = make([]int16, KAFeatureDims*KAHalfDims)
	for i := range n.ftWeights {
		n.ftWeights[i] = int16(rng.Intn(64) - 32)
	}
	n.psqtWeights = make([]int32, KAFeatureDims*KAPSQTBuckets)
	for i := range n.psqtWeights {
		n.psqtWeights[i] = int32(rng.Intn(256) - 128)
	}
	for s := range n.stacks {
		st := &n.stacks[s]
		for i := range st.fc0Bias {
			st.fc0Bias[i] = int32(rng.Intn(2000) - 1000)
		}
		for i := range st.fc0Weights {
			st.fc0Weights[i] = int8(rng.Intn(15) - 7)
		}
		for i := range st.fc1Bias {
			st.fc1Bias[i] = int32(rng.Intn(2000) - 1000)
		}
		for i := range st.fc1Weights {
			st.fc1Weights[i] = int8(rng.Intn(15) - 7)
		}
		st.fc2Bias = int32(rng.Intn(2000) - 1000)
		for i := range st.fc2Weights {
			st.fc2Weights[i] = int8(rng.Intn(15) - 7)
		}
	}
	return n
}
