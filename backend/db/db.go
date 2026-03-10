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
	CapturedAt   time.Time `json:"captured_at"`
	CarsCount    float64   `json:"cars_count"`
	SentLastHour float64   `json:"sent_last_hour"`
	SentLast24h  float64   `json:"sent_last_24h"`
}

// VehicleRow represents a vehicle record.
type VehicleRow struct {
	RegNumber       string    `json:"reg_number"`
	QueueType       string    `json:"queue_type"`
	Status          string    `json:"status"`
	RegisteredAt    time.Time `json:"registered_at"`
	StatusChangedAt time.Time `json:"status_changed_at"`
}

// StatusChange represents a single status observation within a crossing.
type StatusChange struct {
	Status     string    `json:"status"`
	DetectedAt time.Time `json:"detected_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

// CrossingHistory represents a vehicle crossing with all its status changes.
type CrossingHistory struct {
	CrossingID    int64          `json:"crossing_id"`
	ZoneID        string         `json:"zone_id"`
	QueueType     string         `json:"queue_type"`
	RegisteredAt  time.Time      `json:"registered_at"`
	FirstSeenAt   time.Time      `json:"first_seen_at"`
	LastSeenAt    time.Time      `json:"last_seen_at"`
	CurrentStatus string         `json:"current_status"`
	IsActive      bool           `json:"is_active"`
	StatusChanges []StatusChange `json:"status_changes"`
}

// VehicleSearchResult represents a vehicle found by search.
type VehicleSearchResult struct {
	RegNumber string    `json:"reg_number"`
	ZoneID    string    `json:"zone_id"`
	Status    string    `json:"status"`
	LastSeen  time.Time `json:"last_seen"`
}

// VehicleListParams holds parameters for the ListVehicles query.
type VehicleListParams struct {
	Query  string // search filter (ILIKE), empty = no filter
	ZoneID string // zone filter, empty = all zones
	Sort   string // column: "reg_number" | "last_seen_at" | "zone_id" (default "last_seen_at")
	Order  string // "asc" | "desc" (default "desc")
	Limit  int    // 1-100, default 50
	Offset int    // default 0
}

// VehicleListResult holds paginated vehicle list results.
type VehicleListResult struct {
	Data  []VehicleSearchResult `json:"data"`
	Total int                   `json:"total"`
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

// GetCurrentVehicles returns active vehicles for a zone.
func (d *DB) GetCurrentVehicles(ctx context.Context, zoneID string) ([]VehicleRow, error) {
	query := `
		SELECT vc.reg_number, vc.queue_type, vc.current_status,
		       COALESCE(vc.registered_at, '1970-01-01T00:00:00Z'),
		       COALESCE(sc.detected_at, '1970-01-01T00:00:00Z')
		FROM vehicle_crossings vc
		LEFT JOIN LATERAL (
		    SELECT detected_at FROM vehicle_status_changes
		    WHERE crossing_id = vc.id
		    ORDER BY detected_at DESC LIMIT 1
		) sc ON true
		WHERE vc.zone_id = $1 AND vc.is_active = true
		ORDER BY vc.registered_at NULLS LAST`

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

// GetVehicleHistory returns vehicles for a zone within a time range (one per reg_number, most recent).
func (d *DB) GetVehicleHistory(ctx context.Context, zoneID string, from, to time.Time) ([]VehicleRow, error) {
	query := `
		SELECT DISTINCT ON (vc.reg_number) vc.reg_number, vc.queue_type, vc.current_status,
		       COALESCE(vc.registered_at, '1970-01-01T00:00:00Z'),
		       COALESCE(sc.detected_at, '1970-01-01T00:00:00Z')
		FROM vehicle_crossings vc
		LEFT JOIN LATERAL (
		    SELECT detected_at FROM vehicle_status_changes
		    WHERE crossing_id = vc.id
		    ORDER BY detected_at DESC LIMIT 1
		) sc ON true
		WHERE vc.zone_id = $1
		  AND vc.first_seen_at <= $3
		  AND vc.last_seen_at >= $2
		ORDER BY vc.reg_number, vc.last_seen_at DESC`

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

// GetVehicleHistoryGrouped returns all crossings for a reg_number with status changes nested.
// If zoneID is empty, returns crossings across all zones.
func (d *DB) GetVehicleHistoryGrouped(ctx context.Context, regNumber string, zoneID string) ([]CrossingHistory, error) {
	rows, err := d.Pool.Query(ctx,
		`SELECT id, zone_id, queue_type,
		        COALESCE(registered_at, '1970-01-01T00:00:00Z'),
		        first_seen_at, last_seen_at, current_status, is_active
		 FROM vehicle_crossings
		 WHERE reg_number = $1
		   AND ($2 = '' OR zone_id = $2)
		 ORDER BY first_seen_at ASC`,
		regNumber, zoneID,
	)
	if err != nil {
		return nil, fmt.Errorf("query crossings: %w", err)
	}
	defer rows.Close()

	var crossings []CrossingHistory
	crossingByID := make(map[int64]*CrossingHistory)

	for rows.Next() {
		var c CrossingHistory
		if err := rows.Scan(&c.CrossingID, &c.ZoneID, &c.QueueType, &c.RegisteredAt,
			&c.FirstSeenAt, &c.LastSeenAt, &c.CurrentStatus, &c.IsActive); err != nil {
			return nil, fmt.Errorf("scan crossing: %w", err)
		}
		c.StatusChanges = []StatusChange{}
		crossings = append(crossings, c)
		crossingByID[c.CrossingID] = &crossings[len(crossings)-1]
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	if len(crossings) == 0 {
		return []CrossingHistory{}, nil
	}

	ids := make([]int64, len(crossings))
	for i, c := range crossings {
		ids[i] = c.CrossingID
	}

	scRows, err := d.Pool.Query(ctx,
		`SELECT crossing_id, status, detected_at, last_seen_at
		 FROM vehicle_status_changes
		 WHERE crossing_id = ANY($1)
		 ORDER BY crossing_id, detected_at ASC`,
		ids,
	)
	if err != nil {
		return nil, fmt.Errorf("query status changes: %w", err)
	}
	defer scRows.Close()

	for scRows.Next() {
		var crossingID int64
		var sc StatusChange
		if err := scRows.Scan(&crossingID, &sc.Status, &sc.DetectedAt, &sc.LastSeenAt); err != nil {
			return nil, fmt.Errorf("scan status change: %w", err)
		}
		if c, ok := crossingByID[crossingID]; ok {
			c.StatusChanges = append(c.StatusChanges, sc)
		}
	}
	if err := scRows.Err(); err != nil {
		return nil, fmt.Errorf("status change rows: %w", err)
	}

	return crossings, nil
}

// ListVehicles returns a paginated, sortable, filterable list of vehicles.
func (d *DB) ListVehicles(ctx context.Context, params VehicleListParams) (*VehicleListResult, error) {
	// Whitelist sort columns
	sortColumns := map[string]string{
		"reg_number":   "reg_number",
		"last_seen_at": "last_seen_at",
		"zone_id":      "zone_id",
	}
	sortCol := "last_seen_at"
	if col, ok := sortColumns[params.Sort]; ok {
		sortCol = col
	}

	order := "DESC"
	if params.Order == "asc" {
		order = "ASC"
	}

	limit := params.Limit
	if limit < 1 || limit > 100 {
		limit = 50
	}

	offset := params.Offset
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(`
		WITH latest AS (
			SELECT DISTINCT ON (reg_number, zone_id)
			       reg_number, zone_id, current_status, last_seen_at
			FROM vehicle_crossings
			WHERE ($1 = '' OR reg_number ILIKE '%%' || $1 || '%%')
			  AND ($2 = '' OR zone_id = $2)
			ORDER BY reg_number, zone_id, last_seen_at DESC
		)
		SELECT reg_number, zone_id, current_status, last_seen_at, COUNT(*) OVER() AS total
		FROM latest
		ORDER BY %s %s
		LIMIT $3 OFFSET $4`, sortCol, order)

	rows, err := d.Pool.Query(ctx, query, params.Query, params.ZoneID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list vehicles: %w", err)
	}
	defer rows.Close()

	var results []VehicleSearchResult
	var total int
	for rows.Next() {
		var r VehicleSearchResult
		if err := rows.Scan(&r.RegNumber, &r.ZoneID, &r.Status, &r.LastSeen, &total); err != nil {
			return nil, fmt.Errorf("scan vehicle list result: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	if results == nil {
		results = []VehicleSearchResult{}
	}
	return &VehicleListResult{Data: results, Total: total}, nil
}

// ScanZoneWithCount is a helper for tests — not exported, pgx uses rows.Scan directly.
var _ pgx.Rows = nil // ensure pgx import
