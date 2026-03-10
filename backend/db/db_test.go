package db

import (
	"context"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func upMigrations(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob("../../migrations/*up.sql")
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no migration files found")
	}
	sort.Strings(matches)
	return matches
}

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:18-alpine",
		postgres.WithInitScripts(upMigrations(t)...),
		postgres.WithDatabase("border_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}

	t.Cleanup(func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("terminate container: %v", err)
		}
	})

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get connection string: %v", err)
	}

	database, err := New(ctx, connStr)
	if err != nil {
		t.Fatalf("connect to test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	return database
}

func TestGetZones_ReturnsSeededZones(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	database := setupTestDB(t)
	ctx := context.Background()

	zones, err := database.GetZones(ctx)
	if err != nil {
		t.Fatalf("GetZones: %v", err)
	}

	if len(zones) != 7 {
		t.Fatalf("expected 7 zones, got %d", len(zones))
	}

	// Verify zones are ordered by name
	for i := 1; i < len(zones); i++ {
		if zones[i].Name < zones[i-1].Name {
			t.Errorf("zones not ordered by name: %q < %q", zones[i].Name, zones[i-1].Name)
		}
	}

	// Check default values when no snapshots exist
	for _, z := range zones {
		if z.CarsCount != 0 {
			t.Errorf("zone %s: expected 0 cars_count, got %d", z.ID, z.CarsCount)
		}
	}
}

func TestGetZones_WithSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	database := setupTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC()
	_, err := database.Pool.Exec(ctx,
		"INSERT INTO snapshots (zone_id, captured_at, cars_count, sent_last_hour, sent_last_24h) VALUES ($1, $2, $3, $4, $5)",
		"brest", now, 42, 10, 100,
	)
	if err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}

	zones, err := database.GetZones(ctx)
	if err != nil {
		t.Fatalf("GetZones: %v", err)
	}

	var brest *ZoneWithCount
	for i := range zones {
		if zones[i].ID == "brest" {
			brest = &zones[i]
			break
		}
	}
	if brest == nil {
		t.Fatal("brest not found")
	}
	if brest.CarsCount != 42 {
		t.Errorf("expected 42 cars, got %d", brest.CarsCount)
	}
	if brest.LastCaptured.Before(now.Add(-time.Second)) || brest.LastCaptured.After(now.Add(time.Second)) {
		t.Errorf("unexpected last_captured: %v (expected ~%v)", brest.LastCaptured, now)
	}
}

func TestGetSnapshots_NoData(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	database := setupTestDB(t)
	ctx := context.Background()

	from := time.Now().Add(-1 * time.Hour)
	to := time.Now()

	points, err := database.GetSnapshots(ctx, "brest", from, to)
	if err != nil {
		t.Fatalf("GetSnapshots: %v", err)
	}
	if len(points) != 0 {
		t.Fatalf("expected empty slice, got %d points", len(points))
	}
}

func TestGetSnapshots_RawData(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	database := setupTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC()
	// Insert 4 snapshots within last hour (raw range < 24h)
	for i := 0; i < 4; i++ {
		ts := now.Add(-time.Duration(i*15) * time.Minute)
		_, err := database.Pool.Exec(ctx,
			"INSERT INTO snapshots (zone_id, captured_at, cars_count, sent_last_hour, sent_last_24h) VALUES ($1, $2, $3, $4, $5)",
			"brest", ts, 10+i, 5+i, 50+i,
		)
		if err != nil {
			t.Fatalf("insert snapshot %d: %v", i, err)
		}
	}

	from := now.Add(-2 * time.Hour)
	to := now.Add(time.Minute)

	points, err := database.GetSnapshots(ctx, "brest", from, to)
	if err != nil {
		t.Fatalf("GetSnapshots: %v", err)
	}
	if len(points) != 4 {
		t.Fatalf("expected 4 raw points, got %d", len(points))
	}

	// Verify ordering (ascending)
	for i := 1; i < len(points); i++ {
		if points[i].CapturedAt.Before(points[i-1].CapturedAt) {
			t.Errorf("points not in ascending order")
		}
	}
}

