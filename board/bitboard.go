package board

type Bitboard uint64

func (b *Bitboard) Set(pos uint) {
	*b |= Bitboard(uint64(1) << pos)
}
