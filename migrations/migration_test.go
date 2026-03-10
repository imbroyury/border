package migrations_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupPreMigration(t *testing.T, extraInitScripts ...string) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	initScripts := append(
		[]string{"000001_init.up.sql", "000002_seed_zones.up.sql"},
		extraInitScripts...,
	)

	pgContainer, err := postgres.Run(ctx,
		"postgres:18-alpine",
		postgres.WithInitScripts(initScripts...),
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
	t.Cleanup(func() { pgContainer.Terminate(ctx) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get connection string: %v", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}

func TestMigration000003(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()
	pool := setupPreMigration(t)

	base := time.Now().UTC().Truncate(time.Hour)

	insertSnap := func(zoneID string, at time.Time) int64 {
		t.Helper()
		var id int64
		err := pool.QueryRow(ctx,
			`INSERT INTO snapshots (zone_id, captured_at, cars_count, sent_last_hour, sent_last_24h)
			 VALUES ($1, $2, 1, 0, 0) RETURNING id`,
			zoneID, at,
		).Scan(&id)
		if err != nil {
			t.Fatalf("insert snapshot: %v", err)
		}
		return id
	}

	insertVehicle := func(snapID int64, zoneID, reg, status string) {
		t.Helper()
		_, err := pool.Exec(ctx,
			`INSERT INTO vehicles (snapshot_id, zone_id, reg_number, queue_type, status)
			 VALUES ($1, $2, $3, 'live', $4)`,
			snapID, zoneID, reg, status,
		)
		if err != nil {
			t.Fatalf("insert vehicle %s/%s: %v", reg, status, err)
		}
	}

	// Vehicle A: in_queue → called → passed within 2h → 1 crossing, 3 status_changes, is_active=false.
	insertVehicle(insertSnap("brest", base.Add(-2*time.Hour)), "brest", "MIGTEST-A", "in_queue")
	insertVehicle(insertSnap("brest", base.Add(-time.Hour)), "brest", "MIGTEST-A", "called")
	insertVehicle(insertSnap("brest", base), "brest", "MIGTEST-A", "passed")

	// Vehicle B: same status twice → 1 crossing, 1 status_change.
	insertVehicle(insertSnap("brest", base.Add(-30*time.Minute)), "brest", "MIGTEST-B", "in_queue")
	insertVehicle(insertSnap("brest", base.Add(-15*time.Minute)), "brest", "MIGTEST-B", "in_queue")

	// Vehicle C: 4h gap → 2 crossings.
	insertVehicle(insertSnap("brest", base.Add(-4*time.Hour)), "brest", "MIGTEST-C", "in_queue")
	insertVehicle(insertSnap("brest", base.Add(-10*time.Minute)), "brest", "MIGTEST-C", "in_queue")

	// Vehicle D: passed then reappears → 2 crossings.
	insertVehicle(insertSnap("brest", base.Add(-3*time.Hour)), "brest", "MIGTEST-D", "passed")
	insertVehicle(insertSnap("brest", base.Add(-5*time.Minute)), "brest", "MIGTEST-D", "in_queue")

	// Execute migration 000003.
	migSQL, err := os.ReadFile("000003_dedup_vehicles.up.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migSQL)); err != nil {
		t.Fatalf("execute migration: %v", err)
	}

	// vehicles table must no longer exist.
	var tableExists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'vehicles')`,
	).Scan(&tableExists); err != nil {
		t.Fatalf("check vehicles table: %v", err)
	}
	if tableExists {
		t.Error("vehicles table should have been dropped")
	}

	// Both new tables must have rows.
	var crossingCount, scCount int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM vehicle_crossings").Scan(&crossingCount); err != nil {
		t.Fatalf("count vehicle_crossings: %v", err)
	}
	if crossingCount == 0 {
		t.Error("vehicle_crossings should be non-empty")
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM vehicle_status_changes").Scan(&scCount); err != nil {
		t.Fatalf("count vehicle_status_changes: %v", err)
	}
	if scCount == 0 {
		t.Error("vehicle_status_changes should be non-empty")
	}

	// Vehicle A: 1 crossing, current_status=passed, is_active=false, 3 status_changes.
	var aStatus string
	var aActive bool
	if err := pool.QueryRow(ctx,
		"SELECT current_status, is_active FROM vehicle_crossings WHERE reg_number = 'MIGTEST-A'",
	).Scan(&aStatus, &aActive); err != nil {
		t.Fatalf("get A crossing: %v", err)
	}
	if aStatus != "passed" {
		t.Errorf("MIGTEST-A: expected current_status=passed, got %s", aStatus)
	}
	if aActive {
		t.Error("MIGTEST-A: expected is_active=false")
	}
	var aScCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM vehicle_status_changes sc
		 JOIN vehicle_crossings vc ON vc.id = sc.crossing_id
		 WHERE vc.reg_number = 'MIGTEST-A'`,
	).Scan(&aScCount); err != nil {
		t.Fatalf("count A status_changes: %v", err)
	}
	if aScCount != 3 {
		t.Errorf("MIGTEST-A: expected 3 status_changes, got %d", aScCount)
	}

	// Vehicle B: 1 status_change (same status merged).
	var bScCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM vehicle_status_changes sc
		 JOIN vehicle_crossings vc ON vc.id = sc.crossing_id
		 WHERE vc.reg_number = 'MIGTEST-B'`,
	).Scan(&bScCount); err != nil {
		t.Fatalf("count B status_changes: %v", err)
	}
	if bScCount != 1 {
		t.Errorf("MIGTEST-B: expected 1 status_change, got %d", bScCount)
	}

	// Vehicle C: 2 crossings (4h gap).
	var cCount int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM vehicle_crossings WHERE reg_number = 'MIGTEST-C'",
	).Scan(&cCount); err != nil {
		t.Fatalf("count C crossings: %v", err)
	}
	if cCount != 2 {
		t.Errorf("MIGTEST-C: expected 2 crossings, got %d", cCount)
	}

	// Vehicle D: 2 crossings (terminal then new).
	var dCount int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM vehicle_crossings WHERE reg_number = 'MIGTEST-D'",
	).Scan(&dCount); err != nil {
		t.Fatalf("count D crossings: %v", err)
	}
	if dCount != 2 {
		t.Errorf("MIGTEST-D: expected 2 crossings, got %d", dCount)
	}

	// No orphaned status_changes.
	var orphanCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM vehicle_status_changes sc
		 LEFT JOIN vehicle_crossings vc ON vc.id = sc.crossing_id
		 WHERE vc.id IS NULL`,
	).Scan(&orphanCount); err != nil {
		t.Fatalf("check orphans: %v", err)
	}
	if orphanCount != 0 {
		t.Errorf("found %d orphaned status_changes", orphanCount)
	}
}

