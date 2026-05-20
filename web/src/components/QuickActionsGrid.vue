<script setup lang="ts">
import { useRouter } from 'vue-router'

const router = useRouter()

const actions = [
  { title: '创建实例', meta: '纳管节点', accent: 'blue', route: '/servers', icon: '<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>' },
  { title: 'API Token', meta: '访问凭证', accent: 'blue', route: '/users', icon: '<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m15.5 7.5 2.3 2.3a1 1 0 0 0 1.4 0l2.1-2.1a1 1 0 0 0 0-1.4L19 4"/><path d="m21 2-9.6 9.6"/><circle cx="7.5" cy="15.5" r="5.5"/></svg>' },
  { title: '资源监控', meta: '节点状态', accent: 'cyan', route: '/servers', icon: '<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg>' },
  { title: '系统告警', meta: '待处理项', accent: 'red', route: '/alerts', icon: '<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/></svg>' },
]

function openRoute(route: string) {
  void router.push(route)
}
</script>

<template>
  <section class="console-panel actions-panel">
    <div class="panel-header">
      <div>
        <div class="panel-title">快捷操作</div>
      </div>
    </div>

    <div class="actions-grid">
      <button
        v-for="action in actions"
        :key="action.title"
        type="button"
        class="action-card"
        :class="action.accent"
        @click="openRoute(action.route)"
      >
        <div class="action-icon" v-html="action.icon" />
        <div class="action-content">
          <strong>{{ action.title }}</strong>
          <span>{{ action.meta }}</span>
        </div>
        <span class="action-arrow" aria-hidden="true" />
      </button>
    </div>
  </section>
</template>

<style scoped>
.console-panel {
  height: 100%;
  padding: 18px;
  border-radius: 8px;
  border: 1px solid rgba(93, 120, 162, 0.2);
  background: rgba(15, 27, 46, 0.72);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
  display: flex;
  flex-direction: column;
}

.panel-header {
  margin-bottom: 12px;
}

.panel-title {
  color: var(--app-text);
  font-size: 16px;
  font-weight: 700;
}

.actions-grid {
  flex: 1;
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.action-card {
  position: relative;
  min-height: 78px;
  padding: 14px;
  text-align: left;
  border-radius: 8px;
  border: 1px solid rgba(93, 120, 162, 0.2);
  background: rgba(17, 32, 52, 0.5);
  color: var(--app-text);
  cursor: pointer;
  transition: background-color 0.18s ease, border-color 0.18s ease;
  display: flex;
  align-items: center;
  gap: 13px;
}

.action-content {
  flex: 1;
  min-width: 0;
}

.action-content strong {
  display: block;
  font-size: 14px;
  line-height: 1.25;
}

.action-content span {
  display: block;
  margin-top: 5px;
  color: var(--app-text-soft);
  font-size: 12px;
}

.action-icon {
  width: 40px;
  height: 40px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  border: 1px solid transparent;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.06);
}

.action-icon :deep(svg) {
  width: 21px;
  height: 21px;
  stroke-width: 2.2;
}

.action-arrow {
  width: 8px;
  height: 8px;
  flex-shrink: 0;
  border-top: 1px solid #7d8ca4;
  border-right: 1px solid #7d8ca4;
  transform: rotate(45deg);
}

.action-card.blue .action-icon {
  color: #62a8ff;
  background: linear-gradient(180deg, rgba(79, 131, 255, 0.2), rgba(41, 83, 174, 0.18));
  border-color: rgba(79, 131, 255, 0.34);
  filter: drop-shadow(0 0 10px rgba(79, 131, 255, 0.18));
}

.action-card.cyan .action-icon {
  color: var(--app-accent);
  background: linear-gradient(180deg, rgba(32, 212, 255, 0.18), rgba(19, 96, 132, 0.16));
  border-color: rgba(32, 212, 255, 0.3);
  filter: drop-shadow(0 0 10px rgba(32, 212, 255, 0.16));
}

.action-card.red .action-icon {
  color: var(--app-danger);
  background: linear-gradient(180deg, rgba(255, 107, 125, 0.16), rgba(114, 35, 55, 0.16));
  border-color: rgba(255, 107, 125, 0.3);
}

.action-card.blue:hover {
  background: rgba(79, 131, 255, 0.1);
  border-color: rgba(79, 131, 255, 0.34);
}

.action-card.cyan:hover {
  background: rgba(32, 212, 255, 0.08);
  border-color: rgba(32, 212, 255, 0.32);
}

.action-card.red:hover {
  background: rgba(255, 107, 125, 0.08);
  border-color: rgba(255, 107, 125, 0.34);
}

@media (max-width: 720px) {
  .actions-grid {
    grid-template-columns: 1fr;
  }
}
</style>
