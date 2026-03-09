<script setup lang="ts">
import { ref, computed } from 'vue'
import type { Vehicle } from '../api/types'

const props = defineProps<{ vehicles: Vehicle[] }>()

type SortKey = 'reg_number' | 'queue_type' | 'status' | 'registered_at' | 'status_changed_at'

const sortKey = ref<SortKey>('registered_at')
const sortAsc = ref(true)

const sorted = computed(() => {
  const key = sortKey.value
  const dir = sortAsc.value ? 1 : -1
  return [...props.vehicles].sort((a, b) => {
    const va = a[key]
    const vb = b[key]
    if (va < vb) return -1 * dir
    if (va > vb) return 1 * dir
    return 0
  })
})

function toggleSort(key: SortKey) {
  if (sortKey.value === key) {
    sortAsc.value = !sortAsc.value
  } else {
    sortKey.value = key
    sortAsc.value = true
  }
}

function formatTime(iso: string): string {
  const d = new Date(iso)
  if (d.getTime() <= 0) return '-'
  return d.toLocaleString()
}

function statusClass(status: string): string {
  switch (status) {
    case 'in_queue':
      return 'status-queue'
    case 'called':
      return 'status-called'
    case 'passed':
      return 'status-passed'
    case 'registered':
      return 'status-registered'
    default:
      return ''
  }
}
</script>

<template>
  <div class="table-wrapper">
    <table class="vehicle-table">
      <thead>
        <tr>
          <th @click="toggleSort('reg_number')" class="sortable">
            Reg Number
            <span v-if="sortKey === 'reg_number'">{{ sortAsc ? '\u25B2' : '\u25BC' }}</span>
          </th>
          <th @click="toggleSort('queue_type')" class="sortable">
            Queue
            <span v-if="sortKey === 'queue_type'">{{ sortAsc ? '\u25B2' : '\u25BC' }}</span>
          </th>
          <th @click="toggleSort('status')" class="sortable">
            Status
            <span v-if="sortKey === 'status'">{{ sortAsc ? '\u25B2' : '\u25BC' }}</span>
          </th>
          <th @click="toggleSort('registered_at')" class="sortable">
            Registered
            <span v-if="sortKey === 'registered_at'">{{ sortAsc ? '\u25B2' : '\u25BC' }}</span>
          </th>
          <th @click="toggleSort('status_changed_at')" class="sortable">
            Status Changed
            <span v-if="sortKey === 'status_changed_at'">{{ sortAsc ? '\u25B2' : '\u25BC' }}</span>
          </th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="v in sorted" :key="v.reg_number">
          <td class="mono">{{ v.reg_number }}</td>
          <td>{{ v.queue_type }}</td>
          <td><span :class="['status-badge', statusClass(v.status)]">{{ v.status }}</span></td>
          <td>{{ formatTime(v.registered_at) }}</td>
          <td>{{ formatTime(v.status_changed_at) }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
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

@media (prefers-color-scheme: light) {
  .vehicle-table th,
  .vehicle-table td {
    border-bottom-color: #e0e0e0;
  }
}
</style>
