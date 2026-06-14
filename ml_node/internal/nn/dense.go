package nn

import (
	"log"
	"math"
	"math/rand"
	"ml_node/internal/models"
	"runtime"
	"sync"
)

type Embedding struct {
	NumEmbeddings int
	EmbeddingDim  int
	Weight        [][]float64
	GradWeight    [][]float64
}

func NewEmbedding(numEmbeddings, embeddingDim int) *Embedding {
	w := make([][]float64, numEmbeddings)
	gw := make([][]float64, numEmbeddings)
	scale := 1.0 / math.Sqrt(float64(embeddingDim))
	for i := range w {
		w[i] = make([]float64, embeddingDim)
		gw[i] = make([]float64, embeddingDim)
		for j := range w[i] {
			w[i][j] = rand.NormFloat64() * scale
		}
	}
	return &Embedding{NumEmbeddings: numEmbeddings, EmbeddingDim: embeddingDim, Weight: w, GradWeight: gw}
}

func (e *Embedding) Forward(zoneIDs []int) [][]float64 {
	out := make([][]float64, len(zoneIDs))
	for i, id := range zoneIDs {
		out[i] = make([]float64, e.EmbeddingDim)
		copy(out[i], e.Weight[id])
	}
	return out
}

func (e *Embedding) Backward(zoneIDs []int, gradOut [][]float64) {
	for i := range e.GradWeight {
		for j := range e.GradWeight[i] {
			e.GradWeight[i][j] = 0
		}
	}
	for i, id := range zoneIDs {
		for j := 0; j < e.EmbeddingDim; j++ {
			e.GradWeight[id][j] += gradOut[i][j]
		}
	}
}

type Dense struct {
	In    int
	Out   int
	W     []float64
	B     []float64
	GradW []float64
	GradB []float64
	XLast [][]float64
}

func NewDense(in, out int) *Dense {
	w := make([]float64, in*out)
	scale := math.Sqrt(2.0 / float64(in))
	for i := range w {
		w[i] = rand.NormFloat64() * scale
	}
	return &Dense{
		In:    in,
		Out:   out,
		W:     w,
		B:     make([]float64, out),
		GradW: make([]float64, in*out),
		GradB: make([]float64, out),
	}
}

func (d *Dense) ForwardBatchParallel(x [][]float64, y [][]float64) {
	d.XLast = x
	batchSize := len(x)
	workers := runtime.NumCPU()
	var wg sync.WaitGroup
	chunk := (batchSize + workers - 1) / workers

	for w := 0; w < workers; w++ {
		start := w * chunk
		end := start + chunk
		if end > batchSize {
			end = batchSize
		}
		if start >= end {
			break
		}
		wg.Add(1)
		go func(s, e int) {
			defer wg.Done()
			for sample := s; sample < e; sample++ {
				for o := 0; o < d.Out; o++ {
					sum := d.B[o]
					rowW := o * d.In
					for i := 0; i < d.In; i++ {
						sum += d.W[rowW+i] * d.XLast[sample][i]
					}
					y[sample][o] = sum
				}
			}
		}(start, end)
	}
	wg.Wait()
}

func (d *Dense) BackwardBatch(gradOut [][]float64) [][]float64 {
	batchSize := len(gradOut)
	gradInput := make([][]float64, batchSize)
	for i := range gradInput {
		gradInput[i] = make([]float64, d.In)
	}
	for i := range d.GradW {
		d.GradW[i] = 0
	}
	for i := range d.GradB {
		d.GradB[i] = 0
	}
	for s := 0; s < batchSize; s++ {
		for o := 0; o < d.Out; o++ {
			g := gradOut[s][o]
			d.GradB[o] += g
			rowW := o * d.In
			for i := 0; i < d.In; i++ {
				d.GradW[rowW+i] += g * d.XLast[s][i]
				gradInput[s][i] += g * d.W[rowW+i]
			}
		}
	}
	return gradInput
}

func ReLU(x [][]float64) [][]float64 {
	mask := make([][]float64, len(x))
	for s := range x {
		mask[s] = make([]float64, len(x[s]))
		for i := range x[s] {
			if x[s][i] < 0 {
				x[s][i] = 0
				mask[s][i] = 0
			} else {
				mask[s][i] = 1
			}
		}
	}
	return mask
}

func ReLUBackward(gradOut [][]float64, mask [][]float64) {
	for s := range gradOut {
		for i := range gradOut[s] {
			gradOut[s][i] *= mask[s][i]
		}
	}
}

