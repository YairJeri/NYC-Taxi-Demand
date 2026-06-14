package cluster

import (
	"api/internal/models"
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"log"
	"time"
)

type TrainingJob struct {
	nm          *NetworkManager
	ParamServer *ParameterServer
	ModelKey    string
	onComplete  func()
}

func NewTrainingJob(modelType int, steps int, paramServers map[string]*ParameterServer, nm *NetworkManager, onComplete func()) (*TrainingJob, error) {
	var modelKey string
	var cfg models.ModelConfig

	if modelType == models.ModelStatic {
		modelKey = "A"
		cfg = models.ModelConfig{
			ModelType:     models.ModelStatic,
			NumEmbeddings: 300,
			EmbeddingDim:  8,
			InputDim:      6,
			HiddenDims:    []int{128, 64},
			OutputDim:     1,
			TotalSteps:    steps,
			Collection:    "static_aggregations",
		}
	} else {
		modelKey = "B"
		cfg = models.ModelConfig{
			ModelType:     models.ModelTemporal,
			NumEmbeddings: 300,
			EmbeddingDim:  8,
			InputDim:      31,
			HiddenDims:    []int{128, 64},
			OutputDim:     1,
			TotalSteps:    steps,
			Collection:    "temporal_aggregations",
		}
	}

	ps := paramServers[modelKey]

	if len(ps.Weights.EmbWeight) == 0 {
		ps.InitializeWeights(cfg)
	} else {
		ps.Config.TotalSteps = steps
	}

	ps.mu.Lock()
	ps.CurrentStep = 0
	ps.TrainingStartTime = time.Now()
	ps.LossHistory = make([]float64, 0)
	ps.mu.Unlock()

	tj := &TrainingJob{
		nm:          nm,
		ParamServer: ps,
		ModelKey:    modelKey,
		onComplete:  onComplete,
	}

	return tj, nil
}

func (tj *TrainingJob) Start() error {
	activeWorkers := tj.nm.GetWorkersSnapshot()
	activeCount := len(activeWorkers)

	if activeCount == 0 {
		log.Println("[MASTER ERROR] No workers available to start training")
		return fmt.Errorf("no workers available")
	}

	tj.ParamServer.StartMiniBatch(activeCount)
	log.Printf("[MASTER] Broadcasting MsgTrainStart to %d workers...", activeCount)

	for i, wc := range activeWorkers {
		payloadObj := models.TrainStartPayload{
			Config:      tj.ParamServer.Config,
			Weights:     tj.ParamServer.Weights,
			WorkerIndex: i,
			NumWorkers:  activeCount,
		}

		var buf bytes.Buffer
		enc := gob.NewEncoder(&buf)
		if err := enc.Encode(payloadObj); err != nil {
			log.Printf("[MASTER ERROR] failed to encode payload for worker: %v", err)
			continue
		}
		payload := buf.Bytes()

		header := make([]byte, 5)
		header[0] = MsgTrainStart
		binary.BigEndian.PutUint32(header[1:5], uint32(len(payload)))

		go func(w *WorkerConn, h, p []byte) {
			w.Mu.Lock()
			_, err1 := w.Conn.Write(h)
			var err2 error
			if err1 == nil {
				_, err2 = w.Conn.Write(p)
			}
			w.Mu.Unlock()

			if err1 != nil || err2 != nil {
				tj.nm.RemoveWorker(w.ID)
			}
		}(wc, header, payload)
	}

	return nil
}

func (tj *TrainingJob) ProcessMessage(workerID string, msgType uint8, payload []byte) {
	if msgType != MsgGradients {
		return
	}

	var grads models.Gradients
	buf := bytes.NewReader(payload)
	dec := gob.NewDecoder(buf)
	if err := dec.Decode(&grads); err != nil {
		log.Printf("[MASTER ERROR] decode gradients failed: %v", err)
		return
	}

	done, err := tj.ParamServer.AddGradients(&grads)
	if err != nil {
		log.Printf("[MASTER WARN] gradient validation failed: %v", err)
	} else if done {
		avgLoss := tj.ParamServer.ApplySGD()

		tj.ParamServer.mu.Lock()
		tj.ParamServer.CurrentStep++
		currentStep := tj.ParamServer.CurrentStep
		totalSteps := tj.ParamServer.Config.TotalSteps
		tj.ParamServer.mu.Unlock()

		log.Printf("[MASTER] Mini-batch complete. Step %d/%d Avg Loss: %f", currentStep, totalSteps, avgLoss)

		if currentStep < totalSteps {
			tj.BroadcastWeightsUpdate()
		} else {
			log.Printf("[MASTER] Training complete for model %s! Target steps reached.", tj.ModelKey)
			if tj.onComplete != nil {
				tj.onComplete()
			}
		}
	}
}

func (tj *TrainingJob) BroadcastWeightsUpdate() {
	activeWorkers := tj.nm.GetWorkersSnapshot()
	activeCount := len(activeWorkers)

	if activeCount == 0 {
		log.Println("[MASTER] Training paused. No workers.")
		return
	}

	tj.ParamServer.StartMiniBatch(activeCount)

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)

	tj.ParamServer.mu.Lock()
	err := enc.Encode(tj.ParamServer.Weights)
	tj.ParamServer.mu.Unlock()

	if err != nil {
		log.Printf("[MASTER ERROR] failed to encode updated weights: %v", err)
		return
	}

	payload := buf.Bytes()
	header := make([]byte, 5)
	header[0] = MsgWeightsUpdate
	binary.BigEndian.PutUint32(header[1:5], uint32(len(payload)))

	for _, wc := range activeWorkers {
		go func(w *WorkerConn) {
			w.Mu.Lock()
			_, _ = w.Conn.Write(header)
			_, _ = w.Conn.Write(payload)
			w.Mu.Unlock()
		}(wc)
	}
}

func (tj *TrainingJob) GetStatus() map[string]interface{} {
	tj.ParamServer.mu.Lock()
	defer tj.ParamServer.mu.Unlock()

	lossHistory := make([]float64, len(tj.ParamServer.LossHistory))
	copy(lossHistory, tj.ParamServer.LossHistory)

	return map[string]interface{}{
		"isTraining":    true,
		"trainingStep":  tj.ParamServer.CurrentStep,
		"trainingTotal": tj.ParamServer.Config.TotalSteps,
		"trainingTime":  time.Since(tj.ParamServer.TrainingStartTime).Seconds(),
		"activeModel":   tj.ModelKey,
		"avgLoss":       tj.ParamServer.AvgLoss,
		"lossHistory":   lossHistory,
	}
}
