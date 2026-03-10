<script setup lang="ts">
import { ref, watch } from 'vue'
import type { CrossingHistory } from '../api/types'
import { fetchGlobalVehicleHistory } from '../api/client'

const props = defineProps<{ regNumber: string }>()

const crossings = ref<CrossingHistory[]>([])
const loading = ref(true)
const error = ref('')
const expanded = ref<Set<number>>(new Set())

async function load() {
  try {
    loading.value = true
    error.value = ''
    expanded.value = new Set()
    crossings.value = await fetchGlobalVehicleHistory(props.regNumber)
    for (const c of crossings.value) {
      if (c.is_active) expanded.value.add(c.crossing_id)
    }
  } catch (e: any) {
    error.value = e.message || 'Failed to load history'
  } finally {
    loading.value = false
  }
}

watch(() => props.regNumber, load, { immediate: true })

function toggleExpanded(id: number) {
  if (expanded.value.has(id)) {
    expanded.value.delete(id)
  } else {
    expanded.value.add(id)
  }
}

function formatTime(iso: string): string {
  const d = new Date(iso)
  if (d.getTime() <= 0) return '-'
  return d.toLocaleString()
}

function formatTimeShort(iso: string): string {
  const d = new Date(iso)
  if (d.getTime() <= 0) return '-'
  return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
}

function formatDate(iso: string): string {
  const d = new Date(iso)
  if (d.getTime() <= 0) return '-'
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })
}

function statusClass(status: string): string {
  switch (status) {
    case 'in_queue': return 'status-queue'
    case 'called': return 'status-called'
    case 'passed': return 'status-passed'
    case 'registered': return 'status-registered'
    case 'cancelled': return 'status-cancelled'
    default: return ''
  }
}
</script>

<template>
  <div class="vehicle-history">
    <div class="history-header">
      <router-link to="/vehicles" class="back-link">&larr; Back</router-link>
      <h1 class="page-title mono">{{ regNumber }}</h1>
    </div>

    <p v-if="loading" class="status">Loading history...</p>
    <p v-else-if="error" class="status error">{{ error }}</p>
    <p v-else-if="crossings.length === 0" class="status">No history found</p>

    <template v-else>
      <p class="crossing-count">{{ crossings.length }} crossing{{ crossings.length !== 1 ? 's' : '' }}</p>

      <div class="crossings">
        <div
          v-for="c in crossings"
          :key="c.crossing_id"
          :class="['crossing-card', { active: c.is_active }]"
        >
          <div class="crossing-header" @click="toggleExpanded(c.crossing_id)">
            <div class="crossing-meta">
              <router-link :to="`/zone/${c.zone_id}`" class="zone-link" @click.stop>{{ c.zone_id }}</router-link>
              <span class="date-range">
                {{ formatDate(c.first_seen_at) }} – {{ c.is_active ? 'present' : formatDate(c.last_seen_at) }}
              </span>
            </div>
            <div class="crossing-right">
              <span :class="['status-badge', statusClass(c.current_status)]">{{ c.current_status }}</span>
              <span v-if="c.is_active" class="active-indicator">active</span>
              <span class="expand-icon">{{ expanded.has(c.crossing_id) ? '\u25B2' : '\u25BC' }}</span>
            </div>
          </div>

          <div v-if="expanded.has(c.crossing_id)" class="status-timeline">
            <div v-for="(sc, i) in c.status_changes" :key="i" class="timeline-entry">
              <div class="timeline-time">
                {{ formatTime(sc.detected_at) }}
                <span v-if="sc.last_seen_at !== sc.detected_at" class="last-seen">– {{ formatTimeShort(sc.last_seen_at) }}</span>
              </div>
              <span :class="['status-badge', statusClass(sc.status)]">{{ sc.status }}</span>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.history-header {
  display: flex;
  align-items: center;
  gap: 1rem;
  margin-bottom: 1.5rem;
}

.back-link {
  font-size: 0.9rem;
}

.page-title {
  font-size: 1.4rem;
}

.mono {
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
}

.crossing-count {
  color: #888;
  font-size: 0.85rem;
  margin: 0 0 0.75rem;
}

.crossings {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.crossing-card {
  border: 1px solid #2a2a4a;
  border-radius: 6px;
  overflow: hidden;
}

.crossing-card.active {
  border-color: #7c8cf5;
  background: rgba(124, 140, 245, 0.05);
}

.crossing-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.6rem 0.8rem;
  cursor: pointer;
  gap: 1rem;
}

.crossing-header:hover {
  background: rgba(255, 255, 255, 0.03);
}

.crossing-meta {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex: 1;
  min-width: 0;
}

.zone-link {
  color: #7c8cf5;
  font-size: 0.85rem;
  white-space: nowrap;
  text-decoration: none;
}

.zone-link:hover {
  text-decoration: underline;
}

.date-range {
  color: #888;
  font-size: 0.8rem;
  white-space: nowrap;
}

.crossing-right {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.active-indicator {
  font-size: 0.7rem;
  color: #4ecdc4;
  border: 1px solid #4ecdc4;
  border-radius: 3px;
  padding: 0.1em 0.4em;
}

.expand-icon {
  color: #666;
  font-size: 0.7rem;
}

.status-timeline {
  border-top: 1px solid #2a2a4a;
  padding: 0.5rem 0.8rem;
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.timeline-entry {
  display: flex;
  gap: 1rem;
  align-items: center;
  padding: 0.2rem 0;
}

.timeline-time {
  color: #888;
  font-size: 0.8rem;
  min-width: 14rem;
  white-space: nowrap;
}

.last-seen {
  color: #666;
}

.status-badge {
  padding: 0.15em 0.5em;
  border-radius: 3px;
  font-size: 0.8rem;
}

.status-queue {
  background: rgba(124, 140, 245, 0.2);
  color: #7c8cf5;
}

.status-called {
  background: rgba(255, 193, 7, 0.2);
  color: #ffc107;
}

.status-passed {
  background: rgba(78, 205, 196, 0.2);
  color: #4ecdc4;
}

.status-registered {
  background: rgba(170, 170, 170, 0.2);
  color: #aaa;
}

.status-cancelled {
  background: rgba(255, 107, 107, 0.2);
  color: #ff6b6b;
}

.status {
  padding: 2rem;
  text-align: center;
  color: #aaa;
}

.error {
  color: #ff6b6b;
}
</style>
