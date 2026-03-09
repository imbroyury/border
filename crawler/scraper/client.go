package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"
)

// SlugToCheckpointID maps known zone slugs to their API UUIDs.
var SlugToCheckpointID = map[string]string{
	"benyakoni":       "53d94097-2b34-11ec-8467-ac1f6bf889c0",
	"berestovitsa":    "7e46a2d1-ab2f-11ec-bafb-ac1f6bf889c1",
	"brest":           "a9173a85-3fc0-424c-84f0-defa632481e4",
	"grigorovshchina": "ffe81c11-00d6-11e8-a967-b0dd44bde851",
	"kamenny-log":     "b60677d4-8a00-4f93-a781-e129e1692a03",
	"kozlovichi":      "98b5be92-d3a5-4ba2-9106-76eb4eb3df49",
	"bruzgi":          "3b797d4d-706a-440f-a1a4-826c191e1e36",
}

// CheckpointIDToSlug is the reverse mapping.
var CheckpointIDToSlug map[string]string

func init() {
	CheckpointIDToSlug = make(map[string]string, len(SlugToCheckpointID))
	for slug, id := range SlugToCheckpointID {
		CheckpointIDToSlug[id] = slug
	}
}

const (
	defaultBaseURL  = "https://belarusborder.by/info"
	tokenSourceURL  = "https://mon.declarant.by"
)

var (
	mainBundleRe      = regexp.MustCompile(`src="(main\.[a-f0-9]+\.js)"`)
	checkpointTokenRe = regexp.MustCompile(`token="([^"]+)"`)
	monitoringTokenRe = regexp.MustCompile(`tokenTest="([^"]+)"`)
)

// Tokens holds the API tokens scraped from the mon.declarant.by frontend.
type Tokens struct {
	Checkpoint string
	Monitoring string
}

// Client fetches border queue data from the belarusborder.by API.
type Client struct {
	baseURL    string
	tokens     Tokens
	httpClient *http.Client
}

// NewClient creates a new API client with pre-fetched tokens.
func NewClient(baseURL string, tokens Tokens, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{baseURL: baseURL, tokens: tokens, httpClient: httpClient}
}

// FetchTokens scrapes API tokens from the mon.declarant.by frontend bundle.
func FetchTokens(ctx context.Context, httpClient *http.Client) (Tokens, error) {
	return fetchTokensFrom(ctx, httpClient, tokenSourceURL)
}

func fetchTokensFrom(ctx context.Context, httpClient *http.Client, sourceURL string) (Tokens, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	// Step 1: Fetch the index page to find the main bundle filename.
	indexBody, err := doSimpleGet(ctx, httpClient, sourceURL)
	if err != nil {
		return Tokens{}, fmt.Errorf("fetch token source page: %w", err)
	}

	match := mainBundleRe.FindSubmatch(indexBody)
	if match == nil {
		return Tokens{}, fmt.Errorf("main bundle not found in %s", sourceURL)
	}
	bundleFile := string(match[1])

	// Step 2: Fetch the main bundle and extract tokens.
	bundleBody, err := doSimpleGet(ctx, httpClient, sourceURL+"/"+bundleFile)
	if err != nil {
		return Tokens{}, fmt.Errorf("fetch main bundle: %w", err)
	}

	cpMatch := checkpointTokenRe.FindSubmatch(bundleBody)
	if cpMatch == nil {
		return Tokens{}, fmt.Errorf("checkpoint token not found in bundle")
	}

	monMatch := monitoringTokenRe.FindSubmatch(bundleBody)
	if monMatch == nil {
		return Tokens{}, fmt.Errorf("monitoring token not found in bundle")
	}

	return Tokens{
		Checkpoint: string(cpMatch[1]),
		Monitoring: string(monMatch[1]),
	}, nil
}

func doSimpleGet(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

// FetchCheckpoints fetches all checkpoint summaries.
func (c *Client) FetchCheckpoints(ctx context.Context) ([]CheckpointEntry, error) {
	url := fmt.Sprintf("%s/checkpoint?token=%s", c.baseURL, c.tokens.Checkpoint)

	body, err := c.doGet(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetch checkpoints: %w", err)
	}
	defer body.Close()

	var resp CheckpointResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode checkpoints: %w", err)
	}
	return resp.Result, nil
}

