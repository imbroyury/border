<script setup lang="ts">
import { DURATION_PRESETS } from '../api/durations'

const model = defineModel<string>({ required: true })
</script>

<template>
  <div class="duration-picker">
    <button
      v-for="preset in DURATION_PRESETS"
      :key="preset.value"
      :class="['duration-btn', { active: model === preset.value }]"
      @click="model = preset.value"
    >
      {{ preset.label }}
    </button>
  </div>
</template>

<style scoped>
.duration-picker {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
}

.duration-btn {
  padding: 0.3rem 0.7rem;
  font-size: 0.8rem;
  border: 1px solid #2a2a4a;
  border-radius: 4px;
  background: transparent;
  color: #aaa;
  cursor: pointer;
  transition: all 0.15s;
}

.duration-btn:hover {
  border-color: #7c8cf5;
  color: #ccc;
}

.duration-btn.active {
  background: #7c8cf5;
  border-color: #7c8cf5;
  color: #fff;
}

@media (prefers-color-scheme: light) {
  .duration-btn {
    border-color: #d0d0d0;
    color: #666;
  }

  .duration-btn.active {
    background: #5a6bf5;
    border-color: #5a6bf5;
    color: #fff;
  }
}
</style>