func TestGetSnapshots_HourlyAggregation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	database := setupTestDB(t)
	ctx := context.Background()

	// Insert data spanning 3 days (triggers hourly aggregation for 24h–7d range)
	base := time.Now().UTC().Truncate(time.Hour)
	for day := 0; day < 3; day++ {
		for h := 0; h < 24; h++ {
			ts := base.Add(-time.Duration(day*24+h) * time.Hour)
			_, err := database.Pool.Exec(ctx,
				"INSERT INTO snapshots (zone_id, captured_at, cars_count, sent_last_hour, sent_last_24h) VALUES ($1, $2, $3, $4, $5)",
				"brest", ts, 10, 5, 50,
			)
			if err != nil {
				t.Fatalf("insert: %v", err)
			}
			// Second point in same hour
			ts2 := ts.Add(15 * time.Minute)
			_, err = database.Pool.Exec(ctx,
				"INSERT INTO snapshots (zone_id, captured_at, cars_count, sent_last_hour, sent_last_24h) VALUES ($1, $2, $3, $4, $5)",
				"brest", ts2, 20, 15, 100,
			)
			if err != nil {
				t.Fatalf("insert: %v", err)
			}
		}
	}

	from := base.Add(-3 * 24 * time.Hour)
	to := base.Add(time.Hour)

	points, err := database.GetSnapshots(ctx, "brest", from, to)
	if err != nil {
		t.Fatalf("GetSnapshots: %v", err)
	}

	// Should be hourly aggregated: ~72 buckets (3 days * 24 hours)
	if len(points) < 50 || len(points) > 80 {
		t.Fatalf("expected ~72 hourly points, got %d", len(points))
	}

	// Each hourly avg should be (10+20)/2 = 15 for cars_count
	for _, p := range points {
		if p.CarsCount < 14.9 || p.CarsCount > 15.1 {
			t.Errorf("expected hourly avg ~15, got %f at %v", p.CarsCount, p.CapturedAt)
			break
		}
	}
}

func TestGetSnapshots_SixHourAggregation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	database := setupTestDB(t)
	ctx := context.Background()

	// Insert data spanning 30 days (triggers 6-hour aggregation for 7d–90d range)
	base := time.Now().UTC().Truncate(time.Hour)
	for day := 0; day < 30; day++ {
		ts := base.Add(-time.Duration(day) * 24 * time.Hour)
		_, err := database.Pool.Exec(ctx,
			"INSERT INTO snapshots (zone_id, captured_at, cars_count, sent_last_hour, sent_last_24h) VALUES ($1, $2, $3, $4, $5)",
			"brest", ts, 10, 5, 50,
		)
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	from := base.Add(-31 * 24 * time.Hour)
	to := base.Add(time.Hour)

	points, err := database.GetSnapshots(ctx, "brest", from, to)
	if err != nil {
		t.Fatalf("GetSnapshots: %v", err)
	}

	// 30 data points in 30 days, each in different 6-hour bucket
	if len(points) == 0 {
		t.Fatal("expected non-empty result")
	}
	// Should be fewer buckets than raw points would give for >7d range
	if len(points) > 30 {
		t.Errorf("expected at most 30 6-hour buckets, got %d", len(points))
	}
}

func TestGetSnapshots_DailyAggregation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	database := setupTestDB(t)
	ctx := context.Background()

	// Insert data spanning 100 days (triggers daily aggregation for 90d+ range)
	base := time.Now().UTC().Truncate(time.Hour)
	for day := 0; day < 100; day++ {
		ts := base.Add(-time.Duration(day) * 24 * time.Hour)
		_, err := database.Pool.Exec(ctx,
			"INSERT INTO snapshots (zone_id, captured_at, cars_count, sent_last_hour, sent_last_24h) VALUES ($1, $2, $3, $4, $5)",
			"brest", ts, day, 5, 50,
		)
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	from := base.Add(-101 * 24 * time.Hour)
	to := base.Add(time.Hour)

	points, err := database.GetSnapshots(ctx, "brest", from, to)
	if err != nil {
		t.Fatalf("GetSnapshots: %v", err)
	}

	// One data point per day, daily aggregation = ~100 buckets
	if len(points) < 90 || len(points) > 101 {
		t.Fatalf("expected ~100 daily points, got %d", len(points))
	}
}

func TestGetCurrentVehicles_NoData(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	database := setupTestDB(t)
	ctx := context.Background()

	vehicles, err := database.GetCurrentVehicles(ctx, "brest")
	if err != nil {
		t.Fatalf("GetCurrentVehicles: %v", err)
	}
	if len(vehicles) != 0 {
		t.Fatalf("expected empty slice, got %d vehicles", len(vehicles))
	}
}

