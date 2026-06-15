package syzygy

import (
	"encoding/binary"
	"fmt"
)

// This file ports the core of de Man's tbcore.c: the index encoders
// (encode_piece, encode_pawn, pawn_file), the factor/norm helpers, setup_pairs
// / calc_symlen, init_table_wdl and decompress_pairs. The non-CONNECTED_KINGS
// variant is used. We implement the 32-bit decompression path (DECOMP64 is
// disabled in the upstream source by default).

// encodePiece ports encode_piece (non-CONNECTED_KINGS). pos is the array of
// piece squares (0..63); it is mutated in place. encType, norm and factor come
// from the bucket; n is the number of pieces.
func encodePiece(encType, n int, norm []uint8, pos []int, factor []uint64) uint64 {
	var idx uint64

	if pos[0]&0x04 != 0 {
		for i := 0; i < n; i++ {
			pos[i] ^= 0x07
		}
	}
	if pos[0]&0x20 != 0 {
		for i := 0; i < n; i++ {
			pos[i] ^= 0x38
		}
	}

	var i int
	for i = 0; i < n; i++ {
		if offdiag[pos[i]] != 0 {
			break
		}
	}
	threshold := 2
	if encType == 0 {
		threshold = 3
	}
	if i < threshold && offdiag[pos[i]] > 0 {
		for i = 0; i < n; i++ {
			pos[i] = int(flipdiag[pos[i]])
		}
	}

	switch encType {
	case 0: // 111
		ii := b2i(pos[1] > pos[0])
		j := b2i(pos[2] > pos[0]) + b2i(pos[2] > pos[1])
		if offdiag[pos[0]] != 0 {
			idx = uint64(triangle[pos[0]])*63*62 + uint64(pos[1]-ii)*62 + uint64(pos[2]-j)
		} else if offdiag[pos[1]] != 0 {
			idx = 6*63*62 + uint64(diag[pos[0]])*28*62 + uint64(lower[pos[1]])*62 + uint64(pos[2]-j)
		} else if offdiag[pos[2]] != 0 {
			idx = 6*63*62 + 4*28*62 + uint64(diag[pos[0]])*7*28 + uint64(int(diag[pos[1]])-ii)*28 + uint64(lower[pos[2]])
		} else {
			idx = 6*63*62 + 4*28*62 + 4*7*28 + uint64(diag[pos[0]])*7*6 + uint64(int(diag[pos[1]])-ii)*6 + uint64(int(diag[pos[2]])-j)
		}
		i = 3
	case 1: // K3 (3 pieces, two kings encoded together is enc_type 1)
		j := b2i(pos[2] > pos[0]) + b2i(pos[2] > pos[1])
		idx = uint64(kkIdx[triangle[pos[0]]][pos[1]])
		if idx < 441 {
			idx = idx + 441*uint64(pos[2]-j)
		} else {
			idx = 441*62 + (idx - 441) + 21*uint64(lower[pos[2]])
			if offdiag[pos[2]] == 0 {
				idx -= uint64(j) * 21
			}
		}
		i = 3
	default: // K2
		idx = uint64(kkIdx[triangle[pos[0]]][pos[1]])
		i = 2
	}
	idx *= factor[0]

	for i < n {
		t := int(norm[i])
		for j := i; j < i+t; j++ {
			for k := j + 1; k < i+t; k++ {
				if pos[j] > pos[k] {
					pos[j], pos[k] = pos[k], pos[j]
				}
			}
		}
		s := 0
		for m := i; m < i+t; m++ {
			p := pos[m]
			j := 0
			for l := 0; l < i; l++ {
				j += b2i(p > pos[l])
			}
			s += int(binomial[m-i][p-j])
		}
		idx += uint64(s) * factor[i]
		i += t
	}
	return idx
}

// pawnFile ports pawn_file: finds the leftmost pawn's file and sorts pawns by
// flap. pawns0 is ptr->pawns[0].
func pawnFile(pawns0 int, pos []int) int {
	for i := 1; i < pawns0; i++ {
		if flap[pos[0]] > flap[pos[i]] {
			pos[0], pos[i] = pos[i], pos[0]
		}
	}
	return int(fileToFile[pos[0]&0x07])
}

