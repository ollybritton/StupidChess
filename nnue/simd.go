package nnue

// This file and its build-tagged siblings provide SIMD-accelerated kernels for the NNUE hot paths, with
// a pure-Go fallback. The public kernels (dotInt8, ...) are defined per architecture: simd_amd64.go uses
// AVX2 when the CPU has it, simd_noasm.go falls back to the generic Go below on every other platform.
// Each accelerated kernel is gated by a bit-identical test against the generic version (simd_test.go), so
// the assembly can never silently diverge from the reference arithmetic.

// dotInt8Generic is the reference (and fallback) dot product: sum over j of int8 weight * uint8 input,
// accumulated in int32. The accelerated kernels must reproduce this exactly. Inputs are clipped to
// [0,127] before reaching here, so no intermediate overflows.
func dotInt8Generic(in []uint8, row []int8, n int) int32 {
	var sum int32
	for j := 0; j < n; j++ {
		sum += int32(row[j]) * int32(in[j])
	}
	return sum
}

// addInt16Generic / subInt16Generic are the reference (and fallback) feature-transformer accumulator
// updates: acc[j] += / -= col[j], wrapping int16 (exactly as the assembly VPADDW / VPSUBW do). These are
// the hot loops of the accumulator refresh and the incremental add/remove of a feature column.
func addInt16Generic(acc, col []int16, n int) {
	for j := 0; j < n; j++ {
		acc[j] += col[j]
	}
}

func subInt16Generic(acc, col []int16, n int) {
	for j := 0; j < n; j++ {
		acc[j] -= col[j]
	}
}
