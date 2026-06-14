package cluster

import (
	"api/internal/models"
	"fmt"
	"log"
	"math"
	"math/rand"
	"sync"
	"time"
)

type ParameterServer struct {
	mu sync.Mutex

	Config  models.ModelConfig
	Weights models.GlobalWeights
	LR      float64

	expectedWorkers   int
	receivedGrads     []*models.Gradients
	CurrentStep       int
	AvgLoss           float64
	LossHistory       []float64
	TrainingStartTime time.Time
}

func NewParameterServer(lr float64) *ParameterServer {
	return &ParameterServer{
		LR:            lr,
		receivedGrads: make([]*models.Gradients, 0),
		LossHistory:   make([]float64, 0),
	}
}

func (ps *ParameterServer) InitializeWeights(cfg models.ModelConfig) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.Config = cfg

	embScale := 1.0 / math.Sqrt(float64(cfg.EmbeddingDim))
	ps.Weights.EmbWeight = make([][]float64, cfg.NumEmbeddings)
	for i := range ps.Weights.EmbWeight {
		ps.Weights.EmbWeight[i] = make([]float64, cfg.EmbeddingDim)
		for j := range ps.Weights.EmbWeight[i] {
			ps.Weights.EmbWeight[i][j] = rand.NormFloat64() * embScale
		}
	}

	fullInput := cfg.EmbeddingDim + cfg.InputDim
	dims := make([]int, 0, len(cfg.HiddenDims)+2)
	dims = append(dims, fullInput)
	dims = append(dims, cfg.HiddenDims...)
	dims = append(dims, cfg.OutputDim)

	ps.Weights.Layers = make([]models.LayerWeights, len(dims)-1)
	for i := 0; i < len(dims)-1; i++ {
		in, out := dims[i], dims[i+1]
		scale := math.Sqrt(2.0 / float64(in)) // He init
		w := make([]float64, in*out)
		for j := range w {
			w[j] = rand.NormFloat64() * scale
		}
		ps.Weights.Layers[i] = models.LayerWeights{
			W: w,
			B: make([]float64, out),
		}
	}

	log.Printf("[MASTER] Weights initialized: emb(%dx%d), layers=%v -> %d",
		cfg.NumEmbeddings, cfg.EmbeddingDim, dims, cfg.OutputDim)
}

func (ps *ParameterServer) StartMiniBatch(expected int) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.expectedWorkers = expected
	ps.receivedGrads = make([]*models.Gradients, 0, expected)
}

func (ps *ParameterServer) AddGradients(grad *models.Gradients) (bool, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if err := ps.validateGradients(grad); err != nil {
		return false, fmt.Errorf("worker %s sent invalid gradients: %w", grad.WorkerID, err)
	}

	ps.receivedGrads = append(ps.receivedGrads, grad)
	return len(ps.receivedGrads) >= ps.expectedWorkers, nil
}

func (ps *ParameterServer) validateGradients(grad *models.Gradients) error {
	if len(grad.GradEmb) != len(ps.Weights.EmbWeight) {
		return fmt.Errorf("GradEmb rows: got %d, want %d", len(grad.GradEmb), len(ps.Weights.EmbWeight))
	}
	if len(grad.Layers) != len(ps.Weights.Layers) {
		return fmt.Errorf("layer count: got %d, want %d", len(grad.Layers), len(ps.Weights.Layers))
	}
	for i, lg := range grad.Layers {
		wantW := len(ps.Weights.Layers[i].W)
		wantB := len(ps.Weights.Layers[i].B)
		if len(lg.GradW) != wantW {
			return fmt.Errorf("layer[%d].GradW: got %d, want %d", i, len(lg.GradW), wantW)
		}
		if len(lg.GradB) != wantB {
			return fmt.Errorf("layer[%d].GradB: got %d, want %d", i, len(lg.GradB), wantB)
		}
	}
	return nil
}

func (ps *ParameterServer) ApplySGD() float64 {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if len(ps.receivedGrads) == 0 {
		return 0.0
	}

	n := float64(len(ps.receivedGrads))
	totalLoss := 0.0

	clip := func(val float64) float64 {
		if math.IsNaN(val) {
			return 0.0
		}
		if val > 1.0 {
			return 1.0
		} else if val < -1.0 {
			return -1.0
		}
		return val
	}

	for _, grad := range ps.receivedGrads {
		totalLoss += grad.Loss

		for i := range grad.GradEmb {
			for j := range grad.GradEmb[i] {
				g := clip(grad.GradEmb[i][j] / n)
				ps.Weights.EmbWeight[i][j] -= ps.LR * g
			}
		}

		for l, lg := range grad.Layers {
			for j := range lg.GradW {
				g := clip(lg.GradW[j] / n)
				ps.Weights.Layers[l].W[j] -= ps.LR * g
			}
			for j := range lg.GradB {
				g := clip(lg.GradB[j] / n)
				ps.Weights.Layers[l].B[j] -= ps.LR * g
			}
		}
	}

	ps.receivedGrads = make([]*models.Gradients, 0, ps.expectedWorkers)

	avgLoss := totalLoss / n
	ps.AvgLoss = avgLoss

	if ps.CurrentStep%10 == 0 || ps.CurrentStep == 1 {
		ps.LossHistory = append(ps.LossHistory, avgLoss)

		if len(ps.LossHistory) > 100 {
			ps.LossHistory = ps.LossHistory[len(ps.LossHistory)-100:]
		}
	}

	return avgLoss
}

func (ps *ParameterServer) Predict(zoneID int, numericFeatures []float64) (float64, error) {
	ps.mu.Lock()
	if len(ps.Weights.EmbWeight) == 0 {
		ps.mu.Unlock()
		return 0, fmt.Errorf("model not initialized")
	}
	if zoneID < 0 || zoneID >= len(ps.Weights.EmbWeight) {
		ps.mu.Unlock()
		return 0, fmt.Errorf("zoneID out of bounds")
	}
	if len(numericFeatures) != ps.Config.InputDim {
		ps.mu.Unlock()
		return 0, fmt.Errorf("expected %d numeric features, got %d", ps.Config.InputDim, len(numericFeatures))
	}

	emb := make([]float64, len(ps.Weights.EmbWeight[zoneID]))
	copy(emb, ps.Weights.EmbWeight[zoneID])

	layers := make([]models.LayerWeights, len(ps.Weights.Layers))
	for i, lw := range ps.Weights.Layers {
		layers[i].W = make([]float64, len(lw.W))
		copy(layers[i].W, lw.W)
		layers[i].B = make([]float64, len(lw.B))
		copy(layers[i].B, lw.B)
	}
	ps.mu.Unlock()

	input := make([]float64, 0, len(emb)+len(numericFeatures))
	input = append(input, emb...)
	input = append(input, numericFeatures...)

	current := input
	for l, layer := range layers {
		outDim := len(layer.B)
		inDim := len(current)
		next := make([]float64, outDim)

		for o := 0; o < outDim; o++ {
			sum := layer.B[o]
			rowW := o * inDim
			for i := 0; i < inDim; i++ {
				sum += layer.W[rowW+i] * current[i]
			}
			next[o] = sum
		}

		if l < len(layers)-1 {
			for o := 0; o < outDim; o++ {
				if next[o] < 0 {
					next[o] = 0
				}
			}
		}
		current = next
	}

	if len(current) != 1 {
		return 0, fmt.Errorf("expected output dim 1, got %d", len(current))
	}

	prediction := math.Exp(current[0]) - 1.0

	if prediction < 0 {
		prediction = 0
	}

	return prediction, nil
}
