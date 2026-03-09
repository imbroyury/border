package scraper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func loadTestData(t *testing.T, filename string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + filename)
	if err != nil {
		t.Fatalf("read testdata/%s: %v", filename, err)
	}
	return data
}

func TestFetchCheckpoints_Success(t *testing.T) {
	data := loadTestData(t, "checkpoints.json")

	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/checkpoint" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})

	client := NewClient(srv.URL, nil)
	entries, err := client.FetchCheckpoints(context.Background())
	if err != nil {
		t.Fatalf("FetchCheckpoints: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one checkpoint")
	}
	// Verify first entry has expected fields.
	found := false
	for _, e := range entries {
		if e.ID == "a9173a85-3fc0-424c-84f0-defa632481e4" {
			found = true
			if e.Name == "" {
				t.Error("expected non-empty name for Brest")
			}
		}
	}
	if !found {
		t.Error("expected to find Brest checkpoint")
	}
}

func TestFetchCheckpoints_ServerError(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	client := NewClient(srv.URL, nil)
	_, err := client.FetchCheckpoints(context.Background())
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestFetchCheckpoints_InvalidJSON(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	})

	client := NewClient(srv.URL, nil)
	_, err := client.FetchCheckpoints(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestFetchCheckpoints_ContextCancellation(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})

	client := NewClient(srv.URL, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := client.FetchCheckpoints(ctx)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestFetchMonitoring_Success(t *testing.T) {
	data := loadTestData(t, "monitoring.json")

	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/monitoring-new" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		cpID := r.URL.Query().Get("checkpointId")
		if cpID == "" {
			t.Error("missing checkpointId param")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})

	client := NewClient(srv.URL, nil)
	resp, err := client.FetchMonitoring(context.Background(), "98b5be92-d3a5-4ba2-9106-76eb4eb3df49")
	if err != nil {
		t.Fatalf("FetchMonitoring: %v", err)
	}
	if resp.Info.ID == "" {
		t.Error("expected non-empty info ID")
	}
	// Kozlovichi test data should have truck vehicles
	totalVehicles := len(resp.TruckLiveQueue) + len(resp.TruckPriority) +
		len(resp.CarLiveQueue) + len(resp.CarPriority)
	if totalVehicles == 0 {
		t.Error("expected at least one vehicle in monitoring data")
	}
}

func TestFetchMonitoring_ServerError(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	client := NewClient(srv.URL, nil)
	_, err := client.FetchMonitoring(context.Background(), "test-id")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestFetchStatistics_Success(t *testing.T) {
	data := loadTestData(t, "statistics.json")

	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/monitoring/statistics" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})

	client := NewClient(srv.URL, nil)
	resp, err := client.FetchStatistics(context.Background(), "a9173a85-3fc0-424c-84f0-defa632481e4")
	if err != nil {
		t.Fatalf("FetchStatistics: %v", err)
	}
	// Based on real test data, these should be populated.
	if resp.CarLastHour < 0 {
		t.Errorf("unexpected CarLastHour: %d", resp.CarLastHour)
	}
	if resp.CarLastDay < 0 {
		t.Errorf("unexpected CarLastDay: %d", resp.CarLastDay)
	}
}

func TestFetchStatistics_ServerError(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	client := NewClient(srv.URL, nil)
	_, err := client.FetchStatistics(context.Background(), "test-id")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestFetchZoneSummary_Success(t *testing.T) {
	data := loadTestData(t, "checkpoints.json")

	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})

	client := NewClient(srv.URL, nil)
	entries, err := client.FetchZoneSummary(context.Background())
	if err != nil {
		t.Fatalf("FetchZoneSummary: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one zone summary entry")
	}
	// Should only include known checkpoints.
	for _, e := range entries {
		if e.Slug == "" {
			t.Error("entry has empty slug")
		}
		if e.CheckpointID == "" {
			t.Error("entry has empty checkpoint ID")
		}
	}
}

func TestFetchZoneDetail_Success(t *testing.T) {
	monitoringData := loadTestData(t, "monitoring.json")
	statsData := loadTestData(t, "statistics.json")

	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/monitoring-new":
			w.Write(monitoringData)
		case "/monitoring/statistics":
			w.Write(statsData)
		default:
			http.NotFound(w, r)
		}
	})

	client := NewClient(srv.URL, nil)
	detail, err := client.FetchZoneDetail(context.Background(), "test-id")
	if err != nil {
		t.Fatalf("FetchZoneDetail: %v", err)
	}
	if detail.SentLastHour < 0 {
		t.Errorf("unexpected SentLastHour: %d", detail.SentLastHour)
	}
	if detail.SentLast24h < 0 {
		t.Errorf("unexpected SentLast24h: %d", detail.SentLast24h)
	}
}

func TestFetchZoneDetail_MonitoringFails(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	client := NewClient(srv.URL, nil)
	_, err := client.FetchZoneDetail(context.Background(), "test-id")
	if err == nil {
		t.Fatal("expected error when monitoring fails")
	}
}

func TestNewClient_Defaults(t *testing.T) {
	c := NewClient("", nil)
	if c.baseURL != defaultBaseURL {
		t.Errorf("expected default base URL, got %s", c.baseURL)
	}
	if c.httpClient == nil {
		t.Fatal("httpClient should not be nil")
	}
}

func TestNewClient_CustomValues(t *testing.T) {
	custom := &http.Client{Timeout: 5 * time.Second}
	c := NewClient("http://custom.url", custom)
	if c.baseURL != "http://custom.url" {
		t.Errorf("expected custom URL, got %s", c.baseURL)
	}
	if c.httpClient != custom {
		t.Fatal("expected custom HTTP client")
	}
}

func TestParseAPIDateTime(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Minsk")
	tests := []struct {
		input string
		want  time.Time
	}{
		{"16:20:53 09.03.2026", time.Date(2026, 3, 9, 16, 20, 53, 0, loc)},
		{"00:00:00 01.01.2025", time.Date(2025, 1, 1, 0, 0, 0, 0, loc)},
		{"", time.Time{}},
		{"invalid", time.Time{}},
		{"09.03.2026 16:20:53", time.Time{}}, // wrong format
	}
	for _, tt := range tests {
		got := parseAPIDateTime(tt.input)
		if !got.Equal(tt.want) {
			t.Errorf("parseAPIDateTime(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestVehicleStatusString(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{1, "registered"},
		{2, "in_queue"},
		{3, "called"},
		{4, "passed"},
		{99, "unknown_99"},
	}
	for _, tt := range tests {
		got := vehicleStatusString(tt.input)
		if got != tt.want {
			t.Errorf("vehicleStatusString(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSlugToCheckpointID_Mappings(t *testing.T) {
	// Verify bidirectional mapping consistency.
	for slug, id := range SlugToCheckpointID {
		reverseSlug, ok := CheckpointIDToSlug[id]
		if !ok {
			t.Errorf("CheckpointIDToSlug missing ID %s", id)
		}
		if reverseSlug != slug {
			t.Errorf("reverse mapping for %s: got %s, want %s", id, reverseSlug, slug)
		}
	}
	if len(SlugToCheckpointID) != len(CheckpointIDToSlug) {
		t.Error("slug maps have different sizes")
	}
}
