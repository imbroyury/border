package storage

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupTestDB(t *testing.T) (*Store, func()) {
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

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		pgContainer.Terminate(ctx)
		t.Fatalf("get connection string: %v", err)
	}

	store, err := New(ctx, connStr)
	if err != nil {
		pgContainer.Terminate(ctx)
		t.Fatalf("create store: %v", err)
	}

	cleanup := func() {
		store.Close()
		pgContainer.Terminate(ctx)
	}
	return store, cleanup
}

func TestInsertSnapshot_ReturnsValidID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	snap := &Snapshot{
		ZoneID:       "brest",
		CapturedAt:   time.Now().UTC(),
		CarsCount:    42,
		SentLastHour: 5,
		SentLast24h:  100,
	}

	id, err := store.InsertSnapshot(ctx, snap)
	if err != nil {
		t.Fatalf("InsertSnapshot: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}
}

func TestInsertSnapshot_MultipleSnapshots(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC()

	id1, err := store.InsertSnapshot(ctx, &Snapshot{
		ZoneID: "brest", CapturedAt: now, CarsCount: 10,
	})
	if err != nil {
		t.Fatalf("InsertSnapshot 1: %v", err)
	}

	id2, err := store.InsertSnapshot(ctx, &Snapshot{
		ZoneID: "brest", CapturedAt: now.Add(time.Minute), CarsCount: 20,
	})
	if err != nil {
		t.Fatalf("InsertSnapshot 2: %v", err)
	}

	if id2 <= id1 {
		t.Errorf("second ID (%d) should be greater than first (%d)", id2, id1)
	}
}

func TestInsertVehicles_EmptySlice(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	snap := &Snapshot{
		ZoneID:     "brest",
		CapturedAt: time.Now().UTC(),
		CarsCount:  0,
	}
	id, err := store.InsertSnapshot(ctx, snap)
	if err != nil {
		t.Fatalf("InsertSnapshot: %v", err)
	}

	err = store.InsertVehicles(ctx, id, "brest", nil)
	if err != nil {
		t.Fatalf("InsertVehicles with nil: %v", err)
	}

	err = store.InsertVehicles(ctx, id, "brest", []Vehicle{})
	if err != nil {
		t.Fatalf("InsertVehicles with empty slice: %v", err)
	}
}

func TestInsertVehicles_WithData(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC()
	snap := &Snapshot{
		ZoneID:     "benyakoni",
		CapturedAt: now,
		CarsCount:  2,
	}
	id, err := store.InsertSnapshot(ctx, snap)
	if err != nil {
		t.Fatalf("InsertSnapshot: %v", err)
	}

	vehicles := []Vehicle{
		{
			ZoneID:          "benyakoni",
			RegNumber:       "AB1234",
			QueueType:       "live",
			RegisteredAt:    now.Add(-time.Hour),
			StatusChangedAt: now.Add(-30 * time.Minute),
			Status:          "in_queue",
		},
		{
			ZoneID:          "benyakoni",
			RegNumber:       "CD5678",
			QueueType:       "priority",
			RegisteredAt:    now.Add(-2 * time.Hour),
			StatusChangedAt: time.Time{},
			Status:          "called",
		},
	}

	err = store.InsertVehicles(ctx, id, "benyakoni", vehicles)
	if err != nil {
		t.Fatalf("InsertVehicles: %v", err)
	}

	var count int
	err = store.pool.QueryRow(ctx, "SELECT COUNT(*) FROM vehicles WHERE snapshot_id = $1", id).Scan(&count)
	if err != nil {
		t.Fatalf("count vehicles: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 vehicles, got %d", count)
	}
}

func TestInsertCrawlResult_Atomicity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC()

	snap := &Snapshot{
		ZoneID:       "kamenny-log",
		CapturedAt:   now,
		CarsCount:    1,
		SentLastHour: 3,
		SentLast24h:  15,
	}
	vehicles := []Vehicle{
		{
			ZoneID:       "kamenny-log",
			RegNumber:    "EF9012",
			QueueType:    "live",
			RegisteredAt: now.Add(-time.Hour),
			Status:       "in_queue",
		},
	}

	err := store.InsertCrawlResult(ctx, snap, vehicles)
	if err != nil {
		t.Fatalf("InsertCrawlResult: %v", err)
	}

	var snapCount int
	err = store.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM snapshots WHERE zone_id = $1 AND captured_at = $2",
		snap.ZoneID, snap.CapturedAt,
	).Scan(&snapCount)
	if err != nil {
		t.Fatalf("count snapshots: %v", err)
	}
	if snapCount != 1 {
		t.Errorf("expected 1 snapshot, got %d", snapCount)
	}

	var vehCount int
	err = store.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM vehicles WHERE zone_id = $1",
		snap.ZoneID,
	).Scan(&vehCount)
	if err != nil {
		t.Fatalf("count vehicles: %v", err)
	}
	if vehCount != 1 {
		t.Errorf("expected 1 vehicle, got %d", vehCount)
	}
}

