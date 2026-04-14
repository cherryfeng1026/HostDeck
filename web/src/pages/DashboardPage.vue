<script setup lang="ts">
import { NButton, NEmpty, NScrollbar, NTag, useMessage } from 'naive-ui'
import { onMounted, ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import ServerModal from '../components/ServerModal.vue'
import ServerStatusTable from '../components/ServerStatusTable.vue'
import { getAlerts, getOverview, getServerListWithStatus, probeServer, testServerSSH } from '../services/api'
import { useSession } from '../session'
import type { AlertEvent, OverviewResponse, ServerAsset, ServerListItem } from '../types'

const message = useMessage()
const router = useRouter()
const { canManageInfrastructure, currentUser } = useSession()

const loading = ref(false)
const showServerModal = ref(false)
const editingServer = ref<ServerAsset | null>(null)
const stats = ref<OverviewResponse>({
  totalServers: 0,
  onlineServers: 0,
  offlineServers: 0,
  activeAlerts: 0,
  sshFailures: 0,
})
const servers = ref<ServerListItem[]>([])
const alerts = ref<AlertEvent[]>([])

const displayName = computed(() => currentUser.value?.username || 'Admin')

async function loadData() {
  loading.value = true
  try {
    const [overview, items, alertList] = await Promise.all([
      getOverview(),
      getServerListWithStatus(),
      getAlerts(),
    ])
    stats.value = overview
    servers.value = items
    alerts.value = alertList
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载总览数据失败')
  } finally {
    loading.value = false
  }
}

async function handleProbe(serverId: number) {
  try {
    await probeServer(serverId)
    message.success('服务器采集请求已发送')
    await loadData()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '服务器采集失败')
  }
}

async function handleTestSSH(serverId: number) {
  try {
    const result = await testServerSSH(serverId)
    message[result.sshOk ? 'success' : 'warning'](
      result.sshOk ? `SSH 连接正常，耗时 ${result.latencyMs} ms` : result.error || 'SSH 连接异常',
    )
  } catch (error) {
    message.error(error instanceof Error ? error.message : 'SSH 测试失败')
  }
}

function openCreateModal() {
  editingServer.value = null
  showServerModal.value = true
}

function openEditModal(server: ServerListItem) {
  editingServer.value = server
  showServerModal.value = true
}

function handleSubmitted() {
  editingServer.value = null
  void loadData()
}

function goToServer(serverId: number) {
  void router.push(`/servers/${serverId}`)
}

