package tasks

import (
	"bytes"
	"context"
	"encoding/gob"
	"log"
	"math"
	"math/rand"
	"sort"
	"sync"
	"time"

	"ml_node/internal/models"
	"ml_node/internal/nn"

	"go.mongodb.org/mongo-driver/bson"
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

var (
	globalTrainer *Trainer
	trainerMu     sync.Mutex
)

type Trainer struct {
	WorkerID    string
	SendMsg     func(msgType byte, payload []byte) error
	DB          *mongo.Client
	Config      models.ModelConfig
	Weights     models.GlobalWeights
	WeightsChan chan models.GlobalWeights
	WorkerIndex int
	NumWorkers  int

	// Metrics
	metricsMu         sync.RWMutex
	currentStep       int
	currentLoss       float64
	lastGradientTime  time.Time
	gradientsSent     int
	totalGradientTime time.Duration
	samplesProcessed  int
}

type TrainerMetrics struct {
	CurrentStep         int
	CurrentLoss         float64
	LastGradientTime    time.Time
	GradientsSent       int
	AvgGradientInterval time.Duration
	SamplesProcessed    int
}

func GetTrainerMetrics() TrainerMetrics {
	trainerMu.Lock()
	tr := globalTrainer
	trainerMu.Unlock()

	if tr == nil {
		return TrainerMetrics{}
	}

	tr.metricsMu.RLock()
	defer tr.metricsMu.RUnlock()

	var avg time.Duration
	if tr.gradientsSent > 0 {
		avg = time.Duration(int64(tr.totalGradientTime) / int64(tr.gradientsSent))
	}

	return TrainerMetrics{
		CurrentStep:         tr.currentStep,
		CurrentLoss:         tr.currentLoss,
		LastGradientTime:    tr.lastGradientTime,
		GradientsSent:       tr.gradientsSent,
		AvgGradientInterval: avg,
		SamplesProcessed:    tr.samplesProcessed,
	}
}

func RunTrainingLoop(
	workerID string,
	sendMsg func(msgType byte, payload []byte) error,
	db *mongo.Client,
	payload models.TrainStartPayload,
) {
	trainerMu.Lock()
	globalTrainer = &Trainer{
		WorkerID:    workerID,
		SendMsg:     sendMsg,
		DB:          db,
		Config:      payload.Config,
		Weights:     payload.Weights,
		WeightsChan: make(chan models.GlobalWeights, 1),
		WorkerIndex: payload.WorkerIndex,
		NumWorkers:  payload.NumWorkers,
	}
	trainerMu.Unlock()

	globalTrainer.loop()
}

func UpdateTrainerWeights(w models.GlobalWeights) {
	trainerMu.Lock()
	if globalTrainer != nil {
		globalTrainer.WeightsChan <- w
	}
	trainerMu.Unlock()
}

func CheckTensor(name string, x [][]float64) bool {
	for i := range x {
		for j, v := range x[i] {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				log.Printf("[NAN %s] row=%d col=%d val=%v", name, i, j, v)
				return true
			}
		}
	}
	return false
}

func CheckVector(name string, x []float64) bool {
	for i, v := range x {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			log.Printf("[NAN %s] idx=%d val=%v", name, i, v)
			return true
		}
	}
	return false
}

func CheckFeatures(features [][]float64) bool {
	for i := range features {
		for j, v := range features[i] {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				log.Printf("[NAN FEATURE] sample=%d feature=%d val=%v", i, j, v)
				return true
			}
		}
	}
	return false
}

func CheckTargets(targets []float64) bool {
	for i, v := range targets {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			log.Printf("[NAN TARGET] sample=%d val=%v", i, v)
			return true
		}
	}
	return false
}

func CheckZones(zones []int) bool {
	for i, z := range zones {
		if z < 0 || z >= 300 {
			log.Printf("[BAD ZONE] sample=%d zone=%d", i, z)
			return true
		}
	}
	return false
}

