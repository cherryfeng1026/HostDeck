<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import type { ServerListItem } from '../types'

const props = defineProps<{
  server: ServerListItem | null
  rankReason?: string
}>()

const router = useRouter()

const summaryRows = computed(() => {
  if (!props.server) {
    return []
  }
  return [
    {
      label: 'CPU',
      value: `${Math.round(props.server.cpuUsage)}%`,
      percent: props.server.cpuUsage,
    },
    {
      label: '内存',
      value: `${Math.round(props.server.memoryUsage)}%`,
      percent: props.server.memoryUsage,
    },
    {
      label: '磁盘',
      value: `${Math.round(props.server.diskUsage)}%`,
      percent: props.server.diskUsage,
    },
  ]
})

const detailItems = computed(() => {
  if (!props.server) {
    return []
  }
  return [
    {
      label: '运行时长',
      value: formatUptime(props.server.uptimeSeconds),
    },
    {
      label: '最后上报',
      value: formatDateTime(props.server.lastReportAt),
    },
    {
      label: '系统版本',
      value: props.server.osVersion || '未知',
    },
  ]
})

function formatUptime(seconds: number) {
  if (!seconds) return '未知'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days} 天 ${hours} 小时`
  if (hours > 0) return `${hours} 小时 ${minutes} 分钟`
  return `${minutes} 分钟`
}

function formatDateTime(value: string) {
  if (!value) return '未知时间'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

function openServersPage() {
  void router.push('/servers')
}
</script>

<template>
  <section class="console-panel instance-panel">
    <div class="panel-header">
      <div>
        <div class="panel-title">服务实例</div>
      </div>
      <button type="button" class="panel-link" @click="openServersPage">查看全部</button>
    </div>

    <div v-if="server" class="instance-shell">
      <article class="instance-card">
        <div class="instance-head">
          <div class="instance-identity">
            <span class="status-dot" :class="{ offline: !server.online }" />
            <div>
              <strong>{{ server.name }}</strong>
              <p>{{ server.ip }} · {{ server.hostname }}</p>
            </div>
          </div>
          <div class="instance-badges">
            <span class="instance-status" :class="{ offline: !server.online }">
              {{ server.online ? '运行中' : '离线' }}
            </span>
            <span v-if="rankReason" class="instance-reason">{{ rankReason }}</span>
          </div>
        </div>

        <div v-if="server.purpose" class="instance-purpose">{{ server.purpose }}</div>

        <div class="metric-grid">
          <div v-for="item in summaryRows" :key="item.label" class="metric-item">
            <span>{{ item.label }}</span>
            <strong>{{ item.value }}</strong>
            <div class="progress-bar">
              <div class="progress-fill" :style="{ width: `${Math.max(0, Math.min(100, item.percent))}%` }" />
            </div>
          </div>
        </div>

        <div class="detail-grid">
          <div v-for="item in detailItems" :key="item.label" class="detail-item">
            <span>{{ item.label }}</span>
            <strong>{{ item.value }}</strong>
          </div>
        </div>
      </article>
    </div>

    <div v-else class="instance-empty">
      <strong>暂无实例数据</strong>
      <span>当前没有可展示的实例状态，请稍后刷新或先添加服务器。</span>
    </div>
  </section>
</template>

<style scoped>
.console-panel {
  height: 100%;
  padding: 0;
  overflow: hidden;
  border-radius: 8px;
  border: 1px solid rgba(93, 120, 162, 0.2);
  background: linear-gradient(180deg, rgba(15, 27, 46, 0.72), rgba(11, 20, 34, 0.82));
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.05), 0 14px 30px rgba(0, 8, 22, 0.22);
  display: flex;
  flex-direction: column;
}

.panel-header {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: center;
  padding: 18px 18px 12px;
}

.panel-title {
  color: var(--app-text);
  font-size: 16px;
  font-weight: 700;
}

.panel-header p {
  margin: 6px 0 0;
  color: var(--app-text-soft);
  font-size: 12px;
}

.panel-link {
  border: none;
  background: none;
  color: #62a8ff;
  font-size: 13px;
  cursor: pointer;
}

.instance-shell {
  padding: 0 18px 18px;
}

.instance-card {
  border-radius: 8px;
  background: rgba(17, 32, 52, 0.5);
  border: 1px solid rgba(93, 120, 162, 0.18);
  overflow: hidden;
}

.instance-head {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 16px 12px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.instance-identity {
  display: flex;
  gap: 12px;
  min-width: 0;
}

.status-dot {
  width: 10px;
  height: 10px;
  margin-top: 8px;
  border-radius: 50%;
  background: #35d6a3;
  box-shadow: 0 0 12px rgba(53, 214, 163, 0.72);
  flex-shrink: 0;
}

.status-dot.offline {
  background: var(--app-danger);
  box-shadow: 0 0 12px rgba(255, 107, 125, 0.72);
}

.instance-identity strong {
  display: block;
  color: var(--app-text);
  font-size: 16px;
}

.instance-identity p {
  margin: 6px 0 0;
  color: var(--app-text-soft);
  font-size: 13px;
}

.instance-badges {
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: flex-end;
  flex-shrink: 0;
}

.instance-status,
.instance-reason {
  padding: 4px 10px;
  border-radius: 8px;
  font-size: 12px;
}

.instance-status {
  background: rgba(53, 214, 163, 0.12);
  color: #35d6a3;
}

.instance-status.offline {
  background: rgba(251, 113, 133, 0.12);
  color: var(--app-danger);
}

.instance-reason {
  background: rgba(79, 131, 255, 0.12);
  color: #9fc0ff;
}

.instance-purpose {
  padding: 8px 16px 0;
  color: #cfd9e7;
  font-size: 12px;
  line-height: 1.4;
}

.metric-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
  padding: 14px 16px;
}

.metric-item span,
.detail-item span {
  display: block;
  color: var(--app-text-soft);
  font-size: 12px;
}

.metric-item strong,
.detail-item strong {
  display: block;
  margin-top: 6px;
  color: var(--app-text);
  font-size: 14px;
  font-variant-numeric: tabular-nums;
}

.progress-bar {
  height: 4px;
  margin-top: 8px;
  background: rgba(93, 120, 162, 0.16);
  border-radius: 8px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, #4f83ff, #20d4ff);
}

.detail-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
  padding: 0 16px 16px;
}

.instance-empty {
  display: grid;
  gap: 8px;
  padding: 22px;
  color: var(--app-text-soft);
}

.instance-empty strong {
  color: var(--app-text);
  font-size: 15px;
}

@media (max-width: 860px) {
  .instance-head {
    flex-direction: column;
  }

  .instance-badges {
    align-items: flex-start;
  }

  .metric-grid,
  .detail-grid {
    grid-template-columns: 1fr;
  }
}
</style>
