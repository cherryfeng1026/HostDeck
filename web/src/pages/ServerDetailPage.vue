<script setup lang="ts">
import {
  NButton,
  NScrollbar,
  NSpin,
  NTag,
  useMessage,
} from 'naive-ui'
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import StatusTrendCard from '../components/StatusTrendCard.vue'
import { getServerMetrics, getServerStatus, probeServer, testServerSSH } from '../services/api'
import { useSession } from '../session'
import type { MetricPoint, ServerStatusDetail } from '../types'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const { canManageInfrastructure } = useSession()
const loading = ref(false)
const status = ref<ServerStatusDetail | null>(null)
const metrics = ref<MetricPoint[]>([])

const serverId = computed(() => Number(route.params.id))

async function loadData() {
  if (!Number.isFinite(serverId.value)) {
    status.value = null
    metrics.value = []
    return
  }

  loading.value = true
  try {
    const [statusResult, metricsResult] = await Promise.all([
      getServerStatus(serverId.value),
      getServerMetrics(serverId.value, '24h'),
    ])
    status.value = statusResult
    metrics.value = metricsResult.points
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载服务器详情失败')
    status.value = null
    metrics.value = []
  } finally {
    loading.value = false
  }
}

async function handleProbe() {
  if (!status.value) return
  try {
    await probeServer(status.value.id)
    message.success('服务器采集请求已发送')
    await loadData()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '服务器采集失败')
  }
}

async function handleTestSSH() {
  if (!status.value) return
  try {
    const result = await testServerSSH(status.value.id)
    message[result.sshOk ? 'success' : 'warning'](
      result.sshOk ? `SSH 连接正常，耗时 ${result.latencyMs} ms` : result.error || 'SSH 连接异常',
    )
  } catch (error) {
    message.error(error instanceof Error ? error.message : 'SSH 测试失败')
  }
}

function goToCommands() {
  if (!status.value) return
  void router.push({
    path: '/commands',
    query: { serverId: String(status.value.id) },
  })
}

