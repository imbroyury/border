<script setup lang="ts">
import { ref, watch } from 'vue'
import type { VehicleStatusChange } from '../api/types'
import { fetchSingleVehicleHistory } from '../api/client'

const props = defineProps<{ zoneId: string; regNumber: string }>()
const emit = defineEmits<{ close: [] }>()

const changes = ref<VehicleStatusChange[]>([])
const loading = ref(true)
const error = ref('')

async function load() {
  try {
    loading.value = true
    error.value = ''
    changes.value = await fetchSingleVehicleHistory(props.zoneId, props.regNumber)
  } catch (e: any) {
    error.value = e.message || 'Failed to load history'
  } finally {
    loading.value = false
  }
}

watch(() => props.regNumber, load, { immediate: true })

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
    case 'cancelled':
      return 'status-cancelled'
    default:
      return ''
  }
}
</script>

<template>
  <div class="history-panel">
    <div class="panel-header">
      <h3>History: <span class="mono">{{ regNumber }}</span></h3>
      <button class="close-btn" @click="emit('close')">&times;</button>
    </div>

    <p v-if="loading" class="status">Loading...</p>
    <p v-else-if="error" class="status error">{{ error }}</p>
    <p v-else-if="changes.length === 0" class="status">No history found</p>

    <div v-else class="timeline">
      <div v-for="(c, i) in changes" :key="i" class="timeline-entry">
        <div class="timeline-time">
          {{ formatTime(c.captured_at) }}
          <span v-if="c.last_seen_at !== c.captured_at" class="last-seen">– {{ formatTimeShort(c.last_seen_at) }}</span>
        </div>
        <div class="timeline-content">
          <span :class="['status-badge', statusClass(c.status)]">{{ c.status }}</span>
          <span class="queue-label">{{ c.queue_type }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.history-panel {
  border: 1px solid #2a2a4a;
  border-radius: 8px;
  padding: 1rem;
  margin-bottom: 1.5rem;
  background: rgba(30, 30, 60, 0.5);
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}

.panel-header h3 {
  font-size: 1rem;
  margin: 0;
}

.close-btn {
  background: none;
  border: none;
  color: #aaa;
  font-size: 1.5rem;
  cursor: pointer;
  padding: 0 0.3rem;
  line-height: 1;
}

.close-btn:hover {
  color: #fff;
}

.mono {
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
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
  min-width: 14rem;
  white-space: nowrap;
}

.last-seen {
  color: #666;
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
  padding: 1rem;
  text-align: center;
  color: #aaa;
}

.error {
  color: #ff6b6b;
}
</style>
