package syzygy

// This file contains the lookup tables ported verbatim from Ronald de Man's
// tbcore.c (the file that tbprobe.c #includes; the copy in reference/fathom was
// missing it, so the canonical basil00/Fathom tbcore.c is the source of truth).
// Every constant here is copied from that C source and is authoritative.
//
// We use the non-CONNECTED_KINGS variant throughout, which is the standard
// (non-suicide) chess configuration.

// Piece type codes used internally, matching tbcore.c (TB_PAWN..TB_KING, with
// the black pieces ORed with 8):
//
//	1 = white pawn ... 6 = white king, 9 = black pawn ... 14 = black king.
const (
	tbPawn   = 1
	tbKnight = 2
	tbBishop = 3
	tbRook   = 4
	tbQueen  = 5
	tbKing   = 6
	tbWPawn  = tbPawn
	tbBPawn  = tbPawn | 8
)

// tbPieces is TBPIECES from tbcore.h.
const tbPieces = 6

// wdlMagic is WDL_MAGIC (0x5d23e871) as the four little-endian bytes that begin
// every .rtbw file.
var wdlMagic = [4]byte{0x71, 0xE8, 0x23, 0x5D}

// dtzMagic is DTZ_MAGIC (0xa50c66d7) as the four little-endian bytes that begin
// every .rtbz file.
var dtzMagic = [4]byte{0xD7, 0x66, 0x0C, 0xA5}

// DTZ value post-processing tables (tbprobe.c). wdlToMap selects the per-WDL-class
// map sub-table; paFlags is the "value already exact" accuracy bit per WDL class;
// wdlToDtz is the canonical DTZ for a WDL outcome (used in the en-passant ladder).
// All three are indexed by wdl+2 (wdl in -2..2).
var (
	wdlToMap = [5]uint8{1, 3, 0, 2, 0}
	paFlags  = [5]uint8{8, 0, 0, 0, 4}
	wdlToDtz = [5]int{-1, -101, 0, 101, 1}
)

// Material-signature primes (calc_key). Copied from tbprobe.c.
const (
	primeWhiteQueen  uint64 = 11811845319353239651
	primeWhiteRook   uint64 = 10979190538029446137
	primeWhiteBishop uint64 = 12311744257139811149
	primeWhiteKnight uint64 = 15202887380319082783
	primeWhitePawn   uint64 = 17008651141875982339
	primeBlackQueen  uint64 = 15484752644942473553
	primeBlackRook   uint64 = 18264461213049635989
	primeBlackBishop uint64 = 15394650811035483107
	primeBlackKnight uint64 = 13469005675588064321
	primeBlackPawn   uint64 = 11695583624105689831
)

var offdiag = [64]int8{
	0, -1, -1, -1, -1, -1, -1, -1,
	1, 0, -1, -1, -1, -1, -1, -1,
	1, 1, 0, -1, -1, -1, -1, -1,
	1, 1, 1, 0, -1, -1, -1, -1,
	1, 1, 1, 1, 0, -1, -1, -1,
	1, 1, 1, 1, 1, 0, -1, -1,
	1, 1, 1, 1, 1, 1, 0, -1,
	1, 1, 1, 1, 1, 1, 1, 0,
}

var triangle = [64]uint8{
	6, 0, 1, 2, 2, 1, 0, 6,
	0, 7, 3, 4, 4, 3, 7, 0,
	1, 3, 8, 5, 5, 8, 3, 1,
	2, 4, 5, 9, 9, 5, 4, 2,
	2, 4, 5, 9, 9, 5, 4, 2,
	1, 3, 8, 5, 5, 8, 3, 1,
	0, 7, 3, 4, 4, 3, 7, 0,
	6, 0, 1, 2, 2, 1, 0, 6,
}

var flipdiag = [64]uint8{
	0, 8, 16, 24, 32, 40, 48, 56,
	1, 9, 17, 25, 33, 41, 49, 57,
	2, 10, 18, 26, 34, 42, 50, 58,
	3, 11, 19, 27, 35, 43, 51, 59,
	4, 12, 20, 28, 36, 44, 52, 60,
	5, 13, 21, 29, 37, 45, 53, 61,
	6, 14, 22, 30, 38, 46, 54, 62,
	7, 15, 23, 31, 39, 47, 55, 63,
}

var lower = [64]uint8{
	28, 0, 1, 2, 3, 4, 5, 6,
	0, 29, 7, 8, 9, 10, 11, 12,
	1, 7, 30, 13, 14, 15, 16, 17,
	2, 8, 13, 31, 18, 19, 20, 21,
	3, 9, 14, 18, 32, 22, 23, 24,
	4, 10, 15, 19, 22, 33, 25, 26,
	5, 11, 16, 20, 23, 25, 34, 27,
	6, 12, 17, 21, 24, 26, 27, 35,
}

