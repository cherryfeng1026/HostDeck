<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import type { ShellEventItem } from '../types'

const props = defineProps<{
  items: ShellEventItem[]
}>()

const router = useRouter()

const visibleItems = computed(() => props.items.slice(0, 4))

function toneFor(item: ShellEventItem) {
  if (item.severity === 'critical' || item.severity === 'error') return 'danger'
  if (item.kind === 'command') return 'info'
  if (item.kind === 'audit') return 'success'
  return 'warning'
}

function iconFor(item: ShellEventItem) {
  if (item.kind === 'command') {
    return '<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="8 7 3 12 8 17"/><polyline points="16 7 21 12 16 17"/><line x1="10" y1="19" x2="14" y2="5"/></svg>'
  }
  if (item.kind === 'audit') {
    return '<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10Z"/></svg>'
  }
  if (item.kind === 'auth') {
    return '<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>'
  }
  return '<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>'
}

function formatTime(value: string) {
  if (!value) return '未知时间'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

async function openItem(item: ShellEventItem) {
  if (!item.routePath) return
  await router.push(item.routePath)
}
</script>

<template>
  <section class="console-panel timeline-panel">
    <div class="panel-header">
      <div>
        <div class="panel-title">最近活动</div>
      </div>
    </div>

    <div v-if="visibleItems.length" class="timeline-list">
      <article
        v-for="item in visibleItems"
        :key="`${item.kind}-${item.title}-${item.createdAt}`"
        class="timeline-item"
        :class="{ clickable: !!item.routePath }"
        @click="openItem(item)"
      >
        <div class="timeline-icon-wrap" :class="toneFor(item)">
          <span v-html="iconFor(item)" />
        </div>
        <div class="timeline-content">
          <div class="timeline-text">
            <strong>{{ item.title }}</strong>
            <p>{{ item.summary }}</p>
          </div>
          <span class="timeline-time">{{ formatTime(item.createdAt) }}</span>
        </div>
      </article>
    </div>

    <div v-else class="timeline-empty">
      <strong>暂无活动</strong>
      <span>当前没有新的系统事件、告警历史或操作记录。</span>
    </div>
  </section>
</template>

<style scoped>
.console-panel {
  height: 100%;
  padding: 16px 18px 18px;
  border-radius: 8px;
  border: 1px solid rgba(93, 120, 162, 0.2);
  background: linear-gradient(180deg, rgba(15, 27, 46, 0.72), rgba(11, 20, 34, 0.82));
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.05), 0 14px 30px rgba(0, 8, 22, 0.22);
  display: flex;
  flex-direction: column;
}

.panel-header {
  margin-bottom: 10px;
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

.timeline-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.timeline-item {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}

.timeline-item.clickable {
  cursor: pointer;
}

.timeline-icon-wrap {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  margin-top: 2px;
}

.timeline-icon-wrap.success {
  background: rgba(53, 214, 163, 0.18);
  color: #35d6a3;
}

.timeline-icon-wrap.info {
  background: rgba(79, 131, 255, 0.18);
  color: #62a8ff;
}

.timeline-icon-wrap.warning {
  background: rgba(232, 180, 95, 0.2);
  color: var(--app-warning);
}

.timeline-icon-wrap.danger {
  background: rgba(255, 107, 125, 0.2);
  color: var(--app-danger);
}

.timeline-content {
  flex: 1;
  min-width: 0;
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  padding-bottom: 12px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.timeline-item:last-child .timeline-content {
  border-bottom: none;
  padding-bottom: 0;
}

.timeline-text {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.timeline-text strong {
  color: var(--app-text);
  font-size: 14px;
  font-weight: 500;
}

.timeline-text p {
  margin: 0;
  color: var(--app-text-soft);
  font-size: 12px;
  line-height: 1.35;
}

.timeline-time {
  color: var(--app-text-soft);
  font-size: 12px;
  white-space: nowrap;
  flex-shrink: 0;
}

.timeline-empty {
  display: grid;
  gap: 8px;
  color: var(--app-text-soft);
}

.timeline-empty strong {
  color: var(--app-text);
  font-size: 15px;
}
</style>
