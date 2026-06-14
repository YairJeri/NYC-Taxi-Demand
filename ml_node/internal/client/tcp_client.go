package client

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"encoding/json"
	"io"
	"log"
	"ml_node/internal/models"
	"ml_node/internal/tasks"
	"net"
	"os"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"go.mongodb.org/mongo-driver/mongo"
)

type TCPClient struct {
	id          string
	apiAddress  string
	mongoClient *mongo.Client
	connMu      sync.Mutex
}

func NewTCPClient(id, apiAddr string, client *mongo.Client) *TCPClient {
	return &TCPClient{
		id:          id,
		apiAddress:  apiAddr,
		mongoClient: client,
	}
}

func (c *TCPClient) Start() {
	for {
		conn, err := net.Dial("tcp", c.apiAddress)
		if err != nil {
			log.Printf("[WORKER] reconectando a API...")
			time.Sleep(3 * time.Second)
			continue
		}

		log.Printf("[WORKER] conectado como %s", c.id)
		hostname, _ := os.Hostname()
		cores, _ := cpu.Counts(true)
		vmStat, _ := mem.VirtualMemory()

		reg := models.WorkerRegistration{
			WorkerID:      c.id,
			Hostname:      hostname,
			CPUCores:      cores,
			TotalRAMBytes: vmStat.Total,
			WorkerVersion: "1.0",
		}

		regBytes, err := json.Marshal(reg)
		if err == nil {
			_, err = conn.Write(append(regBytes, '\n'))
		}

		if err != nil {
			conn.Close()
			time.Sleep(3 * time.Second)
			continue
		}

		go c.metricsLoop(conn)
		c.listen(conn)

		log.Printf("[WORKER] desconectado, reintentando...")
		conn.Close()
		time.Sleep(3 * time.Second)
	}
}

func (c *TCPClient) listen(conn net.Conn) {

	header := make([]byte, 5)

	for {
		_, err := io.ReadFull(conn, header)
		if err != nil {
			log.Printf("[WORKER] conexión cerrada: %v", err)
			return
		}

		msgType := header[0]
		length := binary.BigEndian.Uint32(header[1:5])

		switch msgType {

		case models.MsgData:
			tasks.HandleData(conn, length)
		case models.MsgTrainStart:
			c.handleTrainStart(conn, length)
		case models.MsgWeightsUpdate:
			c.handleWeightsUpdate(conn, length)

		default:
			log.Printf("[WORKER] tipo desconocido: %d", msgType)
		}
	}
}

func (c *TCPClient) handleTrainStart(conn net.Conn, length uint32) {
	log.Printf("[WORKER %s] iniciando entrenamiento (Data Parallelism)...", c.id)

	payload := make([]byte, length)
	if _, err := io.ReadFull(conn, payload); err != nil {
		log.Printf("[WORKER ERROR] failed to read global weights: %v", err)
		return
	}

	var startPayload models.TrainStartPayload
	buf := bytes.NewReader(payload)
	dec := gob.NewDecoder(buf)
	if err := dec.Decode(&startPayload); err != nil {
		log.Printf("[WORKER ERROR] failed to decode TrainStartPayload: %v", err)
		return
	}

	// Iniciar el loop de entrenamiento asíncrono
	go tasks.RunTrainingLoop(c.id, c.SendMessageFunc(conn), c.mongoClient, startPayload)
}

func (c *TCPClient) SendMessageFunc(conn net.Conn) func(msgType byte, payload []byte) error {
	return func(msgType byte, payload []byte) error {
		header := make([]byte, 5)
		header[0] = msgType
		binary.BigEndian.PutUint32(header[1:5], uint32(len(payload)))

		c.connMu.Lock()
		defer c.connMu.Unlock()
		_, err := conn.Write(header)
		if err == nil {
			_, err = conn.Write(payload)
		}
		return err
	}
}

func (c *TCPClient) handleWeightsUpdate(conn net.Conn, length uint32) {
	payload := make([]byte, length)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return
	}

	var weights models.GlobalWeights
	buf := bytes.NewReader(payload)
	dec := gob.NewDecoder(buf)
	if err := dec.Decode(&weights); err != nil {
		return
	}

	// Update local trainer
	tasks.UpdateTrainerWeights(weights)
}

func (c *TCPClient) metricsLoop(conn net.Conn) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		vmStat, _ := mem.VirtualMemory()
		cpuPercents, _ := cpu.Percent(0, false)
		var cpuUtil float64
		if len(cpuPercents) > 0 {
			cpuUtil = cpuPercents[0]
		}

		trainerInfo := tasks.GetTrainerMetrics()

		metrics := models.WorkerRuntimeMetrics{
			WorkerID:            c.id,
			CurrentStep:         trainerInfo.CurrentStep,
			CurrentLoss:         trainerInfo.CurrentLoss,
			CPUUtilization:      cpuUtil,
			RAMUsageBytes:       vmStat.Used,
			LastGradientTime:    trainerInfo.LastGradientTime,
			GradientsSent:       trainerInfo.GradientsSent,
			AvgGradientInterval: trainerInfo.AvgGradientInterval,
			SamplesProcessed:    trainerInfo.SamplesProcessed,
		}

		metricsBytes, err := json.Marshal(metrics)
		if err != nil {
			continue
		}

		send := c.SendMessageFunc(conn)
		if err := send(models.MsgMetrics, metricsBytes); err != nil {
			return
		}
	}
}
