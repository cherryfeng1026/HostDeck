<script setup lang="ts">
import { LineChart, type LineSeriesOption } from 'echarts/charts'
import {
  GridComponent,
  LegendComponent,
  TooltipComponent,
  type GridComponentOption,
  type LegendComponentOption,
  type TooltipComponentOption,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { NCard, NEmpty } from 'naive-ui'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { init, use, type ComposeOption, type ECharts } from 'echarts/core'
import type { MetricPoint } from '../types'

type ECOption = ComposeOption<
  GridComponentOption | LegendComponentOption | TooltipComponentOption | LineSeriesOption
>

use([LineChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer])

const props = defineProps<{
  title: string
  points: MetricPoint[]
}>()

const chartRef = ref<HTMLDivElement | null>(null)
let chart: ECharts | null = null

const hasData = computed(() => props.points.length > 0)

function renderChart() {
  if (!chartRef.value || !hasData.value) {
    return
  }

  chart ??= init(chartRef.value)
  chart.setOption<ECOption>({
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(10, 16, 28, 0.96)',
      borderColor: 'rgba(148, 163, 184, 0.18)',
      textStyle: { color: '#e5edf7' },
    },
    legend: {
      top: 0,
      textStyle: { color: '#9fb0c7' },
    },
    grid: { left: 16, right: 16, top: 44, bottom: 16, containLabel: true },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      axisLine: { lineStyle: { color: 'rgba(148, 163, 184, 0.18)' } },
      axisTick: { show: false },
      axisLabel: { color: '#6b7a90' },
      data: props.points.map((point) =>
        new Date(point.sampledAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
      ),
    },
    yAxis: {
      type: 'value',
      max: 100,
      axisLabel: { color: '#6b7a90' },
      splitLine: { lineStyle: { color: 'rgba(148, 163, 184, 0.1)' } },
    },
    series: [
      {
        name: 'CPU',
        type: 'line',
        smooth: true,
        showSymbol: false,
        lineStyle: { width: 3, color: '#34d399' },
        areaStyle: { color: 'rgba(52, 211, 153, 0.1)' },
        data: props.points.map((point) => point.cpuUsage),
      },
      {
        name: '内存',
        type: 'line',
        smooth: true,
        showSymbol: false,
        lineStyle: { width: 3, color: '#f59e0b' },
        areaStyle: { color: 'rgba(245, 158, 11, 0.08)' },
        data: props.points.map((point) => point.memoryUsage),
      },
      {
        name: '磁盘',
        type: 'line',
        smooth: true,
        showSymbol: false,
        lineStyle: { width: 3, color: '#38bdf8' },
        areaStyle: { color: 'rgba(56, 189, 248, 0.08)' },
        data: props.points.map((point) => point.diskUsage),
      },
    ],
  })
}

watch(
  () => props.points,
  async () => {
    await nextTick()
    renderChart()
  },
  { deep: true },
)

onMounted(() => {
  renderChart()
  window.addEventListener('resize', handleResize)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleResize)
  chart?.dispose()
  chart = null
})

function handleResize() {
  chart?.resize()
}
</script>

<template>
  <n-card :title="title" :bordered="false" class="trend-card">
    <n-empty v-if="!hasData" description="暂无历史趋势数据" />
    <div v-else ref="chartRef" class="chart-host" />
  </n-card>
</template>

<style scoped>
.trend-card {
  border-radius: var(--app-radius-lg);
  border: 1px solid var(--app-border);
  background: var(--app-surface-muted);
  box-shadow: var(--app-shadow-soft);
}

.trend-card :deep(.n-card-header__main) {
  color: #f8fafc;
  font-size: 16px;
}

.chart-host {
  width: 100%;
  height: 320px;
}
</style>
