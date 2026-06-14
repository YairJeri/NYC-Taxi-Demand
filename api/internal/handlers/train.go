package handlers

import (
	"api/internal/cluster"
	"api/internal/models"
	"context"
	"net/http"
	"strconv"
	"time"
)

type TrainHandler struct {
	hub *cluster.WorkerHub
}

func NewTrainHandler(hub *cluster.WorkerHub) *TrainHandler {
	return &TrainHandler{
		hub: hub,
	}
}

func (h *TrainHandler) HandleTrainStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	modelTypeStr := r.URL.Query().Get("type")
	stepsStr := r.URL.Query().Get("steps")

	steps, err := strconv.Atoi(stepsStr)
	if err != nil || steps <= 0 {
		steps = 5000
	}

	modelType := models.ModelStatic
	collectionName := "static_aggregations"
	if modelTypeStr == "temporal" {
		modelType = models.ModelTemporal
		collectionName = "temporal_aggregations"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	hasData, err := h.hub.AggregationRepo.HasData(ctx, collectionName)
	if err != nil {
		http.Error(w, "Error checking database", http.StatusInternalServerError)
		return
	}
	if !hasData {
		http.Error(w, "Required data is missing. Please run /clean or provide data first.", http.StatusBadRequest)
		return
	}

	err = h.hub.StartTraining(modelType, steps)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Training started asynchronously.\n"))
}
