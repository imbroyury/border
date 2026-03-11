package storage

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

func setupTestDB(t *testing.T) (*Store, func()) {
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

func TestInsertSnapshot_InvalidZone(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	_, err := store.InsertSnapshot(ctx, &Snapshot{
		ZoneID:     "invalid-zone",
		CapturedAt: time.Now().UTC(),
		CarsCount:  5,
	})
	if err == nil {
		t.Fatal("expected error for invalid zone_id (FK violation)")
	}
}

func TestGetActiveCrossings_Empty(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	active, err := store.GetActiveCrossings(ctx, "brest")
	if err != nil {
		t.Fatalf("GetActiveCrossings: %v", err)
	}
	if len(active) != 0 {
		t.Errorf("expected empty map, got %v", active)
	}
}

func TestApplyCrawlDiff_NewVehicle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC()

	updates := []CrossingUpdate{
		{RegNumber: "AB1234", QueueType: "live", Status: "in_queue", CapturedAt: now},
	}
	if err := store.ApplyCrawlDiff(ctx, "brest", now, updates, nil); err != nil {
		t.Fatalf("ApplyCrawlDiff: %v", err)
	}

	var count int
	if err := store.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM vehicle_crossings WHERE zone_id='brest' AND reg_number='AB1234' AND is_active=true",
	).Scan(&count); err != nil {
		t.Fatalf("count crossings: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 crossing, got %d", count)
	}

	var scCount int
	if err := store.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM vehicle_status_changes sc
		 JOIN vehicle_crossings vc ON vc.id = sc.crossing_id
		 WHERE vc.reg_number = 'AB1234'`,
	).Scan(&scCount); err != nil {
		t.Fatalf("count status changes: %v", err)
	}
	if scCount != 1 {
		t.Errorf("expected 1 status_change, got %d", scCount)
	}
}

func TestApplyCrawlDiff_SameStatusUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	t1 := time.Now().UTC().Add(-time.Hour)
	t2 := time.Now().UTC()

	// First crawl: create vehicle
	updates1 := []CrossingUpdate{
		{RegNumber: "AB1234", QueueType: "live", Status: "in_queue", CapturedAt: t1},
	}
	if err := store.ApplyCrawlDiff(ctx, "brest", t1, updates1, nil); err != nil {
		t.Fatalf("ApplyCrawlDiff 1: %v", err)
	}

	// Second crawl: same status
	active, err := store.GetActiveCrossings(ctx, "brest")
	if err != nil {
		t.Fatalf("GetActiveCrossings: %v", err)
	}
	updates2 := []CrossingUpdate{
		{RegNumber: "AB1234", QueueType: "live", Status: "in_queue", CapturedAt: t2},
	}
	if err := store.ApplyCrawlDiff(ctx, "brest", t2, updates2, active); err != nil {
		t.Fatalf("ApplyCrawlDiff 2: %v", err)
	}

	// Should still be 1 crossing and 1 status_change
	var crossingCount, scCount int
	if err := store.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM vehicle_crossings WHERE reg_number='AB1234'",
	).Scan(&crossingCount); err != nil {
		t.Fatalf("count crossings: %v", err)
	}
	if crossingCount != 1 {
		t.Errorf("expected 1 crossing, got %d", crossingCount)
	}

	if err := store.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM vehicle_status_changes sc
		 JOIN vehicle_crossings vc ON vc.id = sc.crossing_id
		 WHERE vc.reg_number = 'AB1234'`,
	).Scan(&scCount); err != nil {
		t.Fatalf("count status changes: %v", err)
	}
	if scCount != 1 {
		t.Errorf("expected 1 status_change (no new row), got %d", scCount)
	}

	// last_seen_at on crossing should be t2
	var lastSeen time.Time
	if err := store.pool.QueryRow(ctx,
		"SELECT last_seen_at FROM vehicle_crossings WHERE reg_number='AB1234'",
	).Scan(&lastSeen); err != nil {
		t.Fatalf("get last_seen_at: %v", err)
	}
	if !lastSeen.Equal(t2) {
		t.Errorf("expected last_seen_at=%v, got %v", t2, lastSeen)
	}
}

