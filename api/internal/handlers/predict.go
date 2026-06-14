package handlers

import (
	"api/internal/cluster"
	"api/internal/database"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type PredictRequest struct {
	Model    string    `json:"model"`
	Zone     int       `json:"zone"`
	Features []float64 `json:"features"`
}

type PredictResponse struct {
	Prediction float64 `json:"prediction"`
	Model      string  `json:"model"`
}

type PredictHandler struct {
	hub   *cluster.WorkerHub
	db    *database.PredictionRepository
	redis *redis.Client
}

func NewPredictHandler(hub *cluster.WorkerHub, db *database.PredictionRepository, rdb *redis.Client) *PredictHandler {
	return &PredictHandler{
		hub:   hub,
		db:    db,
		redis: rdb,
	}
}

func (h *PredictHandler) HandlePredict(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req PredictRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Model != "A" && req.Model != "B" {
		http.Error(w, "Invalid model, expected A or B", http.StatusBadRequest)
		return
	}

	ps := h.hub.ParamServers[req.Model]

	// Check redis cache
	cacheKey := fmt.Sprintf("predict:%s:%d:%v", req.Model, req.Zone, req.Features)
	ctx := context.Background()

	var pred float64
	var fromCache bool

	cachedVal, err := h.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		if p, err := strconv.ParseFloat(cachedVal, 64); err == nil {
			pred = p
			fromCache = true
		}
	}

	if !fromCache {
		pred, err = ps.Predict(req.Zone, req.Features)
		if err != nil {
			http.Error(w, "Prediction failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Save to cache
		h.redis.Set(ctx, cacheKey, pred, 1*time.Hour)

		// Save to DB
		err = h.db.SavePrediction(ctx, req.Model, req.Zone, req.Features, pred)
		if err != nil {
			fmt.Println("Error saving prediction to db:", err)
		}
	}

	resp := PredictResponse{
		Prediction: pred,
		Model:      req.Model,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
