<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { NEmpty, NSpin, NTable, NTag } from 'naive-ui'
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
  (event: 'delete', item: ServerListItem): void
}>()

function usageTone(value: number) {
  if (value >= 80) return 'error'
  if (value >= 60) return 'warning'
  return 'success'
}

function formatPercent(value: number) {
  return `${Math.round(value)}%`
}

function formatExpiresAt(value?: string) {
  if (!value) return '永久'
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime()) || parsed.getUTCFullYear() <= 1) {
    return '永久'
  }
  return parsed.toLocaleString('zh-CN', { hour12: false })
}

function tagTone(tag: string) {
  if (/prod|生产|核心/i.test(tag)) return 'cyan'
  if (/dev|test|staging|测试/i.test(tag)) return 'muted'
  if (/db|cache|worker/i.test(tag)) return 'amber'
  return 'blue'
}
</script>

<template>
  <div class="table-container">
    <n-spin :show="loading">
      <n-empty v-if="!items.length" description="暂无服务器数据" class="empty-state" />
      <div v-else class="table-scroll-wrapper">
        <n-table :bordered="false" :single-line="false" class="dark-table">
          <colgroup>
            <col style="width: 124px" />
            <col style="width: 104px" />
            <col style="width: 92px" />
            <col style="width: 108px" />
            <col style="width: 78px" />
            <col style="width: 78px" />
            <col style="width: 78px" />
            <col style="width: 130px" />
            <col style="width: 178px" />
          </colgroup>
          <thead>
            <tr>
              <th>主机名</th>
              <th>IP 地址</th>
              <th>状态</th>
              <th>标签</th>
              <th>CPU</th>
              <th>内存</th>
              <th>磁盘</th>
              <th>到期时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in items" :key="item.id" :class="{ 'disabled-row': !item.enabled }">
              <td>
                <RouterLink class="server-link" :class="{ 'server-link-disabled': !item.enabled }" :to="`/servers/${item.id}`">{{ item.name }}</RouterLink>
                <div class="server-meta" :class="{ 'server-meta-disabled': !item.enabled }">{{ item.hostname || '未设置主机名' }}</div>
              </td>
              <td>
                <span class="ip-addr">{{ item.ip }}</span>
              </td>
              <td>
                <span class="status-pill" :class="{ online: item.online && item.enabled, offline: !item.online || !item.enabled, warning: item.enabled && item.online && !item.sshOk }">
                  <span class="status-dot" :class="{ online: item.online && item.enabled }" />
                  {{ !item.enabled ? '已禁用' : item.online ? (item.sshOk ? '在线' : 'SSH 异常') : '离线' }}
                </span>
              </td>
              <td>
                <div class="tag-list">
                  <span v-for="tag in item.tags.slice(0, 3)" :key="tag" class="tag-chip" :class="tagTone(tag)">{{ tag }}</span>
                  <span v-if="!item.tags.length" class="server-meta">-</span>
                </div>
              </td>
              <td>
                <div class="usage-cell">
                  <span>{{ item.online ? formatPercent(item.cpuUsage) : '-' }}</span>
                  <div class="usage-bar"><i :class="usageTone(item.cpuUsage)" :style="{ width: `${item.online ? Math.max(0, Math.min(100, item.cpuUsage)) : 0}%` }" /></div>
                </div>
              </td>
              <td>
                <div class="usage-cell">
                  <span>{{ item.online ? formatPercent(item.memoryUsage) : '-' }}</span>
                  <div class="usage-bar"><i :class="usageTone(item.memoryUsage)" :style="{ width: `${item.online ? Math.max(0, Math.min(100, item.memoryUsage)) : 0}%` }" /></div>
                </div>
              </td>
              <td>
                <div class="usage-cell">
                  <span>{{ item.online ? formatPercent(item.diskUsage) : '-' }}</span>
                  <div class="usage-bar"><i :class="usageTone(item.diskUsage)" :style="{ width: `${item.online ? Math.max(0, Math.min(100, item.diskUsage)) : 0}%` }" /></div>
                </div>
              </td>
              <td>
                <div class="server-meta">{{ formatExpiresAt(item.expiresAt) }}</div>
              </td>
              <td>
                <div class="action-stack">
                  <template v-if="canManage !== false">
                    <button type="button" class="icon-action primary" title="采集" aria-label="采集" :disabled="!item.enabled" @click="emit('probe', item.id)">
                      <svg viewBox="0 0 24 24"><path d="M21 12a9 9 0 0 1-9 9 9.7 9.7 0 0 1-6.7-2.7" /><path d="M3 12a9 9 0 0 1 9-9 9.7 9.7 0 0 1 6.7 2.7" /><path d="M3 18h5v-5" /><path d="M21 6h-5v5" /></svg>
                    </button>
                    <button type="button" class="icon-action" title="测试 SSH" aria-label="测试 SSH" :disabled="!item.enabled" @click="emit('test-ssh', item.id)">
                      <svg viewBox="0 0 24 24"><path d="M4 17l6-6-6-6" /><path d="M12 19h8" /></svg>
                    </button>
                    <button type="button" class="icon-action" title="编辑" aria-label="编辑" @click="emit('edit', item)">
                      <svg viewBox="0 0 24 24"><path d="M12 20h9" /><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z" /></svg>
                    </button>
                    <button type="button" class="icon-action danger" title="删除" aria-label="删除" @click="emit('delete', item)">
                      <svg viewBox="0 0 24 24"><path d="M3 6h18" /><path d="M8 6V4h8v2" /><path d="M19 6l-1 14H6L5 6" /><path d="M10 11v5" /><path d="M14 11v5" /></svg>
                    </button>
                  </template>
                  <n-tag v-else size="small" type="default" style="color: var(--app-text-soft); background: transparent;">只读</n-tag>
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
  --n-th-text-color: var(--app-text-soft) !important;
  --n-td-text-color: var(--app-text) !important;
  --n-border-color: rgba(93, 120, 162, 0.12) !important;
  --n-th-font-weight: 500 !important;
  background-color: transparent !important;
  min-width: 1124px;
  table-layout: fixed;
}

