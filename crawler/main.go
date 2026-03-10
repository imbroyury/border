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
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	databaseURL := getEnv("DATABASE_URL", "postgres://border:border@localhost:5432/border?sslmode=disable")
	apiBaseURL := getEnv("API_BASE_URL", "")
	crawlIntervalStr := getEnv("CRAWL_INTERVAL", "15m")

	crawlInterval, err := time.ParseDuration(crawlIntervalStr)
	if err != nil {
		slog.Error("invalid CRAWL_INTERVAL", "value", crawlIntervalStr, "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		slog.Info("received shutdown signal", "signal", sig)
		cancel()
	}()

	if err := runMigrations(databaseURL); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	store, err := storage.New(ctx, databaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	slog.Info("connected to database")

	tokens, err := scraper.FetchTokens(ctx, nil)
	if err != nil {
		slog.Error("failed to fetch API tokens", "error", err)
		os.Exit(1)
	}
	slog.Info("fetched API tokens")

	client := scraper.NewClient(apiBaseURL, tokens, nil)

	// Run immediately on startup, then on the interval.
	crawl(ctx, client, store)

	ticker := time.NewTicker(crawlInterval)
	defer ticker.Stop()

	slog.Info("crawler started", "interval", crawlInterval)

	for {
		select {
		case <-ctx.Done():
			slog.Info("crawler stopped")
			return
		case <-ticker.C:
			crawl(ctx, client, store)
		}
	}
}

func crawl(ctx context.Context, client *scraper.Client, store *storage.Store) {
	slog.Info("starting crawl cycle")
	capturedAt := time.Now().UTC()

	entries, err := client.FetchZoneSummary(ctx)
	if err != nil {
		slog.Error("failed to fetch zone summary", "error", err)
		return
	}
	slog.Info("fetched zone summary", "zones", len(entries))

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			slog.Info("crawl interrupted", "error", err)
			return
		}

		if err := crawlZone(ctx, client, store, entry, capturedAt); err != nil {
			slog.Error("failed to crawl zone",
				"zone", entry.Slug,
				"checkpoint_id", entry.CheckpointID,
				"error", err,
			)
			continue
		}
	}

	slog.Info("crawl cycle completed")
}

func crawlZone(ctx context.Context, client *scraper.Client, store *storage.Store, entry scraper.ZoneSummaryEntry, capturedAt time.Time) error {
	detail, err := client.FetchZoneDetail(ctx, entry.CheckpointID)
	if err != nil {
		return err
	}

	snap := &storage.Snapshot{
		ZoneID:       entry.Slug,
		CapturedAt:   capturedAt,
		CarsCount:    entry.CarsCount,
		SentLastHour: detail.SentLastHour,
		SentLast24h:  detail.SentLast24h,
	}

	if _, err := store.InsertSnapshot(ctx, snap); err != nil {
		return err
	}

	active, err := store.GetActiveCrossings(ctx, entry.Slug)
	if err != nil {
		return err
	}

	updates := make([]storage.CrossingUpdate, 0, len(detail.Vehicles))
	for _, v := range detail.Vehicles {
		updates = append(updates, storage.CrossingUpdate{
			RegNumber:    v.RegNumber,
			QueueType:    v.QueueType,
			RegisteredAt: v.RegisteredAt,
			Status:       v.Status,
			CapturedAt:   capturedAt,
		})
	}

	vehiclesChanged := 0
	for _, u := range updates {
		ac, ok := active[u.RegNumber]
		if !ok || ac.CurrentStatus != u.Status {
			vehiclesChanged++
		}
	}

	if err := store.ApplyCrawlDiff(ctx, entry.Slug, capturedAt, updates, active); err != nil {
		return err
	}

	slog.Info("stored zone data",
		"zone", entry.Slug,
		"cars_count", entry.CarsCount,
		"vehicles_seen", len(updates),
		"vehicles_changed", vehiclesChanged,
		"sent_last_hour", detail.SentLastHour,
		"sent_last_24h", detail.SentLast24h,
	)
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
