package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Snapshot represents a point-in-time observation of a zone's queue.
type Snapshot struct {
	ZoneID       string
	CapturedAt   time.Time
	CarsCount    int
	SentLastHour int
	SentLast24h  int
}

// Vehicle represents a single vehicle entry associated with a snapshot.
type Vehicle struct {
	ZoneID          string
	RegNumber       string
	QueueType       string
	RegisteredAt    time.Time
	StatusChangedAt time.Time
	Status          string
}

// Store provides database operations for the crawler.
type Store struct {
	pool *pgxpool.Pool
}

// New creates a new Store connected to the database at dsn.
func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &Store{pool: pool}, nil
}

// Close closes the database connection pool.
func (s *Store) Close() {
	s.pool.Close()
}

// InsertSnapshot inserts a snapshot row and returns its ID.
func (s *Store) InsertSnapshot(ctx context.Context, snap *Snapshot) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO snapshots (zone_id, captured_at, cars_count, sent_last_hour, sent_last_24h)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		snap.ZoneID, snap.CapturedAt, snap.CarsCount, snap.SentLastHour, snap.SentLast24h,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert snapshot: %w", err)
	}
	return id, nil
}

// InsertVehicles inserts vehicle rows associated with a snapshot.
func (s *Store) InsertVehicles(ctx context.Context, snapshotID int64, zoneID string, vehicles []Vehicle) error {
	if len(vehicles) == 0 {
		return nil
	}

	_, err := s.pool.CopyFrom(ctx,
		pgx.Identifier{"vehicles"},
		[]string{"snapshot_id", "zone_id", "reg_number", "queue_type", "registered_at", "status_changed_at", "status"},
		pgx.CopyFromSlice(len(vehicles), func(i int) ([]any, error) {
			v := vehicles[i]
			var registeredAt, statusChangedAt any
			if v.RegisteredAt.IsZero() {
				registeredAt = nil
			} else {
				registeredAt = v.RegisteredAt
			}
			if v.StatusChangedAt.IsZero() {
				statusChangedAt = nil
			} else {
				statusChangedAt = v.StatusChangedAt
			}
			return []any{
				snapshotID,
				zoneID,
				v.RegNumber,
				v.QueueType,
				registeredAt,
				statusChangedAt,
				v.Status,
			}, nil
		}),
	)
	if err != nil {
		return fmt.Errorf("insert vehicles: %w", err)
	}
	return nil
}

// InsertCrawlResult inserts a snapshot and its vehicles in a single transaction.
func (s *Store) InsertCrawlResult(ctx context.Context, snap *Snapshot, vehicles []Vehicle) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var snapshotID int64
	err = tx.QueryRow(ctx,
		`INSERT INTO snapshots (zone_id, captured_at, cars_count, sent_last_hour, sent_last_24h)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		snap.ZoneID, snap.CapturedAt, snap.CarsCount, snap.SentLastHour, snap.SentLast24h,
	).Scan(&snapshotID)
	if err != nil {
		return fmt.Errorf("insert snapshot in tx: %w", err)
	}

	if len(vehicles) > 0 {
		_, err = tx.CopyFrom(ctx,
			pgx.Identifier{"vehicles"},
			[]string{"snapshot_id", "zone_id", "reg_number", "queue_type", "registered_at", "status_changed_at", "status"},
			pgx.CopyFromSlice(len(vehicles), func(i int) ([]any, error) {
				v := vehicles[i]
				var registeredAt, statusChangedAt any
				if v.RegisteredAt.IsZero() {
					registeredAt = nil
				} else {
					registeredAt = v.RegisteredAt
				}
				if v.StatusChangedAt.IsZero() {
					statusChangedAt = nil
				} else {
					statusChangedAt = v.StatusChangedAt
				}
				return []any{
					snapshotID,
					snap.ZoneID,
					v.RegNumber,
					v.QueueType,
					registeredAt,
					statusChangedAt,
					v.Status,
				}, nil
			}),
		)
		if err != nil {
			return fmt.Errorf("insert vehicles in tx: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
