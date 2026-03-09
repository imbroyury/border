package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/imbroyury/border/backend/db"
)

// mockDB implements Querier for testing.
type mockDB struct {
	zones                  []db.ZoneWithCount
	zonesErr               error
	snapshots              []db.SnapshotPoint
	snapshotsErr           error
	vehicles               []db.VehicleRow
	vehiclesErr            error
	historyVehicles        []db.VehicleRow
	historyErr             error
	singleVehicleHistory   []db.VehicleStatusChange
	singleVehicleHistoryErr error
}

func (m *mockDB) GetZones(_ context.Context) ([]db.ZoneWithCount, error) {
	return m.zones, m.zonesErr
}

func (m *mockDB) GetSnapshots(_ context.Context, _ string, _, _ time.Time) ([]db.SnapshotPoint, error) {
	return m.snapshots, m.snapshotsErr
}

func (m *mockDB) GetCurrentVehicles(_ context.Context, _ string) ([]db.VehicleRow, error) {
	return m.vehicles, m.vehiclesErr
}

func (m *mockDB) GetVehicleHistory(_ context.Context, _ string, _, _ time.Time) ([]db.VehicleRow, error) {
	return m.historyVehicles, m.historyErr
}

func (m *mockDB) GetSingleVehicleHistory(_ context.Context, _, _ string) ([]db.VehicleStatusChange, error) {
	return m.singleVehicleHistory, m.singleVehicleHistoryErr
}

func newTestServer(mock *mockDB) *httptest.Server {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(mock, logger)
	r := NewRouter(h)
	return httptest.NewServer(r)
}

func TestGetZones_Success(t *testing.T) {
	now := time.Now().UTC()
	mock := &mockDB{
		zones: []db.ZoneWithCount{
			{Zone: db.Zone{ID: "brest", Name: "Брест", Border: "BY-PL"}, CarsCount: 42, LastCaptured: now},
			{Zone: db.Zone{ID: "bruzgi", Name: "Брузги", Border: "BY-PL"}, CarsCount: 10, LastCaptured: now},
		},
	}
	srv := newTestServer(mock)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/zones")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}

	var zones []db.ZoneWithCount
	if err := json.NewDecoder(resp.Body).Decode(&zones); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(zones) != 2 {
		t.Fatalf("expected 2 zones, got %d", len(zones))
	}
	if zones[0].ID != "brest" {
		t.Errorf("expected brest, got %s", zones[0].ID)
	}
}

func TestGetZones_DBError(t *testing.T) {
	mock := &mockDB{zonesErr: errors.New("db connection lost")}
	srv := newTestServer(mock)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/zones")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
}

func TestGetSnapshots_Success(t *testing.T) {
	now := time.Now().UTC()
	mock := &mockDB{
		snapshots: []db.SnapshotPoint{
			{CapturedAt: now, CarsCount: 10, SentLastHour: 5, SentLast24h: 50},
		},
	}
	srv := newTestServer(mock)
	defer srv.Close()

	from := now.Add(-time.Hour).Format(time.RFC3339)
	to := now.Format(time.RFC3339)
	resp, err := http.Get(srv.URL + "/api/zones/brest/snapshots?from=" + from + "&to=" + to)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}

	var points []db.SnapshotPoint
	if err := json.NewDecoder(resp.Body).Decode(&points); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(points))
	}
}

