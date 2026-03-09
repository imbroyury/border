# Border Queue Monitor — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a self-hosted system that crawls Belarus border crossing queue data every 15 minutes, stores it in PostgreSQL, and presents interactive time-series graphs.

**Architecture:** 4 Docker containers — crawler (Go), backend (Go API), frontend (Vue 3 + ECharts served by Caddy), PostgreSQL. Monorepo structure.

**Tech Stack:** Go 1.22, chi router, pgx, golang-migrate, Vue 3, TypeScript, Vite, ECharts, Caddy, PostgreSQL 16, Docker Compose

**Design doc:** `docs/plans/2026-03-09-border-queue-monitor-design.md`

---

## Agent Assignments

This plan is designed for parallel agent execution. Tasks are grouped into 4 independent workstreams:

| Agent | Workstream | Tasks |
|-------|-----------|-------|
| Agent 1 | Database + Migrations | Task 1 |
| Agent 2 | Crawler | Tasks 2–3 |
| Agent 3 | Backend API | Tasks 4–6 |
| Agent 4 | Frontend | Tasks 7–9 |
| Sequential | Integration + Docker | Tasks 10–11 |

**Dependencies:** Agents 2, 3, 4 depend on Agent 1 completing first. Tasks 10–11 run after all agents complete.

---

## Task 1: Database Schema & Migrations (Agent 1)

**Files:**
- Create: `migrations/000001_init.up.sql`
- Create: `migrations/000001_init.down.sql`
- Create: `migrations/000002_seed_zones.up.sql`
- Create: `migrations/000002_seed_zones.down.sql`

**Step 1: Create initial schema migration**

```sql
-- migrations/000001_init.up.sql
CREATE TABLE zones (
    id VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    border VARCHAR(10) NOT NULL
);

CREATE TABLE snapshots (
    id BIGSERIAL PRIMARY KEY,
    zone_id VARCHAR(50) NOT NULL REFERENCES zones(id),
    captured_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    cars_count INT NOT NULL DEFAULT 0,
    sent_last_hour INT NOT NULL DEFAULT 0,
    sent_last_24h INT NOT NULL DEFAULT 0
);

CREATE INDEX idx_snapshots_zone_time ON snapshots(zone_id, captured_at);

CREATE TABLE vehicles (
    id BIGSERIAL PRIMARY KEY,
    snapshot_id BIGINT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    zone_id VARCHAR(50) NOT NULL,
    reg_number VARCHAR(20) NOT NULL,
    queue_type VARCHAR(50) NOT NULL DEFAULT '',
    registered_at TIMESTAMPTZ,
    status_changed_at TIMESTAMPTZ,
    status VARCHAR(50) NOT NULL DEFAULT ''
);

CREATE INDEX idx_vehicles_snapshot ON vehicles(snapshot_id);
CREATE INDEX idx_vehicles_zone_time ON vehicles(zone_id, registered_at);
```

```sql
-- migrations/000001_init.down.sql
DROP TABLE IF EXISTS vehicles;
DROP TABLE IF EXISTS snapshots;
DROP TABLE IF EXISTS zones;
```

**Step 2: Create seed migration with known zones**

```sql
-- migrations/000002_seed_zones.up.sql
INSERT INTO zones (id, name, border) VALUES
    ('benyakoni-bts', 'Бенякони', 'BY-LT'),
    ('berestovitsa-bts', 'Берестовица', 'BY-PL'),
    ('brest-bts', 'Брест', 'BY-PL'),
    ('bruzgi-bts', 'Брузги', 'BY-PL'),
    ('grigorovshchina-bts', 'Григоровщина', 'BY-LT'),
    ('kamenny-log-bts', 'Каменный Лог', 'BY-LT'),
    ('kozlovichi-bts', 'Козловичи', 'BY-PL');
```

```sql
-- migrations/000002_seed_zones.down.sql
DELETE FROM zones;
```

**Step 3: Verify migrations run against a local Postgres**

Run: `docker run --rm -e POSTGRES_PASSWORD=test -e POSTGRES_DB=border -p 5433:5432 postgres:16` (in background)

Then apply migrations manually to verify syntax:
```bash
psql "postgres://postgres:test@localhost:5433/border" -f migrations/000001_init.up.sql
psql "postgres://postgres:test@localhost:5433/border" -f migrations/000002_seed_zones.up.sql
```

Verify tables exist and zones are seeded:
```bash
psql "postgres://postgres:test@localhost:5433/border" -c "SELECT * FROM zones;"
```

Expected: 7 rows.

**Step 4: Commit**

```bash
git add migrations/
git commit -m "feat: add database schema and zone seed migrations"
```

---

## Task 2: Crawler — API Discovery & HTTP Client (Agent 2)

**Files:**
- Create: `crawler/go.mod`
- Create: `crawler/main.go`
- Create: `crawler/scraper/scraper.go`
- Create: `crawler/scraper/scraper_test.go`
- Create: `crawler/scraper/parser.go`
- Create: `crawler/scraper/parser_test.go`

**Step 1: Initialize Go module**

```bash
cd crawler && go mod init github.com/imbroyury/border/crawler
```

**Step 2: Write parser tests with sample HTML**

The parser should extract data from the HTML pages. Write tests first using saved HTML samples.

```go
// crawler/scraper/parser_test.go
package scraper

import (
    "testing"
    "time"
)

// Sample HTML from /zone summary page (save actual fetched HTML as testdata)
// For now, define expected structures

func TestParseZoneSummary(t *testing.T) {
    // Use testdata/zone_summary.html
    html := loadTestData(t, "zone_summary.html")
    zones, err := ParseZoneSummary(html)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(zones) == 0 {
        t.Fatal("expected at least one zone")
    }
    for _, z := range zones {
        if z.ZoneID == "" {
            t.Error("zone ID should not be empty")
        }
        if z.CarsCount < 0 {
            t.Errorf("cars count should be >= 0, got %d", z.CarsCount)
        }
    }
}

func TestParseZoneDetail(t *testing.T) {
    html := loadTestData(t, "zone_detail.html")
    detail, err := ParseZoneDetail(html)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    // Verify aggregate fields parsed
    if detail.SentLastHour < 0 {
        t.Errorf("sent_last_hour should be >= 0")
    }
    if detail.SentLast24h < 0 {
        t.Errorf("sent_last_24h should be >= 0")
    }
    // Vehicles may be empty but should not error
}

func TestParseZoneSummary_EmptyHTML(t *testing.T) {
    _, err := ParseZoneSummary("")
    if err == nil {
        t.Error("expected error for empty HTML")
    }
}

func TestParseZoneDetail_EmptyHTML(t *testing.T) {
    _, err := ParseZoneDetail("")
    if err == nil {
        t.Error("expected error for empty HTML")
    }
}
```

**Important:** Before writing the parser, you MUST fetch actual HTML from `https://mon.declarant.by/zone` and `https://mon.declarant.by/zone/brest-bts` and save them as `crawler/scraper/testdata/zone_summary.html` and `crawler/scraper/testdata/zone_detail.html`. Use curl or a browser's "View Source". The Angular SPA may require JavaScript rendering — if the raw HTML contains no data, use the XHR API discovery approach instead:

1. Open browser DevTools → Network tab → reload the page
2. Look for XHR/fetch requests that return JSON with queue data
3. If JSON API found: write tests against JSON responses instead of HTML
4. If no API found: raw HTML is server-rendered enough, proceed with HTML parsing

**Step 3: Implement parser based on actual page structure**

Use `golang.org/x/net/html` or a CSS selector library like `github.com/PuerkitoBio/goquery` to parse the HTML/JSON.

