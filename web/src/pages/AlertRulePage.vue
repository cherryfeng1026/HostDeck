<script setup lang="ts">
import {
  NButton,
  NInput,
  NInputNumber,
  NScrollbar,
  NSelect,
  NSwitch,
  NTag,
  useDialog,
  useMessage,
} from 'naive-ui'
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useAutoRefresh } from '../auto-refresh'
import { loadAlerts, useAlertCache } from '../dashboard-cache'
import { createAlertRule, deleteAlertRule, updateAlertRule } from '../services/api'
import { useSession } from '../session'
import type { AlertRule } from '../types'

const message = useMessage()
const dialog = useDialog()
const router = useRouter()
const { canManageInfrastructure } = useSession()
const alertCache = useAlertCache()

const rules = computed(() => alertCache.rules)
const loading = computed(() => alertCache.loading)
const enabledRuleCount = computed(() => rules.value.filter((rule) => rule.enabled).length)
const disabledRuleCount = computed(() => Math.max(0, rules.value.length - enabledRuleCount.value))
const savingRule = ref(false)
const deletingRuleId = ref<number | null>(null)
const editingRuleId = ref<number | null>(null)

const ruleForm = reactive({
  metric: 'online',
  operator: 'eq',
  threshold: 0,
  durationSeconds: 60,
  enabled: true,
  scopeType: 'all',
  scopeValue: '',
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

const scopeOptions = [
  { label: '全部服务器', value: 'all' },
  { label: '指定服务器 ID', value: 'server' },
  { label: '服务器标签', value: 'tag' },
  { label: '服务器用途', value: 'purpose' },
]

const formTitle = computed(() => (editingRuleId.value ? '编辑规则' : '新增规则'))

async function loadData(force = false, silent = false) {
  try {
    await loadAlerts(canManageInfrastructure.value, { force, silent })
  } catch (error) {
    if (!silent) {
      message.error(error instanceof Error ? error.message : '加载告警规则失败')
    }
  }
}

async function handleSubmitRule() {
  savingRule.value = true
  try {
    const payload = {
      ...ruleForm,
      scopeValue: ruleForm.scopeType === 'all' ? '' : ruleForm.scopeValue.trim(),
    }
    if (editingRuleId.value) {
      await updateAlertRule(editingRuleId.value, payload)
      message.success('告警规则已更新')
    } else {
      await createAlertRule(payload)
      message.success('告警规则已创建')
    }
    resetRuleForm()
    await loadData(true)
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
  ruleForm.scopeType = rule.scopeType || 'all'
  ruleForm.scopeValue = rule.scopeValue || ''
}

function resetRuleForm() {
  editingRuleId.value = null
  ruleForm.metric = 'online'
  ruleForm.operator = 'eq'
  ruleForm.threshold = 0
  ruleForm.durationSeconds = 60
  ruleForm.enabled = true
  ruleForm.scopeType = 'all'
  ruleForm.scopeValue = ''
}

function handleDeleteRule(rule: AlertRule) {
  dialog.warning({
    title: '删除告警规则',
    content: `确认删除「${metricLabel(rule.metric)} ${operatorLabel(rule.operator)} ${rule.threshold}」？关联的当前告警会转入历史记录。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      deletingRuleId.value = rule.id
      try {
        await deleteAlertRule(rule.id)
        if (editingRuleId.value === rule.id) {
          resetRuleForm()
        }
        await loadData(true)
        message.success('告警规则已删除')
      } catch (error) {
        message.error(error instanceof Error ? error.message : '删除告警规则失败')
      } finally {
        deletingRuleId.value = null
      }
    },
  })
}

function metricLabel(metric: string) {
  return metricOptions.find((item) => item.value === metric)?.label || metric
}

function operatorLabel(operator: string) {
  return operatorOptions.find((item) => item.value === operator)?.label || operator
}

function scopeLabel(rule: AlertRule) {
  switch (rule.scopeType || 'all') {
    case 'server':
      return `服务器 #${rule.scopeValue}`
    case 'tag':
      return `标签 ${rule.scopeValue}`
    case 'purpose':
      return `用途 ${rule.scopeValue}`
    default:
      return '全部服务器'
  }
}

function formatDateTime(value?: string) {
  if (!value) return '-'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

function handleManualRefresh() {
  void loadData(true)
}

watch(canManageInfrastructure, () => {
  void loadData(true, alertCache.initialized)
})

useAutoRefresh(() => loadData(false, true), 30000)

onMounted(() => {
  void loadData(false, alertCache.initialized)
})
</script>

<template>
  <n-scrollbar class="content-scroll">
    <div class="page-container alert-rule-page">
      <div class="page-header">
        <div class="header-text">
          <h1>告警规则</h1>
        </div>
        <div class="header-actions">
          <n-button ghost @click="router.push('/alerts')">返回告警</n-button>
          <n-button ghost :loading="loading" @click="handleManualRefresh">刷新</n-button>
          <n-tag v-if="!canManageInfrastructure" type="default" bordered>只读</n-tag>
        </div>
      </div>

      <section class="rule-summary-strip">
        <div class="summary-cell info">
          <span>规则总数</span>
          <strong>{{ rules.length }}</strong>
          <small>全部规则</small>
        </div>
        <div class="summary-cell success">
          <span>已启用</span>
          <strong>{{ enabledRuleCount }}</strong>
          <small>参与计算</small>
        </div>
        <div class="summary-cell muted">
          <span>已停用</span>
          <strong>{{ disabledRuleCount }}</strong>
          <small>保留配置</small>
        </div>
      </section>

      <div class="rules-workspace">
        <section class="rule-panel rule-form-panel">
          <div class="panel-head">
            <div>
              <strong>{{ formTitle }}</strong>
              <span>{{ canManageInfrastructure ? '修改后立即影响后续采集评估' : '当前账号只能查看规则' }}</span>
            </div>
            <n-button v-if="editingRuleId" text size="small" @click="resetRuleForm">取消编辑</n-button>
          </div>

          <div v-if="!canManageInfrastructure" class="empty-state compact">
            当前账号没有维护告警规则的权限。
          </div>
          <div v-else class="rule-grid">
            <div class="field-block">
              <span>指标</span>
              <n-select v-model:value="ruleForm.metric" :options="metricOptions" class="dark-input" />
            </div>
            <div class="field-block">
              <span>条件</span>
              <n-select v-model:value="ruleForm.operator" :options="operatorOptions" class="dark-input" />
            </div>
            <div class="field-block">
              <span>阈值</span>
              <n-input-number v-model:value="ruleForm.threshold" style="width: 100%" class="dark-input" />
            </div>
            <div class="field-block">
              <span>持续秒数</span>
              <n-input-number v-model:value="ruleForm.durationSeconds" :min="0" style="width: 100%" class="dark-input" />
            </div>
            <div class="field-block">
              <span>范围</span>
              <n-select v-model:value="ruleForm.scopeType" :options="scopeOptions" class="dark-input" />
            </div>
            <div v-if="ruleForm.scopeType !== 'all'" class="field-block">
              <span>{{ ruleForm.scopeType === 'server' ? '服务器 ID' : ruleForm.scopeType === 'tag' ? '标签' : '用途' }}</span>
              <n-input v-model:value="ruleForm.scopeValue" placeholder="输入匹配值" class="dark-input" />
            </div>
            <div class="field-block switch-row">
              <span>启用</span>
              <n-switch v-model:value="ruleForm.enabled" />
            </div>
            <n-button type="primary" class="save-button" :loading="savingRule" @click="handleSubmitRule">
              {{ editingRuleId ? '保存规则' : '添加规则' }}
            </n-button>
          </div>
        </section>

        <section class="rule-panel rule-table-panel">
          <div class="panel-head">
            <div>
              <strong>规则列表</strong>
              <span>{{ rules.length ? `${enabledRuleCount} 条启用，${disabledRuleCount} 条停用` : '暂无规则' }}</span>
            </div>
          </div>

          <div v-if="rules.length === 0" class="empty-state">暂无告警规则。</div>
          <div v-else class="rule-table-wrap">
            <table class="rule-table">
              <colgroup>
                <col style="width: 136px" />
                <col style="width: 110px" />
                <col style="width: 152px" />
                <col style="width: 92px" />
                <col style="width: 86px" />
                <col style="width: 148px" />
                <col style="width: 104px" />
              </colgroup>
              <thead>
                <tr>
                  <th>指标</th>
                  <th>条件</th>
                  <th>范围</th>
                  <th>持续</th>
                  <th>状态</th>
                  <th>更新时间</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="rule in rules" :key="rule.id" :class="{ selected: editingRuleId === rule.id }">
                  <td>
                    <div class="rule-name">
                      <span class="metric-dot" :class="rule.metric" />
                      <strong>{{ metricLabel(rule.metric) }}</strong>
                    </div>
                  </td>
                  <td>
                    <span class="condition-text">{{ operatorLabel(rule.operator) }} {{ rule.threshold }}</span>
                  </td>
                  <td class="muted-cell">{{ scopeLabel(rule) }}</td>
                  <td class="muted-cell">{{ rule.durationSeconds }}s</td>
                  <td>
                    <n-tag size="small" :type="rule.enabled ? 'success' : 'default'" bordered>
                      {{ rule.enabled ? '启用' : '停用' }}
                    </n-tag>
                  </td>
                  <td class="muted-cell">{{ formatDateTime(rule.updatedAt) }}</td>
                  <td>
                    <div class="icon-actions">
                      <button
                        v-if="canManageInfrastructure"
                        type="button"
                        class="icon-action"
                        title="编辑"
                        aria-label="编辑告警规则"
                        @click="startEditRule(rule)"
                      >
                        <svg viewBox="0 0 24 24"><path d="M12 20h9" /><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z" /></svg>
                      </button>
                      <button
                        v-if="canManageInfrastructure"
                        type="button"
                        class="icon-action danger"
                        title="删除"
                        aria-label="删除告警规则"
                        :disabled="deletingRuleId === rule.id"
                        @click="handleDeleteRule(rule)"
                      >
                        <svg viewBox="0 0 24 24"><path d="M3 6h18" /><path d="M8 6V4h8v2" /><path d="M19 6l-1 14H6L5 6" /><path d="M10 11v5" /><path d="M14 11v5" /></svg>
                      </button>
                      <span v-if="!canManageInfrastructure" class="muted-cell">只读</span>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
      </div>
    </div>
  </n-scrollbar>
</template>

<style scoped>
.content-scroll {
  flex: 1;
}

.alert-rule-page {
  max-width: 100%;
  margin: 0;
}

.rule-summary-strip {
  min-height: 82px;
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  margin-bottom: 14px;
  overflow: hidden;
  border: 1px solid rgba(93, 120, 162, 0.22);
  border-radius: 8px;
  background: var(--app-panel);
}

.summary-cell {
  display: grid;
  align-content: center;
  gap: 5px;
  padding: 14px 18px;
}

.summary-cell + .summary-cell {
  border-left: 1px solid rgba(93, 120, 162, 0.16);
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

.summary-cell.info strong {
  color: var(--app-accent);
}

.summary-cell.success strong {
  color: #35d6a3;
}

.summary-cell.muted strong {
  color: var(--app-text-soft);
}

.rules-workspace {
  display: grid;
  grid-template-columns: minmax(300px, 360px) minmax(0, 1fr);
  gap: 14px;
  align-items: start;
}

.rule-panel {
  min-width: 0;
  border: 1px solid rgba(93, 120, 162, 0.22);
  background: var(--app-panel);
  border-radius: 8px;
  padding: 14px;
}

.rule-form-panel {
  position: sticky;
  top: 78px;
}

.panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
  padding-bottom: 12px;
  border-bottom: 1px solid rgba(93, 120, 162, 0.16);
}

.panel-head strong {
  display: block;
  color: var(--app-text);
  font-size: 15px;
}

.panel-head span {
  display: block;
  margin-top: 3px;
  color: var(--app-text-soft);
  font-size: 12px;
}

.rule-grid {
  display: grid;
  gap: 12px;
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

.rule-table-wrap {
  overflow: auto;
  border: 1px solid rgba(93, 120, 162, 0.16);
  border-radius: 8px;
}

.rule-table {
  width: 100%;
  min-width: 828px;
  border-collapse: collapse;
  table-layout: fixed;
}

.rule-table th,
.rule-table td {
  padding: 11px 10px;
  border-bottom: 1px solid rgba(93, 120, 162, 0.12);
  text-align: left;
  vertical-align: middle;
}

.rule-table th {
  color: var(--app-text-soft);
  background: rgba(17, 32, 52, 0.52);
  font-size: 12px;
  font-weight: 600;
}

.rule-table td {
  color: #dce7f6;
  font-size: 12px;
}

.rule-table tbody tr:hover td {
  background: rgba(79, 131, 255, 0.07);
}

.rule-table tbody tr.selected td {
  background: rgba(32, 212, 255, 0.08);
}

.rule-table tbody tr:last-child td {
  border-bottom: 0;
}

.rule-name {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.rule-name strong {
  overflow: hidden;
  color: var(--app-text);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.metric-dot {
  width: 8px;
  height: 8px;
  flex: 0 0 auto;
  border-radius: 50%;
  background: var(--app-accent);
  box-shadow: 0 0 12px rgba(32, 212, 255, 0.5);
}

.metric-dot.online {
  background: var(--app-danger);
  box-shadow: 0 0 12px rgba(255, 107, 125, 0.5);
}

.condition-text {
  color: var(--app-text);
  font-weight: 700;
}

.muted-cell {
  color: var(--app-text-soft) !important;
}

.icon-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.icon-action {
  width: 32px;
  height: 32px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(93, 120, 162, 0.22);
  border-radius: 8px;
  background: rgba(12, 24, 42, 0.72);
  color: var(--app-text-soft);
  cursor: pointer;
  transition: background-color 0.16s ease, border-color 0.16s ease, color 0.16s ease, transform 0.08s ease;
}

.icon-action:hover:not(:disabled) {
  border-color: rgba(79, 131, 255, 0.42);
  background: rgba(79, 131, 255, 0.12);
  color: var(--app-text);
}

.icon-action:active:not(:disabled) {
  transform: translateY(1px);
}

.icon-action:focus-visible {
  outline: 2px solid rgba(79, 131, 255, 0.72);
  outline-offset: 2px;
}

.icon-action:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.icon-action.danger:hover:not(:disabled) {
  border-color: rgba(255, 107, 125, 0.48);
  background: rgba(255, 107, 125, 0.12);
  color: var(--app-danger);
}

.icon-action svg {
  width: 17px;
  height: 17px;
  fill: none;
  stroke: currentColor;
  stroke-width: 1.8;
  stroke-linecap: round;
  stroke-linejoin: round;
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

@media (max-width: 980px) {
  .rule-summary-strip,
  .rules-workspace {
    grid-template-columns: 1fr;
  }

  .summary-cell + .summary-cell {
    border-left: 0;
    border-top: 1px solid rgba(93, 120, 162, 0.12);
  }

  .rule-form-panel {
    position: static;
  }
}
</style>
