package nnue

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// Save writes the network to path in the same byte layout that Load reads. See
// the package documentation for the format. The header uses the canonical
// Version word, the architecture-correct hash, and the supplied architecture
// description string (or a default if empty).
func (n *Network) Save(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := n.Serialize(f); err != nil {
		return err
	}
	return f.Close()
}

// Serialize writes the network to w. It is the inverse of ReadNetwork and is
// used both by Save and by tests that round-trip a hand-made network.
func (n *Network) Serialize(w io.Writer) error {
	bw := &byteWriter{w: w}

	arch := n.Architecture
	if arch == "" {
		arch = "HalfKP_256x2-32-32"
	}

	// Network header.
	bw.uint32(Version)
	bw.uint32(ExpectedNetworkHash())
	bw.uint32(uint32(len(arch)))
	bw.bytes([]byte(arch))

	// Feature transformer.
	bw.uint32(featureTransformerHash())
	for _, v := range n.ftBiases {
		bw.int16(v)
	}
	if len(n.ftWeights) != FeatureDimensions*TransformedFeatureDimensions {
		return fmt.Errorf("nnue: ftWeights has length %d, want %d",
			len(n.ftWeights), FeatureDimensions*TransformedFeatureDimensions)
	}
	bw.int16Slice(n.ftWeights)

	// Dense network.
	bw.uint32(networkHash())
	writeAffine(bw, n.l1Biases[:], n.l1Weights[:])
	writeAffine(bw, n.l2Biases[:], n.l2Weights[:])
	writeAffine(bw, []int32{n.outBias}, n.outWeights[:])

	return bw.err
}

func writeAffine(bw *byteWriter, biases []int32, weights []int8) {
	for _, v := range biases {
		bw.int32(v)
	}
	bw.int8Slice(weights)
}

// byteWriter is a small little-endian writer mirroring byteReader. It records
// the first error and becomes a no-op afterwards.
type byteWriter struct {
	w   io.Writer
	err error
	buf [4]byte
}

func (b *byteWriter) write(p []byte) {
	if b.err != nil {
		return
	}
	_, b.err = b.w.Write(p)
}

func (b *byteWriter) uint32(v uint32) {
	binary.LittleEndian.PutUint32(b.buf[:4], v)
	b.write(b.buf[:4])
}

func (b *byteWriter) int32(v int32) { b.uint32(uint32(v)) }

func (b *byteWriter) int16(v int16) {
	binary.LittleEndian.PutUint16(b.buf[:2], uint16(v))
	b.write(b.buf[:2])
}

func (b *byteWriter) bytes(p []byte) { b.write(p) }

func (b *byteWriter) int16Slice(src []int16) {
	if b.err != nil {
		return
	}
	raw := make([]byte, len(src)*2)
	for i, v := range src {
		binary.LittleEndian.PutUint16(raw[i*2:], uint16(v))
	}
	b.write(raw)
}

func (b *byteWriter) int8Slice(src []int8) {
	if b.err != nil {
		return
	}
	raw := make([]byte, len(src))
	for i, v := range src {
		raw[i] = byte(v)
	}
	b.write(raw)
}

// NewEmptyNetwork allocates a Network with zeroed parameters and a correctly
// sized feature-transformer weight slice. It is primarily a building block for
// tests that construct a tiny hand-made network.
func NewEmptyNetwork() *Network {
	return &Network{
		ftWeights: make([]int16, FeatureDimensions*TransformedFeatureDimensions),
		HashOK:    true,
	}
}
