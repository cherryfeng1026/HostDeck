<script setup lang="ts">
import { LineChart, type LineSeriesOption } from 'echarts/charts'
import { GridComponent, TooltipComponent, type GridComponentOption, type TooltipComponentOption } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { init, use, type ComposeOption, type ECharts } from 'echarts/core'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

use([LineChart, GridComponent, TooltipComponent, CanvasRenderer])

type ECOption = ComposeOption<GridComponentOption | TooltipComponentOption | LineSeriesOption>

const props = defineProps<{
  title: string
  value: string | number
  detail: string
  points: number[]
  tone?: 'default' | 'success' | 'warning' | 'danger'
}>()

const chartRef = ref<HTMLDivElement | null>(null)
let chart: ECharts | null = null

const hasSparkline = computed(() => props.points.length > 1)

const iconMarkup = computed(() => {
  if (props.title.includes('运行实例')) {
    return '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="5" width="16" height="4" rx="1.2"/><rect x="4" y="15" width="16" height="4" rx="1.2"/><path d="M7 7h.01M7 17h.01M17 9v6"/></svg>'
  }
  if (props.title.includes('上报节点')) {
    return '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"><path d="M4 18h16"/><rect x="5" y="4" width="14" height="11" rx="1.5"/><path d="M8 11h2l1.3-3.8 2.1 6 1.1-2.2H17"/></svg>'
  }
  if (props.title.includes('CPU')) {
    return '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"><rect x="7" y="7" width="10" height="10" rx="1.6"/><path d="M10 3v3M14 3v3M10 18v3M14 18v3M3 10h3M3 14h3M18 10h3M18 14h3"/></svg>'
  }
  return '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"><path d="M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M10 20h4"/></svg>'
})

const palette = computed(() => {
  switch (props.tone) {
    case 'success':
      return { line: '#20d4ff', area: 'rgba(32, 212, 255, 0.18)' }
    case 'warning':
      return { line: '#e8b45f', area: 'rgba(232, 180, 95, 0.18)' }
    case 'danger':
      return { line: '#ff6b7d', area: 'rgba(255, 107, 125, 0.18)' }
    default:
      return { line: '#4f83ff', area: 'rgba(79, 131, 255, 0.18)' }
  }
})

const stateLabel = computed(() => {
  if (props.title.includes('运行实例')) return '在线'
  if (props.title.includes('上报节点')) return '采集'
  if (props.title.includes('告警')) return props.tone === 'danger' ? '待处理' : '稳定'
  return '实时'
})

function disposeChart() {
  chart?.dispose()
  chart = null
}

function renderChart() {
  if (!hasSparkline.value) {
    disposeChart()
    return
  }
  if (!chartRef.value) return

  const values = props.points
  chart ??= init(chartRef.value)
  chart.setOption<ECOption>({
    animation: false,
    grid: { left: 0, right: 0, top: 8, bottom: 0 },
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(10, 19, 33, 0.96)',
      borderColor: 'rgba(93, 120, 162, 0.22)',
      textStyle: { color: '#f3f7ff' },
    },
    xAxis: {
      type: 'category',
      show: false,
      boundaryGap: false,
      data: values.map((_, index) => index),
    },
    yAxis: {
      type: 'value',
      show: false,
      min: 'dataMin',
      max: 'dataMax',
    },
    series: [
      {
        type: 'line',
        smooth: true,
        showSymbol: false,
        data: values,
        lineStyle: { width: 2.5, color: palette.value.line },
        areaStyle: { color: palette.value.area },
      },
    ],
  })
}

watch(
  [() => props.points, hasSparkline],
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
  disposeChart()
})

function handleResize() {
  chart?.resize()
}
</script>

<template>
  <article class="kpi-card" :class="tone ?? 'default'">
    <div class="kpi-icon" aria-hidden="true" v-html="iconMarkup" />
    <div class="kpi-title">{{ title }}</div>
    <div class="kpi-value">
      <span class="value-text">{{ typeof value === 'string' ? value.split(' ')[0] : value }}</span>
      <span v-if="typeof value === 'string' && value.includes(' ')" class="value-unit">{{ value.substring(value.indexOf(' ')) }}</span>
    </div>
    <div class="kpi-detail">{{ detail }}</div>
    <div v-if="hasSparkline" ref="chartRef" class="sparkline" aria-hidden="true" />
    <div v-else class="kpi-state" aria-hidden="true">
      <span class="state-dot" />
      <span>{{ stateLabel }}</span>
    </div>
  </article>
</template>

<style scoped>
.kpi-card {
  position: relative;
  overflow: hidden;
  min-height: 78px;
  padding: 10px 14px;
  border-radius: 8px;
  border: 1px solid rgba(93, 120, 162, 0.2);
  background: rgba(15, 27, 46, 0.9);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
}

.kpi-card::before {
  content: '';
  position: absolute;
  left: 0;
  top: 14px;
  bottom: 14px;
  width: 3px;
  border-radius: 0 999px 999px 0;
  background: currentColor;
  opacity: 0.68;
}

.kpi-card.success {
  background: rgba(14, 39, 58, 0.72);
  color: #62a8ff;
}

.kpi-card.warning {
  background: rgba(49, 37, 21, 0.7);
  color: var(--app-warning);
}

.kpi-card.danger {
  background: rgba(50, 22, 35, 0.72);
  color: var(--app-danger);
}

.kpi-card.default {
  color: #62a8ff;
}

.kpi-icon {
  position: absolute;
  top: 18px;
  right: 17px;
  width: 34px;
  height: 34px;
  color: currentColor;
  opacity: 0.34;
  filter: drop-shadow(0 0 8px currentColor);
}

.kpi-icon :deep(svg) {
  width: 100%;
  height: 100%;
  display: block;
}

.kpi-title {
  color: var(--app-text-soft);
  font-size: 11px;
}

.kpi-value {
  margin-top: 3px;
  color: var(--app-text);
  line-height: 1;
  font-weight: 700;
  letter-spacing: 0;
  font-variant-numeric: tabular-nums;
  display: flex;
  align-items: baseline;
  gap: 4px;
}
.value-text {
  font-size: 22px;
}
.value-unit {
  font-size: 12px;
  color: var(--app-text-soft);
  font-weight: normal;
}

.kpi-detail {
  margin-top: 4px;
  color: var(--app-text-soft);
  font-size: 11px;
  line-height: 1.35;
}
.text-green { color: var(--app-accent); }
.text-red { color: var(--app-danger); }
.text-gray { color: var(--app-text-soft); }

.sparkline {
  position: absolute;
  right: 10px;
  bottom: 10px;
  width: 82px;
  height: 30px;
  opacity: 0.86;
}

.kpi-state {
  position: absolute;
  right: 12px;
  bottom: 10px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 22px;
  padding: 0 8px;
  border-radius: 8px;
  border: 1px solid rgba(93, 120, 162, 0.18);
  background: rgba(7, 16, 29, 0.28);
  color: currentColor;
  font-size: 11px;
  font-weight: 600;
}

.state-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
  box-shadow: 0 0 8px currentColor;
}
</style>
