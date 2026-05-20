<script setup lang="ts">
import {
  NButton,
  NDataTable,
  NDatePicker,
  NEmpty,
  NInput,
  NInputNumber,
  NModal,
  NPopconfirm,
  NRadio,
  NRadioGroup,
  NScrollbar,
  NSelect,
  NSpin,
  NTag,
  useMessage,
} from 'naive-ui'
import { computed, h, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import {
  createCommandTemplate,
  executeCommand,
  executeCommands,
  getCommandHistory,
  getCommandTemplates,
  getServers,
  setCommandTemplateFavorite,
} from '../services/api'
import { useSession } from '../session'
import type { DataTableColumns } from 'naive-ui'
import type { BatchCommandResult, CommandHistoryRecord, CommandTemplate, ServerAsset } from '../types'

const route = useRoute()
const message = useMessage()
const { canManageInfrastructure, currentUser } = useSession()
const loadingTargets = ref(false)
const loadingTemplates = ref(false)
const creatingTemplate = ref(false)
const togglingFavorite = ref(false)
const executing = ref(false)
const loadingHistory = ref(false)
const servers = ref<ServerAsset[]>([])
const templates = ref<CommandTemplate[]>([])
const selectedServerIds = ref<number[]>([])
const selectedTemplateId = ref<string | null>(null)
const command = ref('df -h')
const timeoutSeconds = ref(15)
const results = ref<BatchCommandResult[]>([])
const historyItems = ref<CommandHistoryRecord[]>([])
const historyKeyword = ref('')
const historyServerId = ref<number | null>(null)
const historyExecutor = ref('')
const historyTimeRange = ref<[number, number] | null>(null)
const selectedHistory = ref<CommandHistoryRecord | null>(null)
const showCreateTemplateModal = ref(false)
const pendingRiskConfirmed = ref(false)
const templateValues = reactive<Record<string, string>>({})
const templateForm = reactive({
  name: '',
  description: '',
  command: '',
  scope: 'personal',
  riskLevel: 'normal',
})

function isEnabledServer(serverId: number) {
  return servers.value.some((server) => server.id === serverId && server.enabled)
}

const serverOptions = computed(() =>
  servers.value.map((server) => ({
    label: server.enabled ? `${server.name} · ${server.ip}` : `${server.name} · ${server.ip}（已禁用）`,
    value: server.id,
    disabled: !server.enabled,
  })),
)

const historyServerOptions = computed(() => [
  { label: '全部服务器', value: 0 },
  ...servers.value.map((server) => ({ label: `${server.name} · ${server.ip}`, value: server.id })),
])

const templateOptions = computed(() =>
  templates.value.map((template) => {
    const badges = [template.scope === 'personal' ? '个人' : '共享']
    if (template.isFavorite) {
      badges.unshift('收藏')
    }
    if (template.riskLevel === 'dangerous') {
      badges.push('高风险')
    }
    return {
      label: `${template.name} · ${badges.join(' · ')}`,
      value: template.id,
    }
  }),
)

const selectedTemplate = computed(() =>
  templates.value.find((template) => template.id === selectedTemplateId.value) ?? null,
)

const canCreateSharedTemplate = computed(() => canManageInfrastructure.value)

const resolvedCommand = computed(() => {
  if (!selectedTemplate.value) {
    return command.value
  }
  return selectedTemplate.value.variables.reduce((acc, variable) => {
    const value = (templateValues[variable.name] ?? variable.defaultValue ?? '').trim()
    return acc.replace(new RegExp(`\\{\\{${variable.name}\\}\\}`, 'g'), value)
  }, selectedTemplate.value.command)
})

const selectedSummary = computed(() => {
  const count = selectedServerIds.value.length
  if (count === 0) {
    return '未选择目标'
  }
  return `已选择 ${count} 个目标`
})

function hasDangerousRemoveCommand(value: string) {
  return /\brm\s+(?=[^\n;&|]*-[a-z]*r)(?=[^\n;&|]*-[a-z]*f)[^\n;&|]*/i.test(value)
}

function hasHighRiskCommand(value: string) {
  return (
    hasDangerousRemoveCommand(value) ||
    /\bshutdown\b|\breboot\b|\bhalt\b|\bpoweroff\b|\bmkfs(?:\.[a-z0-9]+)?\b|\bdd\s+.*\bof=\/dev\/|\bsystemctl\s+(?:stop|restart|disable)\b/i.test(value)
  )
}

const isHighRiskCommand = computed(() => hasHighRiskCommand(resolvedCommand.value))
const selectedExecutionRisk = computed(() => (selectedTemplate.value?.riskLevel === 'dangerous' || isHighRiskCommand.value ? 'dangerous' : 'normal'))
const selectedExecutionSource = computed(() => (selectedTemplate.value ? 'template' : 'custom'))
const requiresRiskConfirmation = computed(() => selectedExecutionRisk.value === 'dangerous' && !pendingRiskConfirmed.value)

const historyColumns: DataTableColumns<CommandHistoryRecord> = [
  {
    title: '执行时间',
    key: 'executedAt',
    render: (row) => h('span', { style: 'color: #dce7f6' }, formatDateTime(row.executedAt)),
  },
  {
    title: '服务器',
    key: 'serverName',
    render: (row) => h('strong', { style: 'color: #f3f7ff' }, row.serverName || `#${row.serverId}`),
  },
  {
    title: '执行人',
    key: 'executorUsername',
    render: (row) => h('span', { style: 'color: #aab8cd' }, row.executorUsername || '系统'),
  },
  {
    title: '命令',
    key: 'command',
    render: (row) => h('code', { style: 'color: #dce7f6; background: rgba(17,32,52,0.64); padding: 2px 6px; border-radius: 4px;' }, row.command),
  },
  {
    title: '风险',
    key: 'riskLevel',
    render: (row) =>
      h(
        NTag,
        {
          size: 'small',
          bordered: false,
          type: row.riskLevel === 'dangerous' ? 'warning' : 'info',
        },
        { default: () => (row.riskLevel === 'dangerous' ? (row.riskConfirmed ? '高风险已确认' : '高风险') : '普通') },
      ),
  },
  {
    title: '结果',
    key: 'exitCode',
    render: (row) =>
      h(
        NTag,
        {
          size: 'small',
          bordered: false,
          type: row.exitCode === 0 ? 'success' : 'warning',
        },
        { default: () => (row.exitCode === 0 ? '成功' : `退出码 ${row.exitCode}`) },
      ),
  },
  {
    title: '耗时',
    key: 'durationMs',
    render: (row) => h('span', { style: 'color: #aab8cd' }, `${row.durationMs} ms`),
  },
  {
    title: '操作',
    key: 'actions',
    render: (row) =>
      h(
        'div',
        { style: 'display:flex; gap:8px; justify-content:flex-end;' },
        [
          h(
            NButton,
            {
              size: 'small',
              ghost: true,
              onClick: () => viewHistory(row),
            },
            { default: () => '详情' },
          ),
          h(
            NButton,
            {
              size: 'small',
              type: 'primary',
              ghost: true,
              disabled: !canManageInfrastructure.value || !isEnabledServer(row.serverId),
              onClick: () => reuseHistory(row),
            },
            { default: () => '复用' },
          ),
          h(
            NButton,
            {
              size: 'small',
              ghost: true,
              disabled: !canManageInfrastructure.value || !isEnabledServer(row.serverId),
              onClick: () => void rerunHistory(row),
            },
            { default: () => '重跑' },
          ),
        ],
      ),
  },
]

function formatDateTime(value?: string) {
  if (!value) return '未记录'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

function applyTemplate(template: CommandTemplate | null) {
  if (!template) {
    return
  }
  for (const variable of template.variables) {
    templateValues[variable.name] = templateValues[variable.name] ?? variable.defaultValue ?? ''
  }
  command.value = template.command
}

function templateRiskLabel(template: CommandTemplate | null) {
  if (!template) {
    return ''
  }
  return template.riskLevel === 'dangerous' ? '高风险模板' : '巡检模板'
}

function templateScopeLabel(template: CommandTemplate | null) {
  if (!template) {
    return ''
  }
  return template.scope === 'personal' ? '个人模板' : '共享模板'
}

function resetTemplateForm() {
  templateForm.name = ''
  templateForm.description = ''
  templateForm.command = selectedTemplate.value?.command || command.value
  templateForm.scope = canCreateSharedTemplate.value ? 'personal' : 'personal'
  templateForm.riskLevel = selectedTemplate.value?.riskLevel || 'normal'
}

function openCreateTemplateModal() {
  if (!canManageInfrastructure.value) {
    message.warning('当前账号没有保存命令模板的权限')
    return
  }
  resetTemplateForm()
  showCreateTemplateModal.value = true
}

async function submitTemplate() {
  if (!templateForm.name.trim()) {
    message.warning('请输入模板名称')
    return
  }
  if (!templateForm.command.trim()) {
    message.warning('请输入模板命令')
    return
  }
  if (templateForm.scope === 'shared' && !canCreateSharedTemplate.value) {
    message.warning('当前账号没有创建共享模板的权限')
    return
  }
  creatingTemplate.value = true
  try {
    const created = await createCommandTemplate({
      name: templateForm.name.trim(),
      description: templateForm.description.trim(),
      command: templateForm.command.trim(),
      scope: templateForm.scope,
      riskLevel: templateForm.riskLevel,
    })
    await loadTemplates(created.id)
    showCreateTemplateModal.value = false
    message.success(templateForm.scope === 'shared' ? '共享模板已创建' : '个人模板已创建')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '创建命令模板失败')
  } finally {
    creatingTemplate.value = false
  }
}

async function toggleTemplateFavorite() {
  if (!selectedTemplate.value) {
    return
  }
  togglingFavorite.value = true
  const nextFavorite = !selectedTemplate.value.isFavorite
  try {
    await setCommandTemplateFavorite(selectedTemplate.value.id, nextFavorite)
    await loadTemplates(selectedTemplate.value.id)
    message.success(nextFavorite ? '已加入收藏' : '已取消收藏')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '更新模板收藏失败')
  } finally {
    togglingFavorite.value = false
  }
}

