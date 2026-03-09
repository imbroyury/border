package scraper

import "time"

// CheckpointEntry represents a checkpoint from the /checkpoint API.
type CheckpointEntry struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Address         string `json:"address"`
	Phone           string `json:"phone"`
	CountAll        int    `json:"countAll"`
	CountCar        int    `json:"countCar"`
	CountTruck      int    `json:"countTruck"`
	CountBus        int    `json:"countBus"`
	CountMotorcycle int    `json:"countMotorcycle"`
	CountLiveQueue  int    `json:"countLiveQueue"`
	CountBookings   int    `json:"countBookings"`
	CountPriority   int    `json:"countPriority"`
}

// CheckpointResponse is the response from the /checkpoint API.
type CheckpointResponse struct {
	Result []CheckpointEntry `json:"result"`
}

// VehicleQueueEntry is a single vehicle in a queue from the /monitoring-new API.
type VehicleQueueEntry struct {
	RegNum           string `json:"regnum"`
	Status           int    `json:"status"`
	OrderID          *int   `json:"order_id"`
	TypeQueue        int    `json:"type_queue"`
	RegistrationDate string `json:"registration_date"`
	ChangedDate      string `json:"changed_date"`
}

// MonitoringInfo contains checkpoint info from the /monitoring-new API.
type MonitoringInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	NameEn  string `json:"nameEn"`
	Address string `json:"address"`
	Phone   string `json:"phone"`
	IsBts   int    `json:"isBts"`
}

// MonitoringResponse is the response from the /monitoring-new API.
type MonitoringResponse struct {
	Info                MonitoringInfo      `json:"info"`
	CarLiveQueue        []VehicleQueueEntry `json:"carLiveQueue"`
	CarPriority         []VehicleQueueEntry `json:"carPriority"`
	TruckLiveQueue      []VehicleQueueEntry `json:"truckLiveQueue"`
	TruckPriority       []VehicleQueueEntry `json:"truckPriority"`
	BusLiveQueue        []VehicleQueueEntry `json:"busLiveQueue"`
	BusPriority         []VehicleQueueEntry `json:"busPriority"`
	MotorcycleLiveQueue []VehicleQueueEntry `json:"motorcycleLiveQueue"`
	MotorcyclePriority  []VehicleQueueEntry `json:"motorcyclePriority"`
}

// StatisticsResponse is the response from the /monitoring/statistics API.
type StatisticsResponse struct {
	CheckpointID       string `json:"checkpointId"`
	CarLastHour        int    `json:"carLastHour"`
	MotorcycleLastHour int    `json:"motorcycleLastHour"`
	TruckLastHour      int    `json:"truckLastHour"`
	BusLastHour        int    `json:"busLastHour"`
	CarLastDay         int    `json:"carLastDay"`
	TruckLastDay       int    `json:"truckLastDay"`
	BusLastDay         int    `json:"busLastDay"`
	MotorcycleLastDay  int    `json:"motorcycleLastDay"`
}

// ZoneSummaryEntry is the processed summary for one zone (used by main loop).
type ZoneSummaryEntry struct {
	CheckpointID string
	Slug         string
	Name         string
	CarsCount    int
}

// VehicleEntry is a processed vehicle record (used by main loop).
type VehicleEntry struct {
	RegNumber       string
	QueueType       string
	RegisteredAt    time.Time
	StatusChangedAt time.Time
	Status          string
}

// ZoneDetail contains processed detail data for a single zone.
type ZoneDetail struct {
	SentLastHour int
	SentLast24h  int
	Vehicles     []VehicleEntry
}