// FetchMonitoring fetches detailed vehicle queues for a checkpoint.
func (c *Client) FetchMonitoring(ctx context.Context, checkpointID string) (*MonitoringResponse, error) {
	url := fmt.Sprintf("%s/monitoring-new?token=%s&checkpointId=%s", c.baseURL, c.tokens.Monitoring, checkpointID)

	body, err := c.doGet(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetch monitoring %s: %w", checkpointID, err)
	}
	defer body.Close()

	var resp MonitoringResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode monitoring %s: %w", checkpointID, err)
	}
	return &resp, nil
}

// FetchStatistics fetches sent-last-hour/day stats for a checkpoint.
func (c *Client) FetchStatistics(ctx context.Context, checkpointID string) (*StatisticsResponse, error) {
	url := fmt.Sprintf("%s/monitoring/statistics?token=%s&checkpointId=%s", c.baseURL, c.tokens.Monitoring, checkpointID)

	body, err := c.doGet(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetch statistics %s: %w", checkpointID, err)
	}
	defer body.Close()

	var resp StatisticsResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode statistics %s: %w", checkpointID, err)
	}
	return &resp, nil
}

// FetchZoneSummary returns processed zone summaries for all known checkpoints.
func (c *Client) FetchZoneSummary(ctx context.Context) ([]ZoneSummaryEntry, error) {
	checkpoints, err := c.FetchCheckpoints(ctx)
	if err != nil {
		return nil, err
	}

	var entries []ZoneSummaryEntry
	for _, cp := range checkpoints {
		slug, ok := CheckpointIDToSlug[cp.ID]
		if !ok {
			continue // skip unknown checkpoints
		}
		entries = append(entries, ZoneSummaryEntry{
			CheckpointID: cp.ID,
			Slug:         slug,
			Name:         cp.Name,
			CarsCount:    cp.CountCar,
		})
	}
	return entries, nil
}

// FetchZoneDetail returns processed detail for a single checkpoint.
func (c *Client) FetchZoneDetail(ctx context.Context, checkpointID string) (*ZoneDetail, error) {
	monitoring, err := c.FetchMonitoring(ctx, checkpointID)
	if err != nil {
		return nil, err
	}

	stats, err := c.FetchStatistics(ctx, checkpointID)
	if err != nil {
		return nil, err
	}

	// Collect only car vehicles (live queue + priority).
	var vehicles []VehicleEntry
	for _, v := range monitoring.CarLiveQueue {
		vehicles = append(vehicles, convertVehicle(v, "live"))
	}
	for _, v := range monitoring.CarPriority {
		vehicles = append(vehicles, convertVehicle(v, "priority"))
	}

	return &ZoneDetail{
		SentLastHour: stats.CarLastHour,
		SentLast24h:  stats.CarLastDay,
		Vehicles:     vehicles,
	}, nil
}

func (c *Client) doGet(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// convertVehicle converts an API vehicle entry to our internal type.
func convertVehicle(v VehicleQueueEntry, queueType string) VehicleEntry {
	return VehicleEntry{
		RegNumber:       v.RegNum,
		QueueType:       queueType,
		RegisteredAt:    parseAPIDateTime(v.RegistrationDate),
		StatusChangedAt: parseAPIDateTime(v.ChangedDate),
		Status:          vehicleStatusString(v.Status),
	}
}

// parseAPIDateTime parses "HH:MM:SS DD.MM.YYYY" format from the API.
func parseAPIDateTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	loc, err := time.LoadLocation("Europe/Minsk")
	if err != nil {
		loc = time.UTC
	}
	t, err := time.ParseInLocation("15:04:05 02.01.2006", s, loc)
	if err != nil {
		return time.Time{}
	}
	return t
}

// vehicleStatusString converts numeric status to human-readable string.
func vehicleStatusString(status int) string {
	switch status {
	case 1:
		return "registered"
	case 2:
		return "in_queue"
	case 3:
		return "called"
	case 4:
		return "passed"
	case 9:
		return "cancelled"
	default:
		return fmt.Sprintf("unknown_%d", status)
	}
}
