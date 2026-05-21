<script setup lang="ts">
import { LineChart, type LineSeriesOption } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  type GridComponentOption,
  type TooltipComponentOption,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { init, use, type ComposeOption, type ECharts } from 'echarts/core'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { DashboardRange, DashboardTrendPoint } from '../types'

use([LineChart, GridComponent, TooltipComponent, CanvasRenderer])

type ECOption = ComposeOption<GridComponentOption | TooltipComponentOption | LineSeriesOption>
type MetricKey = 'avgCpuUsage' | 'avgMemoryUsage' | 'avgDiskUsage' | 'avgLoad1'

const props = defineProps<{
  trends: DashboardTrendPoint[]
  activeRange: DashboardRange
}>()

const emit = defineEmits<{
  rangeChange: [range: DashboardRange]
}>()

const tabs: Array<{ label: string; key: MetricKey; suffix: string }> = [
  { label: 'CPU 使用率', key: 'avgCpuUsage', suffix: '%' },
  { label: '内存使用率', key: 'avgMemoryUsage', suffix: '%' },
  { label: '磁盘使用率', key: 'avgDiskUsage', suffix: '%' },
  { label: '系统负载', key: 'avgLoad1', suffix: '' },
]

const ranges: Array<{ label: string; key: DashboardRange }> = [
  { label: '1h', key: '1h' },
  { label: '6h', key: '6h' },
  { label: '1天', key: '24h' },
  { label: '7天', key: '7d' },
]

const activeTab = ref<MetricKey>('avgCpuUsage')
const chartRef = ref<HTMLDivElement | null>(null)
let chart: ECharts | null = null

const rangeTrends = computed(() => props.trends)
const normalizedTrends = computed(() => downsampleTrendPoints(rangeTrends.value, 120))
const activeTabConfig = computed(() => tabs.find((item) => item.key === activeTab.value) ?? tabs[0])
const chartLabels = computed(() => normalizedTrends.value.map((point) => formatShortTime(point.sampledAt)))
const chartValues = computed(() => normalizedTrends.value.map((point) => Number(point[activeTab.value] ?? 0)))
const rangeValues = computed(() => rangeTrends.value.map((point) => Number(point[activeTab.value] ?? 0)))
const usingFallbackSnapshot = computed(() => rangeTrends.value.length > 0 && rangeTrends.value.every((point) => point.fallback))
const summaryItems = computed(() => {
  const points = rangeValues.value
  if (!points.length) {
    return []
  }

  const latest = points[points.length - 1] ?? 0
  const peak = Math.max(...points)
  const average = points.reduce((sum, value) => sum + value, 0) / points.length
  if (usingFallbackSnapshot.value) {
    const latestPoint = rangeTrends.value[rangeTrends.value.length - 1]
    return [
      { label: '最近快照', value: formatMetricValue(latest) },
      { label: '样本数', value: String(latestPoint?.sampleCount ?? 0) },
      { label: '采样时间', value: formatShortTime(latestPoint?.sampledAt ?? '') },
    ]
  }
  return [
    { label: '当前', value: formatMetricValue(latest) },
    { label: '峰值', value: formatMetricValue(peak) },
    { label: '均值', value: formatMetricValue(average) },
  ]
})
const currentValue = computed(() => {
  const last = rangeValues.value[rangeValues.value.length - 1]
  if (last == null) return '暂无数据'
  return formatMetricValue(last)
})

function renderChart() {
  if (!chartRef.value) return
  chart ??= init(chartRef.value)

  chart.setOption<ECOption>({
    animation: false,
    grid: { left: 30, right: 6, top: 12, bottom: 12 },
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(10, 19, 33, 0.96)',
      borderColor: 'rgba(93, 120, 162, 0.22)',
      textStyle: { color: '#f3f7ff' },
      valueFormatter: (value) => {
        if (typeof value !== 'number') return String(value)
        return activeTabConfig.value.suffix ? `${Math.round(value)}${activeTabConfig.value.suffix}` : value.toFixed(2)
      },
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: chartLabels.value,
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { color: '#aab8cd', margin: 12 },
    },
    yAxis: {
      type: 'value',
      min: 0,
      splitLine: {
        show: true,
        lineStyle: { color: 'rgba(93, 120, 162, 0.14)' },
      },
      axisLabel: {
        color: '#aab8cd',
        formatter: activeTabConfig.value.suffix ? `{value}${activeTabConfig.value.suffix}` : '{value}',
      },
    },
    series: [
      {
        type: 'line',
        smooth: true,
        showSymbol: false,
        data: chartValues.value,
        lineStyle: { width: 2, color: '#4f83ff' },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 0,
            y2: 1,
            colorStops: [
              { offset: 0, color: 'rgba(79, 131, 255, 0.34)' },
              { offset: 1, color: 'rgba(79, 131, 255, 0)' },
            ],
          },
        },
      },
    ],
  })
}

function formatMetricValue(value: number) {
  return activeTabConfig.value.suffix ? `${Math.round(value)}${activeTabConfig.value.suffix}` : value.toFixed(2)
}

