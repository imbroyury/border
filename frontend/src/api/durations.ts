import type { DurationPreset } from './types'

const HOUR = 3600_000
const DAY = 24 * HOUR

export const DURATION_PRESETS: DurationPreset[] = [
  { label: '1h', value: '1h', ms: HOUR },
  { label: '3h', value: '3h', ms: 3 * HOUR },
  { label: '6h', value: '6h', ms: 6 * HOUR },
  { label: '9h', value: '9h', ms: 9 * HOUR },
  { label: '12h', value: '12h', ms: 12 * HOUR },
  { label: '1d', value: '1d', ms: DAY },
  { label: '2d', value: '2d', ms: 2 * DAY },
  { label: '3d', value: '3d', ms: 3 * DAY },
  { label: '5d', value: '5d', ms: 5 * DAY },
  { label: '7d', value: '7d', ms: 7 * DAY },
  { label: '14d', value: '14d', ms: 14 * DAY },
  { label: '1m', value: '1m', ms: 30 * DAY },
  { label: '2m', value: '2m', ms: 60 * DAY },
  { label: '3m', value: '3m', ms: 90 * DAY },
  { label: '6m', value: '6m', ms: 180 * DAY },
  { label: '1y', value: '1y', ms: 365 * DAY },
  { label: 'All', value: 'all', ms: 10 * 365 * DAY },
]