var diag = [64]uint8{
	0, 0, 0, 0, 0, 0, 0, 8,
	0, 1, 0, 0, 0, 0, 9, 0,
	0, 0, 2, 0, 0, 10, 0, 0,
	0, 0, 0, 3, 11, 0, 0, 0,
	0, 0, 0, 12, 4, 0, 0, 0,
	0, 0, 13, 0, 0, 5, 0, 0,
	0, 14, 0, 0, 0, 0, 6, 0,
	15, 0, 0, 0, 0, 0, 0, 7,
}

var flap = [64]uint8{
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 6, 12, 18, 18, 12, 6, 0,
	1, 7, 13, 19, 19, 13, 7, 1,
	2, 8, 14, 20, 20, 14, 8, 2,
	3, 9, 15, 21, 21, 15, 9, 3,
	4, 10, 16, 22, 22, 16, 10, 4,
	5, 11, 17, 23, 23, 17, 11, 5,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var ptwist = [64]uint8{
	0, 0, 0, 0, 0, 0, 0, 0,
	47, 35, 23, 11, 10, 22, 34, 46,
	45, 33, 21, 9, 8, 20, 32, 44,
	43, 31, 19, 7, 6, 18, 30, 42,
	41, 29, 17, 5, 4, 16, 28, 40,
	39, 27, 15, 3, 2, 14, 26, 38,
	37, 25, 13, 1, 0, 12, 24, 36,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var invflap = [24]uint8{
	8, 16, 24, 32, 40, 48,
	9, 17, 25, 33, 41, 49,
	10, 18, 26, 34, 42, 50,
	11, 19, 27, 35, 43, 51,
}

var fileToFile = [8]uint8{0, 1, 2, 3, 3, 2, 1, 0}

// kkIdx is KK_idx[10][64] from tbcore.c (non-CONNECTED_KINGS). -1 entries are
// illegal king placements.
var kkIdx = [10][64]int32{
	{
		-1, -1, -1, 0, 1, 2, 3, 4,
		-1, -1, -1, 5, 6, 7, 8, 9,
		10, 11, 12, 13, 14, 15, 16, 17,
		18, 19, 20, 21, 22, 23, 24, 25,
		26, 27, 28, 29, 30, 31, 32, 33,
		34, 35, 36, 37, 38, 39, 40, 41,
		42, 43, 44, 45, 46, 47, 48, 49,
		50, 51, 52, 53, 54, 55, 56, 57,
	},
	{
		58, -1, -1, -1, 59, 60, 61, 62,
		63, -1, -1, -1, 64, 65, 66, 67,
		68, 69, 70, 71, 72, 73, 74, 75,
		76, 77, 78, 79, 80, 81, 82, 83,
		84, 85, 86, 87, 88, 89, 90, 91,
		92, 93, 94, 95, 96, 97, 98, 99,
		100, 101, 102, 103, 104, 105, 106, 107,
		108, 109, 110, 111, 112, 113, 114, 115,
	},
	{
		116, 117, -1, -1, -1, 118, 119, 120,
		121, 122, -1, -1, -1, 123, 124, 125,
		126, 127, 128, 129, 130, 131, 132, 133,
		134, 135, 136, 137, 138, 139, 140, 141,
		142, 143, 144, 145, 146, 147, 148, 149,
		150, 151, 152, 153, 154, 155, 156, 157,
		158, 159, 160, 161, 162, 163, 164, 165,
		166, 167, 168, 169, 170, 171, 172, 173,
	},
	{
		174, -1, -1, -1, 175, 176, 177, 178,
		179, -1, -1, -1, 180, 181, 182, 183,
		184, -1, -1, -1, 185, 186, 187, 188,
		189, 190, 191, 192, 193, 194, 195, 196,
		197, 198, 199, 200, 201, 202, 203, 204,
		205, 206, 207, 208, 209, 210, 211, 212,
		213, 214, 215, 216, 217, 218, 219, 220,
		221, 222, 223, 224, 225, 226, 227, 228,
	},
	{
		229, 230, -1, -1, -1, 231, 232, 233,
		234, 235, -1, -1, -1, 236, 237, 238,
		239, 240, -1, -1, -1, 241, 242, 243,
		244, 245, 246, 247, 248, 249, 250, 251,
		252, 253, 254, 255, 256, 257, 258, 259,
		260, 261, 262, 263, 264, 265, 266, 267,
		268, 269, 270, 271, 272, 273, 274, 275,
		276, 277, 278, 279, 280, 281, 282, 283,
	},
	{
		284, 285, 286, 287, 288, 289, 290, 291,
		292, 293, -1, -1, -1, 294, 295, 296,
		297, 298, -1, -1, -1, 299, 300, 301,
		302, 303, -1, -1, -1, 304, 305, 306,
		307, 308, 309, 310, 311, 312, 313, 314,
		315, 316, 317, 318, 319, 320, 321, 322,
		323, 324, 325, 326, 327, 328, 329, 330,
		331, 332, 333, 334, 335, 336, 337, 338,
	},
	{
		-1, -1, 339, 340, 341, 342, 343, 344,
		-1, -1, 345, 346, 347, 348, 349, 350,
		-1, -1, 441, 351, 352, 353, 354, 355,
		-1, -1, -1, 442, 356, 357, 358, 359,
		-1, -1, -1, -1, 443, 360, 361, 362,
		-1, -1, -1, -1, -1, 444, 363, 364,
		-1, -1, -1, -1, -1, -1, 445, 365,
		-1, -1, -1, -1, -1, -1, -1, 446,
	},
	{
		-1, -1, -1, 366, 367, 368, 369, 370,
		-1, -1, -1, 371, 372, 373, 374, 375,
		-1, -1, -1, 376, 377, 378, 379, 380,
		-1, -1, -1, 447, 381, 382, 383, 384,
		-1, -1, -1, -1, 448, 385, 386, 387,
		-1, -1, -1, -1, -1, 449, 388, 389,
		-1, -1, -1, -1, -1, -1, 450, 390,
		-1, -1, -1, -1, -1, -1, -1, 451,
	},
	{
		452, 391, 392, 393, 394, 395, 396, 397,
		-1, -1, -1, -1, 398, 399, 400, 401,
		-1, -1, -1, -1, 402, 403, 404, 405,
		-1, -1, -1, -1, 406, 407, 408, 409,
		-1, -1, -1, -1, 453, 410, 411, 412,
		-1, -1, -1, -1, -1, 454, 413, 414,
		-1, -1, -1, -1, -1, -1, 455, 415,
		-1, -1, -1, -1, -1, -1, -1, 456,
	},
	{
		457, 416, 417, 418, 419, 420, 421, 422,
		-1, 458, 423, 424, 425, 426, 427, 428,
		-1, -1, -1, -1, -1, 429, 430, 431,
		-1, -1, -1, -1, -1, 432, 433, 434,
		-1, -1, -1, -1, -1, 435, 436, 437,
		-1, -1, -1, -1, -1, 459, 438, 439,
		-1, -1, -1, -1, -1, -1, 460, 440,
		-1, -1, -1, -1, -1, -1, -1, 461,
	},
}

// binomial[k][n] = C(n, k+1)-style table built in init_indices(). de Man uses
// signed int; we keep the same arithmetic. binomial[i][j] is documented in
// tbcore as Bin(n, k) with binomial[k-1][n].
var binomial [5][64]int64
var pawnidx [5][24]int64
var pfactor [5][4]int64

func init() {
	initIndices()
}

// initIndices ports init_indices() from tbcore.c exactly (non-CONNECTED_KINGS).
func initIndices() {
	for i := 0; i < 5; i++ {
		for j := 0; j < 64; j++ {
			f := int64(j)
			l := int64(1)
			for k := 1; k <= i; k++ {
				f *= int64(j - k)
				l *= int64(k + 1)
			}
			binomial[i][j] = f / l
		}
	}

	for i := 0; i < 5; i++ {
		s := int64(0)
		j := 0
		for ; j < 6; j++ {
			pawnidx[i][j] = s
			if i == 0 {
				s += 1
			} else {
				s += binomial[i-1][ptwist[invflap[j]]]
			}
		}
		pfactor[i][0] = s
		s = 0
		for ; j < 12; j++ {
			pawnidx[i][j] = s
			if i == 0 {
				s += 1
			} else {
				s += binomial[i-1][ptwist[invflap[j]]]
			}
		}
		pfactor[i][1] = s
		s = 0
		for ; j < 18; j++ {
			pawnidx[i][j] = s
			if i == 0 {
				s += 1
			} else {
				s += binomial[i-1][ptwist[invflap[j]]]
			}
		}
		pfactor[i][2] = s
		s = 0
		for ; j < 24; j++ {
			pawnidx[i][j] = s
			if i == 0 {
				s += 1
			} else {
				s += binomial[i-1][ptwist[invflap[j]]]
			}
		}
		pfactor[i][3] = s
	}
}
