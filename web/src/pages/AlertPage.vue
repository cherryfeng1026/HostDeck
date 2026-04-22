<script setup lang="ts">
import {
  NButton,
  NInput,
  NInputNumber,
  NScrollbar,
  NSelect,
  NSwitch,
  NTabPane,
  NTabs,
  NTag,
  useMessage,
} from 'naive-ui'
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  ackAlert,
  createAlertRule,
  getAlertHistory,
  getAlertNotificationSettings,
  getAlertRules,
  getAlerts,
  muteAlert,
  updateAlertNotificationSettings,
  updateAlertRule,
} from '../services/api'
import { useSession } from '../session'
import type { AlertEvent, AlertHistoryEvent, AlertNotificationSettings, AlertRule } from '../types'

const message = useMessage()
const router = useRouter()
const { canManageInfrastructure } = useSession()
const alerts = ref<AlertEvent[]>([])
const historyItems = ref<AlertHistoryEvent[]>([])
const rules = ref<AlertRule[]>([])
const loading = ref(false)
const savingRule = ref(false)
const savingNotificationSettings = ref(false)
const editingRuleId = ref<number | null>(null)
const actingAlertId = ref<number | null>(null)

const notificationSettings = reactive<AlertNotificationSettings>({
  enabled: false,
  webhookURL: '',
  webhookConfigured: false,
  clearWebhookURL: false,
  webhookTimeoutSeconds: 5,
})

const ruleForm = reactive({
  metric: 'online',
  operator: 'eq',
  threshold: 0,
  durationSeconds: 60,
  enabled: true,
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

const muteDurationOptions = [
  { label: '15 分钟', value: 15 },
  { label: '30 分钟', value: 30 },
  { label: '1 小时', value: 60 },
  { label: '4 小时', value: 240 },
]

const muteDurationByAlertId = ref<Record<number, number>>({})

async function loadData() {
  loading.value = true
  try {
    const [alertList, alertHistory, ruleList, alertNotificationConfig] = await Promise.all([
      getAlerts(),
      getAlertHistory(),
      getAlertRules(),
      canManageInfrastructure.value ? getAlertNotificationSettings() : Promise.resolve(null),
    ])
    alerts.value = alertList
    historyItems.value = alertHistory
    rules.value = ruleList
    if (alertNotificationConfig) {
      notificationSettings.enabled = alertNotificationConfig.enabled
      notificationSettings.webhookURL = alertNotificationConfig.webhookURL
      notificationSettings.webhookConfigured = !!alertNotificationConfig.webhookConfigured
      notificationSettings.clearWebhookURL = false
      notificationSettings.webhookTimeoutSeconds = alertNotificationConfig.webhookTimeoutSeconds
    } else {
      notificationSettings.enabled = false
      notificationSettings.webhookURL = ''
      notificationSettings.webhookConfigured = false
      notificationSettings.clearWebhookURL = false
      notificationSettings.webhookTimeoutSeconds = 5
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载告警数据失败')
  } finally {
    loading.value = false
  }
}

async function handleAcknowledge(alert: AlertEvent) {
  actingAlertId.value = alert.id
  try {
    await ackAlert(alert.id)
    message.success('告警已确认')
    await loadData()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '确认告警失败')
  } finally {
    actingAlertId.value = null
  }
}

async function handleMute(alert: AlertEvent) {
  actingAlertId.value = alert.id
  const durationMinutes = muteDurationByAlertId.value[alert.id] || 30
  try {
    await muteAlert(alert.id, durationMinutes)
    message.success(`告警已静默 ${formatMuteDuration(durationMinutes)}`)
    await loadData()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '静默告警失败')
  } finally {
    actingAlertId.value = null
  }
}

async function handleSubmitRule() {
  savingRule.value = true
  try {
    const payload = { ...ruleForm }
    if (editingRuleId.value) {
      await updateAlertRule(editingRuleId.value, payload)
      message.success('告警规则已更新')
    } else {
      await createAlertRule(payload)
      message.success('告警规则已创建')
    }
    resetRuleForm()
    await loadData()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '保存告警规则失败')
  } finally {
    savingRule.value = false
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
    notificationSettings.enabled = saved.enabled
    notificationSettings.webhookURL = saved.webhookURL
    notificationSettings.webhookConfigured = !!saved.webhookConfigured
    notificationSettings.clearWebhookURL = false
    notificationSettings.webhookTimeoutSeconds = saved.webhookTimeoutSeconds
    message.success('通知设置已保存')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '保存通知设置失败')
  } finally {
    savingNotificationSettings.value = false
  }
}