func CheckWeights(weights models.GlobalWeights) bool {
	for l, layer := range weights.Layers {

		for i, w := range layer.W {
			if math.IsNaN(w) || math.IsInf(w, 0) {
				log.Printf("[NAN WEIGHT] layer=%d W[%d]=%v", l, i, w)
				return true
			}
		}

		for i, b := range layer.B {
			if math.IsNaN(b) || math.IsInf(b, 0) {
				log.Printf("[NAN BIAS] layer=%d B[%d]=%v", l, i, b)
				return true
			}
		}
	}

	for i := range weights.EmbWeight {
		for j, v := range weights.EmbWeight[i] {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				log.Printf("[NAN EMB] emb=%d dim=%d val=%v", i, j, v)
				return true
			}
		}
	}

	return false
}

func CheckGradients(grads models.Gradients) bool {

	for l, layer := range grads.Layers {

		for i, v := range layer.GradW {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				log.Printf("[NAN GRADW] layer=%d idx=%d val=%v", l, i, v)
				return true
			}
		}

		for i, v := range layer.GradB {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				log.Printf("[NAN GRADB] layer=%d idx=%d val=%v", l, i, v)
				return true
			}
		}
	}

	for i := range grads.GradEmb {
		for j, v := range grads.GradEmb[i] {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				log.Printf("[NAN EMB GRAD] emb=%d dim=%d val=%v", i, j, v)
				return true
			}
		}
	}

	return false
}

func (t *Trainer) loop() {
	cfg := t.Config

	moduloFilter := bson.M{
		"$expr": bson.M{
			"$eq": []interface{}{
				bson.M{"$mod": []interface{}{"$zone", t.NumWorkers}},
				t.WorkerIndex,
			},
		},
	}

	var allZones []int
	var allFeatures [][]float64
	var allTargets []float64

	log.Printf("[TRAINER %s] Loading %s (index %d/%d)...",
		t.WorkerID, cfg.Collection, t.WorkerIndex, t.NumWorkers)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	cursor, err := t.DB.Database("trips").Collection(cfg.Collection).Find(ctx, moduloFilter)
	if err != nil {
		log.Printf("[TRAINER ERROR] DB query failed: %v", err)
		cancel()
		return
	}

	if cfg.ModelType == models.ModelStatic {
		var docs []StaticDoc
		cursor.All(ctx, &docs)
		cancel()
		allZones, allFeatures, allTargets = buildStaticDataset(docs)
	} else {
		var docs []TemporalDoc
		cursor.All(ctx, &docs)
		cancel()
		allZones, allFeatures, allTargets = buildTemporalDataset(docs)
	}

	totalSamples := len(allTargets)
	if totalSamples == 0 {
		log.Printf("[TRAINER ERROR %s] No data found for assigned partition. Exiting.", t.WorkerID)
		return
	}
	log.Printf("[TRAINER %s] Dataset ready: %d sequences.", t.WorkerID, totalSamples)

	batchSize := 64
	for step := 1; step <= cfg.TotalSteps; step++ {
		batchZones := make([]int, batchSize)
		batchFeatures := make([][]float64, batchSize)
		batchTargets := make([]float64, batchSize)
		for i := 0; i < batchSize; i++ {
			idx := rand.Intn(totalSamples)
			batchZones[i] = allZones[idx]
			batchFeatures[i] = allFeatures[idx]
			batchTargets[i] = allTargets[idx]
		}

		localNet := nn.NewNetFromConfig(cfg)
		localNet.LoadWeights(t.Weights)

		CheckZones(batchZones)
		CheckFeatures(batchFeatures)
		CheckTargets(batchTargets)
		CheckWeights(t.Weights)

		preds := localNet.Forward(batchZones, batchFeatures)

		if CheckTensor("PREDICTIONS", preds) {
			log.Fatal("NaN detected in predictions")
		}

		loss, gradOut := nn.MSELoss(preds, batchTargets)

		if math.IsNaN(loss) || math.IsInf(loss, 0) {
			log.Fatalf("[NAN LOSS] loss=%v", loss)
		}

		if CheckTensor("LOSS_GRAD", gradOut) {
			log.Fatal("NaN detected in loss gradients")
		}

		localNet.Backward(batchZones, gradOut)

		grads := localNet.ExtractGradients(t.WorkerID, loss, batchSize)

		if CheckGradients(grads) {
			log.Fatal("NaN detected in gradients")
		}
		startGrad := time.Now()
		t.sendGradients(grads)
		gradDur := time.Since(startGrad)

		t.metricsMu.Lock()
		t.currentStep = step
		t.currentLoss = loss
		t.lastGradientTime = time.Now()
		t.gradientsSent++
		t.totalGradientTime += gradDur
		t.samplesProcessed += batchSize
		t.metricsMu.Unlock()

		select {
		case newWeights := <-t.WeightsChan:
			t.Weights = newWeights
			if step%50 == 0 {
				log.Printf("[TRAINER %s] Step %d | Loss: %.4f", t.WorkerID, step, loss)
			}
		case <-time.After(30 * time.Second):
			log.Printf("[TRAINER ERROR %s] Timeout waiting for Parameter Server!", t.WorkerID)
			return
		}
	}

	log.Printf("[TRAINER %s] Training complete.", t.WorkerID)
}