function formatLastReportAt(value: string | undefined) {
  if (!value) return '尚未采集'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

function formatUptime(seconds: number | undefined) {
  if (!seconds) return '尚未采集'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const parts = []
  if (days) parts.push(`${days} 天`)
  if (hours || days) parts.push(`${hours} 小时`)
  parts.push(`${minutes} 分钟`)
  return parts.join(' ')
}

function getTone(value: number) {
  if (value >= 80) return '#f43f5e'
  if (value >= 60) return '#f59e0b'
  return '#10b981'
}

watch(
  () => route.params.id,
  () => { void loadData() },
)

onMounted(() => {
  void loadData()
})
</script>

<template>
  <n-scrollbar class="content-scroll">
    <div class="page-container">
      <n-spin :show="loading">
        <!-- 头部区域 -->
        <div class="page-header">
          <div class="header-back">
            <n-button text style="color: #a1a1aa; font-size: 14px;" @click="router.push('/servers')">
              ← 返回服务器列表
            </n-button>
          </div>
          <div class="header-main" v-if="status">
            <div class="header-text">
              <div class="title-row">
                <h1>{{ status.name }}</h1>
                <n-tag :type="status.online ? 'success' : 'error'" size="small" bordered>
                  <div class="status-dot-inline" :style="{ backgroundColor: status.online ? '#10b981' : '#f43f5e' }"></div>
                  {{ status.online ? '在线' : '离线' }}
                </n-tag>
                <n-tag :type="status.sshOk ? 'success' : 'warning'" size="small" bordered>
                  {{ status.sshOk ? 'SSH 正常' : 'SSH 异常' }}
                </n-tag>
                <n-tag v-if="!status.enabled" size="small" type="default" bordered>
                  已禁用
                </n-tag>
              </div>
              <p>{{ status.ip }}:{{ status.sshPort }} • {{ status.osVersion || '未知系统' }}</p>
            </div>
            <div class="header-actions">
              <template v-if="canManageInfrastructure">
                <n-button ghost :disabled="!status.enabled" @click="handleProbe" style="border-color: rgba(255,255,255,0.1)">立即采集</n-button>
                <n-button ghost type="warning" :disabled="!status.enabled" @click="handleTestSSH">测试 SSH</n-button>
                <n-button type="primary" class="glow-btn" :disabled="!status.enabled" @click="goToCommands">前往命令终端</n-button>
              </template>
              <n-tag v-else type="default" style="color: #a1a1aa; background: rgba(255,255,255,0.05)">只读权限</n-tag>
            </div>
          </div>
          <div v-else class="header-main" style="min-height: 80px; display: flex; align-items: center;">
            <p style="color: #a1a1aa;">未找到服务器信息</p>
          </div>
        </div>

        <div v-if="status" class="bento-grid">
          
          <!-- System Info Card -->
          <div class="bento-card span-2 sys-info-card">
            <div class="card-title">系统信息</div>
            <div class="info-grid">
              <div class="info-item">
                <span class="info-label">主机名</span>
                <span class="info-val font-mono">{{ status.hostname || '-' }}</span>
              </div>
              <div class="info-item">
                <span class="info-label">登录用户</span>
                <span class="info-val">{{ status.username }}</span>
              </div>
              <div class="info-item">
                <span class="info-label">采集模式</span>
                <span class="info-val">{{ status.collectorMode }}</span>
              </div>
              <div class="info-item">
                <span class="info-label">内核版本</span>
                <span class="info-val">{{ status.kernelVersion || '尚未采集' }}</span>
              </div>
              <div class="info-item">
                <span class="info-label">持续运行</span>
                <span class="info-val">{{ formatUptime(status.uptimeSeconds) }}</span>
              </div>
              <div class="info-item">
                <span class="info-label">最近上报</span>
                <span class="info-val">{{ formatLastReportAt(status.lastReportAt) }}</span>
              </div>
            </div>
          </div>

          <!-- Resource Cards -->
          <div class="bento-card resource-card">
            <div class="resource-header">
              <span class="res-title">CPU 使用率</span>
            </div>
            <div class="res-percent">{{ Math.round(status.cpuUsage || 0) }}%</div>
            <div class="res-bar">
              <div class="res-fill" :style="{ width: `${status.cpuUsage}%`, background: getTone(status.cpuUsage || 0) }"></div>
            </div>
          </div>

          <div class="bento-card resource-card">
            <div class="resource-header">
              <span class="res-title">内存使用</span>
            </div>
            <div class="res-percent">{{ Math.round(status.memoryUsage || 0) }}%</div>
            <div class="res-bar">
              <div class="res-fill" :style="{ width: `${status.memoryUsage}%`, background: getTone(status.memoryUsage || 0) }"></div>
            </div>
          </div>
          
          <div class="bento-card resource-card">
            <div class="resource-header">
              <span class="res-title">磁盘使用</span>
            </div>
            <div class="res-percent">{{ Math.round(status.diskUsage || 0) }}%</div>
            <div class="res-bar">
              <div class="res-fill" :style="{ width: `${status.diskUsage}%`, background: getTone(status.diskUsage || 0) }"></div>
            </div>
          </div>

          <div class="bento-card resource-card">
            <div class="resource-header">
              <span class="res-title">系统负载 (1m/5m/15m)</span>
            </div>
            <div class="res-percent" style="font-size: 24px; margin-top: 4px;">
              {{ status.load1?.toFixed(2) }}
            </div>
            <div style="color: #a1a1aa; font-size: 13px; margin-top: 8px;">
              / {{ status.load5?.toFixed(2) }} / {{ status.load15?.toFixed(2) }}
            </div>
          </div>

          <!-- Metrics Chart Area -->
          <div class="bento-card span-4 chart-card" style="padding: 0;">
            <StatusTrendCard title="最近 24 小时资源趋势" :points="metrics" />
          </div>

        </div>
      </n-spin>
    </div>
  </n-scrollbar>
</template>

<style scoped>
.content-scroll {
  flex: 1;
}

.page-container {
  padding: 32px;
  max-width: 1600px;
  margin: 0 auto;
}

.header-back {
  margin-bottom: 16px;
}

.header-main {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  margin-bottom: 32px;
}

.title-row {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 8px;
}

.header-text h1 {
  font-size: 32px;
  font-weight: 600;
  margin: 0;
  color: #fff;
  letter-spacing: -0.5px;
}

.header-text p {
  margin: 0;
  color: #a1a1aa;
  font-size: 15px;
}

.status-dot-inline {
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  margin-right: 6px;
  box-shadow: 0 0 8px rgba(255, 255, 255, 0.2);
}

.header-actions {
  display: flex;
  gap: 12px;
}

.glow-btn {
  transition: all 0.3s ease;
  border: none;
}
.glow-btn:not(:disabled):not(.n-button--disabled) {
  box-shadow: 0 0 20px rgba(16, 185, 129, 0.4);
  background-color: #10b981 !important;
  color: #fff !important;
}
.glow-btn:not(:disabled):not(.n-button--disabled):hover {
  box-shadow: 0 0 30px rgba(16, 185, 129, 0.6);
  transform: translateY(-1px);
}

/* Bento Grid */
.bento-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 24px;
}

.bento-card {
  background: rgba(20, 20, 25, 0.6);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 20px;
  padding: 24px;
  position: relative;
  overflow: hidden;
}

.span-2 { grid-column: span 2; }
.span-4 { grid-column: span 4; }

.card-title {
  font-size: 16px;
  font-weight: 600;
  color: #fff;
  margin-bottom: 16px;
}

/* Sys Info */
.info-grid {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
  gap: 16px 24px;
}

.info-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.info-label {
  font-size: 12px;
  color: #71717a;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.info-val {
  font-size: 14px;
  color: #ededed;
  font-weight: 500;
}

.font-mono {
  font-family: 'Fira Code', monospace;
}

/* Resources */
.resource-card {
  display: flex;
  flex-direction: column;
}

.resource-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.res-title {
  color: #a1a1aa;
  font-size: 14px;
}

.res-percent {
  font-size: 36px;
  font-weight: 700;
  color: #fff;
  margin-bottom: 16px;
}

.res-bar {
  height: 6px;
  background: rgba(255,255,255,0.05);
  border-radius: 3px;
  overflow: hidden;
  margin-top: auto;
}

.res-fill {
  height: 100%;
  border-radius: 3px;
  transition: width 0.3s ease, background-color 0.3s ease;
}

/* Chart Area */
.chart-card {
  min-height: 300px;
}

@media (max-width: 1024px) {
  .bento-grid {
    grid-template-columns: repeat(2, 1fr);
  }
  .info-grid {
    grid-template-columns: 1fr 1fr;
  }
}
@media (max-width: 640px) {
  .header-main {
    flex-direction: column;
    align-items: flex-start;
    gap: 16px;
  }
  .header-actions {
    flex-wrap: wrap;
  }
  .bento-grid {
    grid-template-columns: 1fr;
  }
  .span-2, .span-4 {
    grid-column: span 1;
  }
  .info-grid {
    grid-template-columns: 1fr;
  }
}
</style>