func TestGetSnapshots_MissingFrom(t *testing.T) {
	mock := &mockDB{}
	srv := newTestServer(mock)
	defer srv.Close()

	to := time.Now().Format(time.RFC3339)
	resp, err := http.Get(srv.URL + "/api/zones/brest/snapshots?to=" + to)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetSnapshots_MissingTo(t *testing.T) {
	mock := &mockDB{}
	srv := newTestServer(mock)
	defer srv.Close()

	from := time.Now().Format(time.RFC3339)
	resp, err := http.Get(srv.URL + "/api/zones/brest/snapshots?from=" + from)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetSnapshots_InvalidFromDate(t *testing.T) {
	mock := &mockDB{}
	srv := newTestServer(mock)
	defer srv.Close()

	to := time.Now().Format(time.RFC3339)
	resp, err := http.Get(srv.URL + "/api/zones/brest/snapshots?from=not-a-date&to=" + to)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetSnapshots_InvalidToDate(t *testing.T) {
	mock := &mockDB{}
	srv := newTestServer(mock)
	defer srv.Close()

	from := time.Now().Format(time.RFC3339)
	resp, err := http.Get(srv.URL + "/api/zones/brest/snapshots?from=" + from + "&to=bad-date")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetSnapshots_DBError(t *testing.T) {
	mock := &mockDB{snapshotsErr: errors.New("db error")}
	srv := newTestServer(mock)
	defer srv.Close()

	from := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	resp, err := http.Get(srv.URL + "/api/zones/brest/snapshots?from=" + from + "&to=" + to)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}

func TestGetCurrentVehicles_Success(t *testing.T) {
	now := time.Now().UTC()
	mock := &mockDB{
		vehicles: []db.VehicleRow{
			{RegNumber: "AA1234-7", QueueType: "cargo", Status: "waiting", RegisteredAt: now, StatusChangedAt: now},
		},
	}
	srv := newTestServer(mock)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/zones/brest/vehicles")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}

	var vehicles []db.VehicleRow
	if err := json.NewDecoder(resp.Body).Decode(&vehicles); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(vehicles) != 1 {
		t.Fatalf("expected 1 vehicle, got %d", len(vehicles))
	}
	if vehicles[0].RegNumber != "AA1234-7" {
		t.Errorf("expected AA1234-7, got %s", vehicles[0].RegNumber)
	}
}

func TestGetCurrentVehicles_DBError(t *testing.T) {
	mock := &mockDB{vehiclesErr: errors.New("db error")}
	srv := newTestServer(mock)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/zones/brest/vehicles")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}

func TestGetVehicleHistory_Success(t *testing.T) {
	now := time.Now().UTC()
	mock := &mockDB{
		historyVehicles: []db.VehicleRow{
			{RegNumber: "CC9012-1", QueueType: "passenger", Status: "passed", RegisteredAt: now, StatusChangedAt: now},
		},
	}
	srv := newTestServer(mock)
	defer srv.Close()

	from := now.Add(-24 * time.Hour).Format(time.RFC3339)
	to := now.Format(time.RFC3339)
	resp, err := http.Get(srv.URL + "/api/zones/brest/vehicles/history?from=" + from + "&to=" + to)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}

	var vehicles []db.VehicleRow
	if err := json.NewDecoder(resp.Body).Decode(&vehicles); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(vehicles) != 1 {
		t.Fatalf("expected 1 vehicle, got %d", len(vehicles))
	}
}

func TestGetVehicleHistory_MissingFrom(t *testing.T) {
	mock := &mockDB{}
	srv := newTestServer(mock)
	defer srv.Close()

	to := time.Now().Format(time.RFC3339)
	resp, err := http.Get(srv.URL + "/api/zones/brest/vehicles/history?to=" + to)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetVehicleHistory_MissingTo(t *testing.T) {
	mock := &mockDB{}
	srv := newTestServer(mock)
	defer srv.Close()

	from := time.Now().Format(time.RFC3339)
	resp, err := http.Get(srv.URL + "/api/zones/brest/vehicles/history?from=" + from)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetVehicleHistory_InvalidFromDate(t *testing.T) {
	mock := &mockDB{}
	srv := newTestServer(mock)
	defer srv.Close()

	to := time.Now().Format(time.RFC3339)
	resp, err := http.Get(srv.URL + "/api/zones/brest/vehicles/history?from=invalid&to=" + to)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetVehicleHistory_InvalidToDate(t *testing.T) {
	mock := &mockDB{}
	srv := newTestServer(mock)
	defer srv.Close()

	from := time.Now().Format(time.RFC3339)
	resp, err := http.Get(srv.URL + "/api/zones/brest/vehicles/history?from=" + from + "&to=invalid")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetVehicleHistory_DBError(t *testing.T) {
	mock := &mockDB{historyErr: errors.New("db error")}
	srv := newTestServer(mock)
	defer srv.Close()

	from := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	resp, err := http.Get(srv.URL + "/api/zones/brest/vehicles/history?from=" + from + "&to=" + to)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}

func TestCORSHeaders(t *testing.T) {
	mock := &mockDB{zones: []db.ZoneWithCount{}}
	srv := newTestServer(mock)
	defer srv.Close()

	req, _ := http.NewRequest("OPTIONS", srv.URL+"/api/zones", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	acao := resp.Header.Get("Access-Control-Allow-Origin")
	if acao == "" {
		t.Error("missing Access-Control-Allow-Origin header")
	}
}

func TestContentTypeJSON(t *testing.T) {
	mock := &mockDB{zones: []db.ZoneWithCount{}}
	srv := newTestServer(mock)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/zones")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}
