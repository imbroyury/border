-- Remove phantom crossings: terminal crossings (cancelled/passed) that have
-- only a single status_change matching the terminal status, indicating the
-- vehicle was never tracked through a real status progression — it appeared
-- already-terminal from the API.

DELETE FROM vehicle_status_changes
 WHERE crossing_id IN (
     SELECT vc.id
       FROM vehicle_crossings vc
      WHERE vc.current_status IN ('passed', 'cancelled')
        -- Only one status change, and it matches the terminal status
        AND (SELECT COUNT(*) FROM vehicle_status_changes sc WHERE sc.crossing_id = vc.id) = 1
        AND EXISTS (
            SELECT 1 FROM vehicle_status_changes sc
             WHERE sc.crossing_id = vc.id AND sc.status = vc.current_status
        )
        -- No prior non-terminal crossing exists for this vehicle+zone
        -- (i.e., this terminal crossing wasn't preceded by a real crossing
        -- that transitioned to terminal)
        AND vc.first_seen_at = (
            SELECT MIN(vc2.first_seen_at)
              FROM vehicle_crossings vc2
             WHERE vc2.zone_id = vc.zone_id AND vc2.reg_number = vc.reg_number
        )
 );

DELETE FROM vehicle_crossings vc
 WHERE vc.current_status IN ('passed', 'cancelled')
   AND NOT EXISTS (
       SELECT 1 FROM vehicle_status_changes sc WHERE sc.crossing_id = vc.id
   );
