<script setup lang="ts">
import { computed } from 'vue'
import { NCard, NGrid, NGridItem } from 'naive-ui'
import type { OverviewResponse } from '../types'

const props = defineProps<{
  stats: OverviewResponse
}>()

const cards = computed(() => [
  { label: '纳管服务器', value: props.stats.totalServers, accent: 'sea' },
  { label: '在线服务器', value: props.stats.onlineServers, accent: 'mint' },
  { label: '离线服务器', value: props.stats.offlineServers, accent: 'clay' },
  { label: '活动告警', value: props.stats.activeAlerts, accent: 'rose' },
  { label: 'SSH 异常', value: props.stats.sshFailures, accent: 'amber' },
])
</script>

<template>
  <n-grid cols="1 s:2 l:5" responsive="screen" :x-gap="16" :y-gap="16">
    <n-grid-item v-for="card in cards" :key="card.label">
      <n-card class="summary-card" :class="card.accent" size="small" :bordered="false">
        <span class="summary-label">{{ card.label }}</span>
        <strong class="summary-value">{{ card.value }}</strong>
      </n-card>
    </n-grid-item>
  </n-grid>
</template>

<style scoped>
.summary-card {
  min-height: 112px;
  border-radius: var(--app-radius-lg);
  border: 1px solid var(--app-border);
  background: var(--app-surface-muted);
  box-shadow: var(--app-shadow-soft);
}

.summary-card :deep(.n-card__content) {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  min-height: 112px;
  padding: 16px 18px;
}

.summary-card.sea {
  border-top: 2px solid #38bdf8;
}

.summary-card.mint {
  border-top: 2px solid #34d399;
}

.summary-card.clay {
  border-top: 2px solid #f59e0b;
}

.summary-card.rose {
  border-top: 2px solid #fb7185;
}

.summary-card.amber {
  border-top: 2px solid #fbbf24;
}

.summary-label {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--app-text-soft);
}

.summary-value {
  font-size: clamp(28px, 3vw, 36px);
  line-height: 1;
  letter-spacing: -0.03em;
  color: #f8fafc;
}
</style>
