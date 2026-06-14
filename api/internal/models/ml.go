package models

const (
	ModelStatic   = 1
	ModelTemporal = 2
)

type ModelConfig struct {
	ModelType     int
	NumEmbeddings int
	EmbeddingDim  int
	InputDim      int
	HiddenDims    []int
	OutputDim     int
	TotalSteps    int
	Collection    string
}

type LayerWeights struct {
	W []float64
	B []float64
}

type GlobalWeights struct {
	EmbWeight [][]float64
	Layers    []LayerWeights
}

type LayerGradients struct {
	GradW []float64
	GradB []float64
}

type Gradients struct {
	WorkerID  string
	GradEmb   [][]float64
	Layers    []LayerGradients
	Loss      float64
	BatchSize int
}
type TrainStartPayload struct {
	Config      ModelConfig
	Weights     GlobalWeights
	WorkerIndex int
	NumWorkers  int
}
