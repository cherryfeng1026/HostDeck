<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { NButton, NEmpty, NSpin, NTable, NTag } from 'naive-ui'
import type { ServerListItem } from '../types'

defineProps<{
  items: ServerListItem[]
  loading?: boolean
  canManage?: boolean
}>()

const emit = defineEmits<{
  (event: 'probe', id: number): void
  (event: 'test-ssh', id: number): void
  (event: 'edit', item: ServerListItem): void
}>()

function usageTone(value: number) {
  if (value >= 80) return 'error'
  if (value >= 60) return 'warning'
  return 'success'
}

function formatPercent(value: number) {
  return `${Math.round(value)}%`
}
</script>

<template>
  <div class="table-container">
    <n-spin :show="loading">
      <n-empty v-if="!items.length" description="暂无服务器数据" class="empty-state" />
      <div v-else class="table-scroll-wrapper">
        <n-table :bordered="false" :single-line="false" class="dark-table">
          <thead>
            <tr>
              <th>服务器</th>
              <th>连接状态</th>
              <th>资源使用</th>
              <th>系统信息</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in items" :key="item.id" :class="{ 'disabled-row': !item.enabled }">
              <td>
                <RouterLink class="server-link" :class="{ 'server-link-disabled': !item.enabled }" :to="`/servers/${item.id}`">{{ item.name }}</RouterLink>
                <div class="server-meta" :class="{ 'server-meta-disabled': !item.enabled }">{{ item.hostname }} · <span class="ip-addr">{{ item.ip }}</span></div>
              </td>
              <td>
                <div class="metric-stack">
                  <n-tag size="small" :type="item.online ? 'success' : 'error'" bordered :style="{ background: 'transparent' }">
                    <div class="status-dot" :class="{ online: item.online }"></div>
                    {{ item.online ? '在线' : '离线' }}
                  </n-tag>
                  <n-tag size="small" :type="item.sshOk ? 'success' : 'warning'" bordered :style="{ background: 'transparent' }">
                    {{ item.sshOk ? 'SSH 正常' : 'SSH 异常' }}
                  </n-tag>
                  <n-tag size="small" :type="item.passwordConfigured ? 'success' : 'warning'" bordered :style="{ background: 'transparent' }">
                    {{ item.passwordConfigured ? '已配密码' : '未配密码' }}
                  </n-tag>
                  <n-tag v-if="!item.enabled" size="small" type="default" bordered :style="{ background: 'transparent' }">
                    已禁用
                  </n-tag>
                </div>
              </td>
              <td>
                <div class="metric-stack">
                  <n-tag size="small" :type="usageTone(item.cpuUsage)" bordered :style="{ background: 'transparent' }">{{ `CPU ${formatPercent(item.cpuUsage)}` }}</n-tag>
                  <n-tag size="small" :type="usageTone(item.memoryUsage)" bordered :style="{ background: 'transparent' }">{{ `MEM ${formatPercent(item.memoryUsage)}` }}</n-tag>
                  <n-tag size="small" :type="usageTone(item.diskUsage)" bordered :style="{ background: 'transparent' }">{{ `DSK ${formatPercent(item.diskUsage)}` }}</n-tag>
                </div>
              </td>
              <td>
                <div class="os-info" :class="{ 'os-info-disabled': !item.enabled }">{{ item.osVersion || '尚未采集系统信息' }}</div>
                <div class="server-meta" :class="{ 'server-meta-disabled': !item.enabled }">{{ item.kernelVersion || item.collectorMode }}</div>
              </td>
              <td>
                <div class="action-stack">
                  <template v-if="canManage !== false">
                    <n-button size="small" ghost type="primary" :disabled="!item.enabled" @click="emit('probe', item.id)">采集</n-button>
                    <n-button size="small" ghost type="warning" :disabled="!item.enabled" @click="emit('test-ssh', item.id)">测 SSH</n-button>
                    <n-button size="small" ghost @click="emit('edit', item)">编辑</n-button>
                  </template>
                  <n-tag v-else size="small" type="default" style="color: #a1a1aa; background: transparent;">只读</n-tag>
                </div>
              </td>
            </tr>
          </tbody>
        </n-table>
      </div>
    </n-spin>
  </div>
</template>

<style scoped>
.table-container {
  width: 100%;
}

.empty-state {
  padding: 40px 0;
}

.table-scroll-wrapper {
  width: 100%;
  overflow-x: auto;
}

:deep(.dark-table) {
  --n-th-color: transparent !important;
  --n-td-color: transparent !important;
  --n-th-text-color: #a1a1aa !important;
  --n-td-text-color: #d4d4d8 !important;
  --n-border-color: rgba(255, 255, 255, 0.05) !important;
  --n-th-font-weight: 500 !important;
  background-color: transparent !important;
  min-width: 860px;
}

:deep(.n-data-table-th),
:deep(th) {
  border-bottom: 1px solid rgba(255,255,255,0.05) !important;
  background: transparent !important;
}

:deep(.n-data-table-td),
:deep(td) {
  border-bottom: 1px solid rgba(255,255,255,0.02) !important;
  background: transparent !important;
  padding: 12px 0px !important; /* Removed horizontal padding so it aligns with the card */
}

:deep(.n-data-table-wrapper) {
  border: none !important;
  border-radius: 0 !important;
  background: transparent !important;
}

:deep(.n-table) {
  background: transparent !important;
  background-color: transparent !important;
}

:deep(tr:hover td) {
  background-color: rgba(255, 255, 255, 0.02) !important;
}

.server-link {
  color: #10b981;
  font-weight: 600;
  text-decoration: none;
  font-size: 14px;
}
.server-link:hover {
  text-decoration: underline;
}

.server-meta {
  margin-top: 4px;
  color: #71717a;
  font-size: 12px;
}

.ip-addr {
  font-family: 'Fira Code', monospace;
}

.os-info {
  color: #d4d4d8;
  font-size: 13px;
}

.metric-stack {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.action-stack {
  display: flex;
  gap: 8px;
  justify-content: flex-start;
}

.status-dot {
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  margin-right: 6px;
  background-color: #f43f5e;
}
.status-dot.online {
  background-color: #10b981;
  box-shadow: 0 0 8px rgba(16, 185, 129, 0.6);
}

:deep(.disabled-row td) {
  background-color: rgba(255, 255, 255, 0.01) !important;
}

.server-link-disabled {
  color: #6ee7b7;
}

.server-meta-disabled {
  color: #8f8f99;
}

.os-info-disabled {
  color: #b3b3bc;
}
</style>
