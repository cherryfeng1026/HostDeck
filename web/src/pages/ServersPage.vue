<script setup lang="ts">
import { NButton, NInput, NScrollbar, useDialog, useMessage } from 'naive-ui'
import { computed, onMounted, ref } from 'vue'
import ServerModal from '../components/ServerModal.vue'
import ServerStatusTable from '../components/ServerStatusTable.vue'
import { deleteServer, getServerListWithStatus, probeServer, testServerSSH, trustServerHostKey } from '../services/api'
import { useSession } from '../session'
import type { ServerAsset, ServerListItem, TestSSHResponse } from '../types'

const message = useMessage()
const dialog = useDialog()
const { canManageInfrastructure } = useSession()
const loading = ref(false)
const search = ref('')
const servers = ref<ServerListItem[]>([])
const modalVisible = ref(false)
const editingServer = ref<ServerAsset | null>(null)
const statusFilter = ref<'all' | 'online' | 'offline' | 'ssh'>('all')

const filteredServers = computed(() => {
  switch (statusFilter.value) {
    case 'online':
      return servers.value.filter((item) => item.enabled && item.online)
    case 'offline':
      return servers.value.filter((item) => !item.online)
    case 'ssh':
      return servers.value.filter((item) => item.enabled && !item.sshOk)
    default:
      return servers.value
  }
})

const filterOptions = computed(() => [
  { label: '全部', value: 'all' as const, count: servers.value.length },
  { label: '在线', value: 'online' as const, count: servers.value.filter((item) => item.enabled && item.online).length },
  { label: '离线', value: 'offline' as const, count: servers.value.filter((item) => !item.online).length },
  { label: 'SSH 异常', value: 'ssh' as const, count: servers.value.filter((item) => item.enabled && !item.sshOk).length },
])

async function loadData() {
  loading.value = true
  try {
    servers.value = await getServerListWithStatus(search.value.trim() || undefined)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载服务器失败')
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

async function trustHostKeyAndRefresh(serverId: number, result: TestSSHResponse) {
  if (!result.hostKeyFingerprint) return
  await trustServerHostKey(serverId, result.hostKeyFingerprint)
  try {
    await probeServer(serverId)
    message.success('已保存 SSH 主机指纹并完成一次采集')
  } catch (error) {
    message.success('已保存 SSH 主机指纹')
    if (error instanceof Error) {
      message.warning(`自动采集未完成：${error.message}`)
    }
  }
  await loadData()
}

function showSSHResult(serverId: number, result: TestSSHResponse) {
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
      onPositiveClick: () => trustHostKeyAndRefresh(serverId, result),
    })
    return
  }
  if (result.sshOk) {
    message.success(`SSH 连接正常，耗时 ${result.latencyMs ?? 0} ms`)
    return
  }
  message.warning(result.error || 'SSH 连接异常')
}

async function handleTestSSH(serverId: number) {
  try {
    const result = await testServerSSH(serverId)
    showSSHResult(serverId, result)
  } catch (error) {
    message.error(error instanceof Error ? error.message : 'SSH 测试失败')
  }
}

function openCreateModal() {
  editingServer.value = null
  modalVisible.value = true
}

function openEditModal(server: ServerListItem) {
  editingServer.value = server
  modalVisible.value = true
}

function handleDelete(server: ServerListItem) {
  dialog.warning({
    title: '删除服务器',
    content: `确认删除 ${server.name}？该实例的连接凭据、最新状态和活动告警会一并移除。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await deleteServer(server.id)
        message.success('服务器已删除')
        await loadData()
      } catch (error) {
        message.error(error instanceof Error ? error.message : '删除服务器失败')
      }
    },
  })
}

function handleSubmitted() {
  editingServer.value = null
  void loadData()
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
          <h1>服务器管理</h1>
        </div>
      </div>

      <div class="server-toolbar">
        <n-input
          v-model:value="search"
          placeholder="搜索主机、IP、标签"
          clearable
          class="dark-input server-search"
          @keydown.enter.prevent="loadData"
        >
          <template #prefix>
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="11" cy="11" r="8" />
              <path d="m21 21-4.3-4.3" />
            </svg>
          </template>
        </n-input>

        <div class="filter-group">
          <button
            v-for="option in filterOptions"
            :key="option.value"
            type="button"
            class="filter-chip"
            :class="{ active: statusFilter === option.value }"
            @click="statusFilter = option.value"
          >
            <span v-if="option.value !== 'all'" class="filter-dot" :class="option.value" />
            {{ option.label }}
            <em>{{ option.count }}</em>
          </button>
        </div>

        <div class="toolbar-actions">
          <n-button ghost :loading="loading" @click="loadData">刷新</n-button>
          <n-button v-if="canManageInfrastructure" type="primary" class="glow-btn" @click="openCreateModal">
            新增服务器
          </n-button>
        </div>
      </div>

      <div class="table-card server-table-card">
        <ServerStatusTable
          :items="filteredServers"
          :loading="loading"
          :can-manage="canManageInfrastructure"
          @probe="handleProbe"
          @test-ssh="handleTestSSH"
          @edit="openEditModal"
          @delete="handleDelete"
        />
      </div>

      <ServerModal
        v-if="canManageInfrastructure"
        v-model:show="modalVisible"
        :server="editingServer"
        @submitted="handleSubmitted"
      />
    </div>
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

.server-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  margin-bottom: 14px;
}

.server-search {
  width: min(360px, 34vw);
  flex-shrink: 0;
}

.filter-group {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-left: auto;
}

.filter-chip {
  min-height: 36px;
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 0 14px;
  border: 1px solid rgba(93, 120, 162, 0.24);
  border-radius: 8px;
  background: rgba(12, 24, 42, 0.58);
  color: var(--app-text-soft);
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}

.filter-chip:hover,
.filter-chip.active {
  color: var(--app-text);
  border-color: rgba(79, 131, 255, 0.5);
  background: rgba(79, 131, 255, 0.16);
}

.filter-chip em {
  color: #62a8ff;
  font-style: normal;
  font-variant-numeric: tabular-nums;
}

.filter-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--app-text-faint);
}

.filter-dot.online {
  background: #35d6a3;
}

.filter-dot.offline {
  background: var(--app-danger);
}

.filter-dot.ssh {
  background: var(--app-warning);
}

.toolbar-actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.server-table-card {
  padding: 0;
  overflow: hidden;
  width: 100%;
}

@media (max-width: 900px) {
  .server-toolbar {
    flex-direction: column;
    align-items: stretch;
  }

  .server-search {
    width: 100%;
  }

  .filter-group {
    margin-left: 0;
  }

  .toolbar-actions {
    width: 100%;
  }
}
</style>
