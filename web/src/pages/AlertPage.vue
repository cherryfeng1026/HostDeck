<script setup lang="ts">
import {
  NButton,
  NInput,
  NInputNumber,
  NPagination,
  NScrollbar,
  NSwitch,
  NTabPane,
  NTabs,
  NTag,
  useMessage,
} from 'naive-ui'
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useAutoRefresh } from '../auto-refresh'
import {
  loadAlerts,
  prependCachedAlertHistory,
  removeCachedAlert,
  setCachedAlertNotificationSettings,
  useAlertCache,
} from '../dashboard-cache'
import {
  ackAlert,
  testAlertNotificationSettings,
  updateAlertNotificationSettings,
} from '../services/api'
import { useSession } from '../session'
import type { AlertEvent, AlertHistoryEvent, AlertNotificationSettings } from '../types'

const message = useMessage()
const router = useRouter()
const { canManageInfrastructure } = useSession()
const alertCache = useAlertCache()

const alerts = computed(() => alertCache.alerts)
const historyItems = computed(() => alertCache.historyItems)
const rules = computed(() => alertCache.rules)
const loading = computed(() => alertCache.loading)
const savingNotificationSettings = ref(false)
const testingNotification = ref(false)
const actingAlertId = ref<number | null>(null)
const activeTab = ref('active')
const activeAlertFilter = ref<'all' | 'critical' | 'warning'>('all')
const activePage = ref(1)
const historyPage = ref(1)
const notificationPanel = ref<HTMLElement | null>(null)
const notificationHighlighted = ref(false)
const pageSize = 6

const notificationSettings = reactive<AlertNotificationSettings>({
  enabled: false,
  webhookURL: '',
  webhookConfigured: false,
  clearWebhookURL: false,
  webhookTimeoutSeconds: 5,
})

const metricOptions = [
  { label: '在线状态', value: 'online' },
  { label: 'CPU 使用率', value: 'cpu_usage' },
  { label: '内存使用率', value: 'memory_usage' },
  { label: '磁盘使用率', value: 'disk_usage' },
]

const operatorOptions = [
  { label: '等于', value: 'eq' },
  { label: '大于', value: 'gt' },
  { label: '大于等于', value: 'gte' },
  { label: '小于', value: 'lt' },
  { label: '小于等于', value: 'lte' },
]

const activeAlerts = computed(() => alerts.value.filter((alert) => alert.status === 'active'))
const filteredActiveAlerts = computed(() => {
  if (activeAlertFilter.value === 'critical') {
    return activeAlerts.value.filter((alert) => alert.severity === 'critical')
  }
  if (activeAlertFilter.value === 'warning') {
    return activeAlerts.value.filter((alert) => alert.severity !== 'critical')
  }
  return activeAlerts.value
})
const criticalActiveCount = computed(() => activeAlerts.value.filter((alert) => alert.severity === 'critical').length)
const warningActiveCount = computed(() => activeAlerts.value.filter((alert) => alert.severity !== 'critical').length)
const enabledRuleCount = computed(() => rules.value.filter((rule) => rule.enabled).length)
const webhookStatusText = computed(() => (notificationSettings.enabled ? '正常' : '未启用'))
const activeAlertPageItems = computed(() => paginateItems(filteredActiveAlerts.value, activePage.value, pageSize))
const historyPageItems = computed(() => paginateItems(historyItems.value, historyPage.value, pageSize))
const activePageCount = computed(() => Math.max(1, Math.ceil(filteredActiveAlerts.value.length / pageSize)))
const historyPageCount = computed(() => Math.max(1, Math.ceil(historyItems.value.length / pageSize)))
const activeFilterLabel = computed(() => {
  switch (activeAlertFilter.value) {
    case 'critical':
      return '严重告警'
    case 'warning':
      return '警告告警'
    default:
      return '全部活动告警'
  }
})

async function loadData(force = false, silent = false) {
  try {
    await loadAlerts(canManageInfrastructure.value, { force, silent })
    syncNotificationSettings(alertCache.notificationSettings)
  } catch (error) {
    if (!silent) {
      message.error(error instanceof Error ? error.message : '加载告警数据失败')
    }
  }
}

