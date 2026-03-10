-- Redesign: replace flat vehicles table with vehicle_crossings + vehicle_status_changes.

CREATE TABLE vehicle_crossings (
    id             BIGSERIAL PRIMARY KEY,
    zone_id        VARCHAR(50) NOT NULL REFERENCES zones(id),
    reg_number     VARCHAR(20) NOT NULL,
    queue_type     VARCHAR(50) NOT NULL DEFAULT '',
    registered_at  TIMESTAMPTZ,
    first_seen_at  TIMESTAMPTZ NOT NULL,
    last_seen_at   TIMESTAMPTZ NOT NULL,
    current_status VARCHAR(50) NOT NULL DEFAULT '',
    is_active      BOOLEAN     NOT NULL DEFAULT TRUE
);
CREATE INDEX idx_vehicle_crossings_zone_active    ON vehicle_crossings(zone_id, is_active);
CREATE INDEX idx_vehicle_crossings_zone_last_seen ON vehicle_crossings(zone_id, last_seen_at DESC);
CREATE INDEX idx_vehicle_crossings_reg_number     ON vehicle_crossings(reg_number);

CREATE TABLE vehicle_status_changes (
    id           BIGSERIAL PRIMARY KEY,
    crossing_id  BIGINT      NOT NULL REFERENCES vehicle_crossings(id) ON DELETE CASCADE,
    status       VARCHAR(50) NOT NULL DEFAULT '',
    detected_at  TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_vehicle_status_changes_crossing ON vehicle_status_changes(crossing_id, detected_at ASC);

WITH
raw AS (
    SELECT v.zone_id, v.reg_number, v.queue_type, v.registered_at, v.status, s.captured_at
    FROM vehicles v JOIN snapshots s ON s.id = v.snapshot_id
),
boundary_flags AS (
    SELECT *,
        CASE
            WHEN LAG(status) OVER w IN ('passed', 'cancelled') THEN 1
            WHEN LAG(captured_at) OVER w IS NULL THEN 0
            WHEN captured_at - LAG(captured_at) OVER w > INTERVAL '2 hours' THEN 1
            ELSE 0
        END AS is_new_crossing
    FROM raw
    WINDOW w AS (PARTITION BY zone_id, reg_number ORDER BY captured_at ASC)
),
crossing_groups AS (
    SELECT *, SUM(is_new_crossing) OVER (
        PARTITION BY zone_id, reg_number ORDER BY captured_at ASC ROWS UNBOUNDED PRECEDING
    ) AS crossing_seq
    FROM boundary_flags
),
inserted_crossings AS (
    INSERT INTO vehicle_crossings (zone_id, reg_number, queue_type, registered_at,
        first_seen_at, last_seen_at, current_status, is_active)
    SELECT zone_id, reg_number,
        (ARRAY_AGG(queue_type ORDER BY captured_at DESC))[1],
        (ARRAY_AGG(registered_at ORDER BY captured_at ASC) FILTER (WHERE registered_at IS NOT NULL))[1],
        MIN(captured_at), MAX(captured_at),
        (ARRAY_AGG(status ORDER BY captured_at DESC))[1],
        CASE WHEN (ARRAY_AGG(status ORDER BY captured_at DESC))[1]
             NOT IN ('passed', 'cancelled') THEN TRUE ELSE FALSE END
    FROM crossing_groups
    GROUP BY zone_id, reg_number, crossing_seq
    RETURNING id, zone_id, reg_number, first_seen_at
),
rows_with_crossing_id AS (
    SELECT cg.*, ic.id AS crossing_id
    FROM crossing_groups cg
    JOIN inserted_crossings ic
      ON ic.zone_id = cg.zone_id AND ic.reg_number = cg.reg_number
     AND ic.first_seen_at = (
         SELECT MIN(captured_at) FROM crossing_groups cg2
         WHERE cg2.zone_id = cg.zone_id AND cg2.reg_number = cg.reg_number
           AND cg2.crossing_seq = cg.crossing_seq
     )
),
status_boundary_flags AS (
    SELECT *, CASE
        WHEN LAG(status) OVER (PARTITION BY crossing_id ORDER BY captured_at ASC)
             IS DISTINCT FROM status THEN 1 ELSE 0
    END AS is_new_status
    FROM rows_with_crossing_id
),
status_groups AS (
    SELECT *, SUM(is_new_status) OVER (
        PARTITION BY crossing_id ORDER BY captured_at ASC ROWS UNBOUNDED PRECEDING
    ) AS status_seq
    FROM status_boundary_flags
)
INSERT INTO vehicle_status_changes (crossing_id, status, detected_at, last_seen_at)
SELECT crossing_id,
    (ARRAY_AGG(status ORDER BY captured_at ASC))[1],
    MIN(captured_at),
    MAX(captured_at)
FROM status_groups
GROUP BY crossing_id, status_seq
ORDER BY crossing_id, MIN(captured_at);

DROP TABLE vehicles;
