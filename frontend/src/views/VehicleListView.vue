<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import type { VehicleSearchResult, Zone } from '../api/types'
import { fetchVehicleList, fetchZones } from '../api/client'

const router = useRouter()
const route = useRoute()

const data = ref<VehicleSearchResult[]>([])
const total = ref(0)
const loading = ref(true)
const error = ref('')
const zones = ref<Zone[]>([])

const query = ref((route.query.q as string) || '')
const zone = ref((route.query.zone as string) || '')
const sort = ref((route.query.sort as string) || 'last_seen_at')
const order = ref((route.query.order as string) || 'desc')
const page = ref(parseInt((route.query.page as string) || '1', 10))
const limit = 50

let debounceTimer: ReturnType<typeof setTimeout> | null = null

const offset = computed(() => (page.value - 1) * limit)
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / limit)))
const showFrom = computed(() => total.value === 0 ? 0 : offset.value + 1)
const showTo = computed(() => Math.min(offset.value + limit, total.value))

function updateURL() {
  const q: Record<string, string> = {}
  if (query.value) q.q = query.value
  if (zone.value) q.zone = zone.value
  if (sort.value !== 'last_seen_at') q.sort = sort.value
  if (order.value !== 'desc') q.order = order.value
  if (page.value > 1) q.page = String(page.value)
  router.replace({ query: q })
}

async function load() {
  try {
    loading.value = true
    error.value = ''
    const params: Record<string, string> = {
      limit: String(limit),
      offset: String(offset.value),
      sort: sort.value,
      order: order.value,
    }
    if (query.value) params.q = query.value
    if (zone.value) params.zone = zone.value
    const result = await fetchVehicleList(params)
    data.value = result.data
    total.value = result.total
  } catch (e: any) {
    error.value = e.message || 'Failed to load vehicles'
  } finally {
    loading.value = false
  }
}

function onQueryInput() {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => {
    page.value = 1
    updateURL()
    load()
  }, 300)
}

function onZoneChange() {
  page.value = 1
  updateURL()
  load()
}

function toggleSort(col: string) {
  if (sort.value === col) {
    order.value = order.value === 'asc' ? 'desc' : 'asc'
  } else {
    sort.value = col
    order.value = col === 'reg_number' ? 'asc' : 'desc'
  }
  page.value = 1
  updateURL()
  load()
}

function prevPage() {
  if (page.value <= 1) return
  page.value--
  updateURL()
  load()
}

function nextPage() {
  if (page.value >= totalPages.value) return
  page.value++
  updateURL()
  load()
}

function selectVehicle(regNumber: string) {
  router.push({ name: 'vehicle-history', params: { regNumber } })
}

function sortIcon(col: string): string {
  if (sort.value !== col) return ''
  return order.value === 'asc' ? '\u25B2' : '\u25BC'
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

onMounted(async () => {
  try {
    zones.value = await fetchZones()
  } catch {
    // non-critical
  }
  load()
})
</script>

<template>
  <div class="vehicle-list">
    <div class="list-header">
      <router-link to="/" class="back-link">&larr; Back</router-link>
      <h1 class="page-title">Vehicles</h1>
    </div>

    <div class="filters">
      <input
        v-model="query"
        type="text"
        placeholder="Search by registration number..."
        class="search-input"
        @input="onQueryInput"
      />
      <select v-model="zone" class="zone-select" @change="onZoneChange">
        <option value="">All zones</option>
        <option v-for="z in zones" :key="z.id" :value="z.id">{{ z.name }}</option>
      </select>
    </div>

    <p v-if="error" class="status error">{{ error }}</p>
    <p v-else-if="loading && data.length === 0" class="status">Loading...</p>
    <p v-else-if="data.length === 0" class="status">No vehicles found</p>

    <template v-else>
      <div class="table-wrapper">
        <table class="vehicle-table">
          <thead>
            <tr>
              <th class="sortable" @click="toggleSort('reg_number')">
                Reg Number <span>{{ sortIcon('reg_number') }}</span>
              </th>
              <th class="sortable" @click="toggleSort('zone_id')">
                Zone <span>{{ sortIcon('zone_id') }}</span>
              </th>
              <th>Status</th>
              <th class="sortable" @click="toggleSort('crossing_count')">
                Crossings <span>{{ sortIcon('crossing_count') }}</span>
              </th>
              <th class="sortable" @click="toggleSort('last_seen_at')">
                Last Seen <span>{{ sortIcon('last_seen_at') }}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="v in data"
              :key="v.reg_number + v.zone_id"
              class="clickable-row"
              @click="selectVehicle(v.reg_number)"
            >
              <td class="mono">{{ v.reg_number }}</td>
              <td>{{ v.zone_id }}</td>
              <td><span :class="['status-badge', statusClass(v.status)]">{{ v.status }}</span></td>
              <td>{{ v.crossing_count }}</td>
              <td>{{ formatTime(v.last_seen) }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="pagination">
        <button :disabled="page <= 1" @click="prevPage">&larr; Prev</button>
        <span class="page-info">{{ showFrom }}–{{ showTo }} of {{ total }}</span>
        <button :disabled="page >= totalPages" @click="nextPage">Next &rarr;</button>
      </div>
    </template>
  </div>
</template>

<style scoped>
.list-header {
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

.filters {
  display: flex;
  gap: 1rem;
  margin-bottom: 1.5rem;
}

.search-input {
  flex: 1;
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

.zone-select {
  padding: 0.75rem 1rem;
  font-size: 0.9rem;
  border: 1px solid #2a2a4a;
  border-radius: 8px;
  background: rgba(30, 30, 60, 0.5);
  color: #eee;
  min-width: 10rem;
}

.zone-select:focus {
  outline: none;
  border-color: #7c8cf5;
}

.table-wrapper {
  overflow-x: auto;
}

.vehicle-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.85rem;
}

.vehicle-table th,
.vehicle-table td {
  padding: 0.6rem 0.8rem;
  text-align: left;
  border-bottom: 1px solid #2a2a4a;
}

.vehicle-table th {
  color: #aaa;
  font-weight: 600;
  white-space: nowrap;
}

.sortable {
  cursor: pointer;
  user-select: none;
}

.sortable:hover {
  color: #ccc;
}

.clickable-row {
  cursor: pointer;
}

.clickable-row:hover {
  background: rgba(124, 140, 245, 0.08);
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

.pagination {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 1rem;
  margin-top: 1.5rem;
}

.pagination button {
  background: none;
  border: 1px solid #2a2a4a;
  color: #aaa;
  padding: 0.4rem 1rem;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.85rem;
}

.pagination button:hover:not(:disabled) {
  color: #fff;
  border-color: #7c8cf5;
}

.pagination button:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}

.page-info {
  color: #888;
  font-size: 0.85rem;
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
  .vehicle-table th,
  .vehicle-table td {
    border-bottom-color: #e0e0e0;
  }
}
</style>
