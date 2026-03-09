<script setup lang="ts">
import type { Zone } from '../api/types'

defineProps<{ zone: Zone }>()

function borderLabel(border: string): string {
  switch (border) {
    case 'BY-PL':
      return 'Belarus \u2194 Poland'
    case 'BY-LT':
      return 'Belarus \u2194 Lithuania'
    default:
      return border
  }
}

function timeAgo(iso: string): string {
  const ts = new Date(iso).getTime()
  if (ts <= 0) return 'No data'
  const diff = Date.now() - ts
  const minutes = Math.floor(diff / 60_000)
  if (minutes < 1) return 'Just now'
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  return `${Math.floor(hours / 24)}d ago`
}
</script>

<template>
  <router-link :to="`/zone/${zone.id}`" class="zone-card">
    <div class="zone-card-header">
      <span class="zone-name">{{ zone.name }}</span>
      <span class="zone-border">{{ borderLabel(zone.border) }}</span>
    </div>
    <div class="zone-card-body">
      <div class="car-count">
        <span class="count-value">{{ zone.cars_count }}</span>
        <span class="count-label">cars in queue</span>
      </div>
    </div>
    <div class="zone-card-footer">
      <span class="last-update">{{ timeAgo(zone.last_captured) }}</span>
    </div>
  </router-link>
</template>

<style scoped>
.zone-card {
  display: block;
  background: #1a1a2e;
  border: 1px solid #2a2a4a;
  border-radius: 8px;
  padding: 1.2rem;
  text-decoration: none;
  color: inherit;
  transition: border-color 0.2s, transform 0.2s;
}

.zone-card:hover {
  border-color: #7c8cf5;
  transform: translateY(-2px);
}

.zone-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}

.zone-name {
  font-size: 1.1rem;
  font-weight: 600;
}

.zone-border {
  font-size: 0.75rem;
  padding: 0.2em 0.6em;
  background: #2a2a4a;
  border-radius: 4px;
  color: #aaa;
}

.zone-card-body {
  margin-bottom: 0.8rem;
}

.car-count {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
}

.count-value {
  font-size: 2rem;
  font-weight: 700;
  color: #7c8cf5;
}

.count-label {
  font-size: 0.85rem;
  color: #888;
}

.zone-card-footer {
  font-size: 0.75rem;
  color: #666;
}

@media (prefers-color-scheme: light) {
  .zone-card {
    background: #ffffff;
    border-color: #e0e0e0;
  }

  .zone-border {
    background: #f0f0f0;
    color: #666;
  }
}
</style>
