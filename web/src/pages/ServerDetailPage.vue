<script setup lang="ts">
import {
  NButton,
  NScrollbar,
  NSpin,
  NTag,
  useDialog,
  useMessage,
} from 'naive-ui'
import { computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import StatusTrendCard from '../components/StatusTrendCard.vue'
import { useAutoRefresh } from '../auto-refresh'
import { getServerDetailCache, loadServerDetail } from '../dashboard-cache'
import { probeServer, testServerSSH, trustServerHostKey } from '../services/api'
import { useSession } from '../session'
import type { TestSSHResponse } from '../types'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const dialog = useDialog()
const { canManageInfrastructure } = useSession()

const serverId = computed(() => Number(route.params.id))
const currentCache = computed(() => (Number.isFinite(serverId.value) ? getServerDetailCache(serverId.value) : null))
const loading = computed(() => currentCache.value?.loading ?? false)
const status = computed(() => currentCache.value?.status ?? null)
const metrics = computed(() => currentCache.value?.metrics ?? [])

const resourceCards = computed(() => {
  if (!status.value) return []
  return [
    { label: 'CPU', value: status.value.cpuUsage || 0, suffix: '%', icon: 'CPU' },
    { label: '内存', value: status.value.memoryUsage || 0, suffix: '%', icon: 'RAM' },
    { label: '磁盘', value: status.value.diskUsage || 0, suffix: '%', icon: 'IO' },
    { label: '负载', value: status.value.load1 || 0, suffix: '', icon: 'LD' },
  ]
})

const systemInfo = computed(() => {
  if (!status.value) return []
  return [
    { label: '主机名', value: status.value.hostname || '-' },
    { label: '登录用户', value: status.value.username },
    { label: '系统版本', value: status.value.osVersion || '尚未采集' },
    { label: '内核版本', value: status.value.kernelVersion || '尚未采集' },
    { label: '持续运行', value: formatUptime(status.value.uptimeSeconds) },
    { label: '最近上报', value: formatLastReportAt(status.value.lastReportAt) },
    { label: '采集模式', value: status.value.collectorMode },
    { label: '已信任指纹', value: status.value.trustedHostKeyFingerprint || '未保存' },
  ]
})

async function loadData(force = false, silent = false) {
  const currentServerId = serverId.value
  if (!Number.isFinite(currentServerId)) {
    return
  }

  try {
    await loadServerDetail(currentServerId, { force, silent })
  } catch (error) {
    if (!silent && currentServerId === serverId.value) {
      message.error(error instanceof Error ? error.message : '加载服务器详情失败')
    }
  }
}

async function handleProbe() {
  if (!status.value) return
  try {
    await probeServer(status.value.id)
    message.success('服务器采集请求已发送')
    await loadData(true)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '服务器采集失败')
  }
}

async function trustHostKeyAndRefresh(result: TestSSHResponse) {
  if (!status.value || !result.hostKeyFingerprint) return
  await trustServerHostKey(status.value.id, result.hostKeyFingerprint)
  try {
    await probeServer(status.value.id)
    message.success('已保存 SSH 主机指纹并完成一次采集')
  } catch (error) {
    message.success('已保存 SSH 主机指纹')
    if (error instanceof Error) {
      message.warning(`自动采集未完成：${error.message}`)
    }
  }
  await loadData(true)
}

function showSSHResult(result: TestSSHResponse) {
  if (!status.value) return
  const canTrustHostKey = Boolean(
    result.hostKeyFingerprint
      && (result.trustRequired || result.fingerprintMismatch || result.hostKeyFingerprint !== result.trustedHostKeyFingerprint),
  )
  if (canTrustHostKey) {
    dialog.warning({
      title: result.fingerprintMismatch ? 'SSH 指纹不匹配' : '发现 SSH 主机指纹',
      content: `${result.error || '请确认该主机指纹是否可信'}\n\n当前指纹：${result.hostKeyFingerprint}${result.trustedHostKeyFingerprint ? `\n已信任：${result.trustedHostKeyFingerprint}` : ''}`,
      positiveText: '信任并保存',
      negativeText: '取消',
      onPositiveClick: () => trustHostKeyAndRefresh(result),
    })
    return
  }
  if (result.sshOk) {
    message.success(`SSH 连接正常，耗时 ${result.latencyMs ?? 0} ms`)
    return
  }
  message.warning(result.error || 'SSH 连接异常')
}

