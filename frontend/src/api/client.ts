import type { Zone, SnapshotPoint, Vehicle, VehicleStatusChange, VehicleSearchResult, VehicleStatusChangeWithZone } from './types'

async function get<T>(path: string, params?: Record<string, string>): Promise<T> {
  const url = new URL(path, window.location.origin)
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      url.searchParams.set(k, v)
    }
  }
  const resp = await fetch(url, { signal: AbortSignal.timeout(15000) })
  if (!resp.ok) {
    throw new Error(`HTTP ${resp.status}: ${resp.statusText}`)
  }
  return resp.json()
}

export function fetchZones(): Promise<Zone[]> {
  return get('/api/zones')
}

export function fetchSnapshots(zoneId: string, from: Date, to: Date): Promise<SnapshotPoint[]> {
  return get(`/api/zones/${zoneId}/snapshots`, {
    from: from.toISOString(),
    to: to.toISOString(),
  })
}

export function fetchVehicles(zoneId: string): Promise<Vehicle[]> {
  return get(`/api/zones/${zoneId}/vehicles`)
}

export function fetchVehicleHistory(zoneId: string, from: Date, to: Date): Promise<Vehicle[]> {
  return get(`/api/zones/${zoneId}/vehicles/history`, {
    from: from.toISOString(),
    to: to.toISOString(),
  })
}

export function fetchSingleVehicleHistory(zoneId: string, regNumber: string): Promise<VehicleStatusChange[]> {
  return get(`/api/zones/${zoneId}/vehicles/${encodeURIComponent(regNumber)}/history`)
}

export function searchVehicles(query: string): Promise<VehicleSearchResult[]> {
  return get('/api/vehicles/search', { q: query })
}

export function fetchRecentVehicles(): Promise<VehicleSearchResult[]> {
  return get('/api/vehicles/recent')
}

export function fetchGlobalVehicleHistory(regNumber: string): Promise<VehicleStatusChangeWithZone[]> {
  return get(`/api/vehicles/${encodeURIComponent(regNumber)}/history`)
}