function syncNotificationSettings(settings: AlertNotificationSettings | null) {
  if (!settings) {
    notificationSettings.enabled = false
    notificationSettings.webhookURL = ''
    notificationSettings.webhookConfigured = false
    notificationSettings.clearWebhookURL = false
    notificationSettings.webhookTimeoutSeconds = 5
    return
  }
  notificationSettings.enabled = settings.enabled
  notificationSettings.webhookURL = settings.webhookURL
  notificationSettings.webhookConfigured = !!settings.webhookConfigured
  notificationSettings.clearWebhookURL = false
  notificationSettings.webhookTimeoutSeconds = settings.webhookTimeoutSeconds
}

function prependHistoryItemFromAlert(alert: AlertEvent, actorUsername: string) {
  const nextItem: AlertHistoryEvent = {
    id: Date.now(),
    alertId: alert.id,
    ruleId: alert.ruleId,
    serverId: alert.serverId,
    serverName: alert.serverName,
    eventType: 'acknowledged',
    metric: alert.metric,
    operator: alert.operator,
    threshold: alert.threshold,
    currentValue: alert.currentValue,
    severity: alert.severity,
    message: alert.message,
    status: 'acknowledged',
    triggeredAt: alert.triggeredAt,
    createdAt: new Date().toISOString(),
    actorUsername,
    detail: 'operator confirmed',
  }
  prependCachedAlertHistory(nextItem)
}

async function handleAcknowledge(alert: AlertEvent) {
  actingAlertId.value = alert.id
  try {
    const updatedAlert = await ackAlert(alert.id)
    removeCachedAlert(alert.id)
    prependHistoryItemFromAlert(updatedAlert, updatedAlert.acknowledgedBy || '当前用户')
    await loadData(true, true)
    message.success('告警已确认并归入历史')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '确认告警失败')
  } finally {
    actingAlertId.value = null
  }
}

async function handleSubmitNotificationSettings() {
  savingNotificationSettings.value = true
  try {
    const saved = await updateAlertNotificationSettings({
      enabled: notificationSettings.enabled,
      webhookURL: notificationSettings.webhookURL.trim(),
      clearWebhookURL: notificationSettings.clearWebhookURL,
      webhookTimeoutSeconds: notificationSettings.webhookTimeoutSeconds,
    })
    setCachedAlertNotificationSettings(saved)
    syncNotificationSettings(alertCache.notificationSettings)
    await loadData(true, true)
    message.success('通知设置已保存')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '保存通知设置失败')
  } finally {
    savingNotificationSettings.value = false
  }
}

function metricLabel(metric: string) {
  return metricOptions.find((item) => item.value === metric)?.label || metric
}

function operatorLabel(operator: string) {
  return operatorOptions.find((item) => item.value === operator)?.label || operator
}

async function handleTestNotification() {
  testingNotification.value = true
  try {
    await testAlertNotificationSettings()
    await loadData(true, true)
    message.success('测试通知已发送')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '发送测试通知失败')
  } finally {
    testingNotification.value = false
  }
}

