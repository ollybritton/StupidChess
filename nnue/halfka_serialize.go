package nnue

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// Architecture peeks at a network file's header hash and reports which architecture it is: "halfka" for
// the modern HalfKAv2_hm net, "halfkp" for the classic one, or "unknown". It reads only the 8-byte
// version+hash prefix, so the engine can cheaply pick the right loader for an EvalFile.
func Architecture(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var hdr [8]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		return "", err
	}
	switch binary.LittleEndian.Uint32(hdr[4:8]) {
	case ExpectedKANetworkHash():
		return "halfka", nil
	case ExpectedNetworkHash():
		return "halfkp", nil
	default:
		return "unknown", nil
	}
}

// This file is the loader for the HalfKAv2_hm "big" net. It reads the serialized
// layout described in evaluate_nnue.cpp (read_header / read_parameters),
// nnue_feature_transformer.h (FT read_parameters) and affine_transform.h
// (the per-stack affine layers). Everything is little-endian. It reuses the
// byteReader defined in nnue.go.
//
// Serialized layout (after the header):
//
//	uint32 version (==KAVersion), uint32 hashValue, uint32 descLen, desc[descLen]
//	-- feature transformer --
//	uint32 ftHash
//	int16  ftBiases[1024]
//	int16  ftWeights[1024 * 22528]                 (layout [index*1024 + j])
//	int32  psqtWeights[22528 * 8]                  (layout [index*8 + bucket])
//	-- 8 layer stacks, each: --
//	  uint32 stackHash  (the whole Network::get_hash_value, ONE word per stack;
//	                     the sub-layers fc_0/ac_0/fc_1/ac_1/fc_2 are NOT each
//	                     wrapped, because Network::read_parameters calls their
//	                     read_parameters directly, not via Detail::read_parameters)
//	  int32 fc0Bias[16];  int8 fc0Weights[16*1024]
//	  int32 fc1Bias[32];  int8 fc1Weights[32*32]
//	  int32 fc2Bias[1];   int8 fc2Weights[1*32]
//
// Affine weight order on disk: see kaAffineScramble. The oracle build here is
// NEON (no USE_SSSE3), so AffineTransform::get_weight_index(i) == i and the
// distributed net's affine weights are in plain row-major [out*paddedIn + in]
// order; we load them verbatim. (If a net produced by an SSSE3+ build is ever
// loaded, set kaAffineScramble = true to invert get_weight_index_scrambled.)

// kaAffineScramble selects how affine weights are de-permuted on read. false =
// plain row-major (the order the NEON oracle and the distributed net use); true =
// invert get_weight_index_scrambled (the SSSE3/AVX2 on-disk order). The loader is
// verified against the NEON oracle with this false.
const kaAffineScramble = false

// LoadKA reads a serialized HalfKAv2_hm network from path.
func LoadKA(path string) (*KANetwork, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadKANetwork(f)
}

// ReadKANetwork reads a serialized HalfKAv2_hm network from r. The version word
// is enforced; a header-hash mismatch is recorded in HashOK but is not fatal.
func ReadKANetwork(r io.Reader) (*KANetwork, error) {
	br := newByteReader(r)
	n := &KANetwork{}

	// --- Network header ---
	version, err := br.uint32()
	if err != nil {
		return nil, fmt.Errorf("nnue/ka: reading version: %w", err)
	}
	if version != KAVersion {
		return nil, fmt.Errorf("nnue/ka: bad version 0x%08X, want 0x%08X", version, KAVersion)
	}
	fileHash, err := br.uint32()
	if err != nil {
		return nil, fmt.Errorf("nnue/ka: reading hash: %w", err)
	}
	descLen, err := br.uint32()
	if err != nil {
		return nil, fmt.Errorf("nnue/ka: reading desc length: %w", err)
	}
	if descLen > 1<<20 {
		return nil, fmt.Errorf("nnue/ka: implausible description length %d", descLen)
	}
	desc := make([]byte, descLen)
	if _, err := io.ReadFull(br, desc); err != nil {
		return nil, fmt.Errorf("nnue/ka: reading description: %w", err)
	}
	n.Architecture = string(desc)
	n.HashOK = fileHash == ExpectedKANetworkHash()

	// --- Feature transformer ---
	if _, err := br.uint32(); err != nil { // ftHash, verified loosely via HashOK
		return nil, fmt.Errorf("nnue/ka: reading FT hash: %w", err)
	}
	for i := 0; i < KAHalfDims; i++ {
		v, err := br.int16()
		if err != nil {
			return nil, fmt.Errorf("nnue/ka: reading FT bias %d: %w", i, err)
		}
		n.ftBiases[i] = v
	}
	n.ftWeights = make([]int16, KAFeatureDims*KAHalfDims)
	if err := br.int16Slice(n.ftWeights); err != nil {
		return nil, fmt.Errorf("nnue/ka: reading FT weights: %w", err)
	}
	n.psqtWeights = make([]int32, KAFeatureDims*KAPSQTBuckets)
	if err := br.int32Slice(n.psqtWeights); err != nil {
		return nil, fmt.Errorf("nnue/ka: reading FT PSQT weights: %w", err)
	}

	// --- 8 layer stacks ---
	// Each stack is preceded by exactly one hash word (Network::get_hash_value);
	// its sub-layers (fc_0, ac_0, fc_1, ac_1, fc_2) carry no individual hash
	// words, and the two ClippedReLUs carry no data either.
	for s := 0; s < KALayerStacks; s++ {
		stack := &n.stacks[s]

		if _, err := br.uint32(); err != nil { // stack (Network) hash
			return nil, fmt.Errorf("nnue/ka: stack %d hash: %w", s, err)
		}

		// fc_0: 1024 -> 16
		if err := readKAAffine(br, stack.fc0Bias[:], stack.fc0Weights[:], kaFC0InputsPadded, kaFC0Total); err != nil {
			return nil, fmt.Errorf("nnue/ka: stack %d fc0: %w", s, err)
		}
		// fc_1: 30 -> 32 (padded input 32)
		if err := readKAAffine(br, stack.fc1Bias[:], stack.fc1Weights[:], kaFC1InputsPadded, kaFC1Outputs); err != nil {
			return nil, fmt.Errorf("nnue/ka: stack %d fc1: %w", s, err)
		}
		// fc_2: 32 -> 1
		fc2Bias := []int32{0}
		if err := readKAAffine(br, fc2Bias, stack.fc2Weights[:], kaFC2InputsPadded, 1); err != nil {
			return nil, fmt.Errorf("nnue/ka: stack %d fc2: %w", s, err)
		}
		stack.fc2Bias = fc2Bias[0]
	}

	return n, nil
}

