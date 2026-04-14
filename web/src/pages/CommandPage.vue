<script setup lang="ts">
import {
  NButton,
  NEmpty,
  NInput,
  NInputNumber,
  NScrollbar,
  NSelect,
  NSpin,
  NTag,
  useMessage,
} from 'naive-ui'
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { executeCommand, executeCommands, getServers } from '../services/api'
import { useSession } from '../session'
import type { BatchCommandResult, ServerAsset } from '../types'

const route = useRoute()
const message = useMessage()
const { canManageInfrastructure } = useSession()
const loadingTargets = ref(false)
const executing = ref(false)
const servers = ref<ServerAsset[]>([])
const selectedServerIds = ref<number[]>([])
const command = ref('df -h')
const timeoutSeconds = ref(15)
const results = ref<BatchCommandResult[]>([])

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

const selectedSummary = computed(() => {
  const count = selectedServerIds.value.length
  if (count === 0) {
    return '未选择目标'
  }
  return `已选择 ${count} 个目标`
})

async function loadServers() {
  loadingTargets.value = true
  try {
    servers.value = await getServers()
    selectedServerIds.value = selectedServerIds.value.filter((serverId) => isEnabledServer(serverId))
    const preset = Number(route.query.serverId)
    if (Number.isFinite(preset) && preset > 0 && isEnabledServer(preset)) {
      selectedServerIds.value = [preset]
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

async function runCommand() {
  if (!command.value.trim()) {
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
    if (selectedServerIds.value.length === 1) {
      const serverId = selectedServerIds.value[0]
      const result = await executeCommand(serverId, command.value, timeoutSeconds.value || 15)
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
      const response = await executeCommands(selectedServerIds.value, command.value, timeoutSeconds.value || 15)
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
  } catch (error) {
    message.error(error instanceof Error ? error.message : '命令执行失败')
  } finally {
    executing.value = false
  }
}

function resultStatus(result: BatchCommandResult) {
  if (!result.success) {
    return { label: '失败', type: 'error' as const, tone: 'error' }
  }
  if (result.result.exitCode === 0) {
    return { label: '成功', type: 'success' as const, tone: 'success' }
  }
  return { label: `退出码 ${result.result.exitCode}`, type: 'warning' as const, tone: 'warning' }
}

watch(
  () => route.query.serverId,
  (value) => {
    const preset = Number(value)
    if (Number.isFinite(preset) && preset > 0 && isEnabledServer(preset)) {
      selectedServerIds.value = [preset]
    }
  },
)

onMounted(() => {
  void loadServers()
})
</script>

<template>
  <n-scrollbar class="content-scroll">
    <div class="page-container">
      <div class="page-header">
        <div class="header-text">
          <h1>远程终端</h1>
          <p>在单个或多个目标服务器上安全地执行 Shell 命令及脚本。</p>
        </div>
      </div>

      <div class="layout-grid">
        <!-- Control Panel -->
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
            <label>执行命令</label>
            <n-input
              v-model:value="command"
              type="textarea"
              placeholder="输入 Bash 脚本或命令..."
              :autosize="{ minRows: 6, maxRows: 12 }"
              class="code-input dark-input"
            />
          </div>

          <div class="form-group" style="width: 140px;">
            <label>超时时间 (秒)</label>
            <n-input-number v-model:value="timeoutSeconds" :min="1" :max="60" class="dark-input" />
          </div>

          <div class="action-bar">
            <n-button 
              v-if="canManageInfrastructure" 
              type="primary" 
              size="large" 
              class="glow-btn" 
              :loading="executing" 
              @click="runCommand"
            >
              {{ selectedServerIds.length > 1 ? '批量执行命令' : '执行命令' }}
            </n-button>
            <n-button v-else disabled size="large" class="glow-btn">
              只读账号不可执行命令
            </n-button>
          </div>
        </div>

        <!-- Results Panel -->
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

/* Control Panel */
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

/* Results Panel */
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

.results-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.result-card {
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
  min-height: 400px;
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

@keyframes blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0; }
}

@media (max-width: 960px) {
  .layout-grid {
    grid-template-columns: 1fr;
  }
}
</style>
