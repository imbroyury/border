import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { fetchZones, fetchSnapshots, fetchVehicles, fetchVehicleHistory } from '../client'

const mockFetch = vi.fn()

beforeEach(() => {
  mockFetch.mockClear()
  vi.stubGlobal('fetch', mockFetch)
})

afterEach(() => {
  vi.restoreAllMocks()
})

function jsonResponse(data: unknown, status = 200) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 200 ? 'OK' : 'Error',
    json: () => Promise.resolve(data),
  })
}

function calledUrlString(): string {
  const arg = mockFetch.mock.calls[0][0]
  return arg instanceof URL ? arg.toString() : String(arg)
}

describe('fetchZones', () => {
  it('returns zones on success', async () => {
    const zones = [{ id: 'brest', name: 'Брест', border: 'BY-PL', cars_count: 42, last_captured: '2026-01-01T00:00:00Z' }]
    mockFetch.mockReturnValue(jsonResponse(zones))

    const result = await fetchZones()
    expect(result).toEqual(zones)
    expect(calledUrlString()).toContain('/api/zones')
  })

  it('throws on HTTP error', async () => {
    mockFetch.mockReturnValue(jsonResponse(null, 500))
    await expect(fetchZones()).rejects.toThrow('HTTP 500')
  })
})

describe('fetchSnapshots', () => {
  it('passes from/to as query params', async () => {
    mockFetch.mockReturnValue(jsonResponse([]))

    const from = new Date('2026-01-01T00:00:00Z')
    const to = new Date('2026-01-02T00:00:00Z')
    await fetchSnapshots('brest', from, to)

    const url = calledUrlString()
    expect(url).toContain('/api/zones/brest/snapshots')
    expect(url).toContain('from=' + encodeURIComponent(from.toISOString()))
    expect(url).toContain('to=' + encodeURIComponent(to.toISOString()))
  })

  it('throws on HTTP error', async () => {
    mockFetch.mockReturnValue(jsonResponse(null, 404))
    await expect(fetchSnapshots('x', new Date(), new Date())).rejects.toThrow('HTTP 404')
  })
})

describe('fetchVehicles', () => {
  it('calls correct endpoint', async () => {
    const vehicles = [{ reg_number: 'AB1234', queue_type: 'live', status: 'in_queue', registered_at: '', status_changed_at: '' }]
    mockFetch.mockReturnValue(jsonResponse(vehicles))

    const result = await fetchVehicles('bruzgi')
    expect(result).toEqual(vehicles)
    expect(calledUrlString()).toContain('/api/zones/bruzgi/vehicles')
  })
})

describe('fetchVehicleHistory', () => {
  it('passes from/to as query params', async () => {
    mockFetch.mockReturnValue(jsonResponse([]))

    const from = new Date('2026-03-01T00:00:00Z')
    const to = new Date('2026-03-09T00:00:00Z')
    await fetchVehicleHistory('kamenny-log', from, to)

    const url = calledUrlString()
    expect(url).toContain('/api/zones/kamenny-log/vehicles/history')
    expect(url).toContain('from=' + encodeURIComponent(from.toISOString()))
    expect(url).toContain('to=' + encodeURIComponent(to.toISOString()))
  })
})
