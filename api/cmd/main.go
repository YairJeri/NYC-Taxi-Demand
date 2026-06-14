package main

import (
	"api/internal/aggregators"
	"api/internal/cluster"
	"api/internal/database"
	"api/internal/handlers"
	"api/internal/middleware"
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	da := aggregators.NewDataAggregation()

	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("[FATAL WORKER] No se pudo conectar a MongoDB central: %v", err)
	}
	defer mongoClient.Disconnect(context.Background())

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatalf("[FATAL] Invalid Redis URL: %v", err)
	}
	redisClient := redis.NewClient(opt)

	ar := database.NewAggregationRepository(mongoClient.Database("trips"))
	pr := database.NewPredictionRepository(mongoClient.Database("trips"))

	w := cluster.NewWorkerHub(":9090", da, ar)
	go w.StartTCPServer()

	uploadHandler := handlers.NewUploadHandler(w)
	trainHandler := handlers.NewTrainHandler(w)
	dashboardHandler := handlers.NewDashboardHandler(w)
	predictHandler := handlers.NewPredictHandler(w, pr, redisClient)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/clean", uploadHandler.HandleCSVUpload)
	mux.HandleFunc("/api/train", trainHandler.HandleTrainStart)
	mux.HandleFunc("/api/dashboard/metrics", dashboardHandler.HandleMetrics)
	mux.HandleFunc("/api/status", dashboardHandler.HandleStatus)
	mux.HandleFunc("/api/predict", predictHandler.HandlePredict)

	httpPort := os.Getenv("HTTP_PORT")
	if httpPort == "" {
		httpPort = ":8080"
	}

	server := &http.Server{
		Addr:         httpPort,
		Handler:      middleware.WithCORS(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	log.Printf("[API MASTER] Servidor HTTP listo para el Frontend en el puerto %s", httpPort)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("[FATAL] Falló el servidor HTTP: %v", err)
	}
}