func MSELoss(predictions [][]float64, targets []float64) (float64, [][]float64) {
	batchSize := len(predictions)
	loss := 0.0
	gradOut := make([][]float64, batchSize)
	for s := 0; s < batchSize; s++ {
		gradOut[s] = make([]float64, 1)
		diff := predictions[s][0] - targets[s]
		loss += diff * diff
		gradOut[s][0] = (2.0 / float64(batchSize)) * diff
	}

	for s := 0; s < batchSize; s++ {

		if math.IsNaN(predictions[s][0]) || math.IsInf(predictions[s][0], 0) {
			log.Fatalf(
				"[BAD PRED] sample=%d pred=%v target=%v",
				s,
				predictions[s][0],
				targets[s],
			)
		}

		if math.IsNaN(targets[s]) || math.IsInf(targets[s], 0) {
			log.Fatalf(
				"[BAD TARGET] sample=%d target=%v",
				s,
				targets[s],
			)
		}

		diff := predictions[s][0] - targets[s]

		if math.IsNaN(diff) || math.IsInf(diff, 0) {
			log.Fatalf(
				"[BAD DIFF] sample=%d pred=%v target=%v diff=%v",
				s,
				predictions[s][0],
				targets[s],
				diff,
			)
		}
	}
	return loss / float64(batchSize), gradOut
}

type Net struct {
	Emb   *Embedding
	Stack []*Dense
	bufs  [][][]float64
	masks [][][]float64
}

func NewNetFromConfig(cfg models.ModelConfig) *Net {
	fullInput := cfg.EmbeddingDim + cfg.InputDim
	dims := make([]int, 0, len(cfg.HiddenDims)+2)
	dims = append(dims, fullInput)
	dims = append(dims, cfg.HiddenDims...)
	dims = append(dims, cfg.OutputDim)

	stack := make([]*Dense, len(dims)-1)
	for i := 0; i < len(dims)-1; i++ {
		stack[i] = NewDense(dims[i], dims[i+1])
	}

	return &Net{
		Emb:   NewEmbedding(cfg.NumEmbeddings, cfg.EmbeddingDim),
		Stack: stack,
	}
}

func (n *Net) Forward(zones []int, numericFeatures [][]float64) [][]float64 {
	batchSize := len(zones)
	embOut := n.Emb.Forward(zones)
	x := make([][]float64, batchSize)
	for i := 0; i < batchSize; i++ {
		x[i] = append(embOut[i], numericFeatures[i]...)
	}

	n.bufs = make([][][]float64, len(n.Stack))
	for l, layer := range n.Stack {
		n.bufs[l] = make([][]float64, batchSize)
		for i := range n.bufs[l] {
			n.bufs[l][i] = make([]float64, layer.Out)
		}
	}

	n.masks = make([][][]float64, len(n.Stack))
	input := x
	for l, layer := range n.Stack {
		layer.ForwardBatchParallel(input, n.bufs[l])
		if l < len(n.Stack)-1 {
			n.masks[l] = ReLU(n.bufs[l])
		}
		input = n.bufs[l]
	}

	return n.bufs[len(n.bufs)-1]
}

func (n *Net) Backward(zones []int, gradOut [][]float64) {
	grad := gradOut
	for l := len(n.Stack) - 1; l >= 0; l-- {
		grad = n.Stack[l].BackwardBatch(grad)
		if l > 0 {
			ReLUBackward(grad, n.masks[l-1])
		}
	}

	gradEmb := make([][]float64, len(zones))
	for i := range zones {
		gradEmb[i] = grad[i][:n.Emb.EmbeddingDim]
	}
	n.Emb.Backward(zones, gradEmb)
}

func (n *Net) LoadWeights(w models.GlobalWeights) {
	for i := range w.EmbWeight {
		copy(n.Emb.Weight[i], w.EmbWeight[i])
	}
	for l, lw := range w.Layers {
		copy(n.Stack[l].W, lw.W)
		copy(n.Stack[l].B, lw.B)
	}
}

func (n *Net) ExtractGradients(workerID string, loss float64, batchSize int) models.Gradients {
	layerGrads := make([]models.LayerGradients, len(n.Stack))
	for l, layer := range n.Stack {
		gw := make([]float64, len(layer.GradW))
		copy(gw, layer.GradW)
		gb := make([]float64, len(layer.GradB))
		copy(gb, layer.GradB)
		layerGrads[l] = models.LayerGradients{GradW: gw, GradB: gb}
	}

	gradEmb := make([][]float64, len(n.Emb.GradWeight))
	for i, row := range n.Emb.GradWeight {
		gradEmb[i] = make([]float64, len(row))
		copy(gradEmb[i], row)
	}

	return models.Gradients{
		WorkerID:  workerID,
		GradEmb:   gradEmb,
		Layers:    layerGrads,
		Loss:      loss,
		BatchSize: batchSize,
	}
}
