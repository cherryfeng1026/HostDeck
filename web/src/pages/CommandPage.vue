<script setup lang="ts">
import {
  NButton,
  NDataTable,
  NDatePicker,
  NEmpty,
  NInput,
  NInputNumber,
  NModal,
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

const historyColumns: DataTableColumns<CommandHistoryRecord> = [
  {
    title: '执行时间',
    key: 'executedAt',
    render: (row) => h('span', { style: 'color: #d4d4d8' }, formatDateTime(row.executedAt)),
  },
  {
    title: '服务器',
    key: 'serverName',
    render: (row) => h('strong', { style: 'color: #fff' }, row.serverName || `#${row.serverId}`),
  },
  {
    title: '执行人',
    key: 'executorUsername',
    render: (row) => h('span', { style: 'color: #a1a1aa' }, row.executorUsername || '系统'),
  },
  {
    title: '命令',
    key: 'command',
    render: (row) => h('code', { style: 'color: #d4d4d8; background: rgba(255,255,255,0.05); padding: 2px 6px; border-radius: 4px;' }, row.command),
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
    render: (row) => h('span', { style: 'color: #a1a1aa' }, `${row.durationMs} ms`),
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
    if (selectedTemplate.value?.riskLevel === 'dangerous') {
      message.warning('当前为高风险模板，请确认业务影响后再执行')
    }
    if (selectedServerIds.value.length === 1) {
      const serverId = selectedServerIds.value[0]
      const result = await executeCommand(serverId, resolvedCommand.value, timeoutSeconds.value || 15)
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
      const response = await executeCommands(selectedServerIds.value, resolvedCommand.value, timeoutSeconds.value || 15)
      results.value = response.results
    }
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
          <p>在单个或多个目标服务器上安全地执行 Shell 命令及脚本，并查看历史记录。</p>
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

          <div class="form-group">
            <div class="section-title">
              <label>命令模板</label>
              <div class="template-toolbar">
                <n-button ghost size="small" :disabled="!canManageInfrastructure" @click="openCreateTemplateModal">保存为模板</n-button>
                <n-button
                  v-if="selectedTemplate"
                  ghost
                  size="small"
                  :disabled="!canManageInfrastructure"
                  :loading="togglingFavorite"
                  @click="toggleTemplateFavorite"
                >
                  {{ selectedTemplate.isFavorite ? '取消收藏' : '加入收藏' }}
                </n-button>
              </div>
            </div>
            <n-select
              v-model:value="selectedTemplateId"
              clearable
              filterable
              :options="templateOptions"
              :loading="loadingTemplates"
              placeholder="选择常用模板，或直接手输命令"
              class="dark-input"
            />
            <div v-if="selectedTemplate" class="template-meta">
              <div class="template-meta__head">
                <strong>{{ selectedTemplate.name }}</strong>
                <div class="template-badges">
                  <n-tag v-if="selectedTemplate.isFavorite" type="warning" size="small" :bordered="false">收藏</n-tag>
                  <n-tag size="small" :bordered="false">{{ templateScopeLabel(selectedTemplate) }}</n-tag>
                  <n-tag :type="selectedTemplate.riskLevel === 'dangerous' ? 'warning' : 'success'" size="small" :bordered="false">
                    {{ templateRiskLabel(selectedTemplate) }}
                  </n-tag>
                </div>
              </div>
              <p>{{ selectedTemplate.description || '暂无模板描述' }}</p>
            </div>
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

          <div class="form-group">
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
          </div>

          <div class="form-group" style="width: 140px;">
            <label>超时时间 (秒)</label>
            <n-input-number v-model:value="timeoutSeconds" :min="1" :max="60" class="dark-input" />
          </div>

          <div class="action-bar">
            <n-button v-if="canManageInfrastructure" type="primary" size="large" class="glow-btn" :loading="executing" @click="runCommand">
              {{ selectedServerIds.length > 1 ? '批量执行命令' : '执行命令' }}
            </n-button>
            <n-button v-else disabled size="large" class="glow-btn">
              只读账号不可执行命令
            </n-button>
          </div>
        </div>

        <div class="results-panel">
          <div class="results-header">
            <h3>执行结果</h3>
            <n-tag type="info" size="small" bordered style="background: transparent; color: #a1a1aa; border-color: rgba(255,255,255,0.1)">
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
              <div class="empty-prompt">
                <span class="cursor">_</span>
                <p>等待命令执行...</p>
              </div>
            </div>
          </n-spin>

          <div class="history-card">
            <div class="results-header">
              <h3>命令历史</h3>
              <n-button ghost size="small" @click="loadHistory" :loading="loadingHistory" style="color: #a1a1aa; border-color: rgba(255,255,255,0.1)">
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
                <n-button text @click="selectedHistory = null" style="color: #a1a1aa">关闭</n-button>
              </div>
              <div class="result-details detail-stack">
                <div class="detail-row"><span>执行时间:</span><span>{{ formatDateTime(selectedHistory.executedAt) }}</span></div>
                <div class="detail-row"><span>服务器:</span><span>{{ selectedHistory.serverName || `#${selectedHistory.serverId}` }}</span></div>
                <div class="detail-row"><span>执行人:</span><span>{{ selectedHistory.executorUsername || '系统' }}</span></div>
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
  padding: 32px;
  max-width: 1600px;
  margin: 0 auto;
}

.page-header {
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

.layout-grid {
  display: grid;
  grid-template-columns: minmax(360px, 420px) minmax(0, 1fr);
  gap: 24px;
}

.control-panel {
  background: rgba(20, 20, 25, 0.6);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 20px;
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.form-group label {
  display: block;
  font-size: 13px;
  font-weight: 500;
  color: #d4d4d8;
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
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(255, 255, 255, 0.08);
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
  color: #fff;
}

.template-meta p {
  margin: 8px 0 0;
  color: #a1a1aa;
  font-size: 13px;
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
  color: #a1a1aa;
  font-size: 12px;
}

.resolved-command code {
  color: #d4d4d8;
  background: rgba(255,255,255,0.05);
  padding: 10px 12px;
  border-radius: 10px;
  white-space: pre-wrap;
  word-break: break-all;
}

.action-bar {
  display: flex;
  margin-top: auto;
}

.glow-btn {
  box-shadow: 0 0 20px rgba(16, 185, 129, 0.4);
  transition: all 0.3s ease;
  background-color: #10b981 !important;
  color: #fff !important;
  border: none;
  flex: 1;
}

.glow-btn:hover:not(:disabled) {
  box-shadow: 0 0 30px rgba(16, 185, 129, 0.6);
  transform: translateY(-1px);
}

.glow-btn:disabled {
  background-color: rgba(255,255,255,0.1) !important;
  box-shadow: none;
  color: #a1a1aa !important;
  cursor: not-allowed;
}

:deep(.dark-input .n-input__border),
:deep(.dark-input .n-base-selection__border) {
  border-color: rgba(255, 255, 255, 0.1);
}

:deep(.dark-input .n-input__placeholder),
:deep(.dark-input .n-base-selection-placeholder) {
  color: #71717a;
}

:deep(.code-input .n-input__textarea-el) {
  font-family: 'Fira Code', 'Courier New', Courier, monospace !important;
  color: #6ee7b7 !important;
}

.results-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
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
  color: #fff;
}

.results-list,
.history-card {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.history-card {
  background: rgba(20, 20, 25, 0.6);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 20px;
  padding: 20px;
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
  background: rgba(0, 0, 0, 0.4);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 16px;
  overflow: hidden;
}

.result-meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 14px 20px;
  background: rgba(255, 255, 255, 0.02);
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.server-name {
  font-size: 15px;
  font-weight: 600;
  color: #fff;
}

.status-badge {
  font-size: 12px;
  font-weight: 600;
  padding: 2px 8px;
  border-radius: 4px;
}

.status-badge.success { color: #10b981; background: rgba(16, 185, 129, 0.1); }
.status-badge.error { color: #f43f5e; background: rgba(244, 63, 94, 0.1); }
.status-badge.warning { color: #f59e0b; background: rgba(245, 158, 11, 0.1); }

.result-details {
  padding: 12px 20px;
  display: flex;
  gap: 24px;
  font-size: 12px;
  color: #a1a1aa;
  border-bottom: 1px solid rgba(255, 255, 255, 0.02);
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
  color: #d4d4d8;
  font-family: 'Fira Code', monospace;
  background: rgba(255,255,255,0.05);
  padding: 2px 6px;
  border-radius: 4px;
}

.terminal-output {
  padding: 20px;
  font-family: 'Fira Code', 'Courier New', Courier, monospace;
  font-size: 13px;
  line-height: 1.6;
  color: #d4d4d8;
  background: #000;
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
  background: rgba(0, 0, 0, 0.4);
  border: 1px solid rgba(255, 255, 255, 0.05);
  border-radius: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.empty-prompt {
  text-align: center;
  color: #52525b;
  font-family: 'Fira Code', monospace;
}

.empty-prompt p {
  margin: 12px 0 0 0;
  font-size: 14px;
}

.cursor {
  display: inline-block;
  width: 12px;
  height: 24px;
  background: #52525b;
  animation: blink 1s step-end infinite;
}

:deep(.dark-table .n-data-table-th) {
  background: rgba(255,255,255,0.02);
  color: #a1a1aa;
}

:deep(.dark-table .n-data-table-td) {
  background: transparent;
  color: #d4d4d8;
}

:deep(.dark-table .n-data-table-tr:hover .n-data-table-td) {
  background: rgba(255,255,255,0.02);
}

@keyframes blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0; }
}

@media (max-width: 1200px) {
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
}
</style>
