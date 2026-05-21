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
import { useThemeState } from '../theme-state'
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
const { currentThemeMode } = useThemeState()

let chart: ECharts | null = null

const hasData = computed(() => props.points.length > 0)

function getRootStyles() {
  if (typeof window === 'undefined' || typeof document === 'undefined') {
    return null
  }
  return getComputedStyle(document.documentElement)
}

function readThemeColor(styles: CSSStyleDeclaration | null, token: string, fallback: string) {
  const value = styles?.getPropertyValue(token).trim()
  return value || fallback
}

function renderChart() {
  if (!chartRef.value || !hasData.value) {
    return
  }

  const rootStyles = getRootStyles()
  const accent = readThemeColor(rootStyles, '--app-accent', '#24b47e')
  const warning = readThemeColor(rootStyles, '--app-warning', '#f59e0b')
  const info = readThemeColor(rootStyles, '--app-info', '#7dd3a8')
  const border = readThemeColor(rootStyles, '--app-border', 'rgba(140, 158, 151, 0.18)')
  const text = readThemeColor(rootStyles, '--app-text', '#edf2ff')
  const textSoft = readThemeColor(rootStyles, '--app-text-soft', '#9fb0c7')
  const textFaint = readThemeColor(rootStyles, '--app-text-faint', '#6b7a90')
  const tooltipBg =
    currentThemeMode.value === 'dark' ? 'rgba(10, 19, 33, 0.96)' : 'rgba(255, 250, 242, 0.96)'

  chart ??= init(chartRef.value)
  chart.setOption<ECOption>({
    tooltip: {
      trigger: 'axis',
      backgroundColor: tooltipBg,
      borderColor: border,
      textStyle: { color: text },
    },
    legend: {
      top: 2,
      right: 16,
      textStyle: { color: textSoft },
    },
    grid: { left: 18, right: 18, top: 50, bottom: 18, containLabel: true },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      axisLine: { lineStyle: { color: border } },
      axisTick: { show: false },
      axisLabel: { color: textFaint },
      data: props.points.map((point) =>
        new Date(point.sampledAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
      ),
    },
    yAxis: {
      type: 'value',
      max: 100,
      axisLabel: { color: textFaint },
      splitLine: { lineStyle: { color: border } },
    },
    series: [
      {
        name: 'CPU',
        type: 'line',
        smooth: true,
        showSymbol: false,
        lineStyle: { width: 2.5, color: accent },
        areaStyle: { color: `${accent}1f` },
        data: props.points.map((point) => point.cpuUsage),
      },
      {
        name: '内存',
        type: 'line',
        smooth: true,
        showSymbol: false,
        lineStyle: { width: 2.5, color: warning },
        areaStyle: { color: `${warning}1a` },
        data: props.points.map((point) => point.memoryUsage),
      },
      {
        name: '磁盘',
        type: 'line',
        smooth: true,
        showSymbol: false,
        lineStyle: { width: 2.5, color: info },
        areaStyle: { color: `${info}1a` },
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

watch(
  () => currentThemeMode.value,
  async () => {
    await nextTick()
    renderChart()
  },
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
  <n-card :bordered="false" class="trend-card">
    <template #header>
      <div class="trend-title">{{ title }}</div>
    </template>
    <n-empty v-if="!hasData" description="暂无历史趋势数据" />
    <div v-else ref="chartRef" class="chart-host" />
  </n-card>
</template>

<style scoped>
.trend-card {
  border-radius: 8px;
  border: 1px solid var(--app-border);
  background: transparent;
  box-shadow: none;
}

.trend-title {
  color: var(--app-text);
  font-size: 15px;
  font-weight: 700;
}

.chart-host {
  width: 100%;
  height: 360px;
}
</style>