async function handleTestSSH() {
  if (!status.value) return
  try {
    const result = await testServerSSH(status.value.id)
    showSSHResult(result)
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
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime()) || parsed.getUTCFullYear() <= 1) {
    return '尚未采集'
  }
  return parsed.toLocaleString('zh-CN', { hour12: false })
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

function formatResourceValue(value: number, suffix: string) {
  if (suffix) return `${Math.round(value)}${suffix}`
  return value.toFixed(2)
}

function getTone(value: number, suffix: string) {
  if (!suffix) return '#20d4ff'
  if (value >= 80) return '#ff6b7d'
  if (value >= 60) return '#e8b45f'
  return '#20d4ff'
}

watch(
  () => route.params.id,
  () => {
    void loadData(false, Boolean(currentCache.value?.initialized))
  },
)

useAutoRefresh(() => loadData(false, true), 15000)

onMounted(() => {
  void loadData(false, Boolean(currentCache.value?.initialized))
})
</script>

<template>
  <n-scrollbar class="content-scroll">
    <div class="page-container detail-page">
      <n-spin :show="loading">
        <div class="page-header detail-header">
          <div class="header-text">
            <n-button text class="back-link" @click="router.push('/servers')">← 返回服务器列表</n-button>
            <div v-if="status" class="title-row">
              <span class="title-status-dot" :class="{ online: status.online }" />
              <h1>{{ status.name }}</h1>
              <n-tag :type="status.online ? 'success' : 'error'" size="small" bordered>
                <span class="status-dot-inline" :class="{ online: status.online }"></span>
                {{ status.online ? '在线' : '离线' }}
              </n-tag>
              <n-tag v-if="!status.enabled" size="small" type="default" bordered>已禁用</n-tag>
            </div>
          </div>
          <div v-if="status" class="header-actions">
            <template v-if="canManageInfrastructure">
              <n-button ghost :disabled="!status.enabled" @click="handleProbe">立即采集</n-button>
              <n-button ghost :disabled="!status.enabled" @click="handleTestSSH">测试 SSH</n-button>
              <n-button type="primary" :disabled="!status.enabled" @click="goToCommands">命令终端</n-button>
            </template>
            <n-tag v-else type="default" bordered>只读</n-tag>
          </div>
        </div>

        <div v-if="status" class="detail-grid">
          <section class="summary-panel">
            <div class="summary-main">
              <div>
                <span class="summary-label">地址</span>
                <strong>{{ status.ip }}:{{ status.sshPort }}</strong>
              </div>
              <div>
                <span class="summary-label">系统</span>
                <strong>{{ status.osVersion || '未知系统' }}</strong>
              </div>
              <div>
                <span class="summary-label">上报</span>
                <strong>{{ formatLastReportAt(status.lastReportAt) }}</strong>
              </div>
            </div>
          </section>

          <section class="resource-strip">
            <article v-for="item in resourceCards" :key="item.label" class="resource-card">
              <span class="resource-icon" aria-hidden="true">{{ item.icon }}</span>
              <span>{{ item.label }}</span>
              <strong>{{ formatResourceValue(item.value, item.suffix) }}</strong>
              <div v-if="item.suffix" class="res-bar">
                <div class="res-fill" :style="{ width: `${Math.max(0, Math.min(100, item.value))}%`, background: getTone(item.value, item.suffix) }"></div>
              </div>
              <div v-else class="load-note">5m {{ status.load5?.toFixed(2) }} · 15m {{ status.load15?.toFixed(2) }}</div>
            </article>
          </section>

          <section class="info-panel">
            <div class="panel-title">实例信息</div>
            <div class="info-grid">
              <div v-for="item in systemInfo" :key="item.label" class="info-item">
                <span>{{ item.label }}</span>
                <strong>{{ item.value }}</strong>
              </div>
            </div>
          </section>

          <section class="trend-panel">
            <StatusTrendCard title="24 小时资源趋势" :points="metrics" />
          </section>
        </div>

        <div v-else class="empty-state">
          未找到服务器信息。
        </div>
      </n-spin>
    </div>
  </n-scrollbar>