function formatTriggeredAt(value: string) {
  if (!value) return '未知时间'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

onMounted(() => {
  void loadData()
})
</script>

<template>
  <n-scrollbar class="content-scroll">
    <div class="dashboard-content">
      <!-- 欢迎横幅 -->
      <div class="welcome-banner">
        <div class="banner-text">
          <h1>欢迎回来，{{ displayName }}。</h1>
          <p>以下是您的服务器集群当前实时运行状态及告警信息。</p>
        </div>
        <div class="banner-actions">
          <n-button ghost @click="loadData" style="margin-right: 12px; color: #a1a1aa; border-color: rgba(255,255,255,0.1)">
            刷新数据
          </n-button>
          <n-button v-if="canManageInfrastructure" type="primary" size="large" class="glow-btn" @click="openCreateModal">
            + 部署新服务器
          </n-button>
        </div>
      </div>

      <!-- 数据指标区 (Bento Grid) -->
      <div class="bento-grid">
        <div class="bento-card stat-card total">
          <div class="stat-header">纳管服务器</div>
          <div class="stat-value">{{ stats.totalServers }}</div>
          <div class="stat-chart spark-line-1"></div>
        </div>

        <div class="bento-card stat-card online">
          <div class="stat-header">
            在线节点 <div class="status-dot green pulsing" v-if="stats.onlineServers > 0"></div>
          </div>
          <div class="stat-value text-green">{{ stats.onlineServers }}</div>
          <div class="stat-desc" v-if="stats.totalServers > 0">
            可用率 {{ Math.round((stats.onlineServers / stats.totalServers) * 100) }}%
          </div>
        </div>

        <div class="bento-card stat-card alerts">
          <div class="stat-header">活动告警</div>
          <div class="stat-value text-amber">{{ stats.activeAlerts }}</div>
          <n-button size="small" ghost type="warning" class="mt-2" @click="router.push('/alerts')">
            进入告警中心
          </n-button>
        </div>

        <div class="bento-card stat-card ssh">
          <div class="stat-header">SSH 失败状态</div>
          <div class="stat-value text-rose">{{ stats.sshFailures }}</div>
          <div class="stat-desc">请留意连接受阻的节点</div>
        </div>

        <!-- 实时告警追踪 (跨 2 列) -->
        <div class="bento-card span-2 alerts-card">
          <div class="card-title-bar">
            <span class="card-title">实时活动告警</span>
            <n-button text size="small" style="color: #a1a1aa;" @click="router.push('/alerts')">查看全部</n-button>
          </div>
          <div class="alerts-body">
            <n-empty v-if="!alerts.length && !loading" description="当前没有活动告警" class="empty-state" />
            <div v-else class="alert-timeline">
              <div v-for="alert in alerts.slice(0, 4)" :key="`${alert.ruleId}-${alert.serverId}`" class="timeline-item">
                <div class="timeline-dot" :class="alert.severity"></div>
                <div class="timeline-content">
                  <div class="timeline-head">
                    <span class="server-name" @click="goToServer(alert.serverId)">{{ alert.serverName }}</span>
                    <span class="alert-time">{{ formatTriggeredAt(alert.triggeredAt) }}</span>
                  </div>
                  <div class="alert-msg">{{ alert.message }}</div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 资产快捷展示 (跨 2 列) -->
        <div class="bento-card span-2 table-card">
          <div class="card-title-bar">
            <span class="card-title">监控节点概览</span>
            <n-button text size="small" style="color: #a1a1aa;" @click="router.push('/servers')">完整管理列表</n-button>
          </div>
          <div class="table-wrapper">
            <ServerStatusTable
              :items="servers.slice(0, 5)"
              :loading="loading"
              :can-manage="canManageInfrastructure"
              @probe="handleProbe"
              @test-ssh="handleTestSSH"
              @edit="openEditModal"
            />
          </div>
        </div>

      </div>
    </div>

    <!-- 服务器编辑弹窗 -->
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

.dashboard-content {
  padding: 32px;
  max-width: 1600px;
  margin: 0 auto;
}

.welcome-banner {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  margin-bottom: 32px;
}

.banner-text h1 {
  font-size: 32px;
  font-weight: 600;
  margin: 0 0 8px 0;
  color: #fff;
  letter-spacing: -0.5px;
}

.banner-text p {
  margin: 0;
  color: #a1a1aa;
  font-size: 16px;
}

.glow-btn {
  box-shadow: 0 0 20px rgba(16, 185, 129, 0.4);
  transition: all 0.3s ease;
  background-color: #10b981 !important;
  color: #fff !important;
  border: none;
}
.glow-btn:hover {
  box-shadow: 0 0 30px rgba(16, 185, 129, 0.6);
  transform: translateY(-1px);
}

/* Bento Grid */
.bento-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  grid-auto-rows: minmax(140px, auto);
  gap: 24px;
}

.bento-card {
  background: rgba(20, 20, 25, 0.6);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 20px;
  padding: 24px;
  position: relative;
  overflow: hidden;
  transition: border-color 0.2s;
  display: flex;
  flex-direction: column;
}

.bento-card:hover {
  border-color: rgba(255, 255, 255, 0.15);
}

.span-2 {
  grid-column: span 2;
}

.stat-header {
  font-size: 14px;
  color: #a1a1aa;
  font-weight: 500;
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}

