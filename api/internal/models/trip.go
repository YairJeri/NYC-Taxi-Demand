package models

import "time"

const (
	MsgData        = 1
	MsgTrain       = 2
	MsgAggResult     = 3
	MsgGradients     = 4
	MsgWeightsUpdate = 5
	MsgMetrics       = 6
)

type Trip struct {
	PickupTime time.Time
	DropTime   time.Time

	PickupLocationID  int
	DropoffLocationID int

	TripDistance float64

	// Features engineered
	Hour    int
	Weekday int

	DurationMin float64
	Speed       float64

	//Another features
	PaymentType int

	RateCodeId     int
	PassengerCount int
	fareAmount     float64
	extra          float64

	mta_tax               float64
	tipAmount             float64
	tollAmount            float64
	improvement_surcharge float64
	congestion_surcharge  float64
	airport_fee           float64
	totalAmount           float64
}

type StaticKey struct {
	Zone    int
	Hour    int
	Weekday int
	Month   int
}

type TemporalKey struct {
	Zone  int
	Hour  uint8
	Day   uint8
	Month uint8
	Year  uint16
}

type StaticDoc struct {
	JobID     string    `bson:"job_id"`
	Zone      int       `bson:"zone"`
	Hour      int       `bson:"hour"`
	Weekday   int       `bson:"weekday"`
	Month     int       `bson:"month"`
	Count     int       `bson:"count"`
	CreatedAt time.Time `bson:"created_at"`
}

type TemporalDoc struct {
	JobID     string    `bson:"job_id"`
	Zone      int       `bson:"zone"`
	Timestamp time.Time `bson:"timestamp"`
	Count     int       `bson:"count"`
	CreatedAt time.Time `bson:"created_at"`
}