function startEditRule(rule: AlertRule) {
  editingRuleId.value = rule.id
  ruleForm.metric = rule.metric
  ruleForm.operator = rule.operator
  ruleForm.threshold = rule.threshold
  ruleForm.durationSeconds = rule.durationSeconds
  ruleForm.enabled = rule.enabled
}

function resetRuleForm() {
  editingRuleId.value = null
  ruleForm.metric = 'online'
  ruleForm.operator = 'eq'
  ruleForm.threshold = 0
  ruleForm.durationSeconds = 60
  ruleForm.enabled = true
}

function metricLabel(metric: string) {
  return metricOptions.find((item) => item.value === metric)?.label || metric
}

function operatorLabel(operator: string) {
  return operatorOptions.find((item) => item.value === operator)?.label || operator
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

function formatMuteDuration(durationMinutes: number) {
  if (durationMinutes % 60 === 0 && durationMinutes >= 60) {
    return `${durationMinutes / 60} 小时`
  }
  return `${durationMinutes} 分钟`
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

onMounted(() => {
  void loadData()
})
</script>

<template>
  <n-scrollbar class="content-scroll">
    <div class="page-container">
      <div class="page-header">
        <div class="header-text">
          <h1>监控告警</h1>
          <p>管理阈值规则，跟踪活动告警，并查看告警生命周期历史。</p>
        </div>
        <div class="header-actions">
          <n-button ghost @click="loadData" style="color: #a1a1aa; border-color: rgba(255,255,255,0.1)">
            刷新数据
          </n-button>
          <n-tag v-if="!canManageInfrastructure" type="default" style="color: #a1a1aa; background: rgba(255,255,255,0.05)">
            只读权限
          </n-tag>
        </div>
      </div>

      <div class="layout-grid">
        <div class="bento-card alerts-panel">
          <div class="card-title-bar">
            <span class="card-title">告警中心</span>
            <n-tag size="small" :type="alerts.length ? 'warning' : 'success'" bordered>
              {{ alerts.length ? `${alerts.length} 条活动告警` : '当前平稳' }}
            </n-tag>
          </div>

          <n-tabs type="line" animated>
            <n-tab-pane name="active" :tab="`活动告警 (${alerts.length})`">
              <div v-if="!alerts.length" class="empty-state">
                当前没有正式活动告警。
              </div>
              <div v-else class="timeline-container">
                <div v-for="alert in alerts" :key="alert.id" class="alert-card">
                  <div class="alert-icon" :class="alert.severity">
                    <span v-if="alert.severity === 'critical'">!</span>
                    <span v-else>i</span>
                  </div>
                  <div class="alert-content">
                    <div class="alert-header">
                      <span class="alert-server" @click="router.push(`/servers/${alert.serverId}`)">{{ alert.serverName }}</span>
                      <span class="alert-time">{{ formatDateTime(alert.lastTriggeredAt) }}</span>
                    </div>
                    <div class="alert-message">{{ alert.message }}</div>
                    <div class="alert-meta">
                      <n-tag :type="alert.severity === 'critical' ? 'error' : 'warning'" size="small" bordered>
                        {{ alert.severity === 'critical' ? '严重' : '警告' }}
                      </n-tag>
                      <n-tag size="small" bordered type="info">{{ statusLabel(alert.status) }}</n-tag>
                      <span class="meta-text">持续 {{ formatDuration(alert.triggeredAt, alert.lastTriggeredAt) }}</span>
                      <span v-if="alert.durationSeconds > 0" class="meta-text">规则窗口 {{ alert.durationSeconds }} 秒</span>
                      <span v-if="alert.acknowledgedBy" class="meta-text">确认人 {{ alert.acknowledgedBy }}</span>
                      <span v-if="alert.mutedUntil" class="meta-text">静默至 {{ formatDateTime(alert.mutedUntil) }}</span>
                    </div>
                    <div class="alert-footer">
                      <n-button size="small" ghost type="primary" @click="router.push(`/servers/${alert.serverId}`)">
                        查看服务器
                      </n-button>
                      <div v-if="canManageInfrastructure" class="inline-actions">
                        <n-button
                          v-if="alert.status === 'active'"
                          size="small"
                          ghost
                          @click="handleAcknowledge(alert)"
                          :loading="actingAlertId === alert.id"
                        >
                          确认
                        </n-button>
                        <template v-if="alert.status !== 'muted'">
                          <n-select
                            :value="muteDurationByAlertId[alert.id] || 30"
                            :options="muteDurationOptions"
                            size="small"
                            consistent-menu-width
                            class="mute-select"
                            @update:value="(value: number) => (muteDurationByAlertId[alert.id] = value)"
                          />
                          <n-button
                            size="small"
                            ghost
                            type="warning"
                            @click="handleMute(alert)"
                            :loading="actingAlertId === alert.id"
                          >
                            静默
                          </n-button>
                        </template>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </n-tab-pane>

            <n-tab-pane name="history" :tab="`历史事件 (${historyItems.length})`">
              <div v-if="!historyItems.length" class="empty-state">
                暂无告警历史事件。
              </div>
              <div v-else class="timeline-container">
                <div v-for="item in historyItems" :key="item.id" class="alert-card history-card">
                  <div class="alert-icon history-icon">
                    <span>{{ historyEventLabel(item.eventType).slice(0, 1) }}</span>
                  </div>
                  <div class="alert-content">
                    <div class="alert-header">
                      <span class="alert-server" @click="router.push(`/servers/${item.serverId}`)">{{ item.serverName }}</span>
                      <span class="alert-time">{{ formatDateTime(item.createdAt) }}</span>
                    </div>
                    <div class="alert-message">{{ item.message }}</div>
                    <div class="alert-meta">
                      <n-tag size="small" bordered>{{ historyEventLabel(item.eventType) }}</n-tag>
                      <n-tag :type="item.severity === 'critical' ? 'error' : 'warning'" size="small" bordered>
                        {{ item.severity === 'critical' ? '严重' : '警告' }}
                      </n-tag>
                      <span class="meta-text">首次触发 {{ formatDateTime(item.triggeredAt) }}</span>
                      <span v-if="item.actorUsername" class="meta-text">操作者 {{ item.actorUsername }}</span>
                      <span v-if="item.detail" class="meta-text">{{ item.detail }}</span>
                    </div>
                  </div>
                </div>
              </div>
            </n-tab-pane>
          </n-tabs>
        </div>

        <div class="rules-panel">
          <div v-if="canManageInfrastructure" class="bento-card form-card">
            <div class="card-title-bar">
              <span class="card-title">{{ editingRuleId ? '编辑规则' : '新增规则' }}</span>
              <n-button v-if="editingRuleId" size="small" text style="color: #a1a1aa" @click="resetRuleForm">取消编辑</n-button>
            </div>

            <div class="rule-grid">
              <div class="field-block">
                <span>监控指标</span>
                <n-select v-model:value="ruleForm.metric" :options="metricOptions" class="dark-input" />
              </div>
              <div class="field-block">
                <span>触发条件</span>
                <n-select v-model:value="ruleForm.operator" :options="operatorOptions" class="dark-input" />
              </div>
              <div class="field-block">
                <span>危险阈值</span>
                <n-input-number v-model:value="ruleForm.threshold" style="width: 100%" class="dark-input" />
              </div>
              <div class="field-block">
                <span>持续时间(秒)</span>
                <n-input-number v-model:value="ruleForm.durationSeconds" :min="0" style="width: 100%" class="dark-input" />
              </div>
              <div class="field-block field-block--switch">
                <span>状态</span>
                <n-switch v-model:value="ruleForm.enabled" />
              </div>
            </div>
            <div class="rule-actions">
              <n-button type="primary" class="glow-btn" :loading="savingRule" @click="handleSubmitRule" style="width: 100%">
                {{ editingRuleId ? '保存规则更改' : '+ 添加监控规则' }}
              </n-button>
            </div>
          </div>

          <div v-if="canManageInfrastructure" class="bento-card form-card">
            <div class="card-title-bar">
              <span class="card-title">通知通道</span>
              <n-tag size="small" :type="notificationSettings.enabled ? 'success' : 'default'" bordered>
                {{ notificationSettings.enabled ? '已启用' : '已关闭' }}
              </n-tag>
            </div>

            <div class="rule-grid">
              <div class="field-block field-block--switch">
                <span>Webhook 通知</span>
                <n-switch v-model:value="notificationSettings.enabled" :disabled="!canManageInfrastructure" />
              </div>
              <div class="field-block" style="grid-column: span 2">
                <span>Webhook 地址</span>
                <n-input
                  v-model:value="notificationSettings.webhookURL"
                  placeholder="https://hooks.example.com/alerts"
                  :disabled="!canManageInfrastructure || notificationSettings.clearWebhookURL"
                />
                <small v-if="notificationSettings.webhookConfigured && !notificationSettings.webhookURL && !notificationSettings.clearWebhookURL" class="field-hint">
                  已保存地址，当前为安全起见不回显明文。
                </small>
              </div>
              <div class="field-block field-block--switch">
                <span>清空已保存地址</span>
                <n-switch v-model:value="notificationSettings.clearWebhookURL" :disabled="!canManageInfrastructure" />
              </div>
              <div class="field-block">
                <span>超时(秒)</span>
                <n-input-number
                  v-model:value="notificationSettings.webhookTimeoutSeconds"
                  :min="1"
                  style="width: 100%"
                  class="dark-input"
                  :disabled="!canManageInfrastructure"
                />
              </div>
            </div>
            <p class="notification-hint">静默中的告警和维护窗口内的告警不会发送通知。</p>
            <div v-if="canManageInfrastructure" class="rule-actions">
              <n-button
                type="primary"
                class="glow-btn"
                :loading="savingNotificationSettings"
                @click="handleSubmitNotificationSettings"
                style="width: 100%"
              >
                保存通知设置
              </n-button>
            </div>
          </div>

          <div class="bento-card rules-list-card">
            <div class="card-title-bar">
              <span class="card-title">规则列表 ({{ rules.length }})</span>
            </div>

            <div v-if="rules.length === 0" class="empty-state">
              暂未配置任何监控规则。
            </div>

            <div class="rules-list">
              <div v-for="rule in rules" :key="rule.id" class="rule-item">
                <div class="rule-info">
                  <div class="rule-condition">
                    <n-tag size="small" type="info" bordered class="metric-tag">{{ metricLabel(rule.metric) }}</n-tag>
                    <span class="operator">{{ operatorLabel(rule.operator) }}</span>
                    <strong class="threshold">{{ rule.threshold }}</strong>
                    <span class="duration">(持续 {{ rule.durationSeconds }}s)</span>
                  </div>
                  <div class="rule-status">
                    <div class="status-dot-inline" :style="{ background: rule.enabled ? '#10b981' : '#71717a' }"></div>
                    {{ rule.enabled ? '运行中' : '已停用' }}
                  </div>
                </div>
                <div class="rule-btn" v-if="canManageInfrastructure">
                  <n-button size="small" ghost @click="startEditRule(rule)">修改</n-button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
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

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  margin-bottom: 32px;
}

.header-text h1 {
  font-size: 32px;
  font-weight: 600;
  margin: 0 0 8px 0;
  color: #fff;
  letter-spacing: -0.5px;
}

.header-text p {
  margin: 0;
  color: #a1a1aa;
  font-size: 16px;
}

.header-actions {
  display: flex;
  gap: 16px;
  align-items: center;
}

.layout-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.25fr) minmax(420px, 1fr);
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

