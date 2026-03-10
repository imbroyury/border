package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithInitScripts(
			"../../migrations/000001_init.up.sql",
			"../../migrations/000002_seed_zones.up.sql",
		),
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

	// Insert two snapshots
	var snapshotID1, snapshotID2 int64
	err := database.Pool.QueryRow(ctx,
		"INSERT INTO snapshots (zone_id, captured_at, cars_count) VALUES ($1, $2, $3) RETURNING id",
		"brest", now.Add(-30*time.Minute), 2,
	).Scan(&snapshotID1)
	if err != nil {
		t.Fatalf("insert snapshot 1: %v", err)
	}

	err = database.Pool.QueryRow(ctx,
		"INSERT INTO snapshots (zone_id, captured_at, cars_count) VALUES ($1, $2, $3) RETURNING id",
		"brest", now, 1,
	).Scan(&snapshotID2)
	if err != nil {
		t.Fatalf("insert snapshot 2: %v", err)
	}

	// Insert vehicles for both snapshots
	_, err = database.Pool.Exec(ctx,
		"INSERT INTO vehicles (snapshot_id, zone_id, reg_number, queue_type, status, registered_at, status_changed_at) VALUES ($1, $2, $3, $4, $5, $6, $7)",
		snapshotID1, "brest", "AA1234-7", "cargo", "waiting", now.Add(-2*time.Hour), now.Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf("insert vehicle for snapshot 1: %v", err)
	}

	_, err = database.Pool.Exec(ctx,
		"INSERT INTO vehicles (snapshot_id, zone_id, reg_number, queue_type, status, registered_at, status_changed_at) VALUES ($1, $2, $3, $4, $5, $6, $7)",
		snapshotID2, "brest", "BB5678-3", "passenger", "processing", now.Add(-time.Hour), now,
	)
	if err != nil {
		t.Fatalf("insert vehicle for snapshot 2: %v", err)
	}

	vehicles, err := database.GetCurrentVehicles(ctx, "brest")
	if err != nil {
		t.Fatalf("GetCurrentVehicles: %v", err)
	}

	// Should only return vehicle from latest snapshot (snapshot2)
	if len(vehicles) != 1 {
		t.Fatalf("expected 1 vehicle from latest snapshot, got %d", len(vehicles))
	}
	if vehicles[0].RegNumber != "BB5678-3" {
		t.Errorf("expected BB5678-3, got %s", vehicles[0].RegNumber)
	}
	if vehicles[0].QueueType != "passenger" {
		t.Errorf("expected passenger, got %s", vehicles[0].QueueType)
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

	// Create snapshots at different times
	times := []time.Duration{-48 * time.Hour, -24 * time.Hour, -1 * time.Hour}
	regNumbers := []string{"OLD-0001", "MID-0002", "NEW-0003"}

	for i, offset := range times {
		ts := now.Add(offset)
		var sid int64
		err := database.Pool.QueryRow(ctx,
			"INSERT INTO snapshots (zone_id, captured_at, cars_count) VALUES ($1, $2, $3) RETURNING id",
			"brest", ts, 1,
		).Scan(&sid)
		if err != nil {
			t.Fatalf("insert snapshot: %v", err)
		}

		_, err = database.Pool.Exec(ctx,
			"INSERT INTO vehicles (snapshot_id, zone_id, reg_number, queue_type, status, registered_at, status_changed_at) VALUES ($1, $2, $3, $4, $5, $6, $7)",
			sid, "brest", regNumbers[i], "cargo", "waiting", ts, ts,
		)
		if err != nil {
			t.Fatalf("insert vehicle: %v", err)
		}
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

func TestGetVehicleHistory_DeduplicatesVehicles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	database := setupTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC()

	// Same vehicle appears in two snapshots
	for i := 0; i < 2; i++ {
		ts := now.Add(-time.Duration(i) * time.Hour)
		var sid int64
		err := database.Pool.QueryRow(ctx,
			"INSERT INTO snapshots (zone_id, captured_at, cars_count) VALUES ($1, $2, $3) RETURNING id",
			"brest", ts, 1,
		).Scan(&sid)
		if err != nil {
			t.Fatalf("insert snapshot: %v", err)
		}

		_, err = database.Pool.Exec(ctx,
			"INSERT INTO vehicles (snapshot_id, zone_id, reg_number, queue_type, status, registered_at, status_changed_at) VALUES ($1, $2, $3, $4, $5, $6, $7)",
			sid, "brest", "SAME-0001", "cargo", fmt.Sprintf("status-%d", i), now.Add(-3*time.Hour), ts,
		)
		if err != nil {
			t.Fatalf("insert vehicle: %v", err)
		}
	}

	from := now.Add(-3 * time.Hour)
	to := now.Add(time.Minute)

	vehicles, err := database.GetVehicleHistory(ctx, "brest", from, to)
	if err != nil {
		t.Fatalf("GetVehicleHistory: %v", err)
	}

	// Should be deduplicated — only 1 entry, with most recent status
	if len(vehicles) != 1 {
		t.Fatalf("expected 1 deduplicated vehicle, got %d", len(vehicles))
	}
	if vehicles[0].Status != "status-0" {
		t.Errorf("expected latest status 'status-0', got %s", vehicles[0].Status)
	}
}
