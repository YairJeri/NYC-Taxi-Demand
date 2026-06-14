package database

import (
	"api/internal/models"
	"context"
	"fmt"
	"sync"

	"go.mongodb.org/mongo-driver/mongo"
)

type StaticDoc struct {
	Zone    int `bson:"zone"`
	Hour    int `bson:"hour"`
	Weekday int `bson:"weekday"`
	Month   int `bson:"month"`
	Count   int `bson:"count"`
}

type TemporalDoc struct {
	Zone  int    `bson:"zone"`
	Year  uint16 `bson:"year"`
	Month uint8  `bson:"month"`
	Day   uint8  `bson:"day"`
	Hour  uint8  `bson:"hour"`
	Count int    `bson:"count"`
}

type AggregationRepository struct {
	db *mongo.Database
}

func NewAggregationRepository(db *mongo.Database) *AggregationRepository {
	return &AggregationRepository{db: db}
}

func (r *AggregationRepository) HasData(ctx context.Context, collectionName string) (bool, error) {
	count, err := r.db.Collection(collectionName).CountDocuments(ctx, map[string]interface{}{})
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *AggregationRepository) Save(ctx context.Context, static map[models.StaticKey]int, temporal map[models.TemporalKey]int) error {
	staticDocs := make([]interface{}, 0, len(static))
	for k, v := range static {
		staticDocs = append(staticDocs, StaticDoc{
			Zone:    k.Zone,
			Hour:    k.Hour,
			Weekday: k.Weekday,
			Month:   k.Month,
			Count:   v,
		})
	}
	temporalDocs := make([]interface{}, 0, len(temporal))
	for k, v := range temporal {
		temporalDocs = append(temporalDocs, TemporalDoc{
			Zone:  k.Zone,
			Year:  k.Year,
			Month: k.Month,
			Day:   k.Day,
			Hour:  k.Hour,
			Count: v,
		})
	}

	var wg sync.WaitGroup
	var errStatic, errTemporal error

	if len(staticDocs) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errStatic = r.db.Collection("static_aggregations").InsertMany(ctx, staticDocs)
		}()
	}

	if len(temporalDocs) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errTemporal = r.db.Collection("temporal_aggregations").InsertMany(ctx, temporalDocs)
		}()
	}

	wg.Wait()

	if errStatic != nil {
		return fmt.Errorf("error al ejecutar InsertMany en static_aggregations: %w", errStatic)
	}
	if errTemporal != nil {
		return fmt.Errorf("error al ejecutar InsertMany en temporal_aggregations: %w", errTemporal)
	}

	return nil
}
