<script setup lang="ts">
import { NButton, NInput, NScrollbar, useDialog, useMessage } from 'naive-ui'
import { onMounted, ref } from 'vue'
import ServerModal from '../components/ServerModal.vue'
import ServerStatusTable from '../components/ServerStatusTable.vue'
import { getServerListWithStatus, probeServer, testServerSSH, trustServerHostKey } from '../services/api'
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

function showSSHResult(serverId: number, result: TestSSHResponse) {
  const canTrustHostKey = Boolean(
    result.hostKeyFingerprint && (result.fingerprintMismatch || (result.sshOk && result.hostKeyFingerprint !== result.trustedHostKeyFingerprint)),
  )
  if (canTrustHostKey) {
    dialog.warning({
      title: result.fingerprintMismatch ? 'SSH 指纹不匹配' : '发现 SSH 主机指纹',
      content: `${result.error || '请确认该主机指纹是否可信'}\n\n当前指纹：${result.hostKeyFingerprint}${result.trustedHostKeyFingerprint ? `\n已信任：${result.trustedHostKeyFingerprint}` : ''}`,
      positiveText: '信任并保存',
      negativeText: result.sshOk ? '稍后处理' : '取消',
      onPositiveClick: async () => {
        await trustServerHostKey(serverId, result.hostKeyFingerprint!)
        message.success(result.sshOk ? '已保存 SSH 主机指纹，后续连接将强制校验' : '已保存 SSH 主机指纹')
        await loadData()
      },
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
          <h1>服务器实例</h1>
          <p>管理和监控您连接的所有服务器及资产。</p>
        </div>
        <div class="header-actions">
          <div class="search-box">
            <n-input
              v-model:value="search"
              placeholder="按名称、IP 筛选..."
              clearable
              @keydown.enter.prevent="loadData"
              class="dark-input"
            >
              <template #prefix>🔍</template>
            </n-input>
            <n-button type="primary" ghost @click="loadData">查询</n-button>
          </div>
          <n-button ghost @click="loadData" style="color: #a1a1aa; border-color: rgba(255,255,255,0.1)">
            刷新列表
          </n-button>
          <n-button v-if="canManageInfrastructure" type="primary" class="glow-btn" @click="openCreateModal">
            + 添加服务器
          </n-button>
        </div>
      </div>

      <div class="table-card">
        <ServerStatusTable
          :items="servers"
          :loading="loading"
          :can-manage="canManageInfrastructure"
          @probe="handleProbe"
          @test-ssh="handleTestSSH"
          @edit="openEditModal"
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

.search-box {
  display: flex;
  gap: 8px;
  width: 320px;
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

.table-card {
  background: rgba(20, 20, 25, 0.6);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 20px;
  padding: 24px;
  overflow: hidden;
}

:deep(.dark-input .n-input__border) {
  border-color: rgba(255, 255, 255, 0.1);
}
:deep(.dark-input .n-input__placeholder) {
  color: #71717a;
}

@media (max-width: 900px) {
  .page-header {
    flex-direction: column;
    align-items: flex-start;
    gap: 20px;
  }
  .header-actions {
    width: 100%;
    flex-wrap: wrap;
  }
  .search-box {
    width: 100%;
  }
}
</style>