:deep(.n-data-table-th),
:deep(th) {
  border-bottom: 1px solid rgba(93, 120, 162, 0.14) !important;
  background: rgba(17, 32, 52, 0.44) !important;
  padding: 12px 14px !important;
  white-space: nowrap;
}

:deep(.n-data-table-td),
:deep(td) {
  border-bottom: 1px solid rgba(93, 120, 162, 0.1) !important;
  background: transparent !important;
  padding: 12px 14px !important;
  vertical-align: middle;
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
  color: var(--app-text);
  font-weight: 600;
  text-decoration: none;
  font-size: 14px;
  display: inline-block;
  max-width: 110px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  word-break: normal;
}
.server-link:hover {
  color: #62a8ff;
}

.server-meta {
  margin-top: 3px;
  color: var(--app-text-soft);
  font-size: 12px;
  max-width: 110px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ip-addr {
  color: #cfd8ea;
  font-family: var(--app-font-mono);
}

.metric-stack {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.action-stack {
  display: flex;
  gap: 9px;
  align-items: center;
  justify-content: start;
  white-space: nowrap;
}

.icon-action {
  width: 34px;
  height: 34px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 1px solid rgba(93, 120, 162, 0.24);
  border-radius: 8px;
  background: rgba(9, 20, 36, 0.78);
  color: #dce7f6;
  cursor: pointer;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.045);
  transition: background-color 0.18s ease, border-color 0.18s ease, color 0.18s ease, transform 0.18s ease;
}

.icon-action svg {
  width: 16px;
  height: 16px;
  fill: none;
  stroke: currentColor;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.icon-action:hover:not(:disabled) {
  color: var(--app-accent);
  border-color: rgba(32, 212, 255, 0.42);
  background: rgba(32, 212, 255, 0.1);
  transform: translateY(-1px);
}

.icon-action.primary {
  color: #62a8ff;
  border-color: rgba(79, 131, 255, 0.34);
  background: rgba(79, 131, 255, 0.12);
}

.icon-action.danger {
  color: var(--app-danger);
  border-color: rgba(255, 107, 125, 0.28);
  background: rgba(255, 107, 125, 0.08);
}

.icon-action:disabled {
  opacity: 0.36;
  cursor: not-allowed;
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
  background-color: #35d6a3;
  box-shadow: 0 0 8px rgba(53, 214, 163, 0.6);
}

.ssh-state,
.status-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 24px;
  padding: 0 9px;
  border-radius: 7px;
  border: 1px solid rgba(93, 120, 162, 0.18);
  color: var(--app-text-soft);
  background: rgba(17, 32, 52, 0.48);
  font-size: 12px;
  font-weight: 600;
}

.ssh-state.ok,
.status-pill.online {
  color: #35d6a3;
  background: rgba(53, 214, 163, 0.1);
}

.ssh-state.bad,
.status-pill.offline {
  color: var(--app-danger);
  background: rgba(255, 107, 125, 0.1);
}

.status-pill.warning {
  color: var(--app-warning);
  background: rgba(232, 180, 95, 0.1);
}

.tag-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.tag-chip {
  display: inline-flex;
  min-height: 22px;
  align-items: center;
  padding: 0 7px;
  border-radius: 6px;
  border: 1px solid rgba(79, 131, 255, 0.28);
  color: #62a8ff;
  background: rgba(79, 131, 255, 0.1);
  font-size: 12px;
}

.tag-chip.cyan {
  color: var(--app-accent);
  border-color: rgba(32, 212, 255, 0.28);
  background: rgba(32, 212, 255, 0.08);
}

.tag-chip.amber {
  color: var(--app-warning);
  border-color: rgba(232, 180, 95, 0.28);
  background: rgba(232, 180, 95, 0.08);
}

.tag-chip.muted {
  color: var(--app-text-soft);
  border-color: rgba(93, 120, 162, 0.22);
  background: rgba(93, 120, 162, 0.08);
}

.usage-cell {
  display: grid;
  gap: 7px;
  min-width: 58px;
  font-variant-numeric: tabular-nums;
}

.usage-cell span {
  color: #dce7f6;
  font-size: 12px;
}

.usage-bar {
  width: 52px;
  height: 5px;
  overflow: hidden;
  border-radius: 999px;
  background: rgba(93, 120, 162, 0.22);
}

.usage-bar i {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: var(--app-accent);
}

.usage-bar i.warning {
  background: var(--app-warning);
}

.usage-bar i.error {
  background: var(--app-danger);
}

:deep(.disabled-row td) {
  background-color: rgba(255, 255, 255, 0.01) !important;
}

.server-link-disabled {
  color: var(--app-text-soft);
}

.server-meta-disabled {
  color: var(--app-text-faint);
}
</style>