```go
// crawler/scraper/parser.go
package scraper

type ZoneSummaryEntry struct {
    ZoneID    string
    CarsCount int
}

type VehicleEntry struct {
    RegNumber       string
    QueueType       string
    RegisteredAt    time.Time
    StatusChangedAt time.Time
    Status          string
}

type ZoneDetail struct {
    SentLastHour int
    SentLast24h  int
    Vehicles     []VehicleEntry
}

func ParseZoneSummary(html string) ([]ZoneSummaryEntry, error) {
    // Implementation depends on actual page structure
}

func ParseZoneDetail(html string) (*ZoneDetail, error) {
    // Implementation depends on actual page structure
}
```

**Step 4: Write scraper HTTP client with tests**

```go
// crawler/scraper/scraper.go
package scraper

import (
    "context"
    "fmt"
    "io"
    "net/http"
    "time"
)

type Client struct {
    httpClient *http.Client
    baseURL    string
}

func NewClient(baseURL string) *Client {
    return &Client{
        httpClient: &http.Client{Timeout: 30 * time.Second},
        baseURL:    baseURL,
    }
}

func (c *Client) FetchZoneSummary(ctx context.Context) ([]ZoneSummaryEntry, error) {
    body, err := c.fetch(ctx, "/zone")
    if err != nil {
        return nil, fmt.Errorf("fetch zone summary: %w", err)
    }
    return ParseZoneSummary(body)
}

func (c *Client) FetchZoneDetail(ctx context.Context, zoneID string) (*ZoneDetail, error) {
    body, err := c.fetch(ctx, "/zone/"+zoneID)
    if err != nil {
        return nil, fmt.Errorf("fetch zone %s: %w", zoneID, err)
    }
    return ParseZoneDetail(body)
}

func (c *Client) fetch(ctx context.Context, path string) (string, error) {
    req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
    if err != nil {
        return "", err
    }
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
    }
    b, err := io.ReadAll(resp.Body)
    return string(b), err
}
```

```go
// crawler/scraper/scraper_test.go
package scraper

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestFetchZoneSummary_Success(t *testing.T) {
    html := loadTestData(t, "zone_summary.html")
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/zone" {
            t.Errorf("unexpected path: %s", r.URL.Path)
        }
        w.Write([]byte(html))
    }))
    defer srv.Close()

    client := NewClient(srv.URL)
    zones, err := client.FetchZoneSummary(context.Background())
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(zones) == 0 {
        t.Error("expected zones")
    }
}

func TestFetchZoneSummary_ServerError(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusInternalServerError)
    }))
    defer srv.Close()

    client := NewClient(srv.URL)
    _, err := client.FetchZoneSummary(context.Background())
    if err == nil {
        t.Error("expected error on 500")
    }
}

func TestFetchZoneDetail_Success(t *testing.T) {
    html := loadTestData(t, "zone_detail.html")
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte(html))
    }))
    defer srv.Close()

    client := NewClient(srv.URL)
    detail, err := client.FetchZoneDetail(context.Background(), "brest-bts")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if detail == nil {
        t.Fatal("expected non-nil detail")
    }
}
```

Helper:
```go
// Add to parser_test.go
func loadTestData(t *testing.T, filename string) string {
    t.Helper()
    data, err := os.ReadFile(filepath.Join("testdata", filename))
    if err != nil {
        t.Fatalf("failed to load testdata/%s: %v", filename, err)
    }
    return string(data)
}
```

**Step 5: Run tests**

```bash
cd crawler && go test ./scraper/ -v
```

**Step 6: Commit**

```bash
git add crawler/
git commit -m "feat: add crawler HTTP client and HTML/API parser with tests"
```

---

## Task 3: Crawler — Database Storage & Main Loop (Agent 2)

**Files:**
- Create: `crawler/storage/storage.go`
- Create: `crawler/storage/storage_test.go`
- Modify: `crawler/main.go`

**Step 1: Write storage layer tests**

```go
// crawler/storage/storage_test.go
package storage

import (
    "context"
    "os"
    "testing"
    "time"
)

// These tests require a running Postgres.
// Set TEST_DATABASE_URL env var. Skip if not set.

func getTestDB(t *testing.T) *Store {
    t.Helper()
    dsn := os.Getenv("TEST_DATABASE_URL")
    if dsn == "" {
        t.Skip("TEST_DATABASE_URL not set, skipping integration test")
    }
    store, err := New(context.Background(), dsn)
    if err != nil {
        t.Fatalf("failed to connect: %v", err)
    }
    t.Cleanup(func() { store.Close() })
    return store
}

func TestInsertSnapshot(t *testing.T) {
    store := getTestDB(t)
    ctx := context.Background()

    snap := &Snapshot{
        ZoneID:       "brest-bts",
        CapturedAt:   time.Now(),
        CarsCount:    42,
        SentLastHour: 5,
        SentLast24h:  100,
    }
    id, err := store.InsertSnapshot(ctx, snap)
    if err != nil {
        t.Fatalf("insert snapshot: %v", err)
    }
    if id == 0 {
        t.Error("expected non-zero snapshot ID")
    }
}

func TestInsertVehicles(t *testing.T) {
    store := getTestDB(t)
    ctx := context.Background()

    snap := &Snapshot{
        ZoneID:     "brest-bts",
        CapturedAt: time.Now(),
        CarsCount:  2,
    }
    snapID, err := store.InsertSnapshot(ctx, snap)
    if err != nil {
        t.Fatalf("insert snapshot: %v", err)
    }

    vehicles := []Vehicle{
        {RegNumber: "AB1234", QueueType: "live", Status: "waiting"},
        {RegNumber: "CD5678", QueueType: "live", Status: "called"},
    }
    err = store.InsertVehicles(ctx, snapID, "brest-bts", vehicles)
    if err != nil {
        t.Fatalf("insert vehicles: %v", err)
    }
}

func TestInsertSnapshotWithVehicles_Transaction(t *testing.T) {
    store := getTestDB(t)
    ctx := context.Background()

    snap := &Snapshot{
        ZoneID:     "brest-bts",
        CapturedAt: time.Now(),
        CarsCount:  1,
    }
    vehicles := []Vehicle{
        {RegNumber: "TX0001", Status: "waiting"},
    }
    err := store.InsertCrawlResult(ctx, snap, vehicles)
    if err != nil {
        t.Fatalf("insert crawl result: %v", err)
    }
}
```

**Step 2: Implement storage layer**

