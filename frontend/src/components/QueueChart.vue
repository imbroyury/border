<script setup lang="ts">
import { computed } from 'vue'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  TitleComponent,
  TooltipComponent,
  GridComponent,
  LegendComponent,
  DataZoomComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { SnapshotPoint } from '../api/types'

use([
  LineChart,
  TitleComponent,
  TooltipComponent,
  GridComponent,
  LegendComponent,
  DataZoomComponent,
  CanvasRenderer,
])

const props = defineProps<{ data: SnapshotPoint[] }>()

const option = computed(() => {
  const times = props.data.map((p) => new Date(p.captured_at).toLocaleString())
  return {
    tooltip: {
      trigger: 'axis',
    },
    legend: {
      data: ['Cars in Queue', 'Sent/Hour', 'Sent/24h'],
      textStyle: { color: '#aaa' },
    },
    grid: {
      left: '3%',
      right: '4%',
      bottom: '15%',
      containLabel: true,
    },
    dataZoom: [
      {
        type: 'inside',
        start: 0,
        end: 100,
      },
      {
        type: 'slider',
        start: 0,
        end: 100,
      },
    ],
    xAxis: {
      type: 'category',
      data: times,
      axisLabel: { color: '#888' },
    },
    yAxis: {
      type: 'value',
      axisLabel: { color: '#888' },
      splitLine: { lineStyle: { color: '#2a2a4a' } },
    },
    series: [
      {
        name: 'Cars in Queue',
        type: 'line',
        data: props.data.map((p) => Math.round(p.cars_count)),
        smooth: true,
        lineStyle: { width: 2 },
        itemStyle: { color: '#7c8cf5' },
        areaStyle: { color: 'rgba(124, 140, 245, 0.1)' },
      },
      {
        name: 'Sent/Hour',
        type: 'line',
        data: props.data.map((p) => Math.round(p.sent_last_hour)),
        smooth: true,
        lineStyle: { width: 2 },
        itemStyle: { color: '#4ecdc4' },
      },
      {
        name: 'Sent/24h',
        type: 'line',
        data: props.data.map((p) => Math.round(p.sent_last_24h)),
        smooth: true,
        lineStyle: { width: 2 },
        itemStyle: { color: '#ff6b6b' },
      },
    ],
  }
})
</script>

<template>
  <div class="chart-container">
    <p v-if="data.length === 0" class="no-data">No data for selected period</p>
    <v-chart v-else :option="option" autoresize class="chart" />
  </div>
</template>

<style scoped>
.chart-container {
  width: 100%;
  min-height: 400px;
}

.chart {
  width: 100%;
  height: 400px;
}

.no-data {
  text-align: center;
  padding: 4rem 0;
  color: #666;
}
</style>
