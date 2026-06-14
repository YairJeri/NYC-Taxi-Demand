package tasks

import (
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"io"
	"log"
	"ml_node/internal/models"
	"net"
	"strconv"
	"time"
)

type Worker struct {
	staticAgg   map[models.StaticKey]int
	temporalAgg map[models.TemporalKey]int
}

func NewWorker() *Worker {
	return &Worker{
		staticAgg:   make(map[models.StaticKey]int),
		temporalAgg: make(map[models.TemporalKey]int),
	}
}

func HandleData(conn net.Conn, length uint32) {

	start := time.Now()

	data := make([]byte, length)

	_, err := io.ReadFull(conn, data)
	if err != nil {
		log.Printf("[WORKER] read error: %v", err)
		return
	}

	buf := bytes.NewReader(data)

	var chunkID uint32

	if err := binary.Read(
		buf,
		binary.BigEndian,
		&chunkID,
	); err != nil {
		return
	}

	log.Printf("[WORKER] received %d bytes from chunk %d", length, chunkID)
	r := csv.NewReader(buf)

	w := NewWorker()

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		w.processRecord(record)

	}

	if err := w.sendAggregationResult(conn, chunkID); err != nil {
		log.Printf("[WORKER] send result error: %v", err)
		return
	}

	log.Printf("[WORKER DONE] duration=%s", time.Since(start))
}

func (w *Worker) processRecord(f []string) {

	pickup, err := time.Parse("01/02/2006 03:04:05 PM", f[1])
	if err != nil {
		return
	}

	zone, _ := strconv.Atoi(f[7])

	sk := models.StaticKey{
		Zone:    zone,
		Hour:    pickup.Hour(),
		Weekday: int(pickup.Weekday()),
		Month:   int(pickup.Month()),
	}

	w.staticAgg[sk]++

	tk := models.TemporalKey{
		Zone:  zone,
		Hour:  uint8(pickup.Hour()),
		Day:   uint8(pickup.Day()),
		Month: uint8(pickup.Month()),
		Year:  uint16(pickup.Year()),
	}

	w.temporalAgg[tk]++
}

func (w *Worker) sendAggregationResult(conn net.Conn, chunkID uint32) error {
	var buf bytes.Buffer

	// ======================
	// STATIC AGGREGATION
	// ======================
	if err := binary.Write(&buf, binary.BigEndian, chunkID); err != nil {
		return err
	}
	if err := binary.Write(&buf, binary.BigEndian, uint32(len(w.staticAgg))); err != nil {
		return err
	}

	for k, v := range w.staticAgg {
		if err := binary.Write(&buf, binary.BigEndian, int32(k.Zone)); err != nil {
			return err
		}
		if err := binary.Write(&buf, binary.BigEndian, int32(k.Hour)); err != nil {
			return err
		}
		if err := binary.Write(&buf, binary.BigEndian, int32(k.Weekday)); err != nil {
			return err
		}
		if err := binary.Write(&buf, binary.BigEndian, int32(k.Month)); err != nil {
			return err
		}
		if err := binary.Write(&buf, binary.BigEndian, int64(v)); err != nil {
			return err
		}
	}

	// ======================
	// TEMPORAL AGGREGATION
	// ======================
	if err := binary.Write(&buf, binary.BigEndian, uint32(len(w.temporalAgg))); err != nil {
		return err
	}

	for k, v := range w.temporalAgg {
		if err := binary.Write(&buf, binary.BigEndian, int32(k.Zone)); err != nil {
			return err
		}
		if err := binary.Write(&buf, binary.BigEndian, k.Hour); err != nil {
			return err
		}
		if err := binary.Write(&buf, binary.BigEndian, k.Day); err != nil {
			return err
		}
		if err := binary.Write(&buf, binary.BigEndian, k.Month); err != nil {
			return err
		}
		if err := binary.Write(&buf, binary.BigEndian, k.Year); err != nil {
			return err
		}
		if err := binary.Write(&buf, binary.BigEndian, int64(v)); err != nil {
			return err
		}
	}

	payload := buf.Bytes()

	header := make([]byte, 5)
	header[0] = models.MsgAggResult
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))

	if _, err := conn.Write(header); err != nil {
		return err
	}
	if _, err := conn.Write(payload); err != nil {
		return err
	}

	return nil
}
