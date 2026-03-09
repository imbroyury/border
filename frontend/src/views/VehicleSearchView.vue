<script setup lang="ts">
import { ref, watch } from 'vue'
import type { VehicleSearchResult, VehicleStatusChangeWithZone } from '../api/types'
import { searchVehicles, fetchGlobalVehicleHistory } from '../api/client'

const query = ref('')
const results = ref<VehicleSearchResult[]>([])
const searching = ref(false)
const searchError = ref('')

const selectedReg = ref<string | null>(null)
const history = ref<VehicleStatusChangeWithZone[]>([])
const loadingHistory = ref(false)
const historyError = ref('')

let debounceTimer: ReturnType<typeof setTimeout> | null = null

watch(query, (val) => {
  if (debounceTimer) clearTimeout(debounceTimer)
  selectedReg.value = null
  history.value = []

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
  try {
    loadingHistory.value = true
    historyError.value = ''
    history.value = await fetchGlobalVehicleHistory(regNumber)
  } catch (e: any) {
    historyError.value = e.message || 'Failed to load history'
  } finally {
    loadingHistory.value = false
  }
}

function formatTime(iso: string): string {
  const d = new Date(iso)
  if (d.getTime() <= 0) return '-'
  return d.toLocaleString()
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

    <div v-if="results.length > 0 && !selectedReg" class="results-list">
      <div
        v-for="r in results"
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

    <div v-if="selectedReg" class="history-section">
      <div class="history-header">
        <h2 class="mono">{{ selectedReg }}</h2>
        <button class="back-btn" @click="selectedReg = null; history = []">&larr; Back to results</button>
      </div>

      <p v-if="loadingHistory" class="status">Loading history...</p>
      <p v-else-if="historyError" class="status error">{{ historyError }}</p>
      <p v-else-if="history.length === 0" class="status">No history found</p>

      <div v-else class="timeline">
        <div v-for="(c, i) in history" :key="i" class="timeline-entry">
          <div class="timeline-time">{{ formatTime(c.captured_at) }}</div>
          <div class="timeline-content">
            <span :class="['status-badge', statusClass(c.status)]">{{ c.status }}</span>
            <span class="queue-label">{{ c.queue_type }}</span>
            <router-link :to="`/zone/${c.zone_id}`" class="zone-link">{{ c.zone_id }}</router-link>
          </div>
        </div>
      </div>
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

.timeline {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.timeline-entry {
  display: flex;
  gap: 1rem;
  align-items: center;
  padding: 0.4rem 0;
  border-bottom: 1px solid #2a2a4a;
}

.timeline-entry:last-child {
  border-bottom: none;
}

.timeline-time {
  color: #888;
  font-size: 0.8rem;
  min-width: 10rem;
  white-space: nowrap;
}

.timeline-content {
  display: flex;
  gap: 0.5rem;
  align-items: center;
}

.queue-label {
  color: #666;
  font-size: 0.8rem;
}

.zone-link {
  color: #7c8cf5;
  font-size: 0.8rem;
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
