package storage

import (
	"context"
	"fmt"
	"time"

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

// ActiveCrossing represents an is_active=true crossing for a zone.
type ActiveCrossing struct {
	ID            int64
	RegNumber     string
	QueueType     string
	CurrentStatus string
	LastSeenAt    time.Time
}

// CrossingUpdate represents a vehicle observed in the current crawl.
type CrossingUpdate struct {
	RegNumber    string
	QueueType    string
	RegisteredAt time.Time // zero → NULL
	Status       string
	CapturedAt   time.Time
}

// isTerminalStatus returns true for statuses that indicate a crossing is complete.
func isTerminalStatus(status string) bool {
	return status == "passed" || status == "cancelled"
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

// GetActiveCrossings returns is_active=true crossings for a zone, keyed by reg_number.
func (s *Store) GetActiveCrossings(ctx context.Context, zoneID string) (map[string]ActiveCrossing, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, reg_number, queue_type, current_status, last_seen_at
		 FROM vehicle_crossings
		 WHERE zone_id = $1 AND is_active = true`,
		zoneID,
	)
	if err != nil {
		return nil, fmt.Errorf("query active crossings: %w", err)
	}
	defer rows.Close()

	result := make(map[string]ActiveCrossing)
	for rows.Next() {
		var ac ActiveCrossing
		if err := rows.Scan(&ac.ID, &ac.RegNumber, &ac.QueueType, &ac.CurrentStatus, &ac.LastSeenAt); err != nil {
			return nil, fmt.Errorf("scan active crossing: %w", err)
		}
		result[ac.RegNumber] = ac
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active crossings: %w", err)
	}
	return result, nil
}

// ApplyCrawlDiff reconciles current API vehicles against active crossings, in one transaction:
//   - New vehicle (not in active map) → INSERT crossing + first status_change
//   - Same vehicle, same status → UPDATE last_seen_at on crossing + latest status_change
//   - Same vehicle, new status → INSERT status_change, UPDATE crossing.current_status + last_seen_at
//   - Vehicle in active map but absent from current → SET is_active = false
func (s *Store) ApplyCrawlDiff(ctx context.Context, zoneID string, capturedAt time.Time,
	current []CrossingUpdate, active map[string]ActiveCrossing) error {

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	currentSet := make(map[string]struct{}, len(current))
	for _, u := range current {
		currentSet[u.RegNumber] = struct{}{}
	}

	var sameStatusIDs []int64
	var disappeared []int64

	for regNumber, ac := range active {
		if _, ok := currentSet[regNumber]; !ok {
			disappeared = append(disappeared, ac.ID)
		}
	}

	for _, u := range current {
		ac, ok := active[u.RegNumber]
		if !ok {
			// Vehicle not in active map. If it has a terminal status, check for a
			// recent inactive crossing with the same status to avoid duplicates
			// caused by the API intermittently returning terminal vehicles.
			if isTerminalStatus(u.Status) {
				var existingID int64
				err = tx.QueryRow(ctx,
					`SELECT id FROM vehicle_crossings
					 WHERE zone_id = $1 AND reg_number = $2 AND is_active = false AND current_status = $3
					 ORDER BY last_seen_at DESC LIMIT 1`,
					zoneID, u.RegNumber, u.Status,
				).Scan(&existingID)
				if err == nil {
					// Reuse existing terminal crossing — just update last_seen_at
					sameStatusIDs = append(sameStatusIDs, existingID)
					continue
				}
				// No matching inactive crossing found — fall through to create new one
			}

			// New vehicle — INSERT crossing + first status_change
			var registeredAt any
			if !u.RegisteredAt.IsZero() {
				registeredAt = u.RegisteredAt
			}
			var crossingID int64
			err = tx.QueryRow(ctx,
				`INSERT INTO vehicle_crossings
				 (zone_id, reg_number, queue_type, registered_at, first_seen_at, last_seen_at, current_status, is_active)
				 VALUES ($1, $2, $3, $4, $5, $5, $6, true)
				 RETURNING id`,
				zoneID, u.RegNumber, u.QueueType, registeredAt, u.CapturedAt, u.Status,
			).Scan(&crossingID)
			if err != nil {
				return fmt.Errorf("insert crossing for %s: %w", u.RegNumber, err)
			}
			_, err = tx.Exec(ctx,
				`INSERT INTO vehicle_status_changes (crossing_id, status, detected_at, last_seen_at)
				 VALUES ($1, $2, $3, $3)`,
				crossingID, u.Status, u.CapturedAt,
			)
			if err != nil {
				return fmt.Errorf("insert status change for %s: %w", u.RegNumber, err)
			}
		} else if ac.CurrentStatus == u.Status {
			// Same status — batch for last_seen_at update
			sameStatusIDs = append(sameStatusIDs, ac.ID)
		} else {
			// Status changed — INSERT new status_change, UPDATE crossing
			_, err = tx.Exec(ctx,
				`INSERT INTO vehicle_status_changes (crossing_id, status, detected_at, last_seen_at)
				 VALUES ($1, $2, $3, $3)`,
				ac.ID, u.Status, u.CapturedAt,
			)
			if err != nil {
				return fmt.Errorf("insert status change for %s: %w", u.RegNumber, err)
			}
			_, err = tx.Exec(ctx,
				`UPDATE vehicle_crossings SET current_status = $1, last_seen_at = $2 WHERE id = $3`,
				u.Status, u.CapturedAt, ac.ID,
			)
			if err != nil {
				return fmt.Errorf("update crossing for %s: %w", u.RegNumber, err)
			}
		}
	}

	if len(sameStatusIDs) > 0 {
		_, err = tx.Exec(ctx,
			`UPDATE vehicle_crossings SET last_seen_at = $1 WHERE id = ANY($2)`,
			capturedAt, sameStatusIDs,
		)
		if err != nil {
			return fmt.Errorf("batch update crossings last_seen_at: %w", err)
		}
		_, err = tx.Exec(ctx,
			`UPDATE vehicle_status_changes sc
			 SET last_seen_at = $1
			 FROM (
			     SELECT DISTINCT ON (crossing_id) id
			     FROM vehicle_status_changes
			     WHERE crossing_id = ANY($2)
			     ORDER BY crossing_id, detected_at DESC
			 ) latest
			 WHERE sc.id = latest.id`,
			capturedAt, sameStatusIDs,
		)
		if err != nil {
			return fmt.Errorf("batch update status changes last_seen_at: %w", err)
		}
	}

	if len(disappeared) > 0 {
		_, err = tx.Exec(ctx,
			`UPDATE vehicle_crossings SET is_active = false WHERE id = ANY($1)`,
			disappeared,
		)
		if err != nil {
			return fmt.Errorf("mark disappeared crossings inactive: %w", err)
		}
	}

	return tx.Commit(ctx)
}
