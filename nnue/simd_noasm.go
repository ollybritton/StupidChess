//go:build !amd64

package nnue

// dotInt8 on platforms without an assembly kernel is the generic Go version. (arm64 gets its own NEON
// kernel in simd_arm64.go; this covers everything else.)
func dotInt8(in []uint8, row []int8, n int) int32 {
	return dotInt8Generic(in, row, n)
}

func addInt16(acc, col []int16, n int) { addInt16Generic(acc, col, n) }
func subInt16(acc, col []int16, n int) { subInt16Generic(acc, col, n) }