</template>

<style scoped>
.content-scroll {
  flex: 1;
}

.detail-page {
  max-width: 100%;
  margin: 0;
}

.detail-header {
  align-items: end;
}

.back-link {
  margin-bottom: 8px;
  color: var(--app-text-soft);
}

.title-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
}

.title-status-dot {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--app-danger);
  box-shadow: 0 0 16px rgba(255, 107, 125, 0.42);
}

.title-status-dot.online {
  background: #35d6a3;
  box-shadow: 0 0 18px rgba(53, 214, 163, 0.46);
}

.status-dot-inline {
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  margin-right: 6px;
  background: var(--app-danger);
}

.status-dot-inline.online {
  background: #35d6a3;
}

.header-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.detail-grid {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(360px, 1fr);
  gap: 14px;
}

.summary-panel,
.resource-card,
.info-panel,
.trend-panel {
  border: 1px solid rgba(93, 120, 162, 0.22);
  border-radius: 8px;
  background: var(--app-panel);
}

.summary-panel {
  grid-column: 1 / -1;
  padding: 16px 18px;
}

.summary-main {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 14px;
}

.summary-main div,
.info-item {
  min-width: 0;
  display: grid;
  gap: 4px;
}

.summary-label,
.info-item span,
.resource-card span,
.load-note {
  color: var(--app-text-soft);
  font-size: 12px;
}

.summary-main strong,
.info-item strong {
  overflow: hidden;
  color: var(--app-text);
  font-size: 18px;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.resource-strip {
  grid-column: 1 / -1;
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
}

.resource-card {
  min-height: 156px;
  padding: 20px;
  display: grid;
  align-content: start;
  gap: 12px;
  position: relative;
  overflow: hidden;
}

.resource-icon {
  position: absolute;
  right: 20px;
  top: 22px;
  width: 50px;
  height: 50px;
  border-radius: 50%;
  border: 1px solid rgba(79, 131, 255, 0.26);
  background: rgba(79, 131, 255, 0.1);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #62a8ff;
  font-family: var(--app-font-mono);
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0;
}

.resource-card strong {
  color: var(--app-text);
  font-size: 30px;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
  line-height: 1;
}

.res-bar {
  height: 6px;
  overflow: hidden;
  border-radius: 8px;
  background: rgba(93, 120, 162, 0.22);
}

.res-fill {
  height: 100%;
  border-radius: inherit;
}

.info-panel {
  grid-column: 2;
  grid-row: 3;
  padding: 18px;
}

.panel-title {
  margin-bottom: 14px;
  color: var(--app-text);
  font-size: 17px;
  font-weight: 700;
}

.info-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 0;
}

.info-item {
  display: grid;
  grid-template-columns: minmax(92px, 0.55fr) minmax(0, 1fr);
  gap: 12px;
  padding: 10px 0;
  border-bottom: 1px solid rgba(93, 120, 162, 0.12);
  background: transparent;
  border-radius: 0;
}

.trend-panel {
  grid-column: 1;
  grid-row: 3;
  overflow: hidden;
}

.empty-state {
  padding: 48px 0;
  color: var(--app-text-soft);
  text-align: center;
}

@media (max-width: 1100px) {
  .detail-grid {
    grid-template-columns: 1fr;
  }

  .resource-strip,
  .info-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .info-panel,
  .trend-panel {
    grid-column: auto;
    grid-row: auto;
  }
}

@media (max-width: 720px) {
  .detail-header {
    align-items: flex-start;
    flex-direction: column;
  }

  .summary-main,
  .resource-strip,
  .info-grid {
    grid-template-columns: 1fr;
  }
}
</style>
