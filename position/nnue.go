package position

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// nnue.go is a small neural-network evaluation. Unlike the hand-crafted evaluation it learns its
// weights from game results (see cmd gendata / cmd nnuetrain). The input is the classic "piece on
// square" feature set (12 piece types x 64 squares = 768 sparse binary features), fed through one
// clipped-ReLU hidden layer to a single output in centipawns (White's point of view).
//
// The forward pass is recomputed each call rather than incrementally updated, but it is cheap because
// the input is sparse (only the ~32 occupied squares contribute). It is reentrant (a stack-local
// accumulator), so it is safe to use from the parallel search.

// nnueMaxHidden bounds the hidden layer so the per-call accumulator can live on the stack.
const nnueMaxHidden = 256

// nnueInputs is the number of input features (12 coloured piece types x 64 squares).
const nnueInputs = 12 * 64

// nnueMagic identifies a serialized network file.
const nnueMagic uint32 = 0x4E4E5531 // "NNU1"

// Network is a trained evaluation network.
type Network struct {
	Hidden int
	W1     []float32 // [nnueInputs*Hidden], indexed W1[feature*Hidden + j]
	B1     []float32 // [Hidden]
	W2     []float32 // [Hidden]
	B2     float32
}

// featureIndex maps a coloured piece on a square to its input feature.
func featureIndex(piece ColoredPiece, square int) int {
	return int(piece)*64 + square
}

// Eval returns the network's evaluation of the position in centipawns, from White's point of view.
func (n *Network) Eval(pos *Position) int16 {
	var acc [nnueMaxHidden]float32
	h := n.Hidden
	copy(acc[:h], n.B1)

	for sq := 0; sq < 64; sq++ {
		piece := pos.Squares[sq]
		if piece == Empty {
			continue
		}
		base := featureIndex(piece, sq) * h
		w := n.W1[base : base+h]
		for j := 0; j < h; j++ {
			acc[j] += w[j]
		}
	}

	out := n.B2
	for j := 0; j < h; j++ {
		a := acc[j]
		if a < 0 { // clipped ReLU, range [0, 1]
			a = 0
		} else if a > 1 {
			a = 1
		}
		out += a * n.W2[j]
	}

	if out > evalLimit {
		out = evalLimit
	} else if out < -evalLimit {
		out = -evalLimit
	}
	return int16(out)
}

// Evaluator adapts the network to the Evaluator interface, so it can drive a search engine.
func (n *Network) Evaluator() Evaluator {
	return n.Eval
}

// ActiveFeatures returns the input feature indices that are "on" for a position (one per occupied
// square). The trainer uses this so its feature mapping matches Eval exactly.
func ActiveFeatures(pos *Position) []int {
	features := make([]int, 0, 32)
	for sq := 0; sq < 64; sq++ {
		if piece := pos.Squares[sq]; piece != Empty {
			features = append(features, featureIndex(piece, sq))
		}
	}
	return features
}

// NNUEInputs is the network's input feature count (exported for the trainer).
const NNUEInputs = nnueInputs

// NewRandomNetwork builds a small network with weights ready to be trained (zeroed; the trainer
// initialises them).
func NewRandomNetwork(hidden int) *Network {
	if hidden > nnueMaxHidden {
		hidden = nnueMaxHidden
	}
	return &Network{
		Hidden: hidden,
		W1:     make([]float32, nnueInputs*hidden),
		B1:     make([]float32, hidden),
		W2:     make([]float32, hidden),
	}
}

// Save writes the network in a compact binary format.
func (n *Network) Save(w io.Writer) error {
	if err := binary.Write(w, binary.LittleEndian, nnueMagic); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, int32(n.Hidden)); err != nil {
		return err
	}
	for _, slice := range [][]float32{n.W1, n.B1, n.W2} {
		if err := binary.Write(w, binary.LittleEndian, slice); err != nil {
			return err
		}
	}
	return binary.Write(w, binary.LittleEndian, n.B2)
}

// LoadNetwork reads a network written by Save.
func LoadNetwork(r io.Reader) (*Network, error) {
	var magic uint32
	if err := binary.Read(r, binary.LittleEndian, &magic); err != nil {
		return nil, err
	}
	if magic != nnueMagic {
		return nil, fmt.Errorf("not an nnue network (bad magic %x)", magic)
	}

	var hidden int32
	if err := binary.Read(r, binary.LittleEndian, &hidden); err != nil {
		return nil, err
	}
	if hidden < 1 || hidden > nnueMaxHidden {
		return nil, fmt.Errorf("invalid hidden size %d", hidden)
	}

	n := NewRandomNetwork(int(hidden))
	for _, slice := range [][]float32{n.W1, n.B1, n.W2} {
		if err := binary.Read(r, binary.LittleEndian, slice); err != nil {
			return nil, err
		}
	}
	if err := binary.Read(r, binary.LittleEndian, &n.B2); err != nil {
		return nil, err
	}
	return n, nil
}

// WinProbScale converts a centipawn output to a win probability, used by training and matching the
// logistic shape evaluations are usually fitted with.
const WinProbScale = 1.0 / 200.0

// Sigmoid is the logistic function (exposed for the trainer).
func Sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}
