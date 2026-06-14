package cluster

import (
	"api/internal/models"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

type WorkerConn struct {
	ID   string
	Conn net.Conn
	Mu   sync.Mutex
}

func (wc *WorkerConn) Write(data []byte) error {
	wc.Mu.Lock()
	defer wc.Mu.Unlock()
	_, err := wc.Conn.Write(data)
	return err
}

type MessageHandler func(workerID string, msgType uint8, payload []byte)
type WorkerEventHandler func(workerID string)

type NetworkManager struct {
	port          string
	clusterMu     sync.RWMutex
	workers       map[string]*WorkerConn
	order         []string
	index         int

	metricsMu     sync.RWMutex
	workerMetrics map[string]*models.WorkerDashboardMetrics

	OnMessage      MessageHandler
	OnWorkerRemove WorkerEventHandler
}

func NewNetworkManager(port string) *NetworkManager {
	return &NetworkManager{
		port:          port,
		workers:       make(map[string]*WorkerConn),
		order:         make([]string, 0),
		workerMetrics: make(map[string]*models.WorkerDashboardMetrics),
	}
}

func (nm *NetworkManager) StartTCPServer() {
	listener, err := net.Listen("tcp", nm.port)
	if err != nil {
		log.Fatalf("[CLUSTER ERROR] %v", err)
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		go nm.handleWorker(conn)
	}
}

func (nm *NetworkManager) handleWorker(conn net.Conn) {
	var regLine []byte
	buf := make([]byte, 1)
	for {
		_, err := conn.Read(buf)
		if err != nil {
			conn.Close()
			return
		}
		if buf[0] == '\n' {
			break
		}
		regLine = append(regLine, buf[0])
	}

	regStr := strings.TrimSpace(string(regLine))
	var reg models.WorkerRegistration
	workerID := regStr
	if err := json.Unmarshal([]byte(regStr), &reg); err == nil {
		workerID = reg.WorkerID
	} else {
		reg.WorkerID = workerID
	}

	wc := &WorkerConn{ID: workerID, Conn: conn}

	nm.clusterMu.Lock()
	if old, ok := nm.workers[workerID]; ok {
		old.Conn.Close()
		nm.removeFromOrder(workerID)
	}
	nm.workers[workerID] = wc
	nm.order = append(nm.order, workerID)
	nm.clusterMu.Unlock()

	nm.metricsMu.Lock()
	nm.workerMetrics[workerID] = &models.WorkerDashboardMetrics{
		WorkerID:           workerID,
		WorkerRegistration: reg,
		LastHeartbeat:      time.Now(),
		Status:             "Online",
	}
	nm.metricsMu.Unlock()

	go nm.listenWorker(workerID, wc)
}

func (nm *NetworkManager) listenWorker(workerID string, wc *WorkerConn) {
	defer nm.RemoveWorker(workerID)

	for {
		header := make([]byte, 5)
		_, err := io.ReadFull(wc.Conn, header)
		if err != nil {
			return
		}

		msgType := header[0]
		size := binary.BigEndian.Uint32(header[1:5])

		payload := make([]byte, size)
		_, err = io.ReadFull(wc.Conn, payload)
		if err != nil {
			return
		}

		if msgType == MsgMetrics {
			nm.handleMetrics(workerID, payload)
			continue
		}

		if nm.OnMessage != nil {
			nm.OnMessage(workerID, msgType, payload)
		}
	}
}

func (nm *NetworkManager) handleMetrics(workerID string, payload []byte) {
	var runMetrics models.WorkerRuntimeMetrics
	if err := json.Unmarshal(payload, &runMetrics); err != nil {
		log.Printf("[MASTER ERROR] decode metrics failed: %v", err)
		return
	}

	nm.metricsMu.Lock()
	if dash, ok := nm.workerMetrics[workerID]; ok {
		dash.WorkerRuntimeMetrics = runMetrics
		dash.LastHeartbeat = time.Now()
	}
	nm.metricsMu.Unlock()
}

func (nm *NetworkManager) RemoveWorker(id string) {
	nm.clusterMu.Lock()
	wc, ok := nm.workers[id]
	if !ok {
		nm.clusterMu.Unlock()
		return
	}

	wc.Conn.Close()
	delete(nm.workers, id)
	nm.removeFromOrder(id)
	nm.clusterMu.Unlock()

	nm.metricsMu.Lock()
	if dash, ok := nm.workerMetrics[id]; ok {
		dash.Status = "Offline"
	}
	nm.metricsMu.Unlock()

	if nm.OnWorkerRemove != nil {
		nm.OnWorkerRemove(id)
	}
}

func (nm *NetworkManager) removeFromOrder(id string) {
	for i, v := range nm.order {
		if v == id {
			nm.order = append(nm.order[:i], nm.order[i+1:]...)
			if nm.index > i && nm.index > 0 {
				nm.index--
			}
			break
		}
	}
}

func (nm *NetworkManager) GetNextWorker() (string, *WorkerConn, error) {
	nm.clusterMu.Lock()
	defer nm.clusterMu.Unlock()

	if len(nm.order) == 0 {
		return "", nil, fmt.Errorf("no workers")
	}

	for i := 0; i < len(nm.order); i++ {
		if nm.index >= len(nm.order) {
			nm.index = 0
		}

		id := nm.order[nm.index]
		nm.index++

		if wc, ok := nm.workers[id]; ok {
			return id, wc, nil
		}
	}

	return "", nil, fmt.Errorf("no workers activos")
}

func (nm *NetworkManager) GetActiveWorkers() []string {
	nm.clusterMu.RLock()
	defer nm.clusterMu.RUnlock()

	out := make([]string, len(nm.order))
	copy(out, nm.order)
	return out
}

func (nm *NetworkManager) GetDashboardMetrics() []models.WorkerDashboardMetrics {
	nm.metricsMu.RLock()
	defer nm.metricsMu.RUnlock()

	var metrics []models.WorkerDashboardMetrics
	for _, m := range nm.workerMetrics {
		metrics = append(metrics, *m)
	}
	return metrics
}

func (nm *NetworkManager) Broadcast(header, payload []byte) {
	nm.clusterMu.RLock()
	currentWorkers := make([]*WorkerConn, 0, len(nm.workers))
	for _, wc := range nm.workers {
		currentWorkers = append(currentWorkers, wc)
	}
	nm.clusterMu.RUnlock()

	for _, wc := range currentWorkers {
		go func(w *WorkerConn) {
			w.Mu.Lock()
			_, _ = w.Conn.Write(header)
			_, _ = w.Conn.Write(payload)
			w.Mu.Unlock()
		}(wc)
	}
}

func (nm *NetworkManager) GetWorkersSnapshot() []*WorkerConn {
	nm.clusterMu.RLock()
	defer nm.clusterMu.RUnlock()
	currentWorkers := make([]*WorkerConn, 0, len(nm.workers))
	for _, wc := range nm.workers {
		currentWorkers = append(currentWorkers, wc)
	}
	return currentWorkers
}
