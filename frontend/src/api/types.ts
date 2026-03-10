export interface Zone {
  id: string
  name: string
  border: string
  cars_count: number
  last_captured: string
}

export interface SnapshotPoint {
  captured_at: string
  cars_count: number
  sent_last_hour: number
  sent_last_24h: number
}

export interface Vehicle {
  reg_number: string
  queue_type: string
  status: string
  registered_at: string
  status_changed_at: string
}

export interface StatusChange {
  status: string
  detected_at: string
  last_seen_at: string
}

export interface CrossingHistory {
  crossing_id: number
  zone_id: string
  queue_type: string
  registered_at: string
  first_seen_at: string
  last_seen_at: string
  current_status: string
  is_active: boolean
  status_changes: StatusChange[]
}

export interface VehicleSearchResult {
  reg_number: string
  zone_id: string
  status: string
  last_seen: string
}

export interface VehicleListResponse {
  data: VehicleSearchResult[]
  total: number
}

export interface DurationPreset {
  label: string
  value: string
  ms: number
}