function formatDateTime(value?: string) {
  if (!value) return '未知时间'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

function formatDuration(start?: string, end?: string) {
  if (!start) return '未知'
  const startAt = new Date(start).getTime()
  const endAt = end ? new Date(end).getTime() : Date.now()
  const diffSeconds = Math.max(0, Math.floor((endAt - startAt) / 1000))
  const hours = Math.floor(diffSeconds / 3600)
  const minutes = Math.floor((diffSeconds % 3600) / 60)
  const seconds = diffSeconds % 60
  if (hours > 0) return `${hours} 小时 ${minutes} 分钟`
  if (minutes > 0) return `${minutes} 分钟 ${seconds} 秒`
  return `${seconds} 秒`
}

function statusLabel(status: string) {
  switch (status) {
    case 'acknowledged':
      return '已确认'
    case 'muted':
      return '已静默'
    case 'pending':
      return '等待持续时间'
    default:
      return '活动中'
  }
}

function historyEventLabel(eventType: string) {
  switch (eventType) {
    case 'triggered':
      return '触发'
    case 'acknowledged':
      return '确认'
    case 'muted':
      return '静默'
    case 'resolved':
      return '恢复'
    default:
      return eventType
  }
}

function eventIcon(eventType: string) {
  switch (eventType) {
    case 'triggered':
      return '!'
    case 'acknowledged':
    case 'resolved':
      return '✓'
    case 'muted':
      return '-'
    default:
      return 'i'
  }
}

function eventTone(eventType: string, severity: string) {
  if (eventType === 'acknowledged' || eventType === 'resolved') {
    return 'resolved'
  }
  return severity === 'critical' ? 'critical' : 'warning'
}

function paginateItems<T>(items: T[], page: number, size: number) {
  const start = (page - 1) * size
  return items.slice(start, start + size)
}

function handleManualRefresh() {
  void loadData(true)
}

function selectActiveAlertFilter(filter: 'all' | 'critical' | 'warning') {
  activeAlertFilter.value = filter
  activePage.value = 1
  activeTab.value = 'active'
}

function openRulePage() {
  void router.push('/alerts/rules')
}

function focusNotificationPanel() {
  notificationPanel.value?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
  notificationHighlighted.value = true
  window.setTimeout(() => {
    notificationHighlighted.value = false
  }, 900)
}

watch(canManageInfrastructure, () => {
  void loadData(true, alertCache.initialized)
})

watch(filteredActiveAlerts, () => {
  if (activePage.value > activePageCount.value) {
    activePage.value = activePageCount.value
  }
})

watch(historyItems, () => {
  if (historyPage.value > historyPageCount.value) {
    historyPage.value = historyPageCount.value
  }
})

useAutoRefresh(() => loadData(false, true), 30000)

onMounted(() => {
  void loadData(false, alertCache.initialized)
})
</script>

<template>
  <n-scrollbar class="content-scroll">
    <div class="page-container alert-page">
      <div class="page-header">
        <div class="header-text">
          <h1>系统告警</h1>
        </div>
        <div class="header-actions">
          <n-button ghost :loading="loading" @click="handleManualRefresh">刷新</n-button>
          <n-tag v-if="!canManageInfrastructure" type="default" bordered>只读</n-tag>
        </div>
      </div>

      <section class="alert-summary-strip">
        <button type="button" class="summary-cell danger" :class="{ active: activeAlertFilter === 'all' }" @click="selectActiveAlertFilter('all')">
          <i class="summary-mark" aria-hidden="true">
            <svg viewBox="0 0 24 24"><path d="M18 8a6 6 0 0 0-12 0c0 6.5-3 8.5-3 8.5h18S18 14.5 18 8" /><path d="M10 20a2 2 0 0 0 4 0" /></svg>
          </i>
          <span>当前告警</span>
          <strong>{{ activeAlerts.length }}</strong>
          <small>待确认</small>
        </button>
        <button type="button" class="summary-cell danger" :class="{ active: activeAlertFilter === 'critical' }" @click="selectActiveAlertFilter('critical')">
          <i class="summary-mark" aria-hidden="true">
            <svg viewBox="0 0 24 24"><path d="M12 4 21 20H3L12 4Z" /><path d="M12 9v5" /><path d="M12 17h.01" /></svg>
          </i>
          <span>严重告警</span>
          <strong>{{ criticalActiveCount }}</strong>
          <small>严重事件</small>
        </button>
        <button type="button" class="summary-cell warning" :class="{ active: activeAlertFilter === 'warning' }" @click="selectActiveAlertFilter('warning')">
          <i class="summary-mark" aria-hidden="true">
            <svg viewBox="0 0 24 24"><path d="M12 4 21 20H3L12 4Z" /><path d="M12 10v4" /><path d="M12 17h.01" /></svg>
          </i>
          <span>警告告警</span>
          <strong>{{ warningActiveCount }}</strong>
          <small>警告事件</small>
        </button>
        <button type="button" class="summary-cell info" @click="openRulePage">
          <i class="summary-mark" aria-hidden="true">
            <svg viewBox="0 0 24 24"><path d="M8 6h13" /><path d="M8 12h13" /><path d="M8 18h13" /><path d="m3 6 .8.8L5.5 5" /><path d="m3 12 .8.8 1.7-1.8" /><path d="m3 18 .8.8 1.7-1.8" /></svg>
          </i>
          <span>规则</span>
          <strong>{{ enabledRuleCount }} / {{ rules.length }}</strong>
          <small>已启用</small>
        </button>
        <button type="button" class="summary-cell success" :class="{ active: notificationHighlighted }" @click="focusNotificationPanel">
          <i class="summary-mark" aria-hidden="true">
            <svg viewBox="0 0 24 24"><path d="M4 12h4l4-5v10l-4-5H4Z" /><path d="M16 9.5a4 4 0 0 1 0 5" /><path d="M18.5 7a7 7 0 0 1 0 10" /></svg>
          </i>
          <span>告警通知</span>
          <strong>{{ webhookStatusText }}</strong>
          <small>{{ notificationSettings.webhookConfigured ? '已配置' : '未配置' }}</small>
        </button>
      </section>

      <div class="alert-workspace">
        <section class="alert-board">
          <div class="board-head">
            <div>
              <strong>告警处理</strong>
              <span>{{ activeAlerts.length ? `${activeFilterLabel} · ${filteredActiveAlerts.length} / ${activeAlerts.length} 条` : '当前无活动告警' }}</span>
            </div>
            <n-tag size="small" :type="activeAlerts.length ? 'warning' : 'success'" bordered>
              {{ activeAlerts.length ? '需要确认' : '稳定' }}
            </n-tag>
          </div>

          <n-tabs v-model:value="activeTab" type="line" animated class="alert-tabs">
            <n-tab-pane name="active" :tab="`活动告警 (${activeAlerts.length})`">
              <div class="alert-pane">
                <div class="alert-list-region">
                  <div v-if="!filteredActiveAlerts.length" class="empty-state">
                    {{ activeAlerts.length ? '当前筛选条件下没有活动告警。' : '当前没有需要确认的活动告警。' }}
                  </div>
                  <div v-else class="alert-table-wrap">
                    <table class="alert-table">
                      <colgroup>
                        <col style="width: 176px" />
                        <col style="width: 82px" />
                        <col style="width: 70px" />
                        <col style="width: 70px" />
                        <col style="width: 104px" />
                        <col style="width: 74px" />
                        <col style="width: 94px" />
                      </colgroup>
                      <thead>
                        <tr>
                          <th>告警项</th>
                          <th>主机</th>
                          <th>阈值</th>
                          <th>当前值</th>
                          <th>首次发生</th>
                          <th>状态</th>
                          <th>操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        <tr v-for="alert in activeAlertPageItems" :key="alert.id">
                          <td>
                            <div class="alert-name-cell">
                              <span class="severity-dot" :class="alert.severity" />
                              <div>
                                <strong>{{ alert.message }}</strong>
                                <small>{{ metricLabel(alert.metric) }}</small>
                              </div>
                            </div>
                          </td>
                          <td>
                            <button type="button" class="alert-server" @click="router.push(`/servers/${alert.serverId}`)">
                              {{ alert.serverName || `#${alert.serverId}` }}
                            </button>
                          </td>
                          <td class="muted-cell">{{ operatorLabel(alert.operator) }} {{ alert.threshold }}</td>
                          <td :class="alert.severity === 'critical' ? 'danger-cell' : 'warning-cell'">{{ alert.currentValue }}</td>
                          <td>
                            <span>{{ formatDateTime(alert.triggeredAt) }}</span>
                            <small class="block-muted">{{ formatDuration(alert.triggeredAt, alert.lastTriggeredAt) }}</small>
                          </td>
                          <td>
                            <n-tag :type="alert.severity === 'critical' ? 'error' : 'warning'" size="small" bordered>
                              {{ statusLabel(alert.status) }}
                            </n-tag>
                          </td>
                          <td>
                            <div class="table-actions">
                              <n-button size="small" type="primary" ghost @click="router.push(`/servers/${alert.serverId}`)">查看</n-button>
                              <n-button
                                v-if="canManageInfrastructure"
                                size="small"
                                type="primary"
                                :loading="actingAlertId === alert.id"
                                @click="handleAcknowledge(alert)"
                              >
                                确认
                              </n-button>
                            </div>
                          </td>
                        </tr>
                      </tbody>
                    </table>
                  </div>
                </div>
                <div v-if="filteredActiveAlerts.length > pageSize" class="alert-pager-slot">
                  <n-pagination v-model:page="activePage" :page-count="activePageCount" class="pager" />
                </div>
              </div>
            </n-tab-pane>

            <n-tab-pane name="history" :tab="`历史告警 (${historyItems.length})`">
              <div class="alert-pane">
                <div class="alert-list-region">
                  <div v-if="!historyItems.length" class="empty-state">
                    暂无告警历史事件。
                  </div>
                  <div v-else class="alert-table-wrap">
                    <table class="alert-table history-table">
                      <colgroup>
                        <col style="width: 122px" />
                        <col style="width: 82px" />
                        <col style="width: 176px" />
                        <col style="width: 82px" />
                        <col style="width: 68px" />
                        <col style="width: 72px" />
                        <col style="width: 72px" />
                      </colgroup>
                      <thead>
                        <tr>
                          <th>时间</th>
                          <th>级别</th>
                          <th>告警项</th>
                          <th>主机</th>
                          <th>事件</th>
                          <th>确认人</th>
                          <th>状态</th>
                        </tr>
                      </thead>
                      <tbody>
                        <tr v-for="item in historyPageItems" :key="item.id">
                          <td>{{ formatDateTime(item.createdAt) }}</td>
                          <td>
                            <n-tag :type="item.severity === 'critical' ? 'error' : 'warning'" size="small" bordered>
                              {{ item.severity === 'critical' ? '严重' : '警告' }}
                            </n-tag>
                          </td>
                          <td>
                            <strong class="table-strong">{{ item.message }}</strong>
                            <small class="block-muted">{{ metricLabel(item.metric) }} {{ operatorLabel(item.operator) }} {{ item.threshold }}</small>
                          </td>
                          <td>
                            <button type="button" class="alert-server" @click="router.push(`/servers/${item.serverId}`)">
                              {{ item.serverName || `#${item.serverId}` }}
                            </button>
                          </td>
                          <td>{{ historyEventLabel(item.eventType) }}</td>
                          <td class="muted-cell">{{ item.actorUsername || '-' }}</td>
                          <td>
                            <n-tag size="small" bordered>{{ statusLabel(item.status) }}</n-tag>
                          </td>
                        </tr>
                      </tbody>
                    </table>
                  </div>
                </div>
                <div v-if="historyItems.length > pageSize" class="alert-pager-slot">
                  <n-pagination v-model:page="historyPage" :page-count="historyPageCount" class="pager" />
                </div>
              </div>
            </n-tab-pane>
          </n-tabs>
        </section>

        <aside ref="notificationPanel" class="alert-side" :class="{ highlighted: notificationHighlighted }">
          <div v-if="!canManageInfrastructure" class="empty-state compact">
            当前账号没有修改通知设置的权限。
          </div>
          <div v-else class="side-section">
            <div class="side-title">
              <strong>告警通知</strong>
              <n-tag size="small" :type="notificationSettings.enabled ? 'success' : 'default'" bordered>
                {{ notificationSettings.enabled ? '启用' : '关闭' }}
              </n-tag>
            </div>
            <div class="rule-grid single">
              <div class="field-block switch-row">
                <span>启用通知</span>
                <n-switch v-model:value="notificationSettings.enabled" />
              </div>
              <div class="field-block">
                <span>通知地址</span>
                <n-input
                  v-model:value="notificationSettings.webhookURL"
                  placeholder="https://hooks.example.com/alerts"
                  :disabled="notificationSettings.clearWebhookURL"
                  class="dark-input"
                />
              </div>
              <div class="field-block switch-row">
                <span>清空已保存地址</span>
                <n-switch v-model:value="notificationSettings.clearWebhookURL" />
              </div>
              <div class="field-block">
                <span>超时秒数</span>
                <n-input-number v-model:value="notificationSettings.webhookTimeoutSeconds" :min="1" style="width: 100%" class="dark-input" />
              </div>
            </div>
            <div class="notification-actions">
              <n-button type="primary" :loading="savingNotificationSettings" @click="handleSubmitNotificationSettings">保存</n-button>
              <n-button ghost :loading="testingNotification" @click="handleTestNotification">测试</n-button>
            </div>
            <n-button text class="rules-link" @click="openRulePage">进入规则管理</n-button>
          </div>
        </aside>
      </div>
    </div>
  </n-scrollbar>
</template>

<style scoped>
.content-scroll {
  flex: 1;
}

.alert-page {
  max-width: 100%;
  margin: 0;
}

.alert-summary-strip {
  min-height: 86px;
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  margin-bottom: 14px;
  overflow: hidden;
  border: 1px solid rgba(93, 120, 162, 0.22);
  border-radius: 8px;
  background: var(--app-panel);
}

.summary-cell {
  position: relative;
  display: grid;
  grid-template-columns: 38px minmax(0, 1fr);
  align-content: center;
  align-items: center;
  gap: 5px;
  min-width: 0;
  padding: 14px 18px;
  border: 0;
  background: transparent;
  text-align: left;
  font: inherit;
  cursor: pointer;
  transition: background-color 0.16s ease, box-shadow 0.16s ease, color 0.16s ease;
}

.summary-cell + .summary-cell {
  border-left: 1px solid rgba(93, 120, 162, 0.16);
}

.summary-cell:hover {
  background: rgba(79, 131, 255, 0.08);
}

.summary-cell:focus-visible {
  outline: 2px solid rgba(79, 131, 255, 0.72);
  outline-offset: -2px;
}

.summary-cell.active {
  background: rgba(79, 131, 255, 0.12);
  box-shadow: inset 0 3px 0 currentColor;
}

.summary-mark {
  grid-row: span 3;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  border: 1px solid currentColor;
  background: color-mix(in srgb, currentColor 12%, transparent);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  opacity: 0.86;
}

.summary-mark svg {
  width: 17px;
  height: 17px;
  fill: none;
  stroke: currentColor;
  stroke-width: 1.8;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.summary-cell span,
.summary-cell small {
  color: var(--app-text-soft);
  font-size: 12px;
  font-weight: 600;
}

.summary-cell strong {
  color: var(--app-text);
  font-size: 22px;
  line-height: 1;
  font-variant-numeric: tabular-nums;
}

.summary-cell.danger {
  color: var(--app-danger);
}

.summary-cell.danger strong {
  color: var(--app-danger);
}

.summary-cell.warning {
  color: var(--app-warning);
}

.summary-cell.warning strong {
  color: var(--app-warning);
}

.summary-cell.info {
  color: var(--app-accent);
}

.summary-cell.info strong {
  color: var(--app-accent);
}

.summary-cell.success {
  color: #35d6a3;
}

.summary-cell.success strong {
  color: #35d6a3;
}

.status-dot {
  width: 8px;
  height: 8px;
  display: inline-block;
  border-radius: 50%;
  background: var(--app-danger);
}

.status-dot.online {
  background: #35d6a3;
  box-shadow: 0 0 10px rgba(53, 214, 163, 0.58);
}

.alert-workspace {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(320px, 360px);
  gap: 14px;
  align-items: start;
}

.alert-board,
.alert-side {
  border: 1px solid rgba(93, 120, 162, 0.22);
  background: var(--app-panel);
  border-radius: 8px;
}

.alert-board {
  min-width: 0;
  min-height: calc(100vh - 224px);
  padding: 14px;
  display: flex;
  flex-direction: column;
}

.alert-side {
  position: sticky;
  top: 78px;
  padding: 14px;
  max-height: calc(100vh - 180px);
  overflow: auto;
  transition: border-color 0.18s ease, box-shadow 0.18s ease;
}

.alert-side.highlighted {
  border-color: rgba(53, 214, 163, 0.46) !important;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.045), 0 0 0 2px rgba(53, 214, 163, 0.12) !important;
}

.board-head,
.side-title,
.alert-header,
.alert-footer,
.rule-item,
.notification-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.board-head {
  flex-shrink: 0;
  margin-bottom: 12px;
  padding-bottom: 12px;
  border-bottom: 1px solid rgba(93, 120, 162, 0.16);
}

.board-head strong,
.side-title strong {
  display: block;
  color: var(--app-text);
  font-size: 15px;
}

.board-head span {
  display: block;
  margin-top: 3px;
  color: var(--app-text-soft);
  font-size: 12px;
}

.alert-tabs {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.alert-tabs :deep(.n-tabs-nav) {
  flex-shrink: 0;
}

.alert-tabs :deep(.n-tab-pane),
.alert-tabs :deep(.n-tabs-pane-wrapper),
.alert-tabs :deep(.n-tabs-pane-wrapper .n-tabs-pane-wrapper) {
  min-height: 0;
}

.alert-tabs :deep(.n-tabs-pane-wrapper) {
  flex: 1;
}

.alert-pane {
  height: 100%;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.alert-list-region {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding-right: 4px;
}

.alert-table-wrap {
  margin-top: 10px;
  overflow: auto;
  border: 1px solid rgba(93, 120, 162, 0.16);
  border-radius: 8px;
}

.alert-table {
  width: 100%;
  min-width: 670px;
  border-collapse: collapse;
  table-layout: fixed;
}

.alert-table th,
.alert-table td {
  padding: 10px 10px;
  border-bottom: 1px solid rgba(93, 120, 162, 0.12);
  text-align: left;
  vertical-align: middle;
}

.alert-table th {
  color: var(--app-text-soft);
  background: rgba(17, 32, 52, 0.52);
  font-size: 12px;
  font-weight: 600;
}

.alert-table td {
  color: #dce7f6;
  font-size: 12px;
}

.alert-table tbody tr:hover td {
  background: rgba(79, 131, 255, 0.07);
}

.alert-table tbody tr:last-child td {
  border-bottom: 0;
}

.alert-name-cell {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.alert-name-cell strong,
.table-strong {
  display: block;
  color: var(--app-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.alert-name-cell small,
.block-muted {
  display: block;
  margin-top: 3px;
  color: var(--app-text-faint);
  font-size: 12px;
}

.severity-dot {
  width: 8px;
  height: 8px;
  flex: 0 0 auto;
  border-radius: 50%;
  background: var(--app-warning);
  box-shadow: 0 0 12px rgba(232, 180, 95, 0.5);
}

.severity-dot.critical {
  background: var(--app-danger);
  box-shadow: 0 0 12px rgba(255, 107, 125, 0.55);
}

.danger-cell {
  color: var(--app-danger) !important;
  font-weight: 700;
}

.warning-cell {
  color: var(--app-warning) !important;
  font-weight: 700;
}

.muted-cell {
  color: var(--app-text-soft) !important;
}

.table-actions {
  display: flex;
  align-items: center;
  gap: 5px;
  white-space: nowrap;
}

.table-actions :deep(.n-button) {
  min-width: 38px;
  padding-inline: 8px !important;
}

.alert-pager-slot {
  flex-shrink: 0;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid rgba(93, 120, 162, 0.14);
  background: rgba(8, 18, 32, 0.86);
}

.alert-card {
  display: grid;
  grid-template-columns: 38px minmax(0, 1fr);
  gap: 12px;
}

.alert-icon {
  width: 38px;
  height: 38px;
  border-radius: 8px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(93, 120, 162, 0.22);
  color: var(--app-text-soft);
  font-weight: 800;
}

.alert-icon.critical {
  color: var(--app-danger);
  background: rgba(255, 107, 125, 0.1);
  border-color: rgba(255, 107, 125, 0.28);
}

.alert-icon.warning {
  color: var(--app-warning);
  background: rgba(232, 180, 95, 0.1);
  border-color: rgba(232, 180, 95, 0.28);
}

.alert-icon.resolved {
  color: #35d6a3;
  background: rgba(53, 214, 163, 0.1);
  border-color: rgba(53, 214, 163, 0.28);
}

.alert-content {
  min-width: 0;
  padding: 14px;
  border: 1px solid rgba(93, 120, 162, 0.16);
  background: rgba(17, 32, 52, 0.46);
  border-radius: 8px;
}

.history-card .alert-content {
  background: rgba(12, 24, 42, 0.4);
}

.alert-server {
  min-width: 0;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--app-text);
  font: inherit;
  font-weight: 700;
  text-align: left;
  cursor: pointer;
}

.alert-server:hover {
  color: var(--app-accent);
}

.alert-time,
.alert-meta,
.rule-meta {
  color: var(--app-text-soft);
  font-size: 12px;
}

.alert-message {
  margin: 8px 0 10px;
  color: #dce7f6;
  font-size: 14px;
  line-height: 1.55;
}

.alert-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.alert-footer {
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid rgba(93, 120, 162, 0.12);
}

.pager {
  justify-content: flex-end;
}

.side-section {
  display: grid;
  gap: 14px;
  padding-top: 6px;
}

.rules-list-section {
  margin-top: 18px;
  padding-top: 16px;
  border-top: 1px solid rgba(93, 120, 162, 0.14);
}

.rule-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.rule-grid.single {
  grid-template-columns: 1fr;
}

.field-block {
  display: grid;
  gap: 6px;
}

.field-block span {
  color: var(--app-text-soft);
  font-size: 12px;
  font-weight: 600;
}

.switch-row {
  grid-template-columns: 1fr auto;
  align-items: center;
}

.save-button {
  width: 100%;
}

.rules-list {
  display: grid;
  gap: 10px;
}

.rule-item {
  padding: 12px;
  border: 1px solid rgba(93, 120, 162, 0.16);
  border-radius: 8px;
  background: rgba(17, 32, 52, 0.42);
}

.rule-condition {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  color: #dce7f6;
}

.rule-condition strong {
  color: var(--app-text);
}

.rule-meta {
  margin-top: 6px;
}

.notification-actions {
  justify-content: flex-start;
}

.rules-link {
  width: max-content;
  justify-self: start;
  padding-inline: 0 !important;
  color: var(--app-accent) !important;
}

.empty-state {
  padding: 38px 0;
  color: var(--app-text-faint);
  text-align: center;
}

.empty-state.compact {
  padding: 18px 0;
}

:deep(.dark-input .n-input-wrapper),
:deep(.dark-input .n-base-selection),
:deep(.dark-input .n-input-number) {
  border-radius: 8px !important;
  background: rgba(9, 20, 36, 0.88) !important;
  background-color: rgba(9, 20, 36, 0.88) !important;
  border: 1px solid rgba(93, 120, 162, 0.24) !important;
}

:deep(.dark-input .n-input__border),
:deep(.dark-input .n-input__state-border),
:deep(.dark-input .n-base-selection__border),
:deep(.dark-input .n-base-selection__state-border) {
  display: none !important;
}

@media (max-width: 1120px) {
  .alert-summary-strip {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .summary-cell + .summary-cell {
    border-left: 0;
    border-top: 1px solid rgba(93, 120, 162, 0.12);
  }

  .alert-workspace {
    grid-template-columns: 1fr;
  }

  .alert-board {
    height: auto;
    min-height: 0;
  }

  .alert-list-region {
    max-height: none;
    overflow: visible;
  }

  .alert-side {
    position: static;
    max-height: none;
  }
}

@media (max-width: 640px) {
  .alert-summary-strip {
    grid-template-columns: 1fr;
  }

  .alert-card {
    grid-template-columns: 1fr;
  }

  .alert-icon {
    display: none;
  }

  .rule-grid {
    grid-template-columns: 1fr;
  }

  .alert-header,
  .alert-footer {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