```go
// crawler/storage/storage.go
package storage

import (
    "context"
    "fmt"
    "time"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
)

type Snapshot struct {
    ZoneID       string
    CapturedAt   time.Time
    CarsCount    int
    SentLastHour int
    SentLast24h  int
}

type Vehicle struct {
    RegNumber       string
    QueueType       string
    RegisteredAt    time.Time
    StatusChangedAt time.Time
    Status          string
}

type Store struct {
    pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string) (*Store, error) {
    pool, err := pgxpool.New(ctx, dsn)
    if err != nil {
        return nil, fmt.Errorf("connect to db: %w", err)
    }
    if err := pool.Ping(ctx); err != nil {
        return nil, fmt.Errorf("ping db: %w", err)
    }
    return &Store{pool: pool}, nil
}

func (s *Store) Close() {
    s.pool.Close()
}

func (s *Store) InsertSnapshot(ctx context.Context, snap *Snapshot) (int64, error) {
    var id int64
    err := s.pool.QueryRow(ctx,
        `INSERT INTO snapshots (zone_id, captured_at, cars_count, sent_last_hour, sent_last_24h)
         VALUES ($1, $2, $3, $4, $5) RETURNING id`,
        snap.ZoneID, snap.CapturedAt, snap.CarsCount, snap.SentLastHour, snap.SentLast24h,
    ).Scan(&id)
    return id, err
}

func (s *Store) InsertVehicles(ctx context.Context, snapshotID int64, zoneID string, vehicles []Vehicle) error {
    if len(vehicles) == 0 {
        return nil
    }
    _, err := s.pool.CopyFrom(ctx,
        pgx.Identifier{"vehicles"},
        []string{"snapshot_id", "zone_id", "reg_number", "queue_type", "registered_at", "status_changed_at", "status"},
        pgx.CopyFromSlice(len(vehicles), func(i int) ([]any, error) {
            v := vehicles[i]
            return []any{snapshotID, zoneID, v.RegNumber, v.QueueType, v.RegisteredAt, v.StatusChangedAt, v.Status}, nil
        }),
    )
    return err
}

func (s *Store) InsertCrawlResult(ctx context.Context, snap *Snapshot, vehicles []Vehicle) error {
    tx, err := s.pool.Begin(ctx)
    if err != nil {
        return fmt.Errorf("begin tx: %w", err)
    }
    defer tx.Rollback(ctx)

    var snapID int64
    err = tx.QueryRow(ctx,
        `INSERT INTO snapshots (zone_id, captured_at, cars_count, sent_last_hour, sent_last_24h)
         VALUES ($1, $2, $3, $4, $5) RETURNING id`,
        snap.ZoneID, snap.CapturedAt, snap.CarsCount, snap.SentLastHour, snap.SentLast24h,
    ).Scan(&snapID)
    if err != nil {
        return fmt.Errorf("insert snapshot: %w", err)
    }

    for _, v := range vehicles {
        _, err = tx.Exec(ctx,
            `INSERT INTO vehicles (snapshot_id, zone_id, reg_number, queue_type, registered_at, status_changed_at, status)
             VALUES ($1, $2, $3, $4, $5, $6, $7)`,
            snapID, snap.ZoneID, v.RegNumber, v.QueueType, v.RegisteredAt, v.StatusChangedAt, v.Status,
        )
        if err != nil {
            return fmt.Errorf("insert vehicle: %w", err)
        }
    }

    return tx.Commit(ctx)
}
```

**Step 3: Implement main.go with ticker loop**

```go
// crawler/main.go
package main

import (
    "context"
    "log/slog"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/imbroyury/border/crawler/scraper"
    "github.com/imbroyury/border/crawler/storage"
)

func main() {
    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

    dbURL := os.Getenv("DATABASE_URL")
    if dbURL == "" {
        logger.Error("DATABASE_URL is required")
        os.Exit(1)
    }

    baseURL := os.Getenv("SCRAPE_BASE_URL")
    if baseURL == "" {
        baseURL = "https://mon.declarant.by"
    }

    intervalStr := os.Getenv("CRAWL_INTERVAL")
    interval := 15 * time.Minute
    if intervalStr != "" {
        var err error
        interval, err = time.ParseDuration(intervalStr)
        if err != nil {
            logger.Error("invalid CRAWL_INTERVAL", "error", err)
            os.Exit(1)
        }
    }

    ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer cancel()

    store, err := storage.New(ctx, dbURL)
    if err != nil {
        logger.Error("failed to connect to database", "error", err)
        os.Exit(1)
    }
    defer store.Close()

    client := scraper.NewClient(baseURL)

    logger.Info("crawler started", "interval", interval, "baseURL", baseURL)

    // Run immediately on startup
    crawl(ctx, logger, client, store)

    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            logger.Info("crawler shutting down")
            return
        case <-ticker.C:
            crawl(ctx, logger, client, store)
        }
    }
}

func crawl(ctx context.Context, logger *slog.Logger, client *scraper.Client, store *storage.Store) {
    logger.Info("starting crawl")
    now := time.Now()

    zones, err := client.FetchZoneSummary(ctx)
    if err != nil {
        logger.Error("failed to fetch zone summary", "error", err)
        return
    }

    for _, zone := range zones {
        detail, err := client.FetchZoneDetail(ctx, zone.ZoneID)
        if err != nil {
            logger.Error("failed to fetch zone detail", "zone", zone.ZoneID, "error", err)
            continue
        }

        snap := &storage.Snapshot{
            ZoneID:       zone.ZoneID,
            CapturedAt:   now,
            CarsCount:    zone.CarsCount,
            SentLastHour: detail.SentLastHour,
            SentLast24h:  detail.SentLast24h,
        }

        vehicles := make([]storage.Vehicle, len(detail.Vehicles))
        for i, v := range detail.Vehicles {
            vehicles[i] = storage.Vehicle{
                RegNumber:       v.RegNumber,
                QueueType:       v.QueueType,
                RegisteredAt:    v.RegisteredAt,
                StatusChangedAt: v.StatusChangedAt,
                Status:          v.Status,
            }
        }

        if err := store.InsertCrawlResult(ctx, snap, vehicles); err != nil {
            logger.Error("failed to store crawl result", "zone", zone.ZoneID, "error", err)
            continue
        }

        logger.Info("crawled zone", "zone", zone.ZoneID, "cars", zone.CarsCount, "vehicles", len(vehicles))
    }

    logger.Info("crawl complete", "duration", time.Since(now))
}
```

**Step 4: Run tests**

```bash
cd crawler && go test ./... -v
```

**Step 5: Commit**

```bash
git add crawler/
git commit -m "feat: add crawler storage layer and main loop"
```

---

## Task 4: Backend — Go Module, DB Layer & Migration Runner (Agent 3)

**Files:**
- Create: `backend/go.mod`
- Create: `backend/main.go`
- Create: `backend/db/db.go`
- Create: `backend/db/db_test.go`
- Create: `backend/db/migrate.go`
- Create: `backend/db/migrate_test.go`

**Step 1: Initialize Go module**

```bash
cd backend && go mod init github.com/imbroyury/border/backend
```

**Step 2: Write DB query layer tests**

```go
// backend/db/db_test.go
package db

import (
    "context"
    "os"
    "testing"
    "time"
)

func getTestDB(t *testing.T) *DB {
    t.Helper()
    dsn := os.Getenv("TEST_DATABASE_URL")
    if dsn == "" {
        t.Skip("TEST_DATABASE_URL not set")
    }
    d, err := New(context.Background(), dsn)
    if err != nil {
        t.Fatalf("connect: %v", err)
    }
    t.Cleanup(func() { d.Close() })
    return d
}

func TestGetZones(t *testing.T) {
    d := getTestDB(t)
    zones, err := d.GetZones(context.Background())
    if err != nil {
        t.Fatalf("get zones: %v", err)
    }
    if len(zones) != 7 {
        t.Errorf("expected 7 zones, got %d", len(zones))
    }
}

func TestGetSnapshots_EmptyRange(t *testing.T) {
    d := getTestDB(t)
    from := time.Now().Add(-1 * time.Hour)
    to := time.Now()
    snaps, err := d.GetSnapshots(context.Background(), "brest-bts", from, to)
    if err != nil {
        t.Fatalf("get snapshots: %v", err)
    }
    // May be empty, but should not error
    _ = snaps
}

func TestGetSnapshots_WithData(t *testing.T) {
    d := getTestDB(t)
    ctx := context.Background()

    // Insert test data
    _, err := d.pool.Exec(ctx,
        `INSERT INTO snapshots (zone_id, captured_at, cars_count, sent_last_hour, sent_last_24h)
         VALUES ('brest-bts', NOW(), 10, 2, 50)`)
    if err != nil {
        t.Fatalf("insert test data: %v", err)
    }

    from := time.Now().Add(-1 * time.Hour)
    to := time.Now().Add(1 * time.Hour)
    snaps, err := d.GetSnapshots(ctx, "brest-bts", from, to)
    if err != nil {
        t.Fatalf("get snapshots: %v", err)
    }
    if len(snaps) == 0 {
        t.Error("expected at least one snapshot")
    }
}

func TestGetSnapshots_Aggregation(t *testing.T) {
    d := getTestDB(t)
    ctx := context.Background()

    // Insert 10 snapshots over 3 days
    base := time.Now().Add(-72 * time.Hour)
    for i := 0; i < 10; i++ {
        _, err := d.pool.Exec(ctx,
            `INSERT INTO snapshots (zone_id, captured_at, cars_count, sent_last_hour, sent_last_24h)
             VALUES ('brest-bts', $1, $2, 0, 0)`,
            base.Add(time.Duration(i)*7*time.Hour), i*5)
        if err != nil {
            t.Fatalf("insert: %v", err)
        }
    }

    // 3-day range should aggregate hourly
    snaps, err := d.GetSnapshots(ctx, "brest-bts", base, time.Now())
    if err != nil {
        t.Fatalf("get snapshots: %v", err)
    }
    if len(snaps) == 0 {
        t.Error("expected aggregated snapshots")
    }
}

func TestGetCurrentVehicles(t *testing.T) {
    d := getTestDB(t)
    ctx := context.Background()

    vehicles, err := d.GetCurrentVehicles(ctx, "brest-bts")
    if err != nil {
        t.Fatalf("get vehicles: %v", err)
    }
    // May be empty
    _ = vehicles
}
```