.card-title-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
  padding-bottom: 12px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.card-title {
  font-size: 16px;
  font-weight: 600;
  color: #fff;
}

.alerts-panel {
  display: flex;
  flex-direction: column;
}

.timeline-container {
  display: flex;
  flex-direction: column;
  gap: 16px;
  margin-top: 8px;
}

.alert-card {
  display: flex;
  gap: 16px;
}

.alert-icon {
  width: 46px;
  height: 46px;
  border-radius: 50%;
  background: #18181b;
  border: 2px solid #3f3f46;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  font-weight: 700;
  flex-shrink: 0;
}

.alert-icon.critical {
  border-color: #f43f5e;
  color: #f43f5e;
  box-shadow: 0 0 15px rgba(244, 63, 94, 0.2);
}

.alert-icon.warning {
  border-color: #f59e0b;
  color: #f59e0b;
  box-shadow: 0 0 15px rgba(245, 158, 11, 0.2);
}

.history-icon {
  border-color: #3b82f6;
  color: #3b82f6;
  box-shadow: 0 0 15px rgba(59, 130, 246, 0.2);
}

.alert-content {
  flex: 1;
  background: rgba(0, 0, 0, 0.2);
  border: 1px solid rgba(255, 255, 255, 0.05);
  border-radius: 16px;
  padding: 18px;
}