// TestMigration000003_ProdData runs the migration against the real pre-migration
// production snapshot to verify it handles actual data without errors.
func TestMigration000003_ProdData(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	const dumpFile = "testdata/pre_migration_000003.sql"
	if _, err := os.Stat(dumpFile); err != nil {
		t.Skipf("prod snapshot not found (%s), skipping", dumpFile)
	}

	ctx := context.Background()
	pool := setupPreMigration(t, dumpFile)

	migSQL, err := os.ReadFile("000003_dedup_vehicles.up.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migSQL)); err != nil {
		t.Fatalf("execute migration: %v", err)
	}

	// vehicles table must be gone.
	var tableExists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'vehicles')`,
	).Scan(&tableExists); err != nil {
		t.Fatalf("check vehicles table: %v", err)
	}
	if tableExists {
		t.Error("vehicles table should have been dropped")
	}

	// Both new tables must have rows.
	var crossingCount, scCount int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM vehicle_crossings").Scan(&crossingCount); err != nil {
		t.Fatalf("count vehicle_crossings: %v", err)
	}
	if crossingCount == 0 {
		t.Error("vehicle_crossings should be non-empty after migrating prod data")
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM vehicle_status_changes").Scan(&scCount); err != nil {
		t.Fatalf("count vehicle_status_changes: %v", err)
	}
	if scCount == 0 {
		t.Error("vehicle_status_changes should be non-empty after migrating prod data")
	}

	// Every crossing must have at least one status_change.
	var crossingsWithoutSC int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM vehicle_crossings vc
		 WHERE NOT EXISTS (
		     SELECT 1 FROM vehicle_status_changes WHERE crossing_id = vc.id
		 )`,
	).Scan(&crossingsWithoutSC); err != nil {
		t.Fatalf("check crossings without status_changes: %v", err)
	}
	if crossingsWithoutSC != 0 {
		t.Errorf("%d crossings have no status_changes", crossingsWithoutSC)
	}

	// No orphaned status_changes.
	var orphanCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM vehicle_status_changes sc
		 LEFT JOIN vehicle_crossings vc ON vc.id = sc.crossing_id
		 WHERE vc.id IS NULL`,
	).Scan(&orphanCount); err != nil {
		t.Fatalf("check orphans: %v", err)
	}
	if orphanCount != 0 {
		t.Errorf("found %d orphaned status_changes", orphanCount)
	}

	// Passed/cancelled vehicles must have is_active=false.
	var activeTerminal int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM vehicle_crossings
		 WHERE current_status IN ('passed', 'cancelled') AND is_active = true`,
	).Scan(&activeTerminal); err != nil {
		t.Fatalf("check active terminal crossings: %v", err)
	}
	if activeTerminal != 0 {
		t.Errorf("%d passed/cancelled crossings incorrectly marked is_active=true", activeTerminal)
	}

	t.Logf("prod migration: %d crossings, %d status_changes", crossingCount, scCount)
}