**Step 3: Implement DB query layer**

```go
// backend/db/db.go
package db

import (
    "context"
    "fmt"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
    pool *pgxpool.Pool
}

type Zone struct {
    ID     string `json:"id"`
    Name   string `json:"name"`
    Border string `json:"border"`
}

type ZoneWithCount struct {
    Zone
    CarsCount    int       `json:"cars_count"`
    LastCaptured time.Time `json:"last_captured"`
}

type SnapshotPoint struct {
    CapturedAt   time.Time `json:"captured_at"`
    CarsCount    float64   `json:"cars_count"`
    SentLastHour float64   `json:"sent_last_hour"`
    SentLast24h  float64   `json:"sent_last_24h"`
}

type VehicleRow struct {
    RegNumber       string    `json:"reg_number"`
    QueueType       string    `json:"queue_type"`
    RegisteredAt    time.Time `json:"registered_at"`
    StatusChangedAt time.Time `json:"status_changed_at"`
    Status          string    `json:"status"`
}

func New(ctx context.Context, dsn string) (*DB, error) {
    pool, err := pgxpool.New(ctx, dsn)
    if err != nil {
        return nil, fmt.Errorf("connect: %w", err)
    }
    if err := pool.Ping(ctx); err != nil {
        return nil, fmt.Errorf("ping: %w", err)
    }
    return &DB{pool: pool}, nil
}

func (d *DB) Close() { d.pool.Close() }

func (d *DB) GetZones(ctx context.Context) ([]ZoneWithCount, error) {
    rows, err := d.pool.Query(ctx, `
        SELECT z.id, z.name, z.border,
               COALESCE(s.cars_count, 0),
               COALESCE(s.captured_at, '1970-01-01'::timestamptz)
        FROM zones z
        LEFT JOIN LATERAL (
            SELECT cars_count, captured_at FROM snapshots
            WHERE zone_id = z.id ORDER BY captured_at DESC LIMIT 1
        ) s ON true
        ORDER BY z.border, z.name`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var zones []ZoneWithCount
    for rows.Next() {
        var z ZoneWithCount
        if err := rows.Scan(&z.ID, &z.Name, &z.Border, &z.CarsCount, &z.LastCaptured); err != nil {
            return nil, err
        }
        zones = append(zones, z)
    }
    return zones, rows.Err()
}

func (d *DB) GetSnapshots(ctx context.Context, zoneID string, from, to time.Time) ([]SnapshotPoint, error) {
    duration := to.Sub(from)
    query := selectAggregatedQuery(duration)

    rows, err := d.pool.Query(ctx, query, zoneID, from, to)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var points []SnapshotPoint
    for rows.Next() {
        var p SnapshotPoint
        if err := rows.Scan(&p.CapturedAt, &p.CarsCount, &p.SentLastHour, &p.SentLast24h); err != nil {
            return nil, err
        }
        points = append(points, p)
    }
    return points, rows.Err()
}

func selectAggregatedQuery(duration time.Duration) string {
    switch {
    case duration < 24*time.Hour:
        // Raw 15-min snapshots
        return `SELECT captured_at, cars_count::float8, sent_last_hour::float8, sent_last_24h::float8
                FROM snapshots WHERE zone_id=$1 AND captured_at BETWEEN $2 AND $3
                ORDER BY captured_at`
    case duration < 7*24*time.Hour:
        // Hourly averages
        return `SELECT date_trunc('hour', captured_at) AS t,
                       AVG(cars_count), AVG(sent_last_hour), AVG(sent_last_24h)
                FROM snapshots WHERE zone_id=$1 AND captured_at BETWEEN $2 AND $3
                GROUP BY t ORDER BY t`
    case duration < 90*24*time.Hour:
        // 6-hour averages
        return `SELECT date_trunc('day', captured_at) + INTERVAL '6 hours' * FLOOR(EXTRACT(HOUR FROM captured_at)/6) AS t,
                       AVG(cars_count), AVG(sent_last_hour), AVG(sent_last_24h)
                FROM snapshots WHERE zone_id=$1 AND captured_at BETWEEN $2 AND $3
                GROUP BY t ORDER BY t`
    default:
        // Daily averages
        return `SELECT date_trunc('day', captured_at) AS t,
                       AVG(cars_count), AVG(sent_last_hour), AVG(sent_last_24h)
                FROM snapshots WHERE zone_id=$1 AND captured_at BETWEEN $2 AND $3
                GROUP BY t ORDER BY t`
    }
}

func (d *DB) GetCurrentVehicles(ctx context.Context, zoneID string) ([]VehicleRow, error) {
    rows, err := d.pool.Query(ctx, `
        SELECT v.reg_number, v.queue_type, v.registered_at, v.status_changed_at, v.status
        FROM vehicles v
        JOIN snapshots s ON v.snapshot_id = s.id
        WHERE v.zone_id = $1
          AND s.id = (SELECT id FROM snapshots WHERE zone_id=$1 ORDER BY captured_at DESC LIMIT 1)
        ORDER BY v.registered_at`, zoneID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var vehicles []VehicleRow
    for rows.Next() {
        var v VehicleRow
        if err := rows.Scan(&v.RegNumber, &v.QueueType, &v.RegisteredAt, &v.StatusChangedAt, &v.Status); err != nil {
            return nil, err
        }
        vehicles = append(vehicles, v)
    }
    return vehicles, rows.Err()
}

func (d *DB) GetVehicleHistory(ctx context.Context, zoneID string, from, to time.Time) ([]VehicleRow, error) {
    rows, err := d.pool.Query(ctx, `
        SELECT v.reg_number, v.queue_type, v.registered_at, v.status_changed_at, v.status
        FROM vehicles v
        WHERE v.zone_id = $1 AND v.registered_at BETWEEN $2 AND $3
        ORDER BY v.registered_at`, zoneID, from, to)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var vehicles []VehicleRow
    for rows.Next() {
        var v VehicleRow
        if err := rows.Scan(&v.RegNumber, &v.QueueType, &v.RegisteredAt, &v.StatusChangedAt, &v.Status); err != nil {
            return nil, err
        }
        vehicles = append(vehicles, v)
    }
    return vehicles, rows.Err()
}
```

**Step 4: Implement migration runner**

