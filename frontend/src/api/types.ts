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

export interface VehicleStatusChange {
  captured_at: string
  status: string
  queue_type: string
  status_changed_at: string
}

export interface VehicleStatusChangeWithZone extends VehicleStatusChange {
  zone_id: string
}

export interface VehicleSearchResult {
  reg_number: string
  zone_id: string
  status: string
  last_seen: string
}

export interface DurationPreset {
  label: string
  value: string
  ms: number
}
