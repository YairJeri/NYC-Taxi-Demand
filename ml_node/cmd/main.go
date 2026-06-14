package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"ml_node/internal/client"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	log.Println("[WORKER] Iniciando Nodo de Cómputo...")

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
	log.Println("[WORKER] Conectado a MongoDB central")

	apiTCPAddr := os.Getenv("API_TCP_ADDRESS")
	if apiTCPAddr == "" {
		apiTCPAddr = "localhost:9090"
	}
	workerID := os.Getenv("WORKER_ID")
	if workerID == "" {
		workerID = fmt.Sprintf("nodo-ml-%d", time.Now().UnixNano())
	}

	tcpClient := client.NewTCPClient(workerID, apiTCPAddr, mongoClient)
	tcpClient.Start()
}
