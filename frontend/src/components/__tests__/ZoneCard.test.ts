import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import ZoneCard from '../ZoneCard.vue'

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/zone/:id', component: { template: '<div />' } },
    ],
  })
}

const zone = {
  id: 'brest',
  name: 'Брест',
  border: 'BY-PL',
  cars_count: 42,
  last_captured: new Date().toISOString(),
}

describe('ZoneCard', () => {
  it('renders zone name and car count', async () => {
    const router = makeRouter()
    await router.push('/')
    await router.isReady()

    const wrapper = mount(ZoneCard, {
      props: { zone },
      global: { plugins: [router] },
    })

    expect(wrapper.text()).toContain('Брест')
    expect(wrapper.text()).toContain('42')
  })

  it('renders border label', async () => {
    const router = makeRouter()
    await router.push('/')
    await router.isReady()

    const wrapper = mount(ZoneCard, {
      props: { zone },
      global: { plugins: [router] },
    })

    expect(wrapper.text()).toContain('Poland')
  })

  it('links to zone detail page', async () => {
    const router = makeRouter()
    await router.push('/')
    await router.isReady()

    const wrapper = mount(ZoneCard, {
      props: { zone },
      global: { plugins: [router] },
    })

    const link = wrapper.find('a')
    expect(link.attributes('href')).toBe('/zone/brest')
  })

  it('shows "No data" for epoch timestamp', async () => {
    const router = makeRouter()
    await router.push('/')
    await router.isReady()

    const wrapper = mount(ZoneCard, {
      props: { zone: { ...zone, last_captured: '1970-01-01T00:00:00Z' } },
      global: { plugins: [router] },
    })

    expect(wrapper.text()).toContain('No data')
  })
})
