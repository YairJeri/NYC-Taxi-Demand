package cluster

import (
	"api/internal/aggregators"
	"api/internal/database"
	"bytes"
	"context"
	"encoding/binary"
	"log"
	"strings"
	"sync"
	"time"
)

type ChunkTask struct {
	ID       int
	Data     []string
	WorkerID string
	Retries  int
}

type CleaningJob struct {
	id              string
	jobType         int
	totalChunks     int
	doneChunks      int
	pendingChunks   map[int]*ChunkTask
	completedChunks map[int]bool

	startTime time.Time

	dataAggregation *aggregators.DataAggregation
	aggregationRepo *database.AggregationRepository
	chunkQueue      chan *ChunkTask

	nm *NetworkManager
	mu sync.Mutex

	onComplete func()
}

func NewCleaningJob(jobID string, jobType int, da *aggregators.DataAggregation, ar *database.AggregationRepository, nm *NetworkManager, onComplete func()) *CleaningJob {
	cj := &CleaningJob{
		id:              jobID,
		jobType:         jobType,
		totalChunks:     -1,
		pendingChunks:   make(map[int]*ChunkTask),
		completedChunks: make(map[int]bool),
		startTime:       time.Now(),
		dataAggregation: da,
		aggregationRepo: ar,
		chunkQueue:      make(chan *ChunkTask, 100),
		nm:              nm,
		onComplete:      onComplete,
	}
	go cj.dispatcherLoop()
	return cj
}

func (cj *CleaningJob) ProcessMessage(workerID string, msgType uint8, payload []byte) {
	if msgType != MsgAggResult {
		return
	}

	chunkID, staticAgg, temporalAgg, err := aggregators.DecodeAggResult(payload)
	if err != nil {
		return
	}

	cj.dataAggregation.MergeWorkerResult(staticAgg, temporalAgg)

	cj.mu.Lock()
	defer cj.mu.Unlock()

	if cj.completedChunks[int(chunkID)] {
		return
	}

	cj.completedChunks[int(chunkID)] = true
	delete(cj.pendingChunks, int(chunkID))
	cj.doneChunks++

	if cj.totalChunks != -1 {
		log.Printf("[JOB PROGRESS] %s -> %d/%d", cj.id, cj.doneChunks, cj.totalChunks)
		if cj.doneChunks >= cj.totalChunks {
			cj.finalizeJob()
		}
	} else {
		log.Printf("[JOB PROGRESS] %s -> %d procesados (Total temporalmente desconocido)", cj.id, cj.doneChunks)
	}
}

func (cj *CleaningJob) SetTotalChunks(total int) {
	cj.mu.Lock()
	defer cj.mu.Unlock()

	cj.totalChunks = total
	log.Printf("[JOB TOTAL] Asignado total de %d chunks para el job %s", total, cj.id)

	if cj.doneChunks >= cj.totalChunks {
		cj.finalizeJob()
	}
}

func (cj *CleaningJob) SendChunk(chunkID int, lines []string) error {
	cj.mu.Lock()
	if cj.completedChunks[chunkID] {
		cj.mu.Unlock()
		return nil
	}
	cj.mu.Unlock()

	task := &ChunkTask{
		ID:   chunkID,
		Data: lines,
	}

	cj.chunkQueue <- task
	return nil
}

func (cj *CleaningJob) dispatcherLoop() {
	for task := range cj.chunkQueue {
		for {
			workerID, wc, err := cj.nm.GetNextWorker()
			if err != nil {
				time.Sleep(1 * time.Second)
				continue
			}

			cj.mu.Lock()
			if cj.completedChunks[task.ID] {
				cj.mu.Unlock()
				break
			}
			task.WorkerID = workerID
			cj.pendingChunks[task.ID] = task
			cj.mu.Unlock()

			var payload bytes.Buffer
			binary.Write(&payload, binary.BigEndian, uint32(task.ID))
			payload.WriteString(strings.Join(task.Data, "\n"))
			data := payload.Bytes()

			header := make([]byte, 5)
			header[0] = MsgData
			binary.BigEndian.PutUint32(header[1:5], uint32(len(data)))

			go func(wConn *WorkerConn, wID string, t *ChunkTask, hBytes, dBytes []byte) {
				wConn.Mu.Lock()
				_, err1 := wConn.Conn.Write(hBytes)
				var err2 error
				if err1 == nil {
					_, err2 = wConn.Conn.Write(dBytes)
				}
				wConn.Mu.Unlock()

				if err1 != nil || err2 != nil {
					cj.nm.RemoveWorker(wID)
					t.Retries++
					if t.Retries <= 5 {
						cj.chunkQueue <- t
					} else {
						log.Printf("[JOB ERROR] Chunk %d descartado tras demasiados reintentos", t.ID)
					}
				}
			}(wc, workerID, task, header, data)

			break
		}
	}
}

func (cj *CleaningJob) finalizeJob() {
	log.Printf("[JOB DONE] %s (%s)", cj.id, time.Since(cj.startTime))

	staticCopy, temporalCopy := cj.dataAggregation.SnapshotAndClear()
	if cj.onComplete != nil {
		cj.onComplete()
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := cj.aggregationRepo.Save(ctx, staticCopy, temporalCopy)
		if err != nil {
			log.Printf("[JOB ERROR] error al guardar resultados: %v", err)
		} else {
			log.Printf("[JOB DONE] Resultados guardados en MongoDB correctamente.")
		}
	}()
}

func (cj *CleaningJob) ReassignWorkerChunks(workerID string) {
	cj.mu.Lock()
	var tasks []*ChunkTask
	for _, t := range cj.pendingChunks {
		if t.WorkerID == workerID {
			tasks = append(tasks, t)
		}
	}
	cj.mu.Unlock()

	for _, task := range tasks {
		cj.chunkQueue <- task
	}
}

func (cj *CleaningJob) GetStatus() map[string]interface{} {
	cj.mu.Lock()
	defer cj.mu.Unlock()
	return map[string]interface{}{
		"isProcessing":    true,
		"processingDone":  cj.doneChunks,
		"processingTotal": cj.totalChunks,
		"processingTime":  time.Since(cj.startTime).Seconds(),
	}
}
