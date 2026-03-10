<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import type { VehicleSearchResult, CrossingHistory } from '../api/types'
import { searchVehicles, fetchGlobalVehicleHistory, fetchRecentVehicles } from '../api/client'

const query = ref('')
const results = ref<VehicleSearchResult[]>([])
const recentVehicles = ref<VehicleSearchResult[]>([])
const searching = ref(false)
const loadingRecent = ref(true)
const searchError = ref('')

const selectedReg = ref<string | null>(null)
const crossings = ref<CrossingHistory[]>([])
const loadingHistory = ref(false)
const historyError = ref('')
const expanded = ref<Set<number>>(new Set())

let debounceTimer: ReturnType<typeof setTimeout> | null = null

onMounted(async () => {
  try {
    recentVehicles.value = await fetchRecentVehicles()
  } catch {
    // non-critical
  } finally {
    loadingRecent.value = false
  }
})

watch(query, (val) => {
  if (debounceTimer) clearTimeout(debounceTimer)
  selectedReg.value = null
  crossings.value = []

  if (val.length < 2) {
    results.value = []
    searchError.value = ''
    return
  }

  debounceTimer = setTimeout(() => doSearch(val), 300)
})

async function doSearch(q: string) {
  try {
    searching.value = true
    searchError.value = ''
    results.value = await searchVehicles(q)
  } catch (e: any) {
    searchError.value = e.message || 'Search failed'
  } finally {
    searching.value = false
  }
}

async function selectVehicle(regNumber: string) {
  selectedReg.value = regNumber
  expanded.value = new Set()
  try {
    loadingHistory.value = true
    historyError.value = ''
    crossings.value = await fetchGlobalVehicleHistory(regNumber)
    // Auto-expand active crossings
    for (const c of crossings.value) {
      if (c.is_active) expanded.value.add(c.crossing_id)
    }
  } catch (e: any) {
    historyError.value = e.message || 'Failed to load history'
  } finally {
    loadingHistory.value = false
  }
}

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
  <div class="vehicle-search">
    <div class="search-header">
      <router-link to="/" class="back-link">&larr; Back</router-link>
      <h1 class="page-title">Vehicle Search</h1>
    </div>

    <div class="search-box">
      <input
        v-model="query"
        type="text"
        placeholder="Search by registration number..."
        class="search-input"
        autofocus
      />
    </div>

    <p v-if="searchError" class="status error">{{ searchError }}</p>
    <p v-else-if="searching" class="status">Searching...</p>
    <p v-else-if="query.length >= 2 && results.length === 0 && !searching" class="status">No vehicles found</p>

    <div v-if="!selectedReg && (query.length >= 2 ? results.length > 0 : recentVehicles.length > 0)" class="results-section">
      <h2 v-if="query.length < 2" class="section-title">Recent vehicles</h2>
      <p v-if="query.length < 2 && loadingRecent" class="status">Loading...</p>
      <div class="results-list">
        <div
          v-for="r in (query.length >= 2 ? results : recentVehicles)"
          :key="r.reg_number + r.zone_id"
          class="result-item"
          @click="selectVehicle(r.reg_number)"
        >
          <span class="mono reg">{{ r.reg_number }}</span>
          <span class="zone-label">{{ r.zone_id }}</span>
          <span :class="['status-badge', statusClass(r.status)]">{{ r.status }}</span>
          <span class="time-label">{{ formatTime(r.last_seen) }}</span>
        </div>
      </div>
    </div>

    <div v-if="selectedReg" class="history-section">
      <div class="history-header">
        <h2 class="mono">{{ selectedReg }}</h2>
        <button class="back-btn" @click="selectedReg = null; crossings = []">&larr; Back to results</button>
      </div>

      <p v-if="loadingHistory" class="status">Loading history...</p>
      <p v-else-if="historyError" class="status error">{{ historyError }}</p>
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
                <span class="expand-icon">{{ expanded.has(c.crossing_id) ? '▲' : '▼' }}</span>
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
  </div>
</template>

<style scoped>
.search-header {
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

.search-box {
  margin-bottom: 1.5rem;
}

.search-input {
  width: 100%;
  padding: 0.75rem 1rem;
  font-size: 1rem;
  border: 1px solid #2a2a4a;
  border-radius: 8px;
  background: rgba(30, 30, 60, 0.5);
  color: #eee;
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
}

.search-input::placeholder {
  color: #666;
}

.search-input:focus {
  outline: none;
  border-color: #7c8cf5;
}

.section-title {
  font-size: 1rem;
  color: #888;
  margin-bottom: 0.75rem;
}

.results-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.result-item {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 0.75rem 1rem;
  border: 1px solid #2a2a4a;
  border-radius: 6px;
  cursor: pointer;
}

.result-item:hover {
  background: rgba(124, 140, 245, 0.08);
  border-color: #7c8cf5;
}

.reg {
  font-weight: 600;
  min-width: 8rem;
}

.zone-label {
  color: #888;
  font-size: 0.85rem;
  min-width: 6rem;
}

.time-label {
  color: #666;
  font-size: 0.8rem;
  margin-left: auto;
}

.history-section {
  margin-top: 1rem;
}

.history-header {
  display: flex;
  align-items: center;
  gap: 1rem;
  margin-bottom: 1rem;
}

.history-header h2 {
  font-size: 1.2rem;
  margin: 0;
}

.back-btn {
  background: none;
  border: 1px solid #2a2a4a;
  color: #aaa;
  padding: 0.3rem 0.8rem;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.85rem;
}

.back-btn:hover {
  color: #fff;
  border-color: #7c8cf5;
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

.mono {
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
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