function formatShortTime(value: string) {
  if (!value) return '--:--'
  const date = new Date(value)
  const options: Intl.DateTimeFormatOptions = props.activeRange === '7d'
    ? { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }
    : { hour: '2-digit', minute: '2-digit', hour12: false }
  return new Intl.DateTimeFormat('zh-CN', options).format(date)
}

function downsampleTrendPoints(points: DashboardTrendPoint[], maxPoints: number) {
  if (points.length <= maxPoints) {
    return points
  }

  const result: DashboardTrendPoint[] = []
  const lastIndex = points.length - 1
  for (let index = 0; index < maxPoints; index += 1) {
    const sourceIndex = Math.round((index / (maxPoints - 1)) * lastIndex)
    result.push(points[sourceIndex])
  }
  return result
}

watch([normalizedTrends, activeTab, () => props.activeRange], async () => {
  await nextTick()
  renderChart()
}, { deep: true })

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
  <section class="console-panel graph-panel">
    <div class="panel-header">
      <div>
        <div class="panel-title">资源探索</div>
        <p v-if="usingFallbackSnapshot">暂无所选时间范围趋势，当前展示最近一次成功资源快照。</p>
      </div>
      <div class="current-value">{{ currentValue }}</div>
    </div>

    <div class="tabs-container">
      <div class="metric-tabs" aria-label="资源指标">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          type="button"
          class="tab-button"
          :class="{ active: activeTab === tab.key }"
          @click="activeTab = tab.key"
        >
          {{ tab.label }}
        </button>
      </div>
      <div class="range-tabs" aria-label="资源时间范围">
        <button
          v-for="range in ranges"
          :key="range.key"
          type="button"
          class="range-button"
          :class="{ active: activeRange === range.key }"
          @click="emit('rangeChange', range.key)"
        >
          {{ range.label }}
        </button>
      </div>
    </div>

    <template v-if="chartValues.length">
      <div ref="chartRef" class="graph-host" />
      <div class="graph-summary">
        <div v-for="item in summaryItems" :key="item.label" class="summary-chip">
          <span>{{ item.label }}</span>
          <strong>{{ item.value }}</strong>
        </div>
      </div>
    </template>
    <div v-else class="graph-empty">
      <strong>暂无监控趋势</strong>
      <span>当前没有资源采样点可供展示，请等待节点继续上报。</span>
    </div>
  </section>
</template>

<style scoped>
.console-panel {
  position: relative;
  height: 100%;
  padding: 14px 18px;
  overflow: hidden;
  border-radius: 8px;
  border: 1px solid rgba(74, 99, 139, 0.24);
  background: linear-gradient(180deg, rgba(15, 27, 46, 0.72), rgba(11, 20, 34, 0.82));
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.05), 0 14px 30px rgba(0, 8, 22, 0.22);
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 10px;
}

.panel-title {
  color: var(--app-text);
  font-size: 16px;
  font-weight: 700;
}

.panel-header p {
  margin: 6px 0 0;
  color: var(--app-text-soft);
  font-size: 12px;
}

.current-value {
  color: var(--app-accent);
  font-size: 16px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.tabs-container {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}

.metric-tabs,
.range-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  min-width: 0;
}

.range-tabs {
  justify-content: flex-end;
}

.tab-button,
.range-button {
  background: rgba(20, 35, 58, 0.6);
  border: 1px solid rgba(78, 103, 142, 0.3);
  color: var(--app-text-soft);
  padding: 4px 10px;
  font-size: 12px;
  cursor: pointer;
  border-radius: 6px;
  transition: all 0.2s;
}

.range-button {
  min-width: 42px;
}

.tab-button:hover,
.range-button:hover {
  color: var(--app-text);
  border-color: rgba(72, 132, 255, 0.42);
}

.tab-button.active,
.range-button.active {
  background: rgba(72, 132, 255, 0.2);
  color: #62a8ff;
  border-color: rgba(72, 132, 255, 0.5);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.08);
}

.graph-host {
  width: 100%;
  height: 138px;
}

.graph-summary {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
  margin-top: 8px;
}

.summary-chip {
  display: grid;
  gap: 2px;
  padding: 8px 10px;
  border-radius: 8px;
  background: rgba(17, 32, 52, 0.5);
  border: 1px solid rgba(74, 99, 139, 0.22);
}

.summary-chip span {
  color: var(--app-text-soft);
  font-size: 11px;
}

.summary-chip strong {
  color: var(--app-text);
  font-size: 13px;
  font-variant-numeric: tabular-nums;
}

.graph-empty {
  display: grid;
  gap: 8px;
  min-height: 156px;
  align-content: center;
  color: var(--app-text-soft);
}

.graph-empty strong {
  color: var(--app-text);
  font-size: 15px;
}

@media (max-width: 720px) {
  .tabs-container {
    align-items: stretch;
    flex-direction: column;
  }

  .range-tabs {
    justify-content: flex-start;
  }
}
</style>