function missingTemplateVariables() {
  if (!selectedTemplate.value) {
    return []
  }
  return selectedTemplate.value.variables.filter((variable) => variable.required && !(templateValues[variable.name] ?? variable.defaultValue ?? '').trim())
}

function viewHistory(item: CommandHistoryRecord) {
  selectedHistory.value = item
}

function reuseHistory(item: CommandHistoryRecord) {
  selectedTemplateId.value = null
  command.value = item.command
  if (isEnabledServer(item.serverId)) {
    selectedServerIds.value = [item.serverId]
  }
  message.success('已将历史命令填充到执行面板')
}

async function rerunHistory(item: CommandHistoryRecord) {
  reuseHistory(item)
  if (!canManageInfrastructure.value) {
    message.warning('当前账号没有重新执行命令的权限')
    return
  }
  if (!isEnabledServer(item.serverId)) {
    message.warning('目标服务器已禁用，无法重新执行历史命令')
    return
  }
  await runCommand()
}

async function loadServers() {
  loadingTargets.value = true
  try {
    servers.value = await getServers()
    selectedServerIds.value = selectedServerIds.value.filter((serverId) => isEnabledServer(serverId))
    const preset = Number(route.query.serverId)
    if (Number.isFinite(preset) && preset > 0 && isEnabledServer(preset)) {
      selectedServerIds.value = [preset]
      historyServerId.value = preset
    } else if (selectedServerIds.value.length === 0) {
      const firstEnabledServer = servers.value.find((server) => server.enabled)
      if (firstEnabledServer) {
        selectedServerIds.value = [firstEnabledServer.id]
      }
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载服务器列表失败')
  } finally {
    loadingTargets.value = false
  }
}

async function loadTemplates(preferredTemplateId?: string | null) {
  loadingTemplates.value = true
  try {
    const response = await getCommandTemplates()
    templates.value = response.items
    const nextTemplateId = preferredTemplateId ?? selectedTemplateId.value
    if (nextTemplateId && templates.value.some((template) => template.id === nextTemplateId)) {
      selectedTemplateId.value = nextTemplateId
    } else if (selectedTemplateId.value && !templates.value.some((template) => template.id === selectedTemplateId.value)) {
      selectedTemplateId.value = null
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载命令模板失败')
  } finally {
    loadingTemplates.value = false
  }
}

async function loadHistory() {
  if (!canManageInfrastructure.value) {
    historyItems.value = []
    loadingHistory.value = false
    return
  }
  loadingHistory.value = true
  try {
    historyItems.value = await getCommandHistory({
      limit: 50,
      serverId: historyServerId.value || undefined,
      executorUsername: historyExecutor.value || undefined,
      keyword: historyKeyword.value || undefined,
      startTime: historyTimeRange.value ? new Date(historyTimeRange.value[0]).toISOString() : undefined,
      endTime: historyTimeRange.value ? new Date(historyTimeRange.value[1]).toISOString() : undefined,
    })
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载命令历史失败')
  } finally {
    loadingHistory.value = false
  }
}

async function runCommand() {
  if (requiresRiskConfirmation.value) {
    message.warning('高风险命令需要二次确认')
    return
  }
  const missingVariables = missingTemplateVariables()
  if (missingVariables.length > 0) {
    message.warning(`请先填写模板变量：${missingVariables.map((item) => item.label).join('、')}`)
    return
  }
  if (!resolvedCommand.value.trim()) {
    message.warning('请输入要执行的命令')
    return
  }
  if (selectedServerIds.value.length === 0) {
    message.warning('请至少选择一台服务器')
    return
  }
  if (selectedServerIds.value.some((serverId) => !isEnabledServer(serverId))) {
    selectedServerIds.value = selectedServerIds.value.filter((serverId) => isEnabledServer(serverId))
    message.warning('已禁用服务器不能执行命令，请重新确认目标')
    return
  }

  executing.value = true
  results.value = []
  try {
    const executionOptions = {
      source: selectedExecutionSource.value,
      templateId: selectedTemplate.value?.id,
      riskLevel: selectedExecutionRisk.value,
      riskConfirmed: pendingRiskConfirmed.value,
    }
    if (selectedServerIds.value.length === 1) {
      const serverId = selectedServerIds.value[0]
      const result = await executeCommand(serverId, resolvedCommand.value, timeoutSeconds.value || 15, executionOptions)
      const server = servers.value.find((item) => item.id === serverId)
      results.value = [
        {
          serverId,
          serverName: server?.name || `#${serverId}`,
          success: true,
          result,
        },
      ]
    } else {
      const response = await executeCommands(selectedServerIds.value, resolvedCommand.value, timeoutSeconds.value || 15, executionOptions)
      results.value = response.results
    }
    pendingRiskConfirmed.value = false
    const failedCount = results.value.filter((item) => !item.success).length
    if (failedCount === 0) {
      message.success('命令执行完成')
    } else if (failedCount === results.value.length) {
      message.error('命令未在任何目标上执行成功')
    } else {
      message.warning(`命令执行完成，但有 ${failedCount} 台服务器执行失败`)
    }
    await loadHistory()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '命令执行失败')
  } finally {
    executing.value = false
  }
}

async function confirmRiskAndRun() {
  pendingRiskConfirmed.value = true
  await runCommand()
}

function resultStatus(result: BatchCommandResult) {
  if (!result.success) {
    return { label: '失败', tone: 'error' }
  }
  if (result.result.exitCode === 0) {
    return { label: '成功', tone: 'success' }
  }
  return { label: `退出码 ${result.result.exitCode}`, tone: 'warning' }
}

watch(selectedTemplateId, () => {
  applyTemplate(selectedTemplate.value)
})

watch(
  () => route.query.serverId,
  (value) => {
    const preset = Number(value)
    if (Number.isFinite(preset) && preset > 0 && isEnabledServer(preset)) {
      selectedServerIds.value = [preset]
      historyServerId.value = preset
      void loadHistory()
    }
  },
)

onMounted(async () => {
  await Promise.all([loadServers(), loadTemplates()])
  historyExecutor.value = currentUser.value?.username || ''
  await loadHistory()
})
</script>

<template>
  <n-scrollbar class="content-scroll">
    <div class="page-container">
      <div class="page-header">
        <div class="header-text">
          <h1>远程终端</h1>
        </div>
      </div>

      <div class="layout-grid">
        <div class="control-panel">
          <div class="form-group">
            <label>目标服务器</label>
            <n-select
              v-model:value="selectedServerIds"
              multiple
              filterable
              :options="serverOptions"
              :loading="loadingTargets"
              placeholder="选择要执行命令的服务器..."
              class="dark-input"
            />
          </div>

          <div v-if="selectedTemplate && selectedTemplate.variables.length > 0" class="template-vars">
            <div v-for="variable in selectedTemplate.variables" :key="variable.name" class="form-group">
              <label>{{ variable.label }}</label>
              <n-input
                v-model:value="templateValues[variable.name]"
                :placeholder="variable.placeholder || variable.defaultValue || `请输入${variable.label}`"
                class="dark-input"
              />
            </div>
          </div>

          <div class="form-group command-block">
            <label>执行命令</label>
            <n-input
              v-model:value="command"
              type="textarea"
              placeholder="输入 Bash 脚本或命令..."
              :autosize="{ minRows: 6, maxRows: 12 }"
              class="code-input dark-input"
            />
            <div v-if="selectedTemplate" class="resolved-command">
              <span>最终命令预览</span>
              <code>{{ resolvedCommand }}</code>
            </div>

            <div class="execution-row">
              <div class="timeout-field">
                <label>超时 (秒)</label>
                <n-input-number v-model:value="timeoutSeconds" :min="1" :max="60" class="dark-input" />
              </div>

              <div class="action-bar">
                <n-popconfirm v-if="canManageInfrastructure && requiresRiskConfirmation" @positive-click="confirmRiskAndRun">
                  <template #trigger>
                    <n-button type="warning" size="large" class="glow-btn" :loading="executing">
                      {{ selectedServerIds.length > 1 ? '确认高风险批量执行' : '确认高风险执行' }}
                    </n-button>
                  </template>
                  高风险命令可能中断服务或修改系统状态，确认继续执行？
                </n-popconfirm>
                <n-button v-else-if="canManageInfrastructure" type="primary" size="large" class="glow-btn" :loading="executing" @click="runCommand">
                  {{ selectedServerIds.length > 1 ? '批量执行命令' : '执行命令' }}
                </n-button>
                <n-button v-else disabled size="large" class="glow-btn">
                  只读账号不可执行命令
                </n-button>
              </div>
            </div>
          </div>
        </div>

        <aside class="template-panel">
          <div class="results-header">
            <h3>命令模板</h3>
            <n-button ghost size="small" :disabled="!canManageInfrastructure" @click="openCreateTemplateModal">新增</n-button>
          </div>
          <n-select
            v-model:value="selectedTemplateId"
            clearable
            filterable
            :options="templateOptions"
            :loading="loadingTemplates"
            placeholder="搜索模板名称或描述"
            class="dark-input"
          />
          <div v-if="selectedTemplate" class="template-meta">
            <div class="template-meta__head">
              <strong>{{ selectedTemplate.name }}</strong>
              <div class="template-badges">
                <n-tag v-if="selectedTemplate.isFavorite" type="warning" size="small" :bordered="false">收藏</n-tag>
                <n-tag size="small" :bordered="false">{{ templateScopeLabel(selectedTemplate) }}</n-tag>
                <n-tag :type="selectedTemplate.riskLevel === 'dangerous' ? 'warning' : 'info'" size="small" :bordered="false">
                  {{ templateRiskLabel(selectedTemplate) }}
                </n-tag>
              </div>
            </div>
            <p>{{ selectedTemplate.description || '暂无模板描述' }}</p>
            <code>{{ selectedTemplate.command }}</code>
            <div class="template-toolbar">
              <n-button
                ghost
                size="small"
                :disabled="!canManageInfrastructure"
                :loading="togglingFavorite"
                @click="toggleTemplateFavorite"
              >
                {{ selectedTemplate.isFavorite ? '取消收藏' : '加入收藏' }}
              </n-button>
              <n-button ghost size="small" :disabled="!canManageInfrastructure" @click="openCreateTemplateModal">另存为模板</n-button>
            </div>
          </div>
          <div v-else-if="templates.length" class="template-list">
            <button
              v-for="template in templates.slice(0, 4)"
              :key="template.id"
              type="button"
              class="template-row"
              @click="selectedTemplateId = template.id"
            >
              <span>
                <strong>{{ template.name }}</strong>
                <small>{{ template.command }}</small>
              </span>
              <n-tag size="small" :type="template.riskLevel === 'dangerous' ? 'warning' : 'info'" :bordered="false">
                {{ template.riskLevel === 'dangerous' ? '服务治理' : '系统检查' }}
              </n-tag>
            </button>
          </div>
          <div v-else class="template-empty">
            暂无命令模板
          </div>
        </aside>

        <div class="results-panel">
          <div class="results-header">
            <h3>执行结果</h3>
            <n-tag type="info" size="small" bordered style="background: transparent; color: var(--app-text-soft); border-color: rgba(93,120,162,0.22)">
              {{ selectedSummary }}
            </n-tag>
          </div>

          <n-spin :show="executing">
            <div class="results-list" v-if="results.length > 0">
              <div v-for="result in results" :key="result.serverId" class="result-card">
                <div class="result-meta">
                  <span class="server-name">{{ result.serverName || `#${result.serverId}` }}</span>
                  <span class="status-badge" :class="resultStatus(result).tone">
                    {{ resultStatus(result).label }}
                  </span>
                </div>
                <div class="result-details">
                  <div class="detail-row">
                    <span>执行命令:</span>
                    <code>{{ result.result?.command || command }}</code>
                  </div>
                  <div class="detail-row">
                    <span>耗时:</span>
                    <span>{{ result.result?.durationMs || 0 }} ms</span>
                  </div>
                </div>
                <div class="terminal-output">
                  <pre><code>{{ result.result?.stdout || '（无标准输出）' }}</code></pre>
                  <pre v-if="result.result?.stderr" class="error-text"><code>{{ result.result.stderr }}</code></pre>
                  <pre v-if="result.error" class="error-text"><code>{{ result.error }}</code></pre>
                </div>
              </div>
            </div>
            <div v-else class="empty-terminal">
              <span class="cursor" aria-hidden="true" />
            </div>
          </n-spin>
        </div>

          <div class="history-card">
            <div class="results-header">
              <h3>命令历史</h3>
              <n-button ghost size="small" @click="loadHistory" :loading="loadingHistory" style="color: var(--app-text-soft); border-color: rgba(93,120,162,0.22)">
                刷新历史
              </n-button>
            </div>

            <div class="history-filters">
              <n-input v-model:value="historyKeyword" placeholder="搜索命令 / 服务器" class="dark-input" @keyup.enter="loadHistory" />
              <n-select
                v-model:value="historyServerId"
                :options="historyServerOptions"
                placeholder="全部服务器"
                class="dark-input"
                clearable
              />
              <n-input v-model:value="historyExecutor" placeholder="执行人" class="dark-input" @keyup.enter="loadHistory" />
              <n-date-picker
                v-model:value="historyTimeRange"
                type="datetimerange"
                clearable
                class="dark-input"
                start-placeholder="开始时间"
                end-placeholder="结束时间"
                :actions="['clear', 'confirm']"
                :update-value-on-close="true"
              />
              <n-button type="primary" ghost @click="loadHistory">筛选</n-button>
            </div>

            <n-data-table
              :columns="historyColumns"
              :data="historyItems"
              :loading="loadingHistory"
              :bordered="false"
              class="dark-table"
            />

            <div v-if="selectedHistory" class="history-detail">
              <div class="results-header">
                <h3>历史详情</h3>
                <n-button text @click="selectedHistory = null" style="color: var(--app-text-soft)">关闭</n-button>
              </div>
              <div class="result-details detail-stack">
                <div class="detail-row"><span>执行时间:</span><span>{{ formatDateTime(selectedHistory.executedAt) }}</span></div>
                <div class="detail-row"><span>服务器:</span><span>{{ selectedHistory.serverName || `#${selectedHistory.serverId}` }}</span></div>
                <div class="detail-row"><span>执行人:</span><span>{{ selectedHistory.executorUsername || '系统' }}</span></div>
                <div class="detail-row"><span>来源:</span><span>{{ selectedHistory.source === 'template' ? '命令模板' : '自定义命令' }}</span></div>
                <div class="detail-row"><span>风险:</span><span>{{ selectedHistory.riskLevel === 'dangerous' ? (selectedHistory.riskConfirmed ? '高风险已确认' : '高风险未确认') : '普通' }}</span></div>
                <div class="detail-row"><span>命令:</span><code>{{ selectedHistory.command }}</code></div>
              </div>
              <div class="terminal-output">
                <pre><code>{{ selectedHistory.stdout || '（无标准输出）' }}</code></pre>
                <pre v-if="selectedHistory.stderr" class="error-text"><code>{{ selectedHistory.stderr }}</code></pre>
              </div>
            </div>

            <n-empty v-if="!loadingHistory && historyItems.length === 0" description="暂无命令历史" style="padding: 32px 0" />
          </div>
      </div>
    </div>

    <n-modal v-model:show="showCreateTemplateModal" preset="card" title="保存命令模板" style="max-width: 640px">
      <div class="template-modal">
        <div class="form-group">
          <label>模板名称</label>
          <n-input v-model:value="templateForm.name" placeholder="例如：检查 nginx 状态" class="dark-input" />
        </div>
        <div class="form-group">
          <label>模板描述</label>
          <n-input v-model:value="templateForm.description" placeholder="简要说明用途和执行场景" class="dark-input" />
        </div>
        <div class="form-group">
          <label>模板范围</label>
          <n-radio-group v-model:value="templateForm.scope">
            <div class="scope-options">
              <n-radio value="personal">个人模板</n-radio>
              <n-radio value="shared" :disabled="!canCreateSharedTemplate">共享模板</n-radio>
            </div>
          </n-radio-group>
        </div>
        <div class="form-group">
          <label>风险级别</label>
          <n-radio-group v-model:value="templateForm.riskLevel">
            <div class="scope-options">
              <n-radio value="normal">普通</n-radio>
              <n-radio value="dangerous">高风险</n-radio>
            </div>
          </n-radio-group>
        </div>
        <div class="form-group">
          <label>模板命令</label>
          <n-input
            v-model:value="templateForm.command"
            type="textarea"
            :autosize="{ minRows: 5, maxRows: 10 }"
            placeholder="支持使用 {{service}} 这类变量占位符"
            class="code-input dark-input"
          />
        </div>
        <div class="modal-actions">
          <n-button ghost @click="showCreateTemplateModal = false">取消</n-button>
          <n-button type="primary" :loading="creatingTemplate" @click="submitTemplate">保存模板</n-button>
        </div>
      </div>
    </n-modal>
  </n-scrollbar>
</template>

<style scoped>
.content-scroll {
  flex: 1;
}

.page-container {
  max-width: 100%;
  margin: 0;
}

.page-header {
  margin-bottom: 18px;
}

.header-text h1 {
  font-size: 22px;
  font-weight: 700;
  margin: 0;
  color: var(--app-text);
  letter-spacing: 0;
}

.header-text p {
  margin: 0;
  color: var(--app-text-soft);
  font-size: 16px;
}

.layout-grid {
  display: grid;
  grid-template-columns: minmax(430px, 1.05fr) minmax(280px, 0.62fr) minmax(360px, 0.9fr);
  gap: 14px;
}

.control-panel,
.template-panel {
  background: var(--app-panel);
  border: 1px solid rgba(93, 120, 162, 0.22);
  border-radius: 8px;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 14px;
  align-self: start;
}

.form-group label {
  display: block;
  font-size: 13px;
  font-weight: 500;
  color: var(--app-text);
  margin-bottom: 8px;
}

.section-title {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 8px;
}

.section-title label {
  margin-bottom: 0;
}

.template-toolbar {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.template-meta {
  margin-top: 10px;
  padding: 12px;
  border-radius: 8px;
  background: rgba(17, 32, 52, 0.46);
  border: 1px solid rgba(93, 120, 162, 0.14);
}

.template-meta__head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}

.template-badges {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.template-meta strong {
  color: var(--app-text);
}

.template-meta p {
  margin: 8px 0 0;
  color: var(--app-text-soft);
  font-size: 13px;
}

.template-meta code {
  display: block;
  margin-top: 12px;
  padding: 10px;
  border-radius: 8px;
  border: 1px solid rgba(93, 120, 162, 0.16);
  background: rgba(0, 7, 16, 0.48);
  color: #dce7f6;
  font-family: var(--app-font-mono);
  font-size: 12px;
  line-height: 1.55;
  white-space: pre-wrap;
  word-break: break-word;
}

.template-empty {
  min-height: 170px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px dashed rgba(93, 120, 162, 0.22);
  border-radius: 8px;
  color: var(--app-text-faint);
  font-size: 13px;
}

.template-list {
  display: grid;
  gap: 10px;
}

.template-row {
  width: 100%;
  min-height: 64px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 10px 12px;
  border: 1px solid rgba(93, 120, 162, 0.16);
  border-radius: 8px;
  background: rgba(17, 32, 52, 0.42);
  color: var(--app-text);
  text-align: left;
  cursor: pointer;
}

.template-row:hover {
  border-color: rgba(79, 131, 255, 0.42);
  background: rgba(79, 131, 255, 0.1);
}

.template-row span {
  min-width: 0;
  display: grid;
  gap: 5px;
}

.template-row strong,
.template-row small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.template-row strong {
  color: var(--app-text);
  font-size: 13px;
}

.template-row small {
  color: var(--app-text-soft);
  font-family: var(--app-font-mono);
  font-size: 12px;
}

.template-vars {
  display: grid;
  gap: 14px;
}

.resolved-command {
  margin-top: 10px;
  display: grid;
  gap: 8px;
}

.resolved-command span {
  color: var(--app-text-soft);
  font-size: 12px;
}

.resolved-command code {
  color: #dce7f6;
  background: rgba(17, 32, 52, 0.64);
  padding: 10px 12px;
  border-radius: 8px;
  white-space: pre-wrap;
  word-break: break-all;
}

.action-bar {
  display: flex;
  min-width: 180px;
  flex: 1;
}

.command-block {
  display: grid;
  gap: 12px;
}

.execution-row {
  display: flex;
  align-items: end;
  gap: 12px;
  padding-top: 2px;
}

.timeout-field {
  width: 132px;
  flex-shrink: 0;
}

.timeout-field label {
  margin-bottom: 8px;
}

.glow-btn {
  box-shadow: none;
  transition: background-color 0.2s ease, border-color 0.2s ease;
  background-color: var(--app-accent-strong) !important;
  color: #fff !important;
  border: none;
  flex: 1;
}

.glow-btn:hover:not(:disabled) {
  box-shadow: none;
  background-color: #2f72ff !important;
  transform: none;
}

.glow-btn:disabled {
  background-color: rgba(93, 120, 162, 0.14) !important;
  box-shadow: none;
  color: var(--app-text-soft) !important;
  cursor: not-allowed;
}

:deep(.dark-input .n-input__border),
:deep(.dark-input .n-base-selection__border) {
  border-color: rgba(93, 120, 162, 0.24);
}

:deep(.dark-input .n-input__placeholder),
:deep(.dark-input .n-base-selection-placeholder) {
  color: var(--app-text-faint);
}

:deep(.code-input .n-input__textarea-el) {
  font-family: 'Fira Code', 'Courier New', Courier, monospace !important;
  color: var(--app-accent) !important;
}

.results-panel {
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-width: 0;
  min-height: 354px;
  padding: 16px;
  border: 1px solid rgba(93, 120, 162, 0.22);
  border-radius: 8px;
  background: var(--app-panel);
}

.results-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.results-header h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--app-text);
}

