import { describe, it, expect } from 'vitest'
import { DURATION_PRESETS } from '../durations'

describe('DURATION_PRESETS', () => {
  it('has 17 presets', () => {
    expect(DURATION_PRESETS).toHaveLength(17)
  })

  it('has unique values', () => {
    const values = DURATION_PRESETS.map((p) => p.value)
    expect(new Set(values).size).toBe(values.length)
  })

  it('is sorted by ascending duration', () => {
    for (let i = 1; i < DURATION_PRESETS.length; i++) {
      expect(DURATION_PRESETS[i].ms).toBeGreaterThan(DURATION_PRESETS[i - 1].ms)
    }
  })

  it('starts with 1h and ends with All', () => {
    expect(DURATION_PRESETS[0].label).toBe('1h')
    expect(DURATION_PRESETS[DURATION_PRESETS.length - 1].label).toBe('All')
  })
})
