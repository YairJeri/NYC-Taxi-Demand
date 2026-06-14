package models

import "time"

type WorkerRegistration struct {
	WorkerID      string `json:"worker_id"`
	Hostname      string `json:"hostname"`
	CPUCores      int    `json:"cpu_cores"`
	TotalRAMBytes uint64 `json:"total_ram_bytes"`
	WorkerVersion string `json:"worker_version"`
}

type WorkerRuntimeMetrics struct {
	WorkerID            string        `json:"worker_id"`
	CurrentStep         int           `json:"current_step"`
	CurrentLoss         float64       `json:"current_loss"`
	CPUUtilization      float64       `json:"cpu_utilization"`
	RAMUsageBytes       uint64        `json:"ram_usage_bytes"`
	LastGradientTime    time.Time     `json:"last_gradient_time"`
	GradientsSent       int           `json:"gradients_sent"`
	AvgGradientInterval time.Duration `json:"avg_gradient_interval"`
	SamplesProcessed    int           `json:"samples_processed"`
}