func TestGetCurrentVehicles_WithData(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	database := setupTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC()

	// Insert an active crossing with a status change
	var crossingID int64
	err := database.Pool.QueryRow(ctx,
		`INSERT INTO vehicle_crossings (zone_id, reg_number, queue_type, registered_at, first_seen_at, last_seen_at, current_status, is_active)
		 VALUES ($1, $2, $3, $4, $5, $5, $6, true) RETURNING id`,
		"brest", "BB5678-3", "passenger", now.Add(-time.Hour), now, "processing",
	).Scan(&crossingID)
	if err != nil {
		t.Fatalf("insert crossing: %v", err)
	}

	_, err = database.Pool.Exec(ctx,
		`INSERT INTO vehicle_status_changes (crossing_id, status, detected_at, last_seen_at) VALUES ($1, $2, $3, $3)`,
		crossingID, "processing", now,
	)
	if err != nil {
		t.Fatalf("insert status change: %v", err)
	}

	// Insert an inactive crossing (should not appear)
	_, err = database.Pool.Exec(ctx,
		`INSERT INTO vehicle_crossings (zone_id, reg_number, queue_type, first_seen_at, last_seen_at, current_status, is_active)
		 VALUES ($1, $2, $3, $4, $4, $5, false)`,
		"brest", "AA1234-7", "cargo", now.Add(-2*time.Hour), "passed",
	)
	if err != nil {
		t.Fatalf("insert inactive crossing: %v", err)
	}

	vehicles, err := database.GetCurrentVehicles(ctx, "brest")
	if err != nil {
		t.Fatalf("GetCurrentVehicles: %v", err)
	}

	if len(vehicles) != 1 {
		t.Fatalf("expected 1 active vehicle, got %d", len(vehicles))
	}
	if vehicles[0].RegNumber != "BB5678-3" {
		t.Errorf("expected BB5678-3, got %s", vehicles[0].RegNumber)
	}
	if vehicles[0].Status != "processing" {
		t.Errorf("expected processing, got %s", vehicles[0].Status)
	}
}

func TestGetVehicleHistory_NoData(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	database := setupTestDB(t)
	ctx := context.Background()

	from := time.Now().Add(-24 * time.Hour)
	to := time.Now()

	vehicles, err := database.GetVehicleHistory(ctx, "brest", from, to)
	if err != nil {
		t.Fatalf("GetVehicleHistory: %v", err)
	}
	if len(vehicles) != 0 {
		t.Fatalf("expected empty slice, got %d vehicles", len(vehicles))
	}
}

func TestGetVehicleHistory_TimeRangeFiltering(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	database := setupTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC()

	// Old crossing (48h ago)
	_, err := database.Pool.Exec(ctx,
		`INSERT INTO vehicle_crossings (zone_id, reg_number, queue_type, first_seen_at, last_seen_at, current_status, is_active)
		 VALUES ($1, $2, $3, $4, $4, $5, false)`,
		"brest", "OLD-0001", "cargo", now.Add(-48*time.Hour), "passed",
	)
	if err != nil {
		t.Fatalf("insert old crossing: %v", err)
	}

	// Mid crossing (24h ago)
	_, err = database.Pool.Exec(ctx,
		`INSERT INTO vehicle_crossings (zone_id, reg_number, queue_type, first_seen_at, last_seen_at, current_status, is_active)
		 VALUES ($1, $2, $3, $4, $4, $5, false)`,
		"brest", "MID-0002", "cargo", now.Add(-24*time.Hour), "passed",
	)
	if err != nil {
		t.Fatalf("insert mid crossing: %v", err)
	}

	// Recent crossing (1h ago)
	_, err = database.Pool.Exec(ctx,
		`INSERT INTO vehicle_crossings (zone_id, reg_number, queue_type, first_seen_at, last_seen_at, current_status, is_active)
		 VALUES ($1, $2, $3, $4, $4, $5, true)`,
		"brest", "NEW-0003", "cargo", now.Add(-time.Hour), "in_queue",
	)
	if err != nil {
		t.Fatalf("insert new crossing: %v", err)
	}

	// Query only last 25 hours — should get MID and NEW
	from := now.Add(-25 * time.Hour)
	to := now.Add(time.Minute)

	vehicles, err := database.GetVehicleHistory(ctx, "brest", from, to)
	if err != nil {
		t.Fatalf("GetVehicleHistory: %v", err)
	}
	if len(vehicles) != 2 {
		t.Fatalf("expected 2 vehicles in range, got %d", len(vehicles))
	}

	found := map[string]bool{}
	for _, v := range vehicles {
		found[v.RegNumber] = true
	}
	if found["OLD-0001"] {
		t.Error("OLD-0001 should not be in range")
	}
	if !found["MID-0002"] || !found["NEW-0003"] {
		t.Error("expected MID-0002 and NEW-0003 in range")
	}
}

