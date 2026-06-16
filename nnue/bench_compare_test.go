package nnue

import (
	"testing"

	"github.com/ollybritton/StupidChess/position"
)

// midgame is a busy middlegame position used for the timing comparison.
const benchFEN = "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1"

// BenchmarkKAEvalFull times a full HalfKAv2_hm evaluation (refresh + forward).
func BenchmarkKAEvalFull(b *testing.B) {
	if !haveRealKANet() {
		b.Skip("no real KA net")
	}
	n, err := LoadKA(realKANetPath)
	if err != nil {
		b.Fatal(err)
	}
	pos := mustFENB(b, benchFEN)
	b.ResetTimer()
	var sink int32
	for i := 0; i < b.N; i++ {
		sink += n.EvalInternal(pos)
	}
	_ = sink
}

// BenchmarkKAEvalForwardOnly times only the forward pass from a prebuilt KA
// accumulator (the cost paid per node when the accumulator is incremental).
func BenchmarkKAEvalForwardOnly(b *testing.B) {
	if !haveRealKANet() {
		b.Skip("no real KA net")
	}
	n, err := LoadKA(realKANetPath)
	if err != nil {
		b.Fatal(err)
	}
	pos := mustFENB(b, benchFEN)
	var acc KAAccumulator
	acc.Refresh(n, pos)
	pc := KAPieceCount(pos)
	b.ResetTimer()
	var sink int32
	for i := 0; i < b.N; i++ {
		sink += n.EvalWith(&acc, pos.SideToMove, pc)
	}
	_ = sink
}

// BenchmarkKARefresh times only a full accumulator refresh (both perspectives).
func BenchmarkKARefresh(b *testing.B) {
	if !haveRealKANet() {
		b.Skip("no real KA net")
	}
	n, err := LoadKA(realKANetPath)
	if err != nil {
		b.Fatal(err)
	}
	pos := mustFENB(b, benchFEN)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var acc KAAccumulator
		acc.Refresh(n, pos)
	}
}

// BenchmarkHKPEvalFull times a full HalfKP evaluation (refresh + forward) for
// comparison.
func BenchmarkHKPEvalFull(b *testing.B) {
	if !haveRealNet() {
		b.Skip("no real HalfKP net")
	}
	n, err := Load(realNetPath)
	if err != nil {
		b.Fatal(err)
	}
	pos := mustFENB(b, benchFEN)
	b.ResetTimer()
	var sink int16
	for i := 0; i < b.N; i++ {
		sink += n.Eval(pos)
	}
	_ = sink
}

func mustFENB(b *testing.B, fen string) *position.Position {
	b.Helper()
	pos, err := position.NewPositionFromFEN(fen)
	if err != nil {
		b.Fatal(err)
	}
	return pos
}

// BenchmarkKAUpdateNonKing times an incremental Update for a quiet non-king,
// non-capture move (the common case: copy both perspectives forward + a few
// column add/removes). This is the per-node accumulator cost in search.
func BenchmarkKAUpdateNonKing(b *testing.B) {
	if !haveRealKANet() {
		b.Skip("no real KA net")
	}
	n, err := LoadKA(realKANetPath)
	if err != nil {
		b.Fatal(err)
	}
	pos := mustFENB(b, benchFEN)
	var parent KAAccumulator
	parent.Refresh(n, pos)
	// Pick a quiet knight move: Nc3-b1 is not available; use a pawn push g2-g3
	// (white pawn from g2). Find a concrete legal non-king move.
	var move position.Move
	for _, mv := range pos.MovesPseudolegal().AsSlice() {
		clone := *pos
		if clone.MakeMove(mv) && mv.Moved().Colorless() != position.King && mv.Captured() == position.Empty {
			move = mv
			pos = &clone
			break
		}
	}
	b.ResetTimer()
	var child KAAccumulator
	for i := 0; i < b.N; i++ {
		child.Update(n, &parent, pos, move)
	}
	_ = child
}
