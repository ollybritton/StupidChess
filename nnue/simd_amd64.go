//go:build amd64

package nnue

import (
	"os"

	"golang.org/x/sys/cpu"
)

// useAVX2 is set once at start-up. When false (very old x86, an emulator without AVX2, or SC_NO_AVX2 set
// to A/B the speedup) the kernels fall back to the generic Go path, so the package stays correct
// everywhere.
var useAVX2 = cpu.X86.HasAVX2 && os.Getenv("SC_NO_AVX2") == ""

// dotInt8AVX2 computes the int8xuint8 -> int32 dot product of the first n bytes (n a multiple of 32),
// implemented in simd_amd64.s with VPMADDUBSW + VPMADDWD. It is bit-identical to dotInt8Generic for the
// [0,127] clipped inputs the network feeds it.
//
//go:noescape
func dotInt8AVX2(in *uint8, row *int8, n int) int32

//go:noescape
func addInt16AVX2(acc *int16, col *int16, n int)

//go:noescape
func subInt16AVX2(acc *int16, col *int16, n int)

// dotInt8 does the full dot product: the 32-wide aligned body in AVX2 (when available), the tail in Go.
func dotInt8(in []uint8, row []int8, n int) int32 {
	body := n &^ 31 // largest multiple of 32 <= n
	var sum int32
	if useAVX2 && body > 0 {
		sum = dotInt8AVX2(&in[0], &row[0], body)
	} else {
		sum = dotInt8Generic(in, row, body)
	}
	for j := body; j < n; j++ {
		sum += int32(row[j]) * int32(in[j])
	}
	return sum
}

// addInt16 / subInt16 add or subtract a feature column into an accumulator: the 16-wide aligned body in
// AVX2, the tail in Go. The transformer dimensions (256, 1024) are multiples of 16, so the tail is empty
// in practice.
func addInt16(acc, col []int16, n int) {
	body := n &^ 15
	if useAVX2 && body > 0 {
		addInt16AVX2(&acc[0], &col[0], body)
	} else {
		addInt16Generic(acc, col, body)
	}
	for j := body; j < n; j++ {
		acc[j] += col[j]
	}
}

func subInt16(acc, col []int16, n int) {
	body := n &^ 15
	if useAVX2 && body > 0 {
		subInt16AVX2(&acc[0], &col[0], body)
	} else {
		subInt16Generic(acc, col, body)
	}
	for j := body; j < n; j++ {
		acc[j] -= col[j]
	}
}
