<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import type { Zone } from '../api/types'
import { fetchZones } from '../api/client'
import ZoneCard from '../components/ZoneCard.vue'

const zones = ref<Zone[]>([])
const loading = ref(true)
const error = ref('')

let intervalId: ReturnType<typeof setInterval> | null = null

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
    <h1 class="dashboard-title">Border Crossings</h1>

    <p v-if="loading" class="status">Loading...</p>
    <p v-else-if="error" class="status error">{{ error }}</p>

    <div v-else class="zone-grid">
      <ZoneCard v-for="zone in zones" :key="zone.id" :zone="zone" />
    </div>
  </div>
</template>

<style scoped>
.dashboard-title {
  font-size: 1.6rem;
  margin-bottom: 1.5rem;
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
</style>
