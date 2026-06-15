package cmd

import (
	"bufio"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ollybritton/StupidChess/position"
	"github.com/spf13/cobra"
)

// nnuetrainCmd trains an NNUE network on the data produced by gendata, by gradient descent on the
// logistic loss between the network's win-probability prediction and the recorded game result.
var nnuetrainCmd = &cobra.Command{
	Use:   "nnuetrain",
	Short: "train an NNUE network on gendata output",
	Run: func(cmd *cobra.Command, args []string) {
		dataPath, _ := cmd.Flags().GetString("data")
		outPath, _ := cmd.Flags().GetString("out")
		hidden, _ := cmd.Flags().GetInt("hidden")
		epochs, _ := cmd.Flags().GetInt("epochs")
		lr, _ := cmd.Flags().GetFloat64("lr")
		maxSamples, _ := cmd.Flags().GetInt("max")

		samples, err := loadSamples(dataPath, maxSamples)
		if err != nil {
			fmt.Println("could not load data:", err)
			os.Exit(1)
		}
		if len(samples) == 0 {
			fmt.Println("no usable samples")
			os.Exit(1)
		}
		fmt.Printf("loaded %d samples; training a %d-hidden network for %d epochs\n", len(samples), hidden, epochs)

		net := trainNetwork(samples, hidden, epochs, lr)

		f, err := os.Create(outPath)
		if err != nil {
			fmt.Println("could not write network:", err)
			os.Exit(1)
		}
		defer f.Close()
		if err := net.Save(f); err != nil {
			fmt.Println("could not save network:", err)
			os.Exit(1)
		}
		fmt.Printf("wrote network to %s\n", outPath)
	},
}

// sample is one training position: the indices of its active input features and the game result it led
// to (1 white win, 0.5 draw, 0 black win).
type sample struct {
	features []int32
	target   float64
}

func loadSamples(path string, max int) ([]sample, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var samples []sample
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		semi := strings.LastIndexByte(line, ';')
		if semi < 0 {
			continue
		}
		result, err := strconv.ParseFloat(line[semi+1:], 64)
		if err != nil {
			continue
		}
		pos, err := position.NewPositionFromFEN(line[:semi])
		if err != nil {
			continue
		}
		feats := position.ActiveFeatures(pos)
		f32 := make([]int32, len(feats))
		for i, v := range feats {
			f32[i] = int32(v)
		}
		samples = append(samples, sample{features: f32, target: result})
		if max > 0 && len(samples) >= max {
			break
		}
	}
	return samples, sc.Err()
}

// trainNetwork fits a one-hidden-layer network with Adam. The forward pass mirrors Network.Eval
// (clipped-ReLU hidden, linear output in centipawns); the prediction is sigmoid(output * WinProbScale).
func trainNetwork(samples []sample, hidden, epochs int, lr float64) *position.Network {
	rng := rand.New(rand.NewSource(1))
	net := position.NewRandomNetwork(hidden)
	h := net.Hidden

	// Small random initialisation.
	for i := range net.W1 {
		net.W1[i] = float32((rng.Float64()*2 - 1) * 0.05)
	}
	for i := range net.W2 {
		net.W2[i] = float32((rng.Float64()*2 - 1) * 0.1)
	}

	// Adam moments for every parameter, kept in flat slices.
	mW1, vW1 := make([]float64, len(net.W1)), make([]float64, len(net.W1))
	mB1, vB1 := make([]float64, h), make([]float64, h)
	mW2, vW2 := make([]float64, h), make([]float64, h)
	var mB2, vB2 float64
	const beta1, beta2, eps = 0.9, 0.999, 1e-8
	step := 0

	acc := make([]float64, h)
	hid := make([]float64, h)

	for epoch := 0; epoch < epochs; epoch++ {
		rng.Shuffle(len(samples), func(i, j int) { samples[i], samples[j] = samples[j], samples[i] })

		var lossSum float64
		start := time.Now()
		for _, s := range samples {
			step++

			// Forward.
			for j := 0; j < h; j++ {
				acc[j] = float64(net.B1[j])
			}
			for _, feat := range s.features {
				base := int(feat) * h
				for j := 0; j < h; j++ {
					acc[j] += float64(net.W1[base+j])
				}
			}
			out := float64(net.B2)
			for j := 0; j < h; j++ {
				a := acc[j]
				if a < 0 {
					a = 0
				} else if a > 1 {
					a = 1
				}
				hid[j] = a
				out += a * float64(net.W2[j])
			}
			pred := position.Sigmoid(out * position.WinProbScale)
			diff := pred - s.target
			lossSum += diff * diff

			// Backward.
			dOut := 2 * diff * pred * (1 - pred) * position.WinProbScale
			lrt := lr * math.Sqrt(1-math.Pow(beta2, float64(step))) / (1 - math.Pow(beta1, float64(step)))

			// Output bias and weights.
			mB2 = beta1*mB2 + (1-beta1)*dOut
			vB2 = beta2*vB2 + (1-beta2)*dOut*dOut
			net.B2 -= float32(lrt * mB2 / (math.Sqrt(vB2) + eps))

			for j := 0; j < h; j++ {
				gW2 := dOut * hid[j]
				mW2[j] = beta1*mW2[j] + (1-beta1)*gW2
				vW2[j] = beta2*vW2[j] + (1-beta2)*gW2*gW2
				net.W2[j] -= float32(lrt * mW2[j] / (math.Sqrt(vW2[j]) + eps))
			}

			// Hidden layer: gradient flows only through unclipped units.
			for j := 0; j < h; j++ {
				if acc[j] <= 0 || acc[j] >= 1 {
					continue
				}
				dAcc := dOut * float64(net.W2[j])

				mB1[j] = beta1*mB1[j] + (1-beta1)*dAcc
				vB1[j] = beta2*vB1[j] + (1-beta2)*dAcc*dAcc
				net.B1[j] -= float32(lrt * mB1[j] / (math.Sqrt(vB1[j]) + eps))

				for _, feat := range s.features {
					idx := int(feat)*h + j
					mW1[idx] = beta1*mW1[idx] + (1-beta1)*dAcc
					vW1[idx] = beta2*vW1[idx] + (1-beta2)*dAcc*dAcc
					net.W1[idx] -= float32(lrt * mW1[idx] / (math.Sqrt(vW1[idx]) + eps))
				}
			}
		}

		fmt.Printf("epoch %d/%d: mse %.5f (%.1fs)\n", epoch+1, epochs, lossSum/float64(len(samples)), time.Since(start).Seconds())
	}

	return net
}

func init() {
	nnuetrainCmd.Flags().String("data", "nnue-data.txt", "training data from gendata")
	nnuetrainCmd.Flags().StringP("out", "o", "nnue.bin", "output network file")
	nnuetrainCmd.Flags().Int("hidden", 64, "hidden layer size")
	nnuetrainCmd.Flags().Int("epochs", 15, "training epochs")
	nnuetrainCmd.Flags().Float64("lr", 0.01, "Adam learning rate")
	nnuetrainCmd.Flags().Int("max", 3000000, "max samples to load")
	rootCmd.AddCommand(nnuetrainCmd)
}