// encodePawn ports encode_pawn (non-CONNECTED_KINGS). pawns0/pawns1 are
// ptr->pawns[0]/[1].
func encodePawn(n, pawns0, pawns1 int, norm []uint8, pos []int, factor []uint64) uint64 {
	var idx uint64

	if pos[0]&0x04 != 0 {
		for i := 0; i < n; i++ {
			pos[i] ^= 0x07
		}
	}

	for i := 1; i < pawns0; i++ {
		for j := i + 1; j < pawns0; j++ {
			if ptwist[pos[i]] < ptwist[pos[j]] {
				pos[i], pos[j] = pos[j], pos[i]
			}
		}
	}

	t := pawns0 - 1
	idx = uint64(pawnidx[t][flap[pos[0]]])
	for i := t; i > 0; i-- {
		idx += uint64(binomial[t-i][ptwist[pos[i]]])
	}
	idx *= factor[0]

	// Remaining pawns.
	i := pawns0
	t = i + pawns1
	if t > i {
		for j := i; j < t; j++ {
			for k := j + 1; k < t; k++ {
				if pos[j] > pos[k] {
					pos[j], pos[k] = pos[k], pos[j]
				}
			}
		}
		s := 0
		for m := i; m < t; m++ {
			p := pos[m]
			j := 0
			for k := 0; k < i; k++ {
				j += b2i(p > pos[k])
			}
			s += int(binomial[m-i][p-j-8])
		}
		idx += uint64(s) * factor[i]
		i = t
	}

	for i < n {
		t = int(norm[i])
		for j := i; j < i+t; j++ {
			for k := j + 1; k < i+t; k++ {
				if pos[j] > pos[k] {
					pos[j], pos[k] = pos[k], pos[j]
				}
			}
		}
		s := 0
		for m := i; m < i+t; m++ {
			p := pos[m]
			j := 0
			for k := 0; k < i; k++ {
				j += b2i(p > pos[k])
			}
			s += int(binomial[m-i][p-j])
		}
		idx += uint64(s) * factor[i]
		i += t
	}
	return idx
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// subfactor ports subfactor: place k like pieces on n squares.
func subfactor(k, n int) uint64 {
	f := uint64(n)
	l := uint64(1)
	for i := 1; i < k; i++ {
		f *= uint64(n - i)
		l *= uint64(i + 1)
	}
	return f / l
}

// pivfac is the non-CONNECTED_KINGS pivot factor table from calc_factors_piece.
var pivfac = [3]uint64{31332, 28056, 462}

// calcFactorsPiece ports calc_factors_piece.
func calcFactorsPiece(factor []uint64, num, order int, norm []uint8, encType int) uint64 {
	n := 64 - int(norm[0])
	f := uint64(1)
	i := int(norm[0])
	for k := 0; i < num || k == order; k++ {
		if k == order {
			factor[0] = f
			f *= pivfac[encType]
		} else {
			factor[i] = f
			f *= subfactor(int(norm[i]), n)
			n -= int(norm[i])
			i += int(norm[i])
		}
	}
	return f
}

// calcFactorsPawn ports calc_factors_pawn.
func calcFactorsPawn(factor []uint64, num, order, order2 int, norm []uint8, file int) uint64 {
	i := int(norm[0])
	if order2 < 0x0f {
		i += int(norm[i])
	}
	n := 64 - i
	f := uint64(1)
	for k := 0; i < num || k == order || k == order2; k++ {
		if k == order {
			factor[0] = f
			f *= uint64(pfactor[int(norm[0])-1][file])
		} else if k == order2 {
			factor[int(norm[0])] = f
			f *= subfactor(int(norm[int(norm[0])]), 48-int(norm[0]))
		} else {
			factor[i] = f
			f *= subfactor(int(norm[i]), n)
			n -= int(norm[i])
			i += int(norm[i])
		}
	}
	return f
}

// setNormPiece ports set_norm_piece.
func setNormPiece(num, encType int, norm, pieces []uint8) {
	for i := 0; i < num; i++ {
		norm[i] = 0
	}
	switch encType {
	case 0:
		norm[0] = 3
	case 2:
		norm[0] = 2
	default:
		norm[0] = uint8(encType - 1)
	}
	for i := int(norm[0]); i < num; i += int(norm[i]) {
		for j := i; j < num && pieces[j] == pieces[i]; j++ {
			norm[i]++
		}
	}
}

// setNormPawn ports set_norm_pawn.
func setNormPawn(num int, pawns [2]uint8, norm, pieces []uint8) {
	for i := 0; i < num; i++ {
		norm[i] = 0
	}
	norm[0] = pawns[0]
	if pawns[1] != 0 {
		norm[pawns[0]] = pawns[1]
	}
	for i := int(pawns[0]) + int(pawns[1]); i < num; i += int(norm[i]) {
		for j := i; j < num && pieces[j] == pieces[i]; j++ {
			norm[i]++
		}
	}
}

// setupPiecesPiece ports setup_pieces_piece. data is the slice starting at the
// piece-list header byte. Returns tb_size[0], tb_size[1].
func (t *tbTable) setupPiecesPiece(data []byte) (uint64, uint64) {
	b0 := t.buckets[0][0]
	for i := 0; i < t.num; i++ {
		b0.pieces[i] = data[i+1] & 0x0f
	}
	order := int(data[0] & 0x0f)
	setNormPiece(t.num, t.encType, b0.norm[:], b0.pieces[:])
	size0 := calcFactorsPiece(b0.factor[:], t.num, order, b0.norm[:], t.encType)

	b1 := t.buckets[0][1]
	for i := 0; i < t.num; i++ {
		b1.pieces[i] = data[i+1] >> 4
	}
	order = int(data[0] >> 4)
	setNormPiece(t.num, t.encType, b1.norm[:], b1.pieces[:])
	size1 := calcFactorsPiece(b1.factor[:], t.num, order, b1.norm[:], t.encType)

	return size0, size1
}

// setupPiecesPawn ports setup_pieces_pawn for file f. data starts at the
// per-file header. Returns tb_size[0], tb_size[1].
func (t *tbTable) setupPiecesPawn(data []byte, f int) (uint64, uint64) {
	j := 1
	if t.pawns[1] > 0 {
		j = 2
	}
	b0 := t.buckets[f][0]
	order := int(data[0] & 0x0f)
	order2 := 0x0f
	if t.pawns[1] != 0 {
		order2 = int(data[1] & 0x0f)
	}
	for i := 0; i < t.num; i++ {
		b0.pieces[i] = data[i+j] & 0x0f
	}
	setNormPawn(t.num, t.pawns, b0.norm[:], b0.pieces[:])
	size0 := calcFactorsPawn(b0.factor[:], t.num, order, order2, b0.norm[:], f)

	b1 := t.buckets[f][1]
	order = int(data[0] >> 4)
	order2 = 0x0f
	if t.pawns[1] != 0 {
		order2 = int(data[1] >> 4)
	}
	for i := 0; i < t.num; i++ {
		b1.pieces[i] = data[i+j] >> 4
	}
	setNormPawn(t.num, t.pawns, b1.norm[:], b1.pieces[:])
	size1 := calcFactorsPawn(b1.factor[:], t.num, order, order2, b1.norm[:], f)

	return size0, size1
}

// calcSymLen ports calc_symlen.
func calcSymLen(pd *pairsData, s int, tmp []byte) {
	w := pd.sympat[3*s : 3*s+3]
	// w is a little-endian 24-bit value; in C: int w = *(int*)(sympat+3*s);
	// s2 = (w >> 12) & 0xfff; s1 = w & 0xfff.
	wv := uint32(w[0]) | uint32(w[1])<<8 | uint32(w[2])<<16
	s2 := (wv >> 12) & 0x0fff
	if s2 == 0x0fff {
		pd.symlen[s] = 0
	} else {
		s1 := wv & 0x0fff
		if tmp[s1] == 0 {
			calcSymLen(pd, int(s1), tmp)
		}
		if tmp[s2] == 0 {
			calcSymLen(pd, int(s2), tmp)
		}
		pd.symlen[s] = pd.symlen[s1] + pd.symlen[s2] + 1
	}
	tmp[s] = 1
}

// setupPairs ports setup_pairs (wdl=1). raw is the whole file; off is the
// offset of the PairsData header. tbSize is the table size for this bucket.
// Returns the pairsData, the size[3] array, and the offset just past the header
// (the C "next").
func setupPairs(raw []byte, off int, tbSize uint64) (pd *pairsData, size [3]uint64, next int, flags uint8, err error) {
	if off >= len(raw) {
		err = fmt.Errorf("syzygy: pairs header out of range")
		return
	}
	pd = &pairsData{}
	flags = raw[off]
	if raw[off]&0x80 != 0 {
		pd.idxbits = 0
		pd.minLen = int32(raw[off+1]) // wdl=1 path
		pd.isConst = true
		pd.constValue = raw[off+1]
		next = off + 2
		return
	}

	blocksize := uint32(raw[off+1])
	idxbits := uint32(raw[off+2])
	realNumBlocks := le32(raw[off+4:])
	numBlocks := realNumBlocks + uint32(raw[off+3])
	maxLen := int(raw[off+8])
	minLen := int(raw[off+9])
	h := maxLen - minLen + 1
	numSyms := int(le16(raw[off+10+2*h:]))

	pd.blocksize = blocksize
	pd.idxbits = idxbits
	pd.minLen = int32(minLen)

	// offset[] points at &data[10]; it has h entries.
	pd.offset = make([]uint16, h)
	for i := 0; i < h; i++ {
		pd.offset[i] = uint16(le16(raw[off+10+2*i:]))
	}

	pd.symlen = make([]uint8, numSyms)
	pd.sympat = raw[off+12+2*h : off+12+2*h+3*numSyms]

	next = off + 12 + 2*h + 3*numSyms + (numSyms & 1)

	numIndices := (tbSize + (uint64(1) << idxbits) - 1) >> idxbits
	size[0] = 6 * numIndices
	size[1] = 2 * uint64(numBlocks)
	size[2] = (uint64(1) << blocksize) * uint64(realNumBlocks)

	tmp := make([]byte, numSyms)
	for i := 0; i < numSyms; i++ {
		if tmp[i] == 0 {
			calcSymLen(pd, i, tmp)
		}
	}

	// base[] computation (32-bit DECOMP path).
	pd.base = make([]uint64, h)
	pd.base[h-1] = 0
	for i := h - 2; i >= 0; i-- {
		pd.base[i] = (pd.base[i+1] + uint64(pd.offset[i]) - uint64(pd.offset[i+1])) / 2
	}
	for i := 0; i < h; i++ {
		pd.base[i] <<= uint(32 - (minLen + i))
	}
	// In C, d->offset -= d->min_len; we instead index offset[l-minLen] at use.

	return
}

// align2 rounds up to a 2-byte boundary relative to the file start. The C code
// uses ((uintptr_t)data)&0x01 on the mmap pointer; since our base is the file
// start (alignment 0 at offset 0), the parity is just (off & 1).
func align2(off int) int { return off + (off & 1) }

// align64 rounds up to a 64-byte boundary.
func align64(off int) int { return (off + 0x3f) &^ 0x3f }

// initTableWDL ports init_table_wdl. It parses the already-loaded raw bytes and
// fills the buckets with pairsData (index/size/data slices).
func (t *tbTable) initTableWDL() error {
	raw := t.raw
	if len(raw) < 5 {
		return fmt.Errorf("syzygy: file too small")
	}
	if raw[0] != wdlMagic[0] || raw[1] != wdlMagic[1] || raw[2] != wdlMagic[2] || raw[3] != wdlMagic[3] {
		return fmt.Errorf("syzygy: bad magic in %s", t.name)
	}

	t.split = raw[4]&0x01 != 0
	t.files = 1
	if raw[4]&0x02 != 0 {
		t.files = 4
	}

	data := 5

	if !t.hasPawns {
		// Allocate the two buckets.
		t.buckets[0][0] = &bucket{}
		t.buckets[0][1] = &bucket{}

		size0, size1 := t.setupPiecesPiece(raw[data:])
		data += t.num + 1
		data = align2(data)

		var sz0, sz1 [3]uint64
		pd0, s0, next, _, err := setupPairs(raw, data, size0)
		if err != nil {
			return err
		}
		sz0 = s0
		t.buckets[0][0].precomp = pd0
		data = next

		if t.split {
			pd1, s1, next2, _, err := setupPairs(raw, data, size1)
			if err != nil {
				return err
			}
			sz1 = s1
			t.buckets[0][1].precomp = pd1
			data = next2
		} else {
			t.buckets[0][1] = t.buckets[0][0]
		}

		pd0 = t.buckets[0][0].precomp
		pd0.indextable = raw[data:]
		data += int(sz0[0])
		if t.split {
			t.buckets[0][1].precomp.indextable = raw[data:]
			data += int(sz1[0])
		}

		pd0.sizetable = bytesToU16(raw[data : data+int(sz0[1])])
		data += int(sz0[1])
		if t.split {
			t.buckets[0][1].precomp.sizetable = bytesToU16(raw[data : data+int(sz1[1])])
			data += int(sz1[1])
		}

		data = align64(data)
		pd0.data = raw[data:]
		data += int(sz0[2])
		if t.split {
			data = align64(data)
			t.buckets[0][1].precomp.data = raw[data:]
		}
		return nil
	}

	// Pawn table.
	s := 1
	if t.pawns[1] > 0 {
		s = 2
	}
	var tbSize [4][2]uint64
	for f := 0; f < 4; f++ {
		t.buckets[f][0] = &bucket{}
		t.buckets[f][1] = &bucket{}
		size0, size1 := t.setupPiecesPawn(raw[data:], f)
		tbSize[f][0] = size0
		tbSize[f][1] = size1
		data += t.num + s
	}
	data = align2(data)

	var sz [4][2][3]uint64
	for f := 0; f < t.files; f++ {
		pd0, s0, next, _, err := setupPairs(raw, data, tbSize[f][0])
		if err != nil {
			return err
		}
		sz[f][0] = s0
		t.buckets[f][0].precomp = pd0
		data = next
		if t.split {
			pd1, s1, next2, _, err := setupPairs(raw, data, tbSize[f][1])
			if err != nil {
				return err
			}
			sz[f][1] = s1
			t.buckets[f][1].precomp = pd1
			data = next2
		} else {
			t.buckets[f][1] = t.buckets[f][0]
		}
	}

	for f := 0; f < t.files; f++ {
		t.buckets[f][0].precomp.indextable = raw[data:]
		data += int(sz[f][0][0])
		if t.split {
			t.buckets[f][1].precomp.indextable = raw[data:]
			data += int(sz[f][1][0])
		}
	}

	for f := 0; f < t.files; f++ {
		t.buckets[f][0].precomp.sizetable = bytesToU16(raw[data : data+int(sz[f][0][1])])
		data += int(sz[f][0][1])
		if t.split {
			t.buckets[f][1].precomp.sizetable = bytesToU16(raw[data : data+int(sz[f][1][1])])
			data += int(sz[f][1][1])
		}
	}

	for f := 0; f < t.files; f++ {
		data = align64(data)
		t.buckets[f][0].precomp.data = raw[data:]
		data += int(sz[f][0][2])
		if t.split {
			data = align64(data)
			t.buckets[f][1].precomp.data = raw[data:]
			data += int(sz[f][1][2])
		}
	}
	return nil
}

// bytesToU16 reinterprets a byte slice as a little-endian uint16 slice (copy).
func bytesToU16(b []byte) []uint16 {
	out := make([]uint16, len(b)/2)
	for i := range out {
		out[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	return out
}

// decompressPairs ports decompress_pairs (32-bit path). Returns the raw stored
// byte (the WDL value is this minus 2, applied by the caller).
func decompressPairs(d *pairsData, idx uint64) uint8 {
	if d.idxbits == 0 {
		return uint8(d.minLen)
	}

	mainidx := idx >> d.idxbits
	litidx := int64(idx&((uint64(1)<<d.idxbits)-1)) - int64(uint64(1)<<(d.idxbits-1))

	// indextable: 6 bytes per main index: uint32 block + uint16 offset.
	it := d.indextable[6*mainidx:]
	block := binary.LittleEndian.Uint32(it[0:4])
	litidx += int64(binary.LittleEndian.Uint16(it[4:6]))

	if litidx < 0 {
		for litidx < 0 {
			block--
			litidx += int64(d.sizetable[block]) + 1
		}
	} else {
		for litidx > int64(d.sizetable[block]) {
			litidx -= int64(d.sizetable[block]) + 1
			block++
		}
	}

	// Pointer into the compressed block data, 32-bit words read big-endian.
	ptr := d.data[block<<d.blocksize:]
	pos := 0

	m := int(d.minLen)
	base := d.base // base[l-minLen]
	offset := d.offset
	symlen := d.symlen

	var next uint32
	code := bswap32(binary.LittleEndian.Uint32(ptr[pos:]))
	pos += 4
	bitcnt := 0
	var sym int
	for {
		l := m
		for code < uint32(base[l-m]) {
			l++
		}
		sym = int(offset[l-m]) + int((code-uint32(base[l-m]))>>(32-l))
		if litidx < int64(symlen[sym])+1 {
			break
		}
		litidx -= int64(symlen[sym]) + 1
		code <<= uint(l)
		if bitcnt < l {
			if bitcnt != 0 {
				code |= next >> uint(32-l)
				l -= bitcnt
			}
			data := binary.LittleEndian.Uint32(ptr[pos:])
			pos += 4
			next = bswap32(data)
			bitcnt = 32
		}
		code |= next >> uint(32-l)
		next <<= uint(l)
		bitcnt -= l
	}

	sympat := d.sympat
	for symlen[sym] != 0 {
		w := sympat[3*sym : 3*sym+3]
		wv := uint32(w[0]) | uint32(w[1])<<8 | uint32(w[2])<<16
		s1 := int(wv & 0x0fff)
		if litidx < int64(symlen[s1])+1 {
			sym = s1
		} else {
			litidx -= int64(symlen[s1]) + 1
			sym = int((wv >> 12) & 0x0fff)
		}
	}

	return sympat[3*sym]
}

func bswap32(x uint32) uint32 {
	return (x >> 24) | ((x >> 8) & 0x0000ff00) | ((x << 8) & 0x00ff0000) | (x << 24)
}
