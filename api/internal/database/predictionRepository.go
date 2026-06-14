package database

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
)

type PredictionDoc struct {
	Model      string    `bson:"model"`
	Zone       int       `bson:"zone"`
	Features   []float64 `bson:"features"`
	Prediction float64   `bson:"prediction"`
	CreatedAt  time.Time `bson:"createdAt"`
}

type PredictionRepository struct {
	db *mongo.Database
}

func NewPredictionRepository(db *mongo.Database) *PredictionRepository {
	return &PredictionRepository{db: db}
}

func (r *PredictionRepository) SavePrediction(ctx context.Context, model string, zone int, features []float64, prediction float64) error {
	doc := PredictionDoc{
		Model:      model,
		Zone:       zone,
		Features:   features,
		Prediction: prediction,
		CreatedAt:  time.Now(),
	}
	_, err := r.db.Collection("predictions").InsertOne(ctx, doc)
	return err
}
