-- Merge duplicate crossings caused by terminal vehicles (cancelled/passed)
-- flickering in and out of the API. For each (zone_id, reg_number, current_status)
-- group where status is terminal, keep the earliest crossing and extend its
-- last_seen_at to cover all duplicates.

-- Identify duplicates: for each group of terminal crossings with the same
-- (zone_id, reg_number, current_status), the one with the earliest first_seen_at
-- is the "keeper". All others are duplicates to be merged into it.
WITH keepers AS (
    SELECT DISTINCT ON (zone_id, reg_number, current_status)
           id AS keep_id,
           zone_id,
           reg_number,
           current_status
      FROM vehicle_crossings
     WHERE current_status IN ('passed', 'cancelled')
     ORDER BY zone_id, reg_number, current_status, first_seen_at ASC
),
dupes AS (
    SELECT vc.id AS dupe_id, k.keep_id
      FROM vehicle_crossings vc
      JOIN keepers k
        ON vc.zone_id = k.zone_id
       AND vc.reg_number = k.reg_number
       AND vc.current_status = k.current_status
     WHERE vc.id != k.keep_id
       AND vc.current_status IN ('passed', 'cancelled')
),
-- Extend keeper's last_seen_at to the max across all duplicates
updated_crossings AS (
    UPDATE vehicle_crossings vc
       SET last_seen_at = sub.max_last_seen
      FROM (
          SELECT k.keep_id,
                 MAX(all_vc.last_seen_at) AS max_last_seen
            FROM keepers k
            JOIN vehicle_crossings all_vc
              ON all_vc.zone_id = k.zone_id
             AND all_vc.reg_number = k.reg_number
             AND all_vc.current_status = k.current_status
           GROUP BY k.keep_id
      ) sub
     WHERE vc.id = sub.keep_id
       AND vc.last_seen_at < sub.max_last_seen
),
-- Also extend the keeper's latest status_change last_seen_at
updated_status_changes AS (
    UPDATE vehicle_status_changes sc
       SET last_seen_at = sub.max_last_seen
      FROM (
          SELECT DISTINCT ON (k.keep_id)
                 sc2.id AS sc_id,
                 MAX(all_vc.last_seen_at) OVER (PARTITION BY k.keep_id) AS max_last_seen
            FROM keepers k
            JOIN vehicle_status_changes sc2 ON sc2.crossing_id = k.keep_id
            JOIN vehicle_crossings all_vc
              ON all_vc.zone_id = k.zone_id
             AND all_vc.reg_number = k.reg_number
             AND all_vc.current_status = k.current_status
           ORDER BY k.keep_id, sc2.detected_at DESC
      ) sub
     WHERE sc.id = sub.sc_id
       AND sc.last_seen_at < sub.max_last_seen
),
-- Delete status_changes belonging to duplicate crossings
deleted_sc AS (
    DELETE FROM vehicle_status_changes
     WHERE crossing_id IN (SELECT dupe_id FROM dupes)
)
-- Delete the duplicate crossings themselves
DELETE FROM vehicle_crossings
 WHERE id IN (SELECT dupe_id FROM dupes);
