-- Backfill missing status_change rows for crossings that have none.
-- Creates a single status_change using the crossing's current_status,
-- first_seen_at as detected_at, and last_seen_at.

INSERT INTO vehicle_status_changes (crossing_id, status, detected_at, last_seen_at)
SELECT vc.id, vc.current_status, vc.first_seen_at, vc.last_seen_at
  FROM vehicle_crossings vc
 WHERE NOT EXISTS (
     SELECT 1 FROM vehicle_status_changes sc WHERE sc.crossing_id = vc.id
 );