.stat-value {
  font-size: 42px;
  font-weight: 700;
  line-height: 1;
  color: #fff;
  font-feature-settings: "tnum";
}

.text-green { color: #10b981; }
.text-amber { color: #f59e0b; }
.text-rose { color: #f43f5e; }

.stat-desc {
  margin-top: 12px;
  font-size: 13px;
  color: #71717a;
}

.mt-2 { margin-top: 12px; }

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background-color: #10b981;
}

.status-dot.pulsing {
  box-shadow: 0 0 0 0 rgba(16, 185, 129, 0.7);
  animation: pulse-green 2s infinite;
}

@keyframes pulse-green {
  0% { transform: scale(0.95); box-shadow: 0 0 0 0 rgba(16, 185, 129, 0.7); }
  70% { transform: scale(1); box-shadow: 0 0 0 6px rgba(16, 185, 129, 0); }
  100% { transform: scale(0.95); box-shadow: 0 0 0 0 rgba(16, 185, 129, 0); }
}

/* 底部通用卡片标题 */
.card-title-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
  padding-bottom: 12px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.card-title {
  font-size: 16px;
  font-weight: 600;
  color: #fff;
}

/* 告警时间线 */
.alerts-card {
  min-height: 360px;
}

.alerts-body {
  flex: 1;
  overflow: hidden;
}

.alert-timeline {
  display: flex;
  flex-direction: column;
  gap: 16px;
  position: relative;
}
.alert-timeline::before {
  content: '';
  position: absolute;
  left: 5px;
  top: 8px;
  bottom: 8px;
  width: 2px;
  background: rgba(255, 255, 255, 0.05);
  z-index: 0;
}

.timeline-item {
  display: flex;
  gap: 16px;
  position: relative;
  z-index: 1;
}

.timeline-dot {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  margin-top: 4px;
  flex-shrink: 0;
  border: 2px solid #050505;
}
.timeline-dot.critical { background: #f43f5e; box-shadow: 0 0 8px rgba(244, 63, 94, 0.6); }
.timeline-dot.warning { background: #f59e0b; box-shadow: 0 0 8px rgba(245, 158, 11, 0.6); }

.timeline-content {
  flex: 1;
  background: rgba(0, 0, 0, 0.2);
  padding: 12px 16px;
  border-radius: 12px;
  border: 1px solid rgba(255, 255, 255, 0.05);
}

.timeline-head {
  display: flex;
  justify-content: space-between;
  margin-bottom: 4px;
}

.server-name {
  font-weight: 600;
  color: #fff;
  cursor: pointer;
  transition: color 0.2s;
}
.server-name:hover {
  color: #10b981;
}

.alert-time {
  font-size: 12px;
  color: #71717a;
}

.alert-msg {
  font-size: 13px;
  color: #a1a1aa;
  line-height: 1.5;
}

/* 缩略表格 */
.table-card {
  min-height: 360px;
}

.table-wrapper {
  margin: 0 -12px;
}

/* Sparkline mock */
.spark-line-1 {
  height: 40px;
  margin-top: 16px;
  background: linear-gradient(90deg, rgba(16,185,129,0) 0%, rgba(16,185,129,0.1) 50%, rgba(16,185,129,0) 100%);
  position: relative;
  overflow: hidden;
}

.spark-line-1::after {
  content: '';
  position: absolute;
  top: 50%;
  left: 0;
  right: 0;
  height: 2px;
  background: #10b981;
  box-shadow: 0 0 10px #10b981;
  transform: translateY(-50%);
  clip-path: polygon(0 50%, 20% 40%, 40% 60%, 60% 30%, 80% 50%, 100% 20%, 100% 100%, 0 100%);
}

@media (max-width: 1024px) {
  .bento-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 640px) {
  .welcome-banner {
    flex-direction: column;
    align-items: flex-start;
    gap: 16px;
  }
  .bento-grid {
    grid-template-columns: 1fr;
  }
  .span-2 {
    grid-column: span 1;
  }
}
</style>
