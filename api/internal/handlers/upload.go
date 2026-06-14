package handlers

import (
	"api/internal/cluster"
	"bufio"
	"fmt"
	"net/http"
	"os"
	"strings"
)

const ChunkSize = 10000

type UploadHandler struct {
	cluster *cluster.WorkerHub
}

func NewUploadHandler(h *cluster.WorkerHub) *UploadHandler {
	return &UploadHandler{cluster: h}
}

func (h *UploadHandler) HandleCSVUpload(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	path := "./data/2023_Yellow_Taxi_Trip_Data_20260528.csv"
	jobID := "job-1"

	err := h.cluster.TryStartJob(jobID, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	fmt.Println("[UPLOAD] Workers activos:", h.cluster.GetActiveWorkers())

	go h.processFile(jobID, path)

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("Job iniciado: " + jobID))
}

func (h *UploadHandler) processFile(jobID, path string) {

	fmt.Println("[UPLOAD] Procesando archivo:", path)

	file, err := os.Open(path)
	if err != nil {
		fmt.Println("[UPLOAD ERROR] No se pudo abrir archivo:", err)
		return
	}
	defer file.Close()

	reader := bufio.NewReader(file)

	chunk := make([]string, 0, ChunkSize)
	chunkID := 0

	lineCount := 0

	for {
		line, err := reader.ReadString('\n')

		line = strings.TrimSpace(line)
		if line != "" {
			lineCount++
			chunk = append(chunk, line)

			if len(chunk) >= ChunkSize {
				chunkID++

				h.sendChunk(chunkID, chunk)

				chunk = make([]string, 0, ChunkSize)
			}
		}

		if err != nil {
			break
		}
	}

	if len(chunk) > 0 {
		chunkID++
		fmt.Printf("[UPLOAD] Enviando último chunk %d (%d líneas totales)\n", chunkID, lineCount)

		h.sendChunk(chunkID, chunk)
	}

	fmt.Println("[UPLOAD] FIN procesamiento. Total chunks:", chunkID)
	h.cluster.SetTotalChunks(jobID, chunkID)
}

func (h *UploadHandler) sendChunk(chunkID int, chunk []string) {

	err := h.cluster.SendChunk(chunkID, chunk)
	if err != nil {
		fmt.Println("[UPLOAD ERROR] fallo enviando chunk:", err)
		return
	}
}