```go
// backend/db/migrate.go
package db

import (
    "embed"
    "fmt"

    "github.com/golang-migrate/migrate/v4"
    _ "github.com/golang-migrate/migrate/v4/database/postgres"
    "github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func RunMigrations(dsn string) error {
    source, err := iofs.New(migrationsFS, "migrations")
    if err != nil {
        return fmt.Errorf("migration source: %w", err)
    }
    m, err := migrate.NewWithSourceInstance("iofs", source, dsn)
    if err != nil {
        return fmt.Errorf("migrate init: %w", err)
    }
    if err := m.Up(); err != nil && err != migrate.ErrNoChange {
        return fmt.Errorf("migrate up: %w", err)
    }
    return nil
}
```

Note: Copy the migration SQL files from `migrations/` into `backend/db/migrations/` so they can be embedded. Alternatively, symlink them.

**Step 5: Run tests**

```bash
cd backend && go test ./db/ -v
```

**Step 6: Commit**

```bash
git add backend/
git commit -m "feat: add backend DB layer with queries, aggregation, and migration runner"
```

---

## Task 5: Backend — HTTP API Handlers (Agent 3)

**Files:**
- Create: `backend/api/handlers.go`
- Create: `backend/api/handlers_test.go`
- Create: `backend/api/router.go`

**Step 1: Write handler tests**

```go
// backend/api/handlers_test.go
package api

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/imbroyury/border/backend/db"
)

// Mock DB for unit tests
type mockDB struct {
    zones    []db.ZoneWithCount
    snaps    []db.SnapshotPoint
    vehicles []db.VehicleRow
    err      error
}

func (m *mockDB) GetZones(ctx context.Context) ([]db.ZoneWithCount, error) {
    return m.zones, m.err
}
func (m *mockDB) GetSnapshots(ctx context.Context, zoneID string, from, to time.Time) ([]db.SnapshotPoint, error) {
    return m.snaps, m.err
}
func (m *mockDB) GetCurrentVehicles(ctx context.Context, zoneID string) ([]db.VehicleRow, error) {
    return m.vehicles, m.err
}
func (m *mockDB) GetVehicleHistory(ctx context.Context, zoneID string, from, to time.Time) ([]db.VehicleRow, error) {
    return m.vehicles, m.err
}

func TestGetZones_Success(t *testing.T) {
    mock := &mockDB{
        zones: []db.ZoneWithCount{
            {Zone: db.Zone{ID: "brest-bts", Name: "Брест", Border: "BY-PL"}, CarsCount: 5},
        },
    }
    h := NewHandler(mock)
    r := NewRouter(h)

    req := httptest.NewRequest("GET", "/api/zones", nil)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("expected 200, got %d", w.Code)
    }

    var zones []db.ZoneWithCount
    if err := json.NewDecoder(w.Body).Decode(&zones); err != nil {
        t.Fatalf("decode: %v", err)
    }
    if len(zones) != 1 {
        t.Errorf("expected 1 zone, got %d", len(zones))
    }
}

func TestGetSnapshots_Success(t *testing.T) {
    mock := &mockDB{
        snaps: []db.SnapshotPoint{
            {CapturedAt: time.Now(), CarsCount: 10},
        },
    }
    h := NewHandler(mock)
    r := NewRouter(h)

    req := httptest.NewRequest("GET", "/api/zones/brest-bts/snapshots?from=2026-01-01T00:00:00Z&to=2026-12-31T00:00:00Z", nil)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("expected 200, got %d", w.Code)
    }
}

func TestGetSnapshots_MissingParams(t *testing.T) {
    h := NewHandler(&mockDB{})
    r := NewRouter(h)

    req := httptest.NewRequest("GET", "/api/zones/brest-bts/snapshots", nil)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    if w.Code != http.StatusBadRequest {
        t.Errorf("expected 400, got %d", w.Code)
    }
}

func TestGetCurrentVehicles_Success(t *testing.T) {
    mock := &mockDB{
        vehicles: []db.VehicleRow{
            {RegNumber: "AB1234", Status: "waiting"},
        },
    }
    h := NewHandler(mock)
    r := NewRouter(h)

    req := httptest.NewRequest("GET", "/api/zones/brest-bts/vehicles", nil)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("expected 200, got %d", w.Code)
    }
}

func TestGetVehicleHistory_Success(t *testing.T) {
    mock := &mockDB{}
    h := NewHandler(mock)
    r := NewRouter(h)

    req := httptest.NewRequest("GET", "/api/zones/brest-bts/vehicles/history?from=2026-01-01T00:00:00Z&to=2026-12-31T00:00:00Z", nil)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("expected 200, got %d", w.Code)
    }
}
```

**Step 2: Implement handlers and router**

```go
// backend/api/handlers.go
package api

import (
    "context"
    "encoding/json"
    "net/http"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/imbroyury/border/backend/db"
)

type Querier interface {
    GetZones(ctx context.Context) ([]db.ZoneWithCount, error)
    GetSnapshots(ctx context.Context, zoneID string, from, to time.Time) ([]db.SnapshotPoint, error)
    GetCurrentVehicles(ctx context.Context, zoneID string) ([]db.VehicleRow, error)
    GetVehicleHistory(ctx context.Context, zoneID string, from, to time.Time) ([]db.VehicleRow, error)
}

type Handler struct {
    db Querier
}

func NewHandler(db Querier) *Handler {
    return &Handler{db: db}
}

func (h *Handler) GetZones(w http.ResponseWriter, r *http.Request) {
    zones, err := h.db.GetZones(r.Context())
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    writeJSON(w, zones)
}

func (h *Handler) GetSnapshots(w http.ResponseWriter, r *http.Request) {
    zoneID := chi.URLParam(r, "id")
    from, to, err := parseTimeRange(r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    snaps, err := h.db.GetSnapshots(r.Context(), zoneID, from, to)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    writeJSON(w, snaps)
}

func (h *Handler) GetCurrentVehicles(w http.ResponseWriter, r *http.Request) {
    zoneID := chi.URLParam(r, "id")
    vehicles, err := h.db.GetCurrentVehicles(r.Context(), zoneID)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    writeJSON(w, vehicles)
}

func (h *Handler) GetVehicleHistory(w http.ResponseWriter, r *http.Request) {
    zoneID := chi.URLParam(r, "id")
    from, to, err := parseTimeRange(r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    vehicles, err := h.db.GetVehicleHistory(r.Context(), zoneID, from, to)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    writeJSON(w, vehicles)
}

func parseTimeRange(r *http.Request) (time.Time, time.Time, error) {
    fromStr := r.URL.Query().Get("from")
    toStr := r.URL.Query().Get("to")
    if fromStr == "" || toStr == "" {
        return time.Time{}, time.Time{}, fmt.Errorf("from and to query params are required")
    }
    from, err := time.Parse(time.RFC3339, fromStr)
    if err != nil {
        return time.Time{}, time.Time{}, fmt.Errorf("invalid from: %w", err)
    }
    to, err := time.Parse(time.RFC3339, toStr)
    if err != nil {
        return time.Time{}, time.Time{}, fmt.Errorf("invalid to: %w", err)
    }
    return from, to, nil
}

func writeJSON(w http.ResponseWriter, v any) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(v)
}
```

```go
// backend/api/router.go
package api

import (
    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    "github.com/go-chi/cors"
)

func NewRouter(h *Handler) *chi.Mux {
    r := chi.NewRouter()
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)
    r.Use(cors.Handler(cors.Options{
        AllowedOrigins: []string{"*"},
        AllowedMethods: []string{"GET"},
    }))

    r.Route("/api", func(r chi.Router) {
        r.Get("/zones", h.GetZones)
        r.Get("/zones/{id}/snapshots", h.GetSnapshots)
        r.Get("/zones/{id}/vehicles", h.GetCurrentVehicles)
        r.Get("/zones/{id}/vehicles/history", h.GetVehicleHistory)
    })

    return r
}
```

**Step 3: Run tests**

```bash
cd backend && go test ./api/ -v
```

