import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import DashboardView from '../DashboardView.vue'

const mockZones = [
  { id: 'brest-bts', name: 'Брест', border: 'BY-PL', cars_count: 42, last_captured: '2026-03-09T12:00:00Z' },
  { id: 'bruzgi', name: 'Брузги', border: 'BY-PL', cars_count: 10, last_captured: '2026-03-09T12:00:00Z' },
]

vi.mock('../../api/client', () => ({
  fetchZones: vi.fn(),
}))

import { fetchZones } from '../../api/client'
const mockFetchZones = vi.mocked(fetchZones)

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: DashboardView },
      { path: '/zone/:id', component: { template: '<div />' } },
    ],
  })
}

beforeEach(() => {
  vi.useFakeTimers()
  mockFetchZones.mockReset()
})

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('DashboardView', () => {
  it('renders zone cards after loading', async () => {
    mockFetchZones.mockResolvedValue(mockZones)
    const router = makeRouter()
    await router.push('/')
    await router.isReady()

    const wrapper = mount(DashboardView, { global: { plugins: [router] } })
    await flushPromises()

    expect(wrapper.text()).toContain('Брест')
    expect(wrapper.text()).toContain('Брузги')
    expect(wrapper.text()).toContain('42')
  })

  it('shows loading state initially', async () => {
    mockFetchZones.mockReturnValue(new Promise(() => {})) // never resolves
    const router = makeRouter()
    await router.push('/')
    await router.isReady()

    const wrapper = mount(DashboardView, { global: { plugins: [router] } })
    expect(wrapper.text()).toContain('Loading')
  })

  it('shows error on fetch failure', async () => {
    mockFetchZones.mockRejectedValue(new Error('Network error'))
    const router = makeRouter()
    await router.push('/')
    await router.isReady()

    const wrapper = mount(DashboardView, { global: { plugins: [router] } })
    await flushPromises()

    expect(wrapper.text()).toContain('Network error')
  })

  it('polls every 60 seconds', async () => {
    mockFetchZones.mockResolvedValue(mockZones)
    const router = makeRouter()
    await router.push('/')
    await router.isReady()

    const wrapper = mount(DashboardView, { global: { plugins: [router] } })
    await flushPromises()

    const initialCalls = mockFetchZones.mock.calls.length

    await vi.advanceTimersByTimeAsync(60_000)
    expect(mockFetchZones).toHaveBeenCalledTimes(initialCalls + 1)

    await vi.advanceTimersByTimeAsync(60_000)
    expect(mockFetchZones).toHaveBeenCalledTimes(initialCalls + 2)

    wrapper.unmount()
  })
})