.history-card .alert-content {
  opacity: 0.92;
}

.alert-header {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 8px;
}

.alert-server {
  font-weight: 600;
  color: #fff;
  font-size: 15px;
  cursor: pointer;
}

.alert-server:hover {
  color: #10b981;
}

.alert-time {
  font-size: 13px;
  color: #71717a;
}

.alert-message {
  color: #d4d4d8;
  font-size: 14px;
  line-height: 1.6;
  margin-bottom: 14px;
}

.alert-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 14px;
}

.meta-text {
  color: #a1a1aa;
  font-size: 12px;
}

.alert-footer {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: center;
  border-top: 1px solid rgba(255, 255, 255, 0.05);
  padding-top: 14px;
}

.inline-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  align-items: center;
}

:deep(.mute-select) {
  width: 110px;
}

.rules-panel {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.rule-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
}

.field-block {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.field-block span {
  color: #d4d4d8;
  font-size: 13px;
  font-weight: 500;
}

.field-block--switch {
  grid-column: span 2;
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
  margin-top: 8px;
}

.rule-actions {
  margin-top: 24px;
}

.notification-hint {
  margin: 16px 0 0;
  color: #a1a1aa;
  font-size: 12px;
  line-height: 1.6;
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

:deep(.dark-input .n-input__border),
:deep(.dark-input .n-base-selection__border) {
  border-color: rgba(255, 255, 255, 0.1);
}

:deep(.dark-input .n-input__placeholder),
:deep(.dark-input .n-base-selection-placeholder) {
  color: #71717a;
}

.rules-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.rule-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  padding: 16px;
  background: rgba(255, 255, 255, 0.02);
  border: 1px solid rgba(255, 255, 255, 0.05);
  border-radius: 12px;
}

.rule-info {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.rule-condition {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.metric-tag {
  background: transparent;
  color: #3b82f6;
  border-color: rgba(59, 130, 246, 0.3);
}

.operator {
  color: #a1a1aa;
  font-size: 13px;
}

.threshold {
  color: #fff;
  font-size: 16px;
}

.duration {
  color: #71717a;
  font-size: 13px;
}

.rule-status {
  display: flex;
  align-items: center;
  font-size: 12px;
  color: #a1a1aa;
}

.status-dot-inline {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  margin-right: 6px;
}

.empty-state {
  color: #71717a;
  padding: 32px 0;
  text-align: center;
}

@media (max-width: 1024px) {
  .layout-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 640px) {
  .page-header {
    flex-direction: column;
    align-items: flex-start;
    gap: 16px;
  }

  .alert-header,
  .alert-footer {
    flex-direction: column;
    align-items: flex-start;
  }

  .rule-grid {
    grid-template-columns: 1fr;
  }

  .field-block--switch {
    grid-column: span 1;
  }
}
</style>
