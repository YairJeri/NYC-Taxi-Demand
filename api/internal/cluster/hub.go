package cluster

import (
	"api/internal/aggregators"
	"api/internal/database"
	"api/internal/models"
	"sync"
	"time"
)

const (
	MsgData          = 1
	MsgTrainStart    = 2
	MsgAggResult     = 3
	MsgGradients     = 4
	MsgWeightsUpdate = 5
	MsgMetrics       = 6
)

type WorkerHub struct {
	nm           *NetworkManager
	orchestrator *JobOrchestrator

	DataAggregation *aggregators.DataAggregation
	AggregationRepo *database.AggregationRepository
	ParamServers    map[string]*ParameterServer

	mu             sync.RWMutex
	ActiveModelKey string
}

func NewWorkerHub(port string, da *aggregators.DataAggregation, ar *database.AggregationRepository) *WorkerHub {
	h := &WorkerHub{
		nm:              NewNetworkManager(port),
		orchestrator:    NewJobOrchestrator(),
		DataAggregation: da,
		AggregationRepo: ar,
		ParamServers: map[string]*ParameterServer{
			"A": NewParameterServer(0.005),
			"B": NewParameterServer(0.005),
		},
	}

	h.nm.OnMessage = h.orchestrator.RouteMessage
	h.nm.OnWorkerRemove = func(workerID string) {
		job := h.orchestrator.GetActiveJob()
		if cj, ok := job.(*CleaningJob); ok {
			cj.ReassignWorkerChunks(workerID)
		}
	}

	return h
}

func (h *WorkerHub) StartTCPServer() {
	h.nm.StartTCPServer()
}

func (h *WorkerHub) TryStartJob(jobID string, jobType int) error {
	cj := NewCleaningJob(jobID, jobType, h.DataAggregation, h.AggregationRepo, h.nm, func() {
		h.orchestrator.ClearJob(h.orchestrator.GetActiveJob())
	})
	return h.orchestrator.TrySetJob(cj)
}

func (h *WorkerHub) SetTotalChunks(jobID string, total int) {
	job := h.orchestrator.GetActiveJob()
	if cj, ok := job.(*CleaningJob); ok {
		cj.SetTotalChunks(total)
	}
}

func (h *WorkerHub) SendChunk(chunkID int, lines []string) error {
	job := h.orchestrator.GetActiveJob()
	if cj, ok := job.(*CleaningJob); ok {
		return cj.SendChunk(chunkID, lines)
	}
	return nil
}

func (h *WorkerHub) StartTraining(modelType int, steps int) error {
	tj, err := NewTrainingJob(modelType, steps, h.ParamServers, h.nm, func() {
		h.orchestrator.ClearJob(h.orchestrator.GetActiveJob())
	})
	if err != nil {
		return err
	}

	if err := h.orchestrator.TrySetJob(tj); err != nil {
		return err
	}

	h.mu.Lock()
	h.ActiveModelKey = tj.ModelKey
	h.mu.Unlock()

	return tj.Start()
}

func (h *WorkerHub) GetActiveWorkers() []string {
	return h.nm.GetActiveWorkers()
}

func (h *WorkerHub) GetDashboardMetrics() []models.WorkerDashboardMetrics {
	return h.nm.GetDashboardMetrics()
}

func (h *WorkerHub) GetStatus() map[string]interface{} {
	h.mu.RLock()
	activeModelKey := h.ActiveModelKey
	h.mu.RUnlock()

	status := map[string]interface{}{
		"isProcessing":    false,
		"processingDone":  0,
		"processingTotal": 0,
		"processingTime":  0.0,
		"isTraining":      false,
		"trainingStep":    0,
		"trainingTotal":   0,
		"trainingTime":    0.0,
		"activeModel":     activeModelKey,
		"avgLoss":         0.0,
		"lossHistory":     []float64{},
	}

	if activeModelKey != "" && h.ParamServers[activeModelKey] != nil {
		ps := h.ParamServers[activeModelKey]
		ps.mu.Lock()
		status["trainingStep"] = ps.CurrentStep
		status["trainingTotal"] = ps.Config.TotalSteps
		if ps.CurrentStep > 0 && ps.CurrentStep < ps.Config.TotalSteps {
			status["isTraining"] = true
			status["trainingTime"] = time.Since(ps.TrainingStartTime).Seconds()
		}
		status["avgLoss"] = ps.AvgLoss
		lossHist := make([]float64, len(ps.LossHistory))
		copy(lossHist, ps.LossHistory)
		status["lossHistory"] = lossHist
		ps.mu.Unlock()
	}

	job := h.orchestrator.GetActiveJob()
	if cj, ok := job.(*CleaningJob); ok {
		jobStatus := cj.GetStatus()
		for k, v := range jobStatus {
			status[k] = v
		}
	} else if tj, ok := job.(*TrainingJob); ok {
		jobStatus := tj.GetStatus()
		for k, v := range jobStatus {
			status[k] = v
		}
	}

	return status
}
