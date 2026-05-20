<script setup lang="ts">
import { computed } from 'vue'
import { NButton } from 'naive-ui'

const props = defineProps<{
  displayName: string
  lastUpdatedAt: string
  onlineServers: number
  totalServers: number
  activeAlerts: number
  sshFailures: number
  canManageInfrastructure: boolean
}>()

const emit = defineEmits<{
  (e: 'refresh'): void
  (e: 'create'): void
}>()

const metaItems = computed(() => [
  {
    label: '在线实例',
    value: `${props.onlineServers}/${props.totalServers}`,
  },
  {
    label: '活动告警',
    value: String(props.activeAlerts),
  },
  {
    label: 'SSH 异常',
    value: String(props.sshFailures),
  },
  {
    label: '最后更新',
    value: formatDateTime(props.lastUpdatedAt),
  },
])

function formatDateTime(value: string) {
  if (!value) return '等待节点上报'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}
</script>

<template>
  <section class="welcome-bar">
    <div class="welcome-content-wrapper">
      <div class="welcome-copy">
        <h1>{{ displayName }} 的运维概览</h1>
      </div>

      <div class="welcome-actions">
        <div class="meta-pills">
          <span v-for="item in metaItems" :key="item.label" class="meta-pill">
            <small>{{ item.label }}</small>
            <strong>{{ item.value }}</strong>
          </span>
        </div>
        <NButton ghost class="shell-btn icon-only" @click="emit('refresh')">
          <template #icon>
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/><path d="M3 3v5h5"/></svg>
          </template>
        </NButton>
        <NButton v-if="canManageInfrastructure" type="primary" class="create-btn" @click="emit('create')">新增实例</NButton>
      </div>
    </div>
  </section>
</template>

<style scoped>
.welcome-bar {
  position: relative;
  overflow: hidden;
}

.welcome-content-wrapper {
  position: relative;
  z-index: 1;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}

.welcome-copy {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.welcome-copy h1 {
  margin: 0;
  color: var(--app-text);
  font-size: 17px;
  font-weight: 700;
  letter-spacing: 0;
}

.welcome-subtitle {
  color: var(--app-text-soft);
  font-size: 13px;
}

.welcome-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  flex-wrap: wrap;
}

.meta-pills {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 6px;
}

.meta-pill {
  min-width: 96px;
  padding: 6px 10px;
  border-radius: 8px;
  border: 1px solid rgba(93, 120, 162, 0.24);
  background: rgba(20, 35, 58, 0.5);
}

.meta-pill small {
  display: block;
  color: var(--app-text-soft);
  font-size: 11px;
}

.meta-pill strong {
  display: block;
  margin-top: 2px;
  color: var(--app-text);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}

.shell-btn {
  border-radius: 8px;
  min-height: 30px;
}

.shell-btn.icon-only {
  width: 32px;
  min-width: 32px;
  padding: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}

.shell-btn :deep(.n-button__content) {
  color: #cdd9ee;
}

.create-btn {
  border-radius: 8px;
}

@media (max-width: 980px) {
  .welcome-content-wrapper {
    flex-direction: column;
    align-items: flex-start;
  }

  .welcome-actions,
  .meta-pills {
    justify-content: flex-start;
  }
}
</style>
