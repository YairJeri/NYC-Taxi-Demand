package cluster

import (
	"fmt"
	"sync"
)

// Job defines the interface for different types of distributed jobs.
type Job interface {
	ProcessMessage(workerID string, msgType uint8, payload []byte)
	GetStatus() map[string]interface{}
}

type JobOrchestrator struct {
	mu        sync.RWMutex
	activeJob Job
}

func NewJobOrchestrator() *JobOrchestrator {
	return &JobOrchestrator{}
}

func (o *JobOrchestrator) TrySetJob(j Job) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.activeJob != nil {
		return fmt.Errorf("a job is already active")
	}

	o.activeJob = j
	return nil
}

func (o *JobOrchestrator) ClearJob(j Job) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.activeJob == j {
		o.activeJob = nil
	}
}

func (o *JobOrchestrator) GetActiveJob() Job {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.activeJob
}

func (o *JobOrchestrator) RouteMessage(workerID string, msgType uint8, payload []byte) {
	o.mu.RLock()
	job := o.activeJob
	o.mu.RUnlock()

	if job != nil {
		job.ProcessMessage(workerID, msgType, payload)
	}
}
