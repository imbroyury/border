<script setup lang="ts">
import { computed } from 'vue'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  TitleComponent,
  TooltipComponent,
  GridComponent,
  DataZoomComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { SnapshotPoint } from '../api/types'

use([
  LineChart,
  TitleComponent,
  TooltipComponent,
  GridComponent,
  DataZoomComponent,
  CanvasRenderer,
])

const props = defineProps<{ data: SnapshotPoint[] }>()

function makeOption(title: string, field: keyof SnapshotPoint, color: string, areaColor?: string) {
  return computed(() => {
    const times = props.data.map((p) => new Date(p.captured_at).toLocaleString())
    const dataMax = Math.max(0, ...props.data.map((p) => p[field] as number))
    const yMax = dataMax < 20 ? 20 : undefined
    return {
      title: {
        text: title,
        textStyle: { color: '#ccc', fontSize: 14, fontWeight: 500 },
        left: 'center',
      },
      tooltip: { trigger: 'axis' },
      grid: {
        left: '3%',
        right: '4%',
        bottom: '15%',
        containLabel: true,
      },
      dataZoom: [
        { type: 'inside', start: 0, end: 100 },
        { type: 'slider', start: 0, end: 100 },
      ],
      xAxis: {
        type: 'category',
        data: times,
        axisLabel: { color: '#888' },
      },
      yAxis: {
        type: 'value',
        min: 0,
        max: yMax,
        axisLabel: { color: '#888' },
        splitLine: { lineStyle: { color: '#2a2a4a' } },
      },
      series: [
        {
          name: title,
          type: 'line',
          data: props.data.map((p) => Math.round(p[field] as number)),
          smooth: true,
          lineStyle: { width: 2 },
          itemStyle: { color },
          ...(areaColor ? { areaStyle: { color: areaColor } } : {}),
        },
      ],
    }
  })
}

const carsOption = makeOption('Cars in Queue', 'cars_count', '#7c8cf5', 'rgba(124, 140, 245, 0.1)')
const sentHourOption = makeOption('Sent / Hour', 'sent_last_hour', '#4ecdc4')
const sentDayOption = makeOption('Sent / 24h', 'sent_last_24h', '#ff6b6b')
</script>

<template>
  <div class="charts-container">
    <p v-if="data.length === 0" class="no-data">No data for selected period</p>
    <template v-else>
      <v-chart :option="carsOption" autoresize class="chart" />
      <v-chart :option="sentHourOption" autoresize class="chart" />
      <v-chart :option="sentDayOption" autoresize class="chart" />
    </template>
  </div>
</template>

<style scoped>
.charts-container {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.chart {
  width: 100%;
  height: 350px;
}

.no-data {
  text-align: center;
  padding: 4rem 0;
  color: #666;
}
</style>