func TestInsertCrawlResult_EmptyVehicles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	snap := &Snapshot{
		ZoneID:     "brest",
		CapturedAt: time.Now().UTC(),
		CarsCount:  0,
	}

	err := store.InsertCrawlResult(ctx, snap, nil)
	if err != nil {
		t.Fatalf("InsertCrawlResult with no vehicles: %v", err)
	}
}

func TestInsertCrawlResult_InvalidZone(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	snap := &Snapshot{
		ZoneID:     "nonexistent-zone",
		CapturedAt: time.Now().UTC(),
		CarsCount:  0,
	}

	err := store.InsertCrawlResult(ctx, snap, nil)
	if err == nil {
		t.Fatal("expected error for invalid zone_id (FK violation)")
	}
}

func TestGetLatestVehicleStatuses_NoSnapshots(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	statuses, err := store.GetLatestVehicleStatuses(ctx, "brest")
	if err != nil {
		t.Fatalf("GetLatestVehicleStatuses: %v", err)
	}
	if len(statuses) != 0 {
		t.Errorf("expected empty map, got %v", statuses)
	}
}

func TestGetLatestVehicleStatuses_ReturnsLatestSnapshotOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC()

	// First (older) snapshot with vehicle AB1234 as "in_queue".
	err := store.InsertCrawlResult(ctx, &Snapshot{
		ZoneID: "brest", CapturedAt: now.Add(-time.Hour), CarsCount: 1,
	}, []Vehicle{
		{ZoneID: "brest", RegNumber: "AB1234", QueueType: "live", Status: "in_queue"},
	})
	if err != nil {
		t.Fatalf("InsertCrawlResult 1: %v", err)
	}

	// Second (latest) snapshot with AB1234 now "called".
	err = store.InsertCrawlResult(ctx, &Snapshot{
		ZoneID: "brest", CapturedAt: now, CarsCount: 1,
	}, []Vehicle{
		{ZoneID: "brest", RegNumber: "AB1234", QueueType: "live", Status: "called"},
	})
	if err != nil {
		t.Fatalf("InsertCrawlResult 2: %v", err)
	}

	statuses, err := store.GetLatestVehicleStatuses(ctx, "brest")
	if err != nil {
		t.Fatalf("GetLatestVehicleStatuses: %v", err)
	}
	if got, want := statuses["AB1234"], "called"; got != want {
		t.Errorf("AB1234 status: got %q, want %q", got, want)
	}
	if len(statuses) != 1 {
		t.Errorf("expected 1 entry, got %d", len(statuses))
	}
}

func TestGetLatestVehicleStatuses_EmptyVehiclesInLatestSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC()

	// First snapshot has a vehicle.
	err := store.InsertCrawlResult(ctx, &Snapshot{
		ZoneID: "brest", CapturedAt: now.Add(-time.Hour), CarsCount: 1,
	}, []Vehicle{
		{ZoneID: "brest", RegNumber: "XY9999", QueueType: "live", Status: "in_queue"},
	})
	if err != nil {
		t.Fatalf("InsertCrawlResult 1: %v", err)
	}

	// Latest snapshot is empty (no vehicles).
	err = store.InsertCrawlResult(ctx, &Snapshot{
		ZoneID: "brest", CapturedAt: now, CarsCount: 0,
	}, nil)
	if err != nil {
		t.Fatalf("InsertCrawlResult 2: %v", err)
	}

	statuses, err := store.GetLatestVehicleStatuses(ctx, "brest")
	if err != nil {
		t.Fatalf("GetLatestVehicleStatuses: %v", err)
	}
	if len(statuses) != 0 {
		t.Errorf("expected empty map, got %v", statuses)
	}
}

func TestInsertSnapshot_InvalidZone(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	snap := &Snapshot{
		ZoneID:     "invalid-zone",
		CapturedAt: time.Now().UTC(),
		CarsCount:  5,
	}

	_, err := store.InsertSnapshot(ctx, snap)
	if err == nil {
		t.Fatal("expected error for invalid zone_id (FK violation)")
	}
}
