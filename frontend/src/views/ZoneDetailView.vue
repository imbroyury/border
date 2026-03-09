<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import type { SnapshotPoint, Vehicle, Zone } from '../api/types'
import { fetchZones, fetchSnapshots, fetchVehicles } from '../api/client'
import { DURATION_PRESETS } from '../api/durations'
import QueueChart from '../components/QueueChart.vue'
import DurationPicker from '../components/DurationPicker.vue'
import VehicleTable from '../components/VehicleTable.vue'
import VehicleHistoryPanel from '../components/VehicleHistoryPanel.vue'

const props = defineProps<{ id: string }>()

const zone = ref<Zone | null>(null)
const selectedDuration = ref('1d')
const snapshots = ref<SnapshotPoint[]>([])
const vehicles = ref<Vehicle[]>([])
const loadingChart = ref(true)
const loadingVehicles = ref(true)
const error = ref('')
const selectedVehicle = ref<string | null>(null)

let intervalId: ReturnType<typeof setInterval> | null = null

const timeRange = computed(() => {
  const preset = DURATION_PRESETS.find((p) => p.value === selectedDuration.value)
  const ms = preset?.ms ?? 24 * 3600_000
  const to = new Date()
  const from = new Date(to.getTime() - ms)
  return { from, to }
})

async function loadSnapshots() {
  try {
    loadingChart.value = true
    const { from, to } = timeRange.value
    snapshots.value = await fetchSnapshots(props.id, from, to)
    error.value = ''
  } catch (e: any) {
    error.value = e.message || 'Failed to load snapshots'
  } finally {
    loadingChart.value = false
  }
}

async function loadVehicles() {
  try {
    loadingVehicles.value = true
    vehicles.value = await fetchVehicles(props.id)
  } catch {
    // non-critical
  } finally {
    loadingVehicles.value = false
  }
}

async function loadZone() {
  try {
    const zones = await fetchZones()
    zone.value = zones.find((z) => z.id === props.id) ?? null
  } catch {
    // non-critical, fallback to id
  }
}

function loadAll() {
  loadSnapshots()
  loadVehicles()
}

watch(selectedDuration, () => {
  loadSnapshots()
})

onMounted(() => {
  loadZone()
  loadAll()
  intervalId = setInterval(loadAll, 60_000)
})

onUnmounted(() => {
  if (intervalId) clearInterval(intervalId)
})
</script>

<template>
  <div class="zone-detail">
    <div class="zone-header">
      <router-link to="/" class="back-link">&larr; Back</router-link>
      <h1 class="zone-title">{{ zone?.name ?? id }}</h1>
    </div>

    <div class="chart-section">
      <div class="chart-controls">
        <DurationPicker v-model="selectedDuration" />
      </div>

      <p v-if="error" class="status error">{{ error }}</p>
      <p v-else-if="loadingChart && snapshots.length === 0" class="status">Loading chart...</p>
      <QueueChart v-else :data="snapshots" />
    </div>

    <div class="vehicles-section">
      <h2>Current Vehicles</h2>
      <VehicleHistoryPanel
        v-if="selectedVehicle"
        :zone-id="id"
        :reg-number="selectedVehicle"
        @close="selectedVehicle = null"
      />
      <p v-if="loadingVehicles && vehicles.length === 0" class="status">Loading...</p>
      <p v-else-if="vehicles.length === 0" class="status">No vehicles in queue</p>
      <VehicleTable v-else :vehicles="vehicles" @select="selectedVehicle = $event" />
    </div>
  </div>
</template>

<style scoped>
.zone-header {
  display: flex;
  align-items: center;
  gap: 1rem;
  margin-bottom: 1.5rem;
}

.back-link {
  font-size: 0.9rem;
}

.zone-title {
  font-size: 1.4rem;
}

.chart-section {
  margin-bottom: 2rem;
}

.chart-controls {
  margin-bottom: 1rem;
}

.vehicles-section h2 {
  font-size: 1.2rem;
  margin-bottom: 1rem;
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