**Step 4: Commit**

```bash
git add backend/api/
git commit -m "feat: add backend HTTP API handlers with tests"
```

---

## Task 6: Backend — Main Entrypoint (Agent 3)

**Files:**
- Modify: `backend/main.go`

**Step 1: Write main.go**

```go
// backend/main.go
package main

import (
    "context"
    "log/slog"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/imbroyury/border/backend/api"
    "github.com/imbroyury/border/backend/db"
)

func main() {
    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

    dbURL := os.Getenv("DATABASE_URL")
    if dbURL == "" {
        logger.Error("DATABASE_URL is required")
        os.Exit(1)
    }

    // Run migrations
    if err := db.RunMigrations(dbURL); err != nil {
        logger.Error("migration failed", "error", err)
        os.Exit(1)
    }
    logger.Info("migrations applied")

    ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer cancel()

    database, err := db.New(ctx, dbURL)
    if err != nil {
        logger.Error("failed to connect to database", "error", err)
        os.Exit(1)
    }
    defer database.Close()

    handler := api.NewHandler(database)
    router := api.NewRouter(handler)

    addr := os.Getenv("LISTEN_ADDR")
    if addr == "" {
        addr = ":8080"
    }

    srv := &http.Server{Addr: addr, Handler: router}

    go func() {
        logger.Info("backend listening", "addr", addr)
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            logger.Error("server error", "error", err)
            os.Exit(1)
        }
    }()

    <-ctx.Done()
    logger.Info("shutting down")

    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer shutdownCancel()
    srv.Shutdown(shutdownCtx)
}
```

**Step 2: Verify it compiles**

```bash
cd backend && go build -o /dev/null .
```

**Step 3: Commit**

```bash
git add backend/main.go
git commit -m "feat: add backend main entrypoint with migration and graceful shutdown"
```

---

## Task 7: Frontend — Project Setup & API Client (Agent 4)

**Files:**
- Create: `frontend/` (via `npm create vue@latest`)
- Create: `frontend/src/api/client.ts`
- Create: `frontend/src/api/client.test.ts`
- Create: `frontend/src/api/types.ts`

**Step 1: Scaffold Vue 3 project**

```bash
cd frontend
npm create vue@latest . -- --typescript --router --pinia
npm install
npm install vue-echarts echarts
npm install -D vitest @vue/test-utils jsdom
```

Add to `frontend/vite.config.ts`:
```ts
test: {
  environment: 'jsdom',
}
```

**Step 2: Define API types**

```ts
// frontend/src/api/types.ts
export interface Zone {
  id: string
  name: string
  border: string
  cars_count: number
  last_captured: string
}

export interface SnapshotPoint {
  captured_at: string
  cars_count: number
  sent_last_hour: number
  sent_last_24h: number
}

export interface Vehicle {
  reg_number: string
  queue_type: string
  registered_at: string
  status_changed_at: string
  status: string
}

export type DurationPreset =
  | '1h' | '3h' | '6h' | '9h' | '12h'
  | '1d' | '2d' | '3d' | '5d' | '7d' | '14d'
  | '1m' | '2m' | '3m' | '6m' | '1y' | 'all'
```

**Step 3: Write API client with tests**

```ts
// frontend/src/api/client.ts
import type { Zone, SnapshotPoint, Vehicle } from './types'

const BASE_URL = import.meta.env.VITE_API_URL || '/api'

async function fetchJSON<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`)
  if (!res.ok) throw new Error(`API error: ${res.status}`)
  return res.json()
}

export function getZones(): Promise<Zone[]> {
  return fetchJSON('/zones')
}

export function getSnapshots(zoneId: string, from: Date, to: Date): Promise<SnapshotPoint[]> {
  return fetchJSON(`/zones/${zoneId}/snapshots?from=${from.toISOString()}&to=${to.toISOString()}`)
}

export function getCurrentVehicles(zoneId: string): Promise<Vehicle[]> {
  return fetchJSON(`/zones/${zoneId}/vehicles`)
}

export function getVehicleHistory(zoneId: string, from: Date, to: Date): Promise<Vehicle[]> {
  return fetchJSON(`/zones/${zoneId}/vehicles/history?from=${from.toISOString()}&to=${to.toISOString()}`)
}
```

```ts
// frontend/src/api/client.test.ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { getZones, getSnapshots, getCurrentVehicles } from './client'

const mockFetch = vi.fn()
vi.stubGlobal('fetch', mockFetch)

beforeEach(() => {
  mockFetch.mockReset()
})

describe('API client', () => {
  it('getZones returns zone list', async () => {
    const zones = [{ id: 'brest-bts', name: 'Брест', border: 'BY-PL', cars_count: 5 }]
    mockFetch.mockResolvedValue({ ok: true, json: () => Promise.resolve(zones) })

    const result = await getZones()
    expect(result).toEqual(zones)
    expect(mockFetch).toHaveBeenCalledWith(expect.stringContaining('/zones'))
  })

  it('getSnapshots sends correct time range', async () => {
    mockFetch.mockResolvedValue({ ok: true, json: () => Promise.resolve([]) })

    const from = new Date('2026-01-01')
    const to = new Date('2026-01-02')
    await getSnapshots('brest-bts', from, to)

    const url = mockFetch.mock.calls[0][0] as string
    expect(url).toContain('brest-bts')
    expect(url).toContain('from=')
    expect(url).toContain('to=')
  })

  it('throws on non-OK response', async () => {
    mockFetch.mockResolvedValue({ ok: false, status: 500 })
    await expect(getZones()).rejects.toThrow('API error: 500')
  })

  it('getCurrentVehicles returns vehicle list', async () => {
    const vehicles = [{ reg_number: 'AB123', status: 'waiting' }]
    mockFetch.mockResolvedValue({ ok: true, json: () => Promise.resolve(vehicles) })

    const result = await getCurrentVehicles('brest-bts')
    expect(result).toEqual(vehicles)
  })
})
```

**Step 4: Run tests**

```bash
cd frontend && npx vitest run
```

**Step 5: Commit**

```bash
git add frontend/
git commit -m "feat: scaffold Vue 3 project with API client and types"
```

---

## Task 8: Frontend — Dashboard & Zone Cards (Agent 4)

**Files:**
- Create: `frontend/src/views/DashboardView.vue`
- Create: `frontend/src/views/ZoneDetailView.vue`
- Create: `frontend/src/components/ZoneCard.vue`
- Create: `frontend/src/components/ZoneCard.test.ts`
- Modify: `frontend/src/router/index.ts`

**Step 1: Set up routes**

```ts
// frontend/src/router/index.ts
import { createRouter, createWebHistory } from 'vue-router'
import DashboardView from '@/views/DashboardView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'dashboard', component: DashboardView },
    {
      path: '/zone/:id',
      name: 'zone-detail',
      component: () => import('@/views/ZoneDetailView.vue'),
    },
  ],
})

export default router
```

**Step 2: Write ZoneCard component with test**

```vue
<!-- frontend/src/components/ZoneCard.vue -->
<script setup lang="ts">
import type { Zone } from '@/api/types'

const props = defineProps<{ zone: Zone }>()
</script>

<template>
  <router-link :to="`/zone/${zone.id}`" class="zone-card">
    <div class="zone-header">
      <h3>{{ zone.name }}</h3>
      <span class="border-badge">{{ zone.border }}</span>
    </div>
    <div class="zone-count">
      <span class="count">{{ zone.cars_count }}</span>
      <span class="label">cars in queue</span>
    </div>
  </router-link>
</template>

