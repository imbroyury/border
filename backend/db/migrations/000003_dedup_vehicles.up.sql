-- Remove consecutive-duplicate vehicle rows (same reg_number+zone_id, same status as prior snapshot).
WITH ranked AS (
    SELECT v.id, v.status,
        LAG(v.status) OVER (
            PARTITION BY v.zone_id, v.reg_number
            ORDER BY s.captured_at ASC
        ) AS prev_status
    FROM vehicles v
    JOIN snapshots s ON s.id = v.snapshot_id
)
DELETE FROM vehicles
WHERE id IN (
    SELECT id FROM ranked WHERE status IS NOT DISTINCT FROM prev_status
);