func TestGetVehicleHistoryGrouped_NoData(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	database := setupTestDB(t)
	ctx := context.Background()

	crossings, err := database.GetVehicleHistoryGrouped(ctx, "NONEXISTENT", "")
	if err != nil {
		t.Fatalf("GetVehicleHistoryGrouped: %v", err)
	}
	if len(crossings) != 0 {
		t.Fatalf("expected empty slice, got %d", len(crossings))
	}
}

func TestGetVehicleHistoryGrouped_WithData(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	database := setupTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC()

	// Insert a crossing with two status changes
	var crossingID int64
	err := database.Pool.QueryRow(ctx,
		`INSERT INTO vehicle_crossings (zone_id, reg_number, queue_type, first_seen_at, last_seen_at, current_status, is_active)
		 VALUES ($1, $2, $3, $4, $5, $6, true) RETURNING id`,
		"brest", "TEST-001", "live", now.Add(-2*time.Hour), now, "called",
	).Scan(&crossingID)
	if err != nil {
		t.Fatalf("insert crossing: %v", err)
	}

	_, err = database.Pool.Exec(ctx,
		`INSERT INTO vehicle_status_changes (crossing_id, status, detected_at, last_seen_at) VALUES ($1, $2, $3, $4)`,
		crossingID, "in_queue", now.Add(-2*time.Hour), now.Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf("insert status change 1: %v", err)
	}

	_, err = database.Pool.Exec(ctx,
		`INSERT INTO vehicle_status_changes (crossing_id, status, detected_at, last_seen_at) VALUES ($1, $2, $3, $3)`,
		crossingID, "called", now.Add(-time.Hour), now,
	)
	if err != nil {
		t.Fatalf("insert status change 2: %v", err)
	}

	crossings, err := database.GetVehicleHistoryGrouped(ctx, "TEST-001", "")
	if err != nil {
		t.Fatalf("GetVehicleHistoryGrouped: %v", err)
	}

	if len(crossings) != 1 {
		t.Fatalf("expected 1 crossing, got %d", len(crossings))
	}
	if crossings[0].ZoneID != "brest" {
		t.Errorf("expected zone brest, got %s", crossings[0].ZoneID)
	}
	if crossings[0].CurrentStatus != "called" {
		t.Errorf("expected current_status=called, got %s", crossings[0].CurrentStatus)
	}
	if !crossings[0].IsActive {
		t.Error("expected is_active=true")
	}
	if len(crossings[0].StatusChanges) != 2 {
		t.Errorf("expected 2 status changes, got %d", len(crossings[0].StatusChanges))
	}
	if crossings[0].StatusChanges[0].Status != "in_queue" {
		t.Errorf("expected first status=in_queue, got %s", crossings[0].StatusChanges[0].Status)
	}
}

func TestGetVehicleHistoryGrouped_FilterByZone(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	database := setupTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC()

	for _, zone := range []string{"brest", "bruzgi"} {
		var cid int64
		err := database.Pool.QueryRow(ctx,
			`INSERT INTO vehicle_crossings (zone_id, reg_number, queue_type, first_seen_at, last_seen_at, current_status, is_active)
			 VALUES ($1, $2, $3, $4, $4, $5, true) RETURNING id`,
			zone, "MULTI-001", "live", now, "in_queue",
		).Scan(&cid)
		if err != nil {
			t.Fatalf("insert crossing for %s: %v", zone, err)
		}
		_, err = database.Pool.Exec(ctx,
			`INSERT INTO vehicle_status_changes (crossing_id, status, detected_at, last_seen_at) VALUES ($1, $2, $3, $3)`,
			cid, "in_queue", now,
		)
		if err != nil {
			t.Fatalf("insert status change for %s: %v", zone, err)
		}
	}

	// Filter by zone
	crossings, err := database.GetVehicleHistoryGrouped(ctx, "MULTI-001", "brest")
	if err != nil {
		t.Fatalf("GetVehicleHistoryGrouped: %v", err)
	}
	if len(crossings) != 1 {
		t.Fatalf("expected 1 crossing for brest, got %d", len(crossings))
	}
	if crossings[0].ZoneID != "brest" {
		t.Errorf("expected brest, got %s", crossings[0].ZoneID)
	}

	// All zones
	all, err := database.GetVehicleHistoryGrouped(ctx, "MULTI-001", "")
	if err != nil {
		t.Fatalf("GetVehicleHistoryGrouped all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 crossings across all zones, got %d", len(all))
	}
}