// readKAAffine reads one affine layer: int32 biases (one per output) then
// outDims*paddedIn int8 weights. The weights are placed into the plain
// [out*paddedIn + in] layout, de-scrambling on the way if kaAffineScramble.
func readKAAffine(br *byteReader, biases []int32, weights []int8, paddedIn, outDims int) error {
	for i := range biases {
		v, err := br.int32()
		if err != nil {
			return fmt.Errorf("bias %d: %w", i, err)
		}
		biases[i] = v
	}
	total := outDims * paddedIn
	if len(weights) != total {
		return fmt.Errorf("weight buffer len %d, want %d", len(weights), total)
	}
	raw := make([]int8, total)
	if err := br.int8Slice(raw); err != nil {
		return err
	}
	if !kaAffineScramble {
		copy(weights, raw)
		return nil
	}
	for i := 0; i < total; i++ {
		weights[scrambledWeightIndex(i, paddedIn, outDims)] = raw[i]
	}
	return nil
}

// scrambledWeightIndex mirrors AffineTransform::get_weight_index_scrambled:
// the logical [out*paddedIn + in] slot that the i-th streamed weight belongs to
// when the file was written by an SSSE3+ build.
func scrambledWeightIndex(i, paddedIn, outDims int) int {
	return (i/4)%(paddedIn/4)*outDims*4 + (i/paddedIn)*4 + i%4
}

// ---------------------------------------------------------------------------
// Hash computation (validation only, non-fatal; mirrors nnue_architecture.h)
// ---------------------------------------------------------------------------

// kaFeatureTransformerHash is FeatureTransformer::get_hash_value():
// FeatureSet::HashValue ^ (OutputDimensions*2) = 0x7f234cb8 ^ 2048.
func kaFeatureTransformerHash() uint32 {
	return kaFeatureHash ^ (KAHalfDims * 2)
}

func kaAffineHash(outDims, prev uint32) uint32 {
	h := uint32(0xCC03DAE4)
	h += outDims
	h ^= prev >> 1
	h ^= prev << 31
	return h
}

func kaClippedReLUHash(prev uint32) uint32 {
	return 0x538D24C7 + prev
}

// kaLayerStackHash mirrors Network::get_hash_value() for one stack: input slice
// then fc_0, ac_0, fc_1, ac_1, fc_2 (the same chain used for the file header).
func kaLayerStackHash() uint32 {
	h := uint32(0xEC42E90D)
	h ^= KAHalfDims * 2               // input slice
	h = kaAffineHash(kaFC0Total, h)   // fc_0 (16 outputs)
	h = kaClippedReLUHash(h)          // ac_0
	h = kaAffineHash(kaFC1Outputs, h) // fc_1 (32 outputs)
	h = kaClippedReLUHash(h)          // ac_1
	h = kaAffineHash(1, h)            // fc_2 (1 output)
	return h
}

// ExpectedKANetworkHash returns the architecture hash this package expects in the
// file header for HalfKAv2_hm (1024x2, 8 stacks). It is the FT hash XOR-combined
// with the per-stack network hash, matching how Stockfish derives HashValue.
func ExpectedKANetworkHash() uint32 {
	return kaFeatureTransformerHash() ^ kaLayerStackHash()
}
