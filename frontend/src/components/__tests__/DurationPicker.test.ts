import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import DurationPicker from '../DurationPicker.vue'
import { DURATION_PRESETS } from '../../api/durations'

describe('DurationPicker', () => {
  it('renders all preset buttons', () => {
    const wrapper = mount(DurationPicker, {
      props: { modelValue: '1d', 'onUpdate:modelValue': () => {} },
    })

    const buttons = wrapper.findAll('button')
    expect(buttons).toHaveLength(DURATION_PRESETS.length)
  })

  it('marks active button', () => {
    const wrapper = mount(DurationPicker, {
      props: { modelValue: '7d', 'onUpdate:modelValue': () => {} },
    })

    const activeBtn = wrapper.find('button.active')
    expect(activeBtn.text()).toBe('7d')
  })

  it('emits update on click', async () => {
    const wrapper = mount(DurationPicker, {
      props: { modelValue: '1d', 'onUpdate:modelValue': () => {} },
    })

    const btn3h = wrapper.findAll('button').find((b) => b.text() === '3h')!
    await btn3h.trigger('click')

    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['3h'])
  })
})
