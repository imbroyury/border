import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import VehicleTable from '../VehicleTable.vue'
import type { Vehicle } from '../../api/types'

const vehicles: Vehicle[] = [
  {
    reg_number: 'AB1234',
    queue_type: 'live',
    status: 'in_queue',
    registered_at: '2026-03-09T10:00:00Z',
    status_changed_at: '2026-03-09T11:00:00Z',
  },
  {
    reg_number: 'CD5678',
    queue_type: 'priority',
    status: 'called',
    registered_at: '2026-03-09T08:00:00Z',
    status_changed_at: '2026-03-09T09:00:00Z',
  },
]

describe('VehicleTable', () => {
  it('renders all vehicles', () => {
    const wrapper = mount(VehicleTable, { props: { vehicles } })
    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(2)
  })

  it('displays reg numbers', () => {
    const wrapper = mount(VehicleTable, { props: { vehicles } })
    expect(wrapper.text()).toContain('AB1234')
    expect(wrapper.text()).toContain('CD5678')
  })

  it('displays status badges', () => {
    const wrapper = mount(VehicleTable, { props: { vehicles } })
    expect(wrapper.text()).toContain('in_queue')
    expect(wrapper.text()).toContain('called')
  })

  it('sorts by column on click', async () => {
    const wrapper = mount(VehicleTable, { props: { vehicles } })

    // Default sort is registered_at ascending, so CD5678 (08:00) comes first
    let rows = wrapper.findAll('tbody tr')
    expect(rows[0]!.text()).toContain('CD5678')

    // Click reg_number header to sort by it
    const regHeader = wrapper.findAll('th').find((th) => th.text().includes('Reg Number'))
    await regHeader!.trigger('click')

    rows = wrapper.findAll('tbody tr')
    expect(rows[0]!.text()).toContain('AB1234')
  })

  it('toggles sort direction on second click', async () => {
    const wrapper = mount(VehicleTable, { props: { vehicles } })

    const regHeader = wrapper.findAll('th').find((th) => th.text().includes('Reg Number'))
    await regHeader!.trigger('click') // asc
    await regHeader!.trigger('click') // desc

    const rows = wrapper.findAll('tbody tr')
    expect(rows[0]!.text()).toContain('CD5678')
  })

  it('shows dash for zero timestamp', () => {
    const zeroVehicle: Vehicle = {
      reg_number: 'AB1234',
      queue_type: 'live',
      status: 'in_queue',
      registered_at: '2026-03-09T10:00:00Z',
      status_changed_at: '1970-01-01T00:00:00Z',
    }
    const wrapper = mount(VehicleTable, {
      props: { vehicles: [zeroVehicle] },
    })
    const cells = wrapper.findAll('td')
    const lastCell = cells[cells.length - 1]!
    expect(lastCell.text()).toBe('-')
  })
})
