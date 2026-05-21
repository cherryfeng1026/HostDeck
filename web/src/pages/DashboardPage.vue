<script setup lang="ts">
import { NScrollbar, useMessage } from 'naive-ui'
import { computed, onMounted, ref } from 'vue'
import ConsoleKpiCard from '../components/ConsoleKpiCard.vue'
import DashboardWelcome from '../components/DashboardWelcome.vue'
import InstanceDetailPanel from '../components/InstanceDetailPanel.vue'
import QuickActionsGrid from '../components/QuickActionsGrid.vue'
import RecentActivityTimeline from '../components/RecentActivityTimeline.vue'
import ResourceExplorerGraph from '../components/ResourceExplorerGraph.vue'
import ServerModal from '../components/ServerModal.vue'
import { useAutoRefresh } from '../auto-refresh'
import { loadDashboard, useDashboardCache } from '../dashboard-cache'
import { useSession } from '../session'
import type { DashboardRange, ServerAsset } from '../types'

const message = useMessage()
const { canManageInfrastructure, currentUser } = useSession()

const showServerModal = ref(false)
const editingServer = ref<ServerAsset | null>(null)
const dashboardCache = useDashboardCache()
const activeDashboardRange = ref<DashboardRange>('24h')

const dashboard = computed(() => dashboardCache.dashboard)
const servers = computed(() => dashboardCache.servers)
const activityItems = computed(() => dashboardCache.activityItems)
const displayName = computed(() => currentUser.value?.username || 'admin')
const cpuTrendValues = computed(() => dashboard.value.trends.map((point) => Math.round(point.avgCpuUsage)))

const focusedServer = computed(() => {
  const topServerID = dashboard.value.topServers[0]?.id
  if (topServerID) {
    return servers.value.find((item) => item.id === topServerID) ?? servers.value[0] ?? null
  }
  return servers.value[0] ?? null
})

const focusedRankReason = computed(() => dashboard.value.topServers[0]?.rankReason || '')

const kpiCards = computed(() => [
  {
    title: '运行实例',
    value: dashboard.value.headline.onlineServers,
    detail: `总计 ${dashboard.value.headline.totalServers} 台 · 离线 ${dashboard.value.headline.offlineServers} 台`,
    points: [] as number[],
    tone: 'success' as const,
  },
  {
    title: '上报节点',
    value: dashboard.value.resourceSummary.reportingServers,
    detail: `异常 ${dashboard.value.resourceSummary.unhealthyServers} 台`,
    points: [] as number[],
    tone: 'default' as const,
  },
  {
    title: '平均 CPU 使用率',
    value: `${Math.round(dashboard.value.resourceSummary.avgCpuUsage)}%`,
    detail: `峰值 ${Math.round(dashboard.value.resourceSummary.peakCpuUsage)}%`,
    points: cpuTrendValues.value,
    tone: 'default' as const,
  },
  {
    title: '告警数量',
    value: dashboard.value.alertSummary.total,
    detail: `Critical ${dashboard.value.alertSummary.critical} · Warning ${dashboard.value.alertSummary.warning}`,
    points: [] as number[],
    tone: dashboard.value.alertSummary.total > 0 ? ('danger' as const) : ('warning' as const),
  },
])

async function loadData(force = false, silent = false) {
  try {
    await loadDashboard({ force, silent, range: activeDashboardRange.value })
  } catch (error) {
    if (!silent) {
      message.error(error instanceof Error ? error.message : '加载总览数据失败')
    }
  }
}

function handleResourceRangeChange(range: DashboardRange) {
  if (activeDashboardRange.value === range) {
    return
  }
  activeDashboardRange.value = range
  void loadData(true)
}

function openCreateModal() {
  editingServer.value = null
  showServerModal.value = true
}

function handleSubmitted() {
  editingServer.value = null
  void loadData(true)
}

useAutoRefresh(() => loadData(false, true), 30000)

onMounted(() => {
  void loadData(false, dashboardCache.initialized)
})
</script>

<template>
  <n-scrollbar class="content-scroll">
    <div class="dashboard-shell">
      <section class="hero-panel">
        <DashboardWelcome
          :display-name="displayName"
          :last-updated-at="dashboard.headline.lastUpdatedAt"
          :online-servers="dashboard.headline.onlineServers"
          :total-servers="dashboard.headline.totalServers"
          :active-alerts="dashboard.headline.activeAlerts"
          :ssh-failures="dashboard.headline.sshFailures"
          :can-manage-infrastructure="canManageInfrastructure"
          @refresh="() => loadData(true)"
          @create="openCreateModal"
        />

        <section class="kpi-grid">
          <ConsoleKpiCard
            v-for="card in kpiCards"
            :key="card.title"
            :title="card.title"
            :value="card.value"
            :detail="card.detail"
            :points="card.points"
            :tone="card.tone"
          />
        </section>
      </section>

      <section class="content-grid">
        <div class="content-card content-card--wide">
          <InstanceDetailPanel :server="focusedServer" :rank-reason="focusedRankReason" />
        </div>
        <div class="content-card">
          <ResourceExplorerGraph
            :trends="dashboard.trends"
            :active-range="activeDashboardRange"
            @range-change="handleResourceRangeChange"
          />
        </div>
        <div class="content-card">
          <QuickActionsGrid />
        </div>
        <div class="content-card">
          <RecentActivityTimeline :items="activityItems" />
        </div>
      </section>
    </div>

    <ServerModal
      v-if="canManageInfrastructure"
      v-model:show="showServerModal"
      :server="editingServer"
      @submitted="handleSubmitted"
    />
  </n-scrollbar>
</template>

<style scoped>
.content-scroll {
  flex: 1;
}

.dashboard-shell {
  max-width: 100%;
  padding: 18px 20px 24px;
}

.hero-panel {
  position: relative;
  padding: 14px;
  border-radius: 8px;
  background: rgba(10, 19, 33, 0.66);
  border: 1px solid rgba(93, 120, 162, 0.2);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
}

.kpi-grid {
  position: relative;
  z-index: 1;
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
  margin-top: 12px;
}

.content-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
  margin-top: 14px;
  align-items: stretch;
}

.content-card {
  min-width: 0;
  min-height: 100%;
  display: flex;
}

.content-card--wide {
  grid-column: span 1;
}

.content-card :deep(.console-panel) {
  width: 100%;
  height: 100%;
}

@media (max-width: 1280px) {
  .kpi-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 1024px) {
  .content-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 860px) {
  .dashboard-shell {
    padding: 22px 18px 28px;
  }

  .kpi-grid {
    grid-template-columns: 1fr;
  }
}
</style>
