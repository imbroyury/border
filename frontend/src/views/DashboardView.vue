<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import type { Zone } from '../api/types'
import { fetchZones } from '../api/client'
import ZoneCard from '../components/ZoneCard.vue'

const zones = ref<Zone[]>([])
const loading = ref(true)
const error = ref('')
const collapsed = ref<Record<string, boolean>>({})

let intervalId: ReturnType<typeof setInterval> | null = null

const BORDER_ORDER: Record<string, number> = { 'BY-PL': 0, 'BY-LT': 1 }
const BORDER_LABELS: Record<string, string> = {
  'BY-PL': 'Belarus \u2194 Poland',
  'BY-LT': 'Belarus \u2194 Lithuania',
}
const ZONE_ORDER: Record<string, number> = { brest: 0 }

const groups = computed(() => {
  const map = new Map<string, Zone[]>()
  for (const z of zones.value) {
    const list = map.get(z.border) ?? []
    list.push(z)
    map.set(z.border, list)
  }

  return [...map.entries()]
    .sort(([a], [b]) => (BORDER_ORDER[a] ?? 99) - (BORDER_ORDER[b] ?? 99))
    .map(([border, zoneList]) => ({
      border,
      label: BORDER_LABELS[border] ?? border,
      zones: zoneList.sort(
        (a, b) => (ZONE_ORDER[a.id] ?? 99) - (ZONE_ORDER[b.id] ?? 99) || a.name.localeCompare(b.name)
      ),
    }))
})

function toggle(border: string) {
  collapsed.value[border] = !collapsed.value[border]
}

async function load() {
  try {
    zones.value = await fetchZones()
    error.value = ''
  } catch (e: any) {
    error.value = e.message || 'Failed to load zones'
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  load()
  intervalId = setInterval(load, 60_000)
})

onUnmounted(() => {
  if (intervalId) clearInterval(intervalId)
})
</script>

<template>
  <div class="dashboard">
    <div class="dashboard-header">
      <h1 class="dashboard-title">Border Crossings</h1>
      <router-link to="/vehicles" class="search-link">Search vehicles</router-link>
    </div>

    <p v-if="loading" class="status">Loading...</p>
    <p v-else-if="error" class="status error">{{ error }}</p>

    <template v-else>
      <section v-for="group in groups" :key="group.border" class="border-group">
        <button class="group-header" @click="toggle(group.border)">
          <span class="group-arrow">{{ collapsed[group.border] ? '\u25B6' : '\u25BC' }}</span>
          <span class="group-label">{{ group.label }}</span>
          <span class="group-count">{{ group.zones.length }} crossings</span>
        </button>

        <div v-if="!collapsed[group.border]" class="zone-grid">
          <ZoneCard v-for="zone in group.zones" :key="zone.id" :zone="zone" />
        </div>
      </section>
    </template>
  </div>
</template>

<style scoped>
.dashboard-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 1.5rem;
}

.dashboard-title {
  font-size: 1.6rem;
}

.search-link {
  font-size: 0.9rem;
  color: #7c8cf5;
}

.border-group {
  margin-bottom: 1.5rem;
}

.group-header {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  width: 100%;
  padding: 0.8rem 1rem;
  background: #1a1a2e;
  border: 1px solid #2a2a4a;
  border-radius: 6px;
  color: #e0e0e0;
  font-size: 1.1rem;
  font-weight: 600;
  cursor: pointer;
  margin-bottom: 1rem;
  text-align: left;
}

.group-header:hover {
  border-color: #7c8cf5;
}

.group-arrow {
  font-size: 0.8rem;
  color: #888;
}

.group-count {
  margin-left: auto;
  font-size: 0.8rem;
  font-weight: 400;
  color: #666;
}

.zone-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 1rem;
}

.status {
  padding: 2rem;
  text-align: center;
  color: #aaa;
}

.error {
  color: #ff6b6b;
}

@media (prefers-color-scheme: light) {
  .group-header {
    background: #ffffff;
    border-color: #e0e0e0;
    color: #213547;
  }
}
</style>