.results-list,
.history-card {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.history-card {
  grid-column: 1 / -1;
  background: var(--app-panel);
  border: 1px solid rgba(93, 120, 162, 0.22);
  border-radius: 8px;
  padding: 16px;
}

.history-filters {
  display: grid;
  grid-template-columns: minmax(0, 1.4fr) minmax(180px, 1fr) minmax(160px, 1fr) minmax(260px, 1.2fr) auto;
  gap: 12px;
}

.template-modal {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.scope-options {
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 8px;
}

.result-card,
.history-detail {
  background: rgba(7, 17, 31, 0.48);
  border: 1px solid rgba(93, 120, 162, 0.18);
  border-radius: 8px;
  overflow: hidden;
}

.result-meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 14px 20px;
  background: rgba(17, 32, 52, 0.52);
  border-bottom: 1px solid rgba(93, 120, 162, 0.12);
}

.server-name {
  font-size: 15px;
  font-weight: 600;
  color: var(--app-text);
}

.status-badge {
  font-size: 12px;
  font-weight: 600;
  padding: 2px 8px;
  border-radius: 4px;
}

.status-badge.success { color: #35d6a3; background: rgba(53, 214, 163, 0.1); }
.status-badge.error { color: var(--app-danger); background: rgba(255, 107, 125, 0.1); }
.status-badge.warning { color: var(--app-warning); background: rgba(232, 180, 95, 0.1); }

.result-details {
  padding: 12px 20px;
  display: flex;
  gap: 24px;
  font-size: 12px;
  color: var(--app-text-soft);
  border-bottom: 1px solid rgba(93, 120, 162, 0.1);
}

.detail-stack {
  flex-direction: column;
  gap: 10px;
}

.detail-row {
  display: flex;
  gap: 8px;
  align-items: center;
}

.detail-row code {
  color: #dce7f6;
  font-family: 'Fira Code', monospace;
  background: rgba(17,32,52,0.64);
  padding: 2px 6px;
  border-radius: 4px;
}

.terminal-output {
  padding: 20px;
  font-family: 'Fira Code', 'Courier New', Courier, monospace;
  font-size: 13px;
  line-height: 1.6;
  color: #dce7f6;
  background: rgba(0, 7, 16, 0.9);
  overflow-x: auto;
}

.terminal-output pre {
  margin: 0;
  white-space: pre-wrap;
}

.error-text {
  color: #fca5a5;
  margin-top: 12px !important;
}

.empty-terminal {
  min-height: 220px;
  background: rgba(7, 17, 31, 0.48);
  border: 1px solid rgba(93, 120, 162, 0.18);
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.cursor {
  display: inline-block;
  width: 8px;
  height: 16px;
  border-radius: 2px;
  background: rgba(32, 212, 255, 0.62);
  animation: blink 1s step-end infinite;
}

:deep(.dark-table .n-data-table-th) {
  background: rgba(17,32,52,0.52);
  color: var(--app-text-soft);
}

:deep(.dark-table .n-data-table-td) {
  background: transparent;
  color: #dce7f6;
}

:deep(.dark-table .n-data-table-tr:hover .n-data-table-td) {
  background: rgba(79, 131, 255, 0.08);
}

@keyframes blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0; }
}

@media (max-width: 1500px) {
  .layout-grid {
    grid-template-columns: minmax(420px, 1fr) minmax(320px, 0.8fr);
  }

  .template-panel {
    grid-column: 2;
    grid-row: 1;
  }

  .results-panel {
    grid-column: 1 / -1;
  }

  .history-filters {
    grid-template-columns: 1fr 1fr;
  }
}

@media (max-width: 960px) {
  .layout-grid {
    grid-template-columns: 1fr;
  }

  .history-filters {
    grid-template-columns: 1fr;
  }

  .execution-row {
    align-items: stretch;
    flex-direction: column;
  }

  .timeout-field,
  .action-bar {
    width: 100%;
  }
}
</style>
