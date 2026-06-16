package nnue

import (
	"math/rand"
	"testing"
)

// TestDotInt8MatchesGeneric is the safety net for the SIMD kernels: the dispatched dotInt8 (which uses
// the AVX2/NEON assembly when available) must equal the pure-Go reference for every length, exactly. Run
// it under the target arch to exercise the assembly, e.g. `GOARCH=amd64 go test ./nnue/` on an arm64 host
// (Rosetta) to cover AVX2, or natively on amd64/arm64.
func TestDotInt8MatchesGeneric(t *testing.T) {
	rng := rand.New(rand.NewSource(20260616))
	// Cover sub-32 (pure tail), exact multiples of 32, and just-over/under, plus the real layer widths.
	lengths := []int{0, 1, 5, 17, 30, 31, 32, 33, 47, 64, 95, 96, 127, 128, 256, 512, 1023, 1024, 2048}
	for _, n := range lengths {
		for trial := 0; trial < 300; trial++ {
			in := make([]uint8, n)
			row := make([]int8, n)
			for j := range in {
				in[j] = uint8(rng.Intn(128)) // activations are clipped to [0,127]
				row[j] = int8(rng.Intn(256) - 128)
			}
			if got, want := dotInt8(in, row, n), dotInt8Generic(in, row, n); got != want {
				t.Fatalf("n=%d trial=%d: dotInt8=%d, generic=%d", n, trial, got, want)
			}
		}
	}
}

// TestAddSubInt16MatchesGeneric checks the feature-transformer add/sub kernels against the reference,
// including values that overflow int16 (the wrap must match VPADDW/VPSUBW exactly).
func TestAddSubInt16MatchesGeneric(t *testing.T) {
	rng := rand.New(rand.NewSource(424242))
	for _, n := range []int{0, 1, 15, 16, 17, 31, 256, 1024, 1025} {
		for trial := 0; trial < 300; trial++ {
			base := make([]int16, n)
			col := make([]int16, n)
			for j := range base {
				base[j] = int16(rng.Intn(1 << 16))
				col[j] = int16(rng.Intn(1 << 16))
			}
			gotA := append([]int16(nil), base...)
			wantA := append([]int16(nil), base...)
			addInt16(gotA, col, n)
			addInt16Generic(wantA, col, n)
			for j := range gotA {
				if gotA[j] != wantA[j] {
					t.Fatalf("add n=%d trial=%d j=%d: %d != %d", n, trial, j, gotA[j], wantA[j])
				}
			}
			gotS := append([]int16(nil), base...)
			wantS := append([]int16(nil), base...)
			subInt16(gotS, col, n)
			subInt16Generic(wantS, col, n)
			for j := range gotS {
				if gotS[j] != wantS[j] {
					t.Fatalf("sub n=%d trial=%d j=%d: %d != %d", n, trial, j, gotS[j], wantS[j])
				}
			}
		}
	}
}
