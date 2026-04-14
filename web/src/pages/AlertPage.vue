<script setup lang="ts">
import {
  NButton,
  NInputNumber,
  NScrollbar,
  NSelect,
  NSwitch,
  NTag,
  useMessage,
} from 'naive-ui'
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { createAlertRule, getAlertRules, getAlerts, updateAlertRule } from '../services/api'
import { useSession } from '../session'
import type { AlertEvent, AlertRule } from '../types'

const message = useMessage()
const router = useRouter()
const { canManageInfrastructure } = useSession()
const alerts = ref<AlertEvent[]>([])
const rules = ref<AlertRule[]>([])
const loading = ref(false)
const savingRule = ref(false)
const editingRuleId = ref<number | null>(null)

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

async function loadData() {
  loading.value = true
  try {
    const [alertList, ruleList] = await Promise.all([getAlerts(), getAlertRules()])
    alerts.value = alertList
    rules.value = ruleList
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载告警数据失败')
  } finally {
    loading.value = false
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

function formatDateTime(value: string) {
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
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
          <p>实时监控并追踪所有的警告与错误，管理自定义告警规则。</p>
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
        <!-- 实时告警区域 -->
        <div class="bento-card alerts-panel">
          <div class="card-title-bar">
            <span class="card-title">当前活动告警 ({{ alerts.length }})</span>
          </div>
          
          <div v-if="alerts.length === 0" class="empty-state">
            当前系统运行平稳，无活动告警。
          </div>
          
          <div v-else class="timeline-container">
            <div v-for="alert in alerts" :key="`${alert.ruleId}-${alert.serverId}`" class="alert-card">
              <div class="alert-icon" :class="alert.severity">
                <span v-if="alert.severity === 'critical'">!</span>
                <span v-else>i</span>
              </div>
              <div class="alert-content">
                <div class="alert-header">
                  <span class="alert-server" @click="router.push(`/servers/${alert.serverId}`)">{{ alert.serverName }}</span>
                  <span class="alert-time">{{ formatDateTime(alert.triggeredAt) }}</span>
                </div>
                <div class="alert-message">{{ alert.message }}</div>
                <div class="alert-footer">
                  <n-tag :type="alert.severity === 'critical' ? 'error' : 'warning'" size="small" bordered>
                    {{ alert.severity === 'critical' ? '严重' : '警告' }}
                  </n-tag>
                  <n-button size="small" ghost type="primary" class="ml-auto" @click="router.push(`/servers/${alert.serverId}`)">
                    查看服务器
                  </n-button>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 规则管理区域 -->
        <div class="rules-panel">
          
          <!-- 编辑规则表单 -->
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

          <!-- 已有规则列表 -->
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
                <div class="rule-btn">
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
  grid-template-columns: minmax(0, 1.2fr) minmax(400px, 1fr);
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

/* 告警列表区 */
.alerts-panel {
  display: flex;
  flex-direction: column;
}

.timeline-container {
  display: flex;
  flex-direction: column;
  gap: 20px;
  position: relative;
}

.timeline-container::before {
  content: '';
  position: absolute;
  top: 0;
  bottom: 0;
  left: 24px;
  width: 2px;
  background: rgba(255, 255, 255, 0.05);
  z-index: 0;
}

.alert-card {
  display: flex;
  gap: 24px;
  position: relative;
  z-index: 1;
}

.alert-icon {
  width: 50px;
  height: 50px;
  border-radius: 50%;
  background: #18181b;
  border: 2px solid #3f3f46;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  font-weight: bold;
  flex-shrink: 0;
  box-shadow: 0 0 0 4px #050505;
}
.alert-icon.critical { border-color: #f43f5e; color: #f43f5e; box-shadow: 0 0 15px rgba(244, 63, 94, 0.2); }
.alert-icon.warning { border-color: #f59e0b; color: #f59e0b; box-shadow: 0 0 15px rgba(245, 158, 11, 0.2); }

.alert-content {
  flex: 1;
  background: rgba(0, 0, 0, 0.2);
  border: 1px solid rgba(255, 255, 255, 0.05);
  border-radius: 16px;
  padding: 20px;
  transition: transform 0.2s;
}
.alert-card:hover .alert-content {
  border-color: rgba(255, 255, 255, 0.15);
}

.alert-header {
  display: flex;
  justify-content: space-between;
  margin-bottom: 8px;
}

.alert-server {
  font-weight: 600;
  color: #fff;
  font-size: 15px;
  cursor: pointer;
}
.alert-server:hover { color: #10b981; }

.alert-time {
  font-size: 13px;
  color: #71717a;
}

.alert-message {
  color: #d4d4d8;
  font-size: 14px;
  line-height: 1.6;
  margin-bottom: 16px;
}

.alert-footer {
  display: flex;
  align-items: center;
  border-top: 1px solid rgba(255, 255, 255, 0.05);
  padding-top: 16px;
}
.ml-auto {
  margin-left: auto;
}

/* 规则区域 */
.rules-panel {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.form-card {
  padding-bottom: 24px;
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

/* 规则列表 */
.rules-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.rule-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px;
  background: rgba(255, 255, 255, 0.02);
  border: 1px solid rgba(255, 255, 255, 0.05);
  border-radius: 12px;
  transition: border-color 0.2s;
}
.rule-item:hover {
  border-color: rgba(255, 255, 255, 0.15);
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
  padding: 24px 0;
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
}
</style>