func TestApplyCrawlDiff_StatusChange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	t1 := time.Now().UTC().Add(-time.Hour)
	t2 := time.Now().UTC()

	// First crawl: in_queue
	updates1 := []CrossingUpdate{
		{RegNumber: "AB1234", QueueType: "live", Status: "in_queue", CapturedAt: t1},
	}
	if err := store.ApplyCrawlDiff(ctx, "brest", t1, updates1, nil); err != nil {
		t.Fatalf("ApplyCrawlDiff 1: %v", err)
	}

	// Second crawl: called
	active, err := store.GetActiveCrossings(ctx, "brest")
	if err != nil {
		t.Fatalf("GetActiveCrossings: %v", err)
	}
	updates2 := []CrossingUpdate{
		{RegNumber: "AB1234", QueueType: "live", Status: "called", CapturedAt: t2},
	}
	if err := store.ApplyCrawlDiff(ctx, "brest", t2, updates2, active); err != nil {
		t.Fatalf("ApplyCrawlDiff 2: %v", err)
	}

	// Should be 1 crossing with current_status="called" and 2 status_changes
	var currentStatus string
	if err := store.pool.QueryRow(ctx,
		"SELECT current_status FROM vehicle_crossings WHERE reg_number='AB1234'",
	).Scan(&currentStatus); err != nil {
		t.Fatalf("get current_status: %v", err)
	}
	if currentStatus != "called" {
		t.Errorf("expected current_status=called, got %s", currentStatus)
	}

	var scCount int
	if err := store.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM vehicle_status_changes sc
		 JOIN vehicle_crossings vc ON vc.id = sc.crossing_id
		 WHERE vc.reg_number = 'AB1234'`,
	).Scan(&scCount); err != nil {
		t.Fatalf("count status changes: %v", err)
	}
	if scCount != 2 {
		t.Errorf("expected 2 status_changes, got %d", scCount)
	}
}

func TestApplyCrawlDiff_VehicleDisappears(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	t1 := time.Now().UTC().Add(-time.Hour)
	t2 := time.Now().UTC()

	// First crawl: vehicle present
	updates1 := []CrossingUpdate{
		{RegNumber: "AB1234", QueueType: "live", Status: "in_queue", CapturedAt: t1},
	}
	if err := store.ApplyCrawlDiff(ctx, "brest", t1, updates1, nil); err != nil {
		t.Fatalf("ApplyCrawlDiff 1: %v", err)
	}

	// Second crawl: vehicle absent
	active, err := store.GetActiveCrossings(ctx, "brest")
	if err != nil {
		t.Fatalf("GetActiveCrossings: %v", err)
	}
	if err := store.ApplyCrawlDiff(ctx, "brest", t2, nil, active); err != nil {
		t.Fatalf("ApplyCrawlDiff 2: %v", err)
	}

	var isActive bool
	if err := store.pool.QueryRow(ctx,
		"SELECT is_active FROM vehicle_crossings WHERE reg_number='AB1234'",
	).Scan(&isActive); err != nil {
		t.Fatalf("get is_active: %v", err)
	}
	if isActive {
		t.Error("expected is_active=false after vehicle disappeared")
	}
}

func TestApplyCrawlDiff_VehicleReturns(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	t1 := time.Now().UTC().Add(-2 * time.Hour)
	t2 := time.Now().UTC().Add(-time.Hour)
	t3 := time.Now().UTC()

	// First crawl: vehicle appears
	if err := store.ApplyCrawlDiff(ctx, "brest", t1, []CrossingUpdate{
		{RegNumber: "AB1234", QueueType: "live", Status: "in_queue", CapturedAt: t1},
	}, nil); err != nil {
		t.Fatalf("ApplyCrawlDiff 1: %v", err)
	}

	// Second crawl: vehicle disappears
	active, err := store.GetActiveCrossings(ctx, "brest")
	if err != nil {
		t.Fatalf("GetActiveCrossings 1: %v", err)
	}
	if err := store.ApplyCrawlDiff(ctx, "brest", t2, nil, active); err != nil {
		t.Fatalf("ApplyCrawlDiff 2: %v", err)
	}

	// Third crawl: vehicle returns
	active, err = store.GetActiveCrossings(ctx, "brest")
	if err != nil {
		t.Fatalf("GetActiveCrossings 2: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected no active crossings, got %d", len(active))
	}
	if err := store.ApplyCrawlDiff(ctx, "brest", t3, []CrossingUpdate{
		{RegNumber: "AB1234", QueueType: "live", Status: "in_queue", CapturedAt: t3},
	}, active); err != nil {
		t.Fatalf("ApplyCrawlDiff 3: %v", err)
	}

	var count int
	if err := store.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM vehicle_crossings WHERE reg_number='AB1234'",
	).Scan(&count); err != nil {
		t.Fatalf("count crossings: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 crossings after return, got %d", count)
	}
}

func TestApplyCrawlDiff_TerminalFlicker(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	t1 := time.Now().UTC().Add(-3 * time.Hour)
	t2 := time.Now().UTC().Add(-2 * time.Hour)
	t3 := time.Now().UTC().Add(-time.Hour)
	t4 := time.Now().UTC()

	// Crawl 1: vehicle appears as cancelled
	if err := store.ApplyCrawlDiff(ctx, "brest", t1, []CrossingUpdate{
		{RegNumber: "AB1234", QueueType: "live", Status: "cancelled", CapturedAt: t1},
	}, nil); err != nil {
		t.Fatalf("ApplyCrawlDiff 1: %v", err)
	}

	// Crawl 2: vehicle disappears
	active, err := store.GetActiveCrossings(ctx, "brest")
	if err != nil {
		t.Fatalf("GetActiveCrossings 1: %v", err)
	}
	if err := store.ApplyCrawlDiff(ctx, "brest", t2, nil, active); err != nil {
		t.Fatalf("ApplyCrawlDiff 2: %v", err)
	}

	// Crawl 3: vehicle reappears, still cancelled (API flicker)
	active, err = store.GetActiveCrossings(ctx, "brest")
	if err != nil {
		t.Fatalf("GetActiveCrossings 2: %v", err)
	}
	if err := store.ApplyCrawlDiff(ctx, "brest", t3, []CrossingUpdate{
		{RegNumber: "AB1234", QueueType: "live", Status: "cancelled", CapturedAt: t3},
	}, active); err != nil {
		t.Fatalf("ApplyCrawlDiff 3: %v", err)
	}

	// Crawl 4: vehicle disappears again, then reappears again
	active, err = store.GetActiveCrossings(ctx, "brest")
	if err != nil {
		t.Fatalf("GetActiveCrossings 3: %v", err)
	}
	if err := store.ApplyCrawlDiff(ctx, "brest", t4, nil, active); err != nil {
		t.Fatalf("ApplyCrawlDiff 4 (disappear): %v", err)
	}
	active, err = store.GetActiveCrossings(ctx, "brest")
	if err != nil {
		t.Fatalf("GetActiveCrossings 4: %v", err)
	}
	if err := store.ApplyCrawlDiff(ctx, "brest", t4, []CrossingUpdate{
		{RegNumber: "AB1234", QueueType: "live", Status: "cancelled", CapturedAt: t4},
	}, active); err != nil {
		t.Fatalf("ApplyCrawlDiff 5 (reappear): %v", err)
	}

	// Should still be just 1 crossing (not 3 duplicates)
	var count int
	if err := store.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM vehicle_crossings WHERE reg_number='AB1234'",
	).Scan(&count); err != nil {
		t.Fatalf("count crossings: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 crossing (terminal flicker should not duplicate), got %d", count)
	}

	// last_seen_at should be updated to t4
	var lastSeen time.Time
	if err := store.pool.QueryRow(ctx,
		"SELECT last_seen_at FROM vehicle_crossings WHERE reg_number='AB1234'",
	).Scan(&lastSeen); err != nil {
		t.Fatalf("get last_seen_at: %v", err)
	}
	if !lastSeen.Equal(t4) {
		t.Errorf("expected last_seen_at=%v, got %v", t4, lastSeen)
	}
}

func TestApplyCrawlDiff_TerminalThenNew(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	t1 := time.Now().UTC().Add(-2 * time.Hour)
	t2 := time.Now().UTC().Add(-time.Hour)
	t3 := time.Now().UTC()

	// First crawl: vehicle passes
	if err := store.ApplyCrawlDiff(ctx, "brest", t1, []CrossingUpdate{
		{RegNumber: "AB1234", QueueType: "live", Status: "passed", CapturedAt: t1},
	}, nil); err != nil {
		t.Fatalf("ApplyCrawlDiff 1: %v", err)
	}

	// Second crawl: vehicle disappears after passing
	active, err := store.GetActiveCrossings(ctx, "brest")
	if err != nil {
		t.Fatalf("GetActiveCrossings 1: %v", err)
	}
	if err := store.ApplyCrawlDiff(ctx, "brest", t2, nil, active); err != nil {
		t.Fatalf("ApplyCrawlDiff 2: %v", err)
	}

	// Third crawl: same vehicle reappears (new queue entry)
	active, err = store.GetActiveCrossings(ctx, "brest")
	if err != nil {
		t.Fatalf("GetActiveCrossings 2: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected no active crossings, got %d", len(active))
	}
	if err := store.ApplyCrawlDiff(ctx, "brest", t3, []CrossingUpdate{
		{RegNumber: "AB1234", QueueType: "live", Status: "in_queue", CapturedAt: t3},
	}, active); err != nil {
		t.Fatalf("ApplyCrawlDiff 3: %v", err)
	}

	var count int
	if err := store.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM vehicle_crossings WHERE reg_number='AB1234'",
	).Scan(&count); err != nil {
		t.Fatalf("count crossings: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 crossings (terminal then new), got %d", count)
	}
}
