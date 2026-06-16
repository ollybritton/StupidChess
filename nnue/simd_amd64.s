#include "textflag.h"

// func dotInt8AVX2(in *uint8, row *int8, n int) int32
//
// Computes sum_{j<n} int32(row[j]) * int32(in[j]) with n a positive multiple of 32.
// in[] are uint8 in [0,127] (clipped activations); row[] are int8 weights. The pairwise products fit in
// int16 for that range, so VPMADDUBSW does not saturate and the result is bit-identical to the scalar
// int32 accumulation in dotInt8Generic.
TEXT ·dotInt8AVX2(SB), NOSPLIT, $0-32
	MOVQ in+0(FP), SI
	MOVQ row+8(FP), DI
	MOVQ n+16(FP), CX

	VPXOR    Y0, Y0, Y0 // int32 accumulator
	VPCMPEQW Y3, Y3, Y3 // Y3 = 0xFFFF... (int16 -1 in every lane)
	VPABSW   Y3, Y3     // Y3 = int16 +1 in every lane (for VPMADDWD pair-sum)

loop:
	VMOVDQU    (SI), Y4      // 32 uint8 inputs
	VMOVDQU    (DI), Y5      // 32 int8 weights
	VPMADDUBSW Y5, Y4, Y6    // Y6[k] = in[2k]*w[2k] + in[2k+1]*w[2k+1]   (16 int16)
	VPMADDWD   Y3, Y6, Y6    // Y6[m] = Y6[2m] + Y6[2m+1]                 (8 int32)
	VPADDD     Y6, Y0, Y0    // accumulate
	ADDQ       $32, SI
	ADDQ       $32, DI
	SUBQ       $32, CX
	JNZ        loop

	// Horizontal sum of the 8 int32 lanes in Y0 -> AX.
	VEXTRACTI128 $1, Y0, X1
	VPADDD       X1, X0, X0
	VPSHUFD      $0x4E, X0, X1
	VPADDD       X1, X0, X0
	VPSHUFD      $0xB1, X0, X1
	VPADDD       X1, X0, X0
	VMOVD        X0, AX

	MOVL AX, ret+24(FP)
	VZEROUPPER
	RET

// func addInt16AVX2(acc *int16, col *int16, n int)
// acc[j] += col[j] for j<n, n a positive multiple of 16. Wrapping int16 add (VPADDW), bit-identical to
// the Go loop.
TEXT ·addInt16AVX2(SB), NOSPLIT, $0-24
	MOVQ acc+0(FP), DI
	MOVQ col+8(FP), SI
	MOVQ n+16(FP), CX

addloop:
	VMOVDQU (DI), Y0
	VMOVDQU (SI), Y1
	VPADDW  Y1, Y0, Y0
	VMOVDQU Y0, (DI)
	ADDQ    $32, DI // 16 int16 = 32 bytes
	ADDQ    $32, SI
	SUBQ    $16, CX
	JNZ     addloop

	VZEROUPPER
	RET

// func subInt16AVX2(acc *int16, col *int16, n int)
// acc[j] -= col[j] for j<n, n a positive multiple of 16. Wrapping int16 sub (VPSUBW).
TEXT ·subInt16AVX2(SB), NOSPLIT, $0-24
	MOVQ acc+0(FP), DI
	MOVQ col+8(FP), SI
	MOVQ n+16(FP), CX

subloop:
	VMOVDQU (DI), Y0
	VMOVDQU (SI), Y1
	VPSUBW  Y1, Y0, Y0 // Y0 = Y0 - Y1 = acc - col
	VMOVDQU Y0, (DI)
	ADDQ    $32, DI
	ADDQ    $32, SI
	SUBQ    $16, CX
	JNZ     subloop

	VZEROUPPER
	RET