<style scoped>
.zone-card {
  display: block;
  padding: 1rem;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  text-decoration: none;
  color: inherit;
  transition: box-shadow 0.2s;
}
.zone-card:hover {
  box-shadow: 0 2px 8px rgba(0,0,0,0.1);
}
.zone-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
}
.border-badge {
  font-size: 0.75rem;
  padding: 2px 6px;
  border-radius: 4px;
  background: #edf2f7;
}
.count {
  font-size: 2rem;
  font-weight: bold;
}
.label {
  display: block;
  color: #718096;
  font-size: 0.875rem;
}
</style>
```

```ts
// frontend/src/components/ZoneCard.test.ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import ZoneCard from './ZoneCard.vue'

const router = createRouter({
  history: createMemoryHistory(),
  routes: [{ path: '/zone/:id', component: { template: '<div />' } }],
})

describe('ZoneCard', () => {
  it('renders zone name and count', () => {
    const zone = { id: 'brest-bts', name: 'Брест', border: 'BY-PL', cars_count: 42, last_captured: '' }
    const wrapper = mount(ZoneCard, {
      props: { zone },
      global: { plugins: [router] },
    })
    expect(wrapper.text()).toContain('Брест')
    expect(wrapper.text()).toContain('42')
    expect(wrapper.text()).toContain('BY-PL')
  })

  it('links to zone detail page', () => {
    const zone = { id: 'brest-bts', name: 'Брест', border: 'BY-PL', cars_count: 0, last_captured: '' }
    const wrapper = mount(ZoneCard, {
      props: { zone },
      global: { plugins: [router] },
    })
    const link = wrapper.find('a')
    expect(link.attributes('href')).toBe('/zone/brest-bts')
  })
})
```

**Step 3: Write DashboardView**

```vue
<!-- frontend/src/views/DashboardView.vue -->
<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import type { Zone } from '@/api/types'
import { getZones } from '@/api/client'
import ZoneCard from '@/components/ZoneCard.vue'

const zones = ref<Zone[]>([])
const error = ref('')
let interval: ReturnType<typeof setInterval>

async function fetchData() {
  try {
    zones.value = await getZones()
    error.value = ''
  } catch (e) {
    error.value = String(e)
  }
}

onMounted(() => {
  fetchData()
  interval = setInterval(fetchData, 60_000)
})

onUnmounted(() => clearInterval(interval))
</script>

<template>
  <div class="dashboard">
    <h1>Border Queue Monitor</h1>
    <p v-if="error" class="error">{{ error }}</p>
    <div class="zone-grid">
      <ZoneCard v-for="zone in zones" :key="zone.id" :zone="zone" />
    </div>
  </div>
</template>

<style scoped>
.dashboard { max-width: 1200px; margin: 0 auto; padding: 2rem; }
.zone-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 1rem; }
.error { color: red; }
</style>
```

**Step 4: Write ZoneDetailView (placeholder for now)**

```vue
<!-- frontend/src/views/ZoneDetailView.vue -->
<script setup lang="ts">
import { useRoute } from 'vue-router'

const route = useRoute()
const zoneId = route.params.id as string
</script>

<template>
  <div class="zone-detail">
    <router-link to="/">← Back</router-link>
    <h1>Zone: {{ zoneId }}</h1>
    <!-- Chart and vehicle table added in Task 9 -->
  </div>
</template>

<style scoped>
.zone-detail { max-width: 1200px; margin: 0 auto; padding: 2rem; }
</style>
```

**Step 5: Run tests**

```bash
cd frontend && npx vitest run
```

**Step 6: Commit**

```bash
git add frontend/src/
git commit -m "feat: add dashboard view with zone cards and routing"
```

---

## Task 9: Frontend — Charts, Duration Picker & Vehicle Table (Agent 4)

**Files:**
- Create: `frontend/src/components/QueueChart.vue`
- Create: `frontend/src/components/QueueChart.test.ts`
- Create: `frontend/src/components/DurationPicker.vue`
- Create: `frontend/src/components/DurationPicker.test.ts`
- Create: `frontend/src/components/VehicleTable.vue`
- Create: `frontend/src/components/VehicleTable.test.ts`
- Modify: `frontend/src/views/ZoneDetailView.vue`

**Step 1: Write DurationPicker with test**

```vue
<!-- frontend/src/components/DurationPicker.vue -->
<script setup lang="ts">
import type { DurationPreset } from '@/api/types'

const model = defineModel<DurationPreset>({ default: '24h' })

const options: { value: DurationPreset; label: string }[] = [
  { value: '1h', label: '1 hour' },
  { value: '3h', label: '3 hours' },
  { value: '6h', label: '6 hours' },
  { value: '9h', label: '9 hours' },
  { value: '12h', label: '12 hours' },
  { value: '1d', label: '1 day' },
  { value: '2d', label: '2 days' },
  { value: '3d', label: '3 days' },
  { value: '5d', label: '5 days' },
  { value: '7d', label: '7 days' },
  { value: '14d', label: '14 days' },
  { value: '1m', label: '1 month' },
  { value: '2m', label: '2 months' },
  { value: '3m', label: '3 months' },
  { value: '6m', label: '6 months' },
  { value: '1y', label: '1 year' },
  { value: 'all', label: 'All time' },
]
</script>

<template>
  <select v-model="model" class="duration-picker">
    <option v-for="opt in options" :key="opt.value" :value="opt.value">
      {{ opt.label }}
    </option>
  </select>
</template>
```

```ts
// frontend/src/components/DurationPicker.test.ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import DurationPicker from './DurationPicker.vue'

describe('DurationPicker', () => {
  it('renders all duration options', () => {
    const wrapper = mount(DurationPicker)
    const options = wrapper.findAll('option')
    expect(options.length).toBe(17)
  })

  it('emits update on selection', async () => {
    const wrapper = mount(DurationPicker)
    await wrapper.find('select').setValue('7d')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['7d'])
  })
})
```

**Step 2: Write QueueChart with test**

```vue
<!-- frontend/src/components/QueueChart.vue -->
<script setup lang="ts">
import { computed } from 'vue'
import VChart from 'vue-echarts'
import type { SnapshotPoint } from '@/api/types'

const props = defineProps<{ data: SnapshotPoint[] }>()

const chartOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  xAxis: {
    type: 'time',
  },
  yAxis: {
    type: 'value',
    name: 'Cars',
    minInterval: 1,
  },
  series: [
    {
      name: 'Cars in queue',
      type: 'line',
      smooth: true,
      data: props.data.map(p => [p.captured_at, p.cars_count]),
      areaStyle: { opacity: 0.1 },
    },
  ],
  dataZoom: [{ type: 'inside' }],
}))
</script>

<template>
  <v-chart :option="chartOption" autoresize style="height: 400px" />
</template>
```

```ts
// frontend/src/components/QueueChart.test.ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import QueueChart from './QueueChart.vue'

// vue-echarts needs a mock in test env
vi.mock('vue-echarts', () => ({
  default: { template: '<div class="chart-mock" />', props: ['option'] },
}))

describe('QueueChart', () => {
  it('renders without data', () => {
    const wrapper = mount(QueueChart, { props: { data: [] } })
    expect(wrapper.find('.chart-mock').exists()).toBe(true)
  })

  it('renders with data', () => {
    const data = [
      { captured_at: '2026-01-01T00:00:00Z', cars_count: 10, sent_last_hour: 1, sent_last_24h: 20 },
    ]
    const wrapper = mount(QueueChart, { props: { data } })
    expect(wrapper.find('.chart-mock').exists()).toBe(true)
  })
})
```

**Step 3: Write VehicleTable with test**

```vue
<!-- frontend/src/components/VehicleTable.vue -->
<script setup lang="ts">
import type { Vehicle } from '@/api/types'

defineProps<{ vehicles: Vehicle[] }>()
</script>

