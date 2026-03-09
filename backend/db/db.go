package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Zone represents a border crossing zone.
type Zone struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Border string `json:"border"`
}

// ZoneWithCount extends Zone with latest snapshot info.
type ZoneWithCount struct {
	Zone
	CarsCount    int       `json:"cars_count"`
	LastCaptured time.Time `json:"last_captured"`
}

// SnapshotPoint represents a single (possibly aggregated) snapshot data point.
type SnapshotPoint struct {
	CapturedAt  time.Time `json:"captured_at"`
	CarsCount   float64   `json:"cars_count"`
	SentLastHour float64  `json:"sent_last_hour"`
	SentLast24h float64   `json:"sent_last_24h"`
}

// VehicleRow represents a vehicle record.
type VehicleRow struct {
	RegNumber       string    `json:"reg_number"`
	QueueType       string    `json:"queue_type"`
	Status          string    `json:"status"`
	RegisteredAt    time.Time `json:"registered_at"`
	StatusChangedAt time.Time `json:"status_changed_at"`
}

// DB wraps a pgx connection pool.
type DB struct {
	Pool *pgxpool.Pool
}

// New creates a new DB from a connection string.
func New(ctx context.Context, connStr string) (*DB, error) {
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &DB{Pool: pool}, nil
}

// Close closes the connection pool.
func (d *DB) Close() {
	d.Pool.Close()
}

// GetZones returns all zones with their latest snapshot count.
func (d *DB) GetZones(ctx context.Context) ([]ZoneWithCount, error) {
	query := `
		SELECT z.id, z.name, z.border,
		       COALESCE(s.cars_count, 0),
		       COALESCE(s.captured_at, '1970-01-01T00:00:00Z')
		FROM zones z
		LEFT JOIN LATERAL (
			SELECT captured_at, cars_count
			FROM snapshots
			WHERE zone_id = z.id
			ORDER BY captured_at DESC
			LIMIT 1
		) s ON true
		ORDER BY z.name`

	rows, err := d.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query zones: %w", err)
	}
	defer rows.Close()

	var zones []ZoneWithCount
	for rows.Next() {
		var z ZoneWithCount
		if err := rows.Scan(&z.ID, &z.Name, &z.Border, &z.CarsCount, &z.LastCaptured); err != nil {
			return nil, fmt.Errorf("scan zone: %w", err)
		}
		zones = append(zones, z)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return zones, nil
}

