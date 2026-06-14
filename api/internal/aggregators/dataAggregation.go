package aggregators

import (
	"api/internal/models"
	"bytes"
	"encoding/binary"
	"sync"
)

type DataAggregation struct {
	mu          sync.Mutex
	StaticAgg   map[models.StaticKey]int
	TemporalAgg map[models.TemporalKey]int
}

func NewDataAggregation() *DataAggregation {
	return &DataAggregation{
		StaticAgg:   make(map[models.StaticKey]int),
		TemporalAgg: make(map[models.TemporalKey]int),
	}
}

func (d *DataAggregation) MergeWorkerResult(
	static map[models.StaticKey]int,
	temporal map[models.TemporalKey]int,
) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for k, v := range static {
		d.StaticAgg[k] += v
	}

	for k, v := range temporal {
		d.TemporalAgg[k] += v
	}
}

func DecodeAggResult(payload []byte) (
	uint32,
	map[models.StaticKey]int,
	map[models.TemporalKey]int,
	error,
) {
	buf := bytes.NewReader(payload)

	staticAgg := make(map[models.StaticKey]int)
	temporalAgg := make(map[models.TemporalKey]int)

	var chunkID uint32

	if err := binary.Read(buf, binary.BigEndian, &chunkID); err != nil {
		return 0, nil, nil, err
	}

	// ======================
	// STATIC
	// ======================
	var staticLen uint32
	if err := binary.Read(buf, binary.BigEndian, &staticLen); err != nil {
		return 0, nil, nil, err
	}

	for i := uint32(0); i < staticLen; i++ {
		var zone, hour, weekday, month int32
		var count int64

		if err := binary.Read(buf, binary.BigEndian, &zone); err != nil {
			return 0, nil, nil, err
		}
		if err := binary.Read(buf, binary.BigEndian, &hour); err != nil {
			return 0, nil, nil, err
		}
		if err := binary.Read(buf, binary.BigEndian, &weekday); err != nil {
			return 0, nil, nil, err
		}
		if err := binary.Read(buf, binary.BigEndian, &month); err != nil {
			return 0, nil, nil, err
		}
		if err := binary.Read(buf, binary.BigEndian, &count); err != nil {
			return 0, nil, nil, err
		}

		key := models.StaticKey{
			Zone:    int(zone),
			Hour:    int(hour),
			Weekday: int(weekday),
			Month:   int(month),
		}

		staticAgg[key] += int(count)
	}

	// ======================
	// TEMPORAL
	// ======================
	var temporalLen uint32
	if err := binary.Read(buf, binary.BigEndian, &temporalLen); err != nil {
		return 0, nil, nil, err
	}

	for i := uint32(0); i < temporalLen; i++ {
		var zone int32
		var hour, day, month uint8
		var year uint16
		var count int64

		if err := binary.Read(buf, binary.BigEndian, &zone); err != nil {
			return 0, nil, nil, err
		}
		if err := binary.Read(buf, binary.BigEndian, &hour); err != nil {
			return 0, nil, nil, err
		}
		if err := binary.Read(buf, binary.BigEndian, &day); err != nil {
			return 0, nil, nil, err
		}
		if err := binary.Read(buf, binary.BigEndian, &month); err != nil {
			return 0, nil, nil, err
		}
		if err := binary.Read(buf, binary.BigEndian, &year); err != nil {
			return 0, nil, nil, err
		}
		if err := binary.Read(buf, binary.BigEndian, &count); err != nil {
			return 0, nil, nil, err
		}

		key := models.TemporalKey{
			Zone:  int(zone),
			Hour:  hour,
			Day:   day,
			Month: month,
			Year:  year,
		}

		temporalAgg[key] += int(count)
	}

	return chunkID, staticAgg, temporalAgg, nil
}

func (d *DataAggregation) SnapshotAndClear() (map[models.StaticKey]int, map[models.TemporalKey]int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	staticCopy := d.StaticAgg
	temporalCopy := d.TemporalAgg

	d.StaticAgg = make(map[models.StaticKey]int)
	d.TemporalAgg = make(map[models.TemporalKey]int)

	return staticCopy, temporalCopy
}