func (t *Trainer) sendGradients(grads models.Gradients) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(grads); err != nil {
		log.Printf("[TRAINER ERROR] encode failed: %v", err)
		return
	}
	payload := buf.Bytes()

	t.SendMsg(models.MsgGradients, payload)
}

func buildStaticDataset(docs []StaticDoc) ([]int, [][]float64, []float64) {
	zones := make([]int, len(docs))
	features := make([][]float64, len(docs))
	targets := make([]float64, len(docs))

	for i, doc := range docs {
		zones[i] = doc.Zone
		targets[i] = math.Log1p(float64(doc.Count))

		h := float64(doc.Hour)
		wd := float64(doc.Weekday)
		m := float64(doc.Month)

		sinH, cosH := math.Sincos(2 * math.Pi * h / 24.0)
		sinWd, cosWd := math.Sincos(2 * math.Pi * wd / 7.0)
		sinM, cosM := math.Sincos(2 * math.Pi * m / 12.0)

		features[i] = []float64{sinH, cosH, sinWd, cosWd, sinM, cosM}
	}
	return zones, features, targets
}

func buildTemporalDataset(docs []TemporalDoc) ([]int, [][]float64, []float64) {
	byZone := make(map[int][]TemporalDoc)
	for _, doc := range docs {
		byZone[doc.Zone] = append(byZone[doc.Zone], doc)
	}

	var allZones []int
	var allFeatures [][]float64
	var allTargets []float64

	for z, zDocs := range byZone {
		sort.Slice(zDocs, func(i, j int) bool {
			di, dj := zDocs[i], zDocs[j]
			if di.Year != dj.Year {
				return di.Year < dj.Year
			}
			if di.Month != dj.Month {
				return di.Month < dj.Month
			}
			if di.Day != dj.Day {
				return di.Day < dj.Day
			}
			return di.Hour < dj.Hour
		})

		timeToCount := make(map[int64]float64)
		for _, d := range zDocs {
			ts := time.Date(int(d.Year), time.Month(d.Month), int(d.Day), int(d.Hour), 0, 0, 0, time.UTC).Unix()
			timeToCount[ts] = float64(d.Count)
		}

		for _, d := range zDocs {
			dt := time.Date(int(d.Year), time.Month(d.Month), int(d.Day), int(d.Hour), 0, 0, 0, time.UTC)

			history := make([]float64, 24)
			missing := false
			for h := 1; h <= 24; h++ {
				prevTs := dt.Add(time.Duration(-h) * time.Hour).Unix()
				val, exists := timeToCount[prevTs]
				if !exists {
					missing = true
					break
				}
				history[24-h] = math.Log1p(val)
			}

			lagTs := dt.Add(-168 * time.Hour).Unix()
			lagVal, lagExists := timeToCount[lagTs]
			if missing || !lagExists {
				continue
			}

			h := float64(d.Hour)
			wd := float64(dt.Weekday())
			m := float64(d.Month)
			sinH, cosH := math.Sincos(2 * math.Pi * h / 24.0)
			sinWd, cosWd := math.Sincos(2 * math.Pi * wd / 7.0)
			sinM, cosM := math.Sincos(2 * math.Pi * m / 12.0)

			feat := make([]float64, 0, 31)
			feat = append(feat, sinH, cosH, sinWd, cosWd, sinM, cosM)
			feat = append(feat, history...)
			feat = append(feat, math.Log1p(lagVal))

			allZones = append(allZones, z)
			allFeatures = append(allFeatures, feat)
			allTargets = append(allTargets, math.Log1p(float64(d.Count)))
		}
	}

	return allZones, allFeatures, allTargets
}