<template>
  <table class="vehicle-table">
    <thead>
      <tr>
        <th>Reg Number</th>
        <th>Queue Type</th>
        <th>Registered</th>
        <th>Status Changed</th>
        <th>Status</th>
      </tr>
    </thead>
    <tbody>
      <tr v-if="vehicles.length === 0">
        <td colspan="5" class="empty">No vehicles in queue</td>
      </tr>
      <tr v-for="v in vehicles" :key="v.reg_number">
        <td>{{ v.reg_number }}</td>
        <td>{{ v.queue_type }}</td>
        <td>{{ new Date(v.registered_at).toLocaleString() }}</td>
        <td>{{ new Date(v.status_changed_at).toLocaleString() }}</td>
        <td>{{ v.status }}</td>
      </tr>
    </tbody>
  </table>
</template>

<style scoped>
.vehicle-table { width: 100%; border-collapse: collapse; }
.vehicle-table th, .vehicle-table td { padding: 0.5rem; border-bottom: 1px solid #e2e8f0; text-align: left; }
.vehicle-table th { background: #f7fafc; font-weight: 600; }
.empty { text-align: center; color: #a0aec0; padding: 2rem; }
</style>
```

```ts
// frontend/src/components/VehicleTable.test.ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import VehicleTable from './VehicleTable.vue'

describe('VehicleTable', () => {
  it('shows empty message when no vehicles', () => {
    const wrapper = mount(VehicleTable, { props: { vehicles: [] } })
    expect(wrapper.text()).toContain('No vehicles in queue')
  })

  it('renders vehicle rows', () => {
    const vehicles = [
      { reg_number: 'AB123', queue_type: 'live', registered_at: '2026-01-01T00:00:00Z', status_changed_at: '2026-01-01T01:00:00Z', status: 'waiting' },
    ]
    const wrapper = mount(VehicleTable, { props: { vehicles } })
    expect(wrapper.text()).toContain('AB123')
    expect(wrapper.text()).toContain('waiting')
  })
})
```

**Step 4: Complete ZoneDetailView**

```vue
<!-- frontend/src/views/ZoneDetailView.vue -->
<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import type { SnapshotPoint, Vehicle, DurationPreset } from '@/api/types'
import { getSnapshots, getCurrentVehicles } from '@/api/client'
import QueueChart from '@/components/QueueChart.vue'
import DurationPicker from '@/components/DurationPicker.vue'
import VehicleTable from '@/components/VehicleTable.vue'

const route = useRoute()
const zoneId = route.params.id as string

const duration = ref<DurationPreset>('1d')
const snapshots = ref<SnapshotPoint[]>([])
const vehicles = ref<Vehicle[]>([])
const error = ref('')
let interval: ReturnType<typeof setInterval>

function durationToMs(d: DurationPreset): number {
  const map: Record<DurationPreset, number> = {
    '1h': 3600_000, '3h': 10800_000, '6h': 21600_000, '9h': 32400_000,
    '12h': 43200_000, '1d': 86400_000, '2d': 172800_000, '3d': 259200_000,
    '5d': 432000_000, '7d': 604800_000, '14d': 1209600_000,
    '1m': 2592000_000, '2m': 5184000_000, '3m': 7776000_000,
    '6m': 15552000_000, '1y': 31536000_000, 'all': Date.now(),
  }
  return map[d]
}

async function fetchData() {
  try {
    const now = new Date()
    const from = new Date(now.getTime() - durationToMs(duration.value))
    const [snaps, vehs] = await Promise.all([
      getSnapshots(zoneId, from, now),
      getCurrentVehicles(zoneId),
    ])
    snapshots.value = snaps
    vehicles.value = vehs
    error.value = ''
  } catch (e) {
    error.value = String(e)
  }
}

watch(duration, fetchData)

onMounted(() => {
  fetchData()
  interval = setInterval(fetchData, 60_000)
})

onUnmounted(() => clearInterval(interval))
</script>

<template>
  <div class="zone-detail">
    <router-link to="/">← Back</router-link>
    <h1>{{ zoneId }}</h1>
    <p v-if="error" class="error">{{ error }}</p>

    <div class="chart-controls">
      <DurationPicker v-model="duration" />
    </div>

    <QueueChart :data="snapshots" />

    <h2>Current Vehicles</h2>
    <VehicleTable :vehicles="vehicles" />
  </div>
</template>

<style scoped>
.zone-detail { max-width: 1200px; margin: 0 auto; padding: 2rem; }
.chart-controls { margin: 1rem 0; }
.error { color: red; }
h2 { margin-top: 2rem; }
</style>
```

**Step 5: Run all frontend tests**

```bash
cd frontend && npx vitest run
```

**Step 6: Commit**

```bash
git add frontend/src/
git commit -m "feat: add chart, duration picker, vehicle table, and zone detail view"
```

---

## Task 10: Docker Setup (Sequential)

**Files:**
- Create: `crawler/Dockerfile`
- Create: `backend/Dockerfile`
- Create: `frontend/Dockerfile`
- Create: `frontend/Caddyfile`
- Create: `docker-compose.yml`
- Create: `.env.example`

**Step 1: Write Dockerfiles**

```dockerfile
# crawler/Dockerfile
FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /crawler .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=build /crawler /crawler
CMD ["/crawler"]
```

```dockerfile
# backend/Dockerfile
FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /backend .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=build /backend /backend
CMD ["/backend"]
```

```dockerfile
# frontend/Dockerfile
FROM node:20-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM caddy:2-alpine
COPY --from=build /app/dist /srv
COPY Caddyfile /etc/caddy/Caddyfile
```

```caddyfile
# frontend/Caddyfile
:80 {
    root * /srv
    file_server
    try_files {path} /index.html

    handle /api/* {
        reverse_proxy backend:8080
    }
}
```

**Step 2: Write docker-compose.yml**

```yaml
# docker-compose.yml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: border
      POSTGRES_USER: border
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-border_dev}
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U border"]
      interval: 5s
      retries: 5

  crawler:
    build: ./crawler
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      DATABASE_URL: postgres://border:${POSTGRES_PASSWORD:-border_dev}@postgres:5432/border?sslmode=disable
      CRAWL_INTERVAL: 15m
      SCRAPE_BASE_URL: https://mon.declarant.by
    restart: unless-stopped

  backend:
    build: ./backend
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      DATABASE_URL: postgres://border:${POSTGRES_PASSWORD:-border_dev}@postgres:5432/border?sslmode=disable
      LISTEN_ADDR: ":8080"
    restart: unless-stopped

  frontend:
    build: ./frontend
    depends_on:
      - backend
    ports:
      - "80:80"
    restart: unless-stopped

volumes:
  pgdata:
```

```env
# .env.example
POSTGRES_PASSWORD=change_me_in_production
```

**Step 3: Verify docker compose config is valid**

```bash
docker compose config
```

**Step 4: Commit**

```bash
git add crawler/Dockerfile backend/Dockerfile frontend/Dockerfile frontend/Caddyfile docker-compose.yml .env.example
git commit -m "feat: add Docker setup with compose, Caddy, and multi-stage builds"
```

---

## Task 11: Integration Test & First Deploy (Sequential)

**Step 1: Build and start all containers**

```bash
docker compose build
docker compose up -d
```

**Step 2: Verify all containers are running**

```bash
docker compose ps
```

Expected: all 4 services `Up` / `healthy`.

**Step 3: Verify backend API responds**

```bash
curl http://localhost:80/api/zones
```

Expected: JSON array of 7 zones.

**Step 4: Verify frontend loads**

```bash
curl -s http://localhost:80 | head -20
```

Expected: HTML with Vue app.

**Step 5: Check crawler logs**

```bash
docker compose logs crawler --tail=20
```

Expected: crawl started/completed messages.

**Step 6: Fix any issues found, commit**

```bash
git add -A
git commit -m "fix: integration fixes from first deploy"
```