// GetSnapshots returns snapshot data points for a zone, auto-aggregating based on range.
func (d *DB) GetSnapshots(ctx context.Context, zoneID string, from, to time.Time) ([]SnapshotPoint, error) {
	duration := to.Sub(from)

	var query string
	switch {
	case duration <= 24*time.Hour:
		// Raw snapshots
		query = `
			SELECT captured_at, cars_count::float8, sent_last_hour::float8, sent_last_24h::float8
			FROM snapshots
			WHERE zone_id = $1 AND captured_at >= $2 AND captured_at <= $3
			ORDER BY captured_at`
	case duration <= 7*24*time.Hour:
		// Hourly averages
		query = `
			SELECT date_trunc('hour', captured_at) AS captured_at,
			       AVG(cars_count) AS cars_count,
			       AVG(sent_last_hour) AS sent_last_hour,
			       AVG(sent_last_24h) AS sent_last_24h
			FROM snapshots
			WHERE zone_id = $1 AND captured_at >= $2 AND captured_at <= $3
			GROUP BY date_trunc('hour', captured_at)
			ORDER BY captured_at`
	case duration <= 90*24*time.Hour:
		// 6-hour averages
		query = `
			SELECT date_trunc('day', captured_at) + INTERVAL '6 hour' * FLOOR(EXTRACT(HOUR FROM captured_at) / 6) AS captured_at,
			       AVG(cars_count) AS cars_count,
			       AVG(sent_last_hour) AS sent_last_hour,
			       AVG(sent_last_24h) AS sent_last_24h
			FROM snapshots
			WHERE zone_id = $1 AND captured_at >= $2 AND captured_at <= $3
			GROUP BY date_trunc('day', captured_at) + INTERVAL '6 hour' * FLOOR(EXTRACT(HOUR FROM captured_at) / 6)
			ORDER BY captured_at`
	default:
		// Daily averages
		query = `
			SELECT date_trunc('day', captured_at) AS captured_at,
			       AVG(cars_count) AS cars_count,
			       AVG(sent_last_hour) AS sent_last_hour,
			       AVG(sent_last_24h) AS sent_last_24h
			FROM snapshots
			WHERE zone_id = $1 AND captured_at >= $2 AND captured_at <= $3
			GROUP BY date_trunc('day', captured_at)
			ORDER BY captured_at`
	}

	rows, err := d.Pool.Query(ctx, query, zoneID, from, to)
	if err != nil {
		return nil, fmt.Errorf("query snapshots: %w", err)
	}
	defer rows.Close()

	var points []SnapshotPoint
	for rows.Next() {
		var p SnapshotPoint
		if err := rows.Scan(&p.CapturedAt, &p.CarsCount, &p.SentLastHour, &p.SentLast24h); err != nil {
			return nil, fmt.Errorf("scan snapshot: %w", err)
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	if points == nil {
		points = []SnapshotPoint{}
	}
	return points, nil
}

// GetCurrentVehicles returns vehicles from the latest snapshot for a zone.
func (d *DB) GetCurrentVehicles(ctx context.Context, zoneID string) ([]VehicleRow, error) {
	query := `
		SELECT v.reg_number, v.queue_type, v.status,
		       COALESCE(v.registered_at, '1970-01-01T00:00:00Z'),
		       COALESCE(v.status_changed_at, '1970-01-01T00:00:00Z')
		FROM vehicles v
		JOIN snapshots s ON s.id = v.snapshot_id
		WHERE v.zone_id = $1
		  AND s.id = (
		      SELECT id FROM snapshots
		      WHERE zone_id = $1
		      ORDER BY captured_at DESC
		      LIMIT 1
		  )
		ORDER BY v.registered_at`

	rows, err := d.Pool.Query(ctx, query, zoneID)
	if err != nil {
		return nil, fmt.Errorf("query current vehicles: %w", err)
	}
	defer rows.Close()

	var vehicles []VehicleRow
	for rows.Next() {
		var v VehicleRow
		if err := rows.Scan(&v.RegNumber, &v.QueueType, &v.Status, &v.RegisteredAt, &v.StatusChangedAt); err != nil {
			return nil, fmt.Errorf("scan vehicle: %w", err)
		}
		vehicles = append(vehicles, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	if vehicles == nil {
		vehicles = []VehicleRow{}
	}
	return vehicles, nil
}

// GetVehicleHistory returns vehicles for a zone within a time range.
func (d *DB) GetVehicleHistory(ctx context.Context, zoneID string, from, to time.Time) ([]VehicleRow, error) {
	query := `
		SELECT DISTINCT ON (v.reg_number) v.reg_number, v.queue_type, v.status,
		       COALESCE(v.registered_at, '1970-01-01T00:00:00Z'),
		       COALESCE(v.status_changed_at, '1970-01-01T00:00:00Z')
		FROM vehicles v
		JOIN snapshots s ON s.id = v.snapshot_id
		WHERE v.zone_id = $1
		  AND s.captured_at >= $2
		  AND s.captured_at <= $3
		ORDER BY v.reg_number, s.captured_at DESC`

	rows, err := d.Pool.Query(ctx, query, zoneID, from, to)
	if err != nil {
		return nil, fmt.Errorf("query vehicle history: %w", err)
	}
	defer rows.Close()

	var vehicles []VehicleRow
	for rows.Next() {
		var v VehicleRow
		if err := rows.Scan(&v.RegNumber, &v.QueueType, &v.Status, &v.RegisteredAt, &v.StatusChangedAt); err != nil {
			return nil, fmt.Errorf("scan vehicle: %w", err)
		}
		vehicles = append(vehicles, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	if vehicles == nil {
		vehicles = []VehicleRow{}
	}
	return vehicles, nil
}

// VehicleStatusChange represents a single status observation for a vehicle.
type VehicleStatusChange struct {
	CapturedAt      time.Time `json:"captured_at"`
	Status          string    `json:"status"`
	QueueType       string    `json:"queue_type"`
	StatusChangedAt time.Time `json:"status_changed_at"`
}

// GetSingleVehicleHistory returns all status observations for a specific vehicle.
func (d *DB) GetSingleVehicleHistory(ctx context.Context, zoneID, regNumber string) ([]VehicleStatusChange, error) {
	query := `
		SELECT s.captured_at, v.status, v.queue_type,
		       COALESCE(v.status_changed_at, '1970-01-01T00:00:00Z')
		FROM vehicles v
		JOIN snapshots s ON s.id = v.snapshot_id
		WHERE v.zone_id = $1 AND v.reg_number = $2
		ORDER BY s.captured_at ASC`

	rows, err := d.Pool.Query(ctx, query, zoneID, regNumber)
	if err != nil {
		return nil, fmt.Errorf("query single vehicle history: %w", err)
	}
	defer rows.Close()

	var changes []VehicleStatusChange
	for rows.Next() {
		var c VehicleStatusChange
		if err := rows.Scan(&c.CapturedAt, &c.Status, &c.QueueType, &c.StatusChangedAt); err != nil {
			return nil, fmt.Errorf("scan vehicle status change: %w", err)
		}
		changes = append(changes, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	if changes == nil {
		changes = []VehicleStatusChange{}
	}
	return changes, nil
}

// ScanZoneWithCount is a helper for tests — not exported, pgx uses rows.Scan directly.
var _ pgx.Rows = nil // ensure pgx import
