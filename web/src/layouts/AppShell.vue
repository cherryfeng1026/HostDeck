<script setup lang="ts">
import { NBadge, NDropdown, NInput, NPopover, useMessage } from 'naive-ui'
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getActivityFeed, getNotifications, searchWorkspace } from '../services/api'
import { logoutSession, useSession } from '../session'
import type { ActivityItem, NotificationItem } from '../types'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const { currentUser } = useSession()
const notifications = ref<NotificationItem[]>([])
const activityItems = ref<ActivityItem[]>([])
const searchKeyword = ref('')
const searchLoading = ref(false)
const searchResults = reactive({
  alerts: [] as NotificationItem[],
  commands: [] as ActivityItem[],
  authEvents: [] as ActivityItem[],
})
const isSidebarCollapsed = ref(false)

const dashboardIcon = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="7" height="9" x="3" y="3" rx="1"/><rect width="7" height="5" x="14" y="3" rx="1"/><rect width="7" height="9" x="14" y="12" rx="1"/><rect width="7" height="5" x="3" y="16" rx="1"/></svg>'
const serversIcon = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="20" height="8" x="2" y="2" rx="2" ry="2"/><rect width="20" height="8" x="2" y="14" rx="2" ry="2"/><line x1="6" x2="6.01" y1="6" y2="6"/><line x1="6" x2="6.01" y1="18" y2="18"/></svg>'
const commandsIcon = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="4 17 10 11 4 5"/><line x1="12" x2="20" y1="19" y2="19"/></svg>'
const usersIcon = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>'
const alertsIcon = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/></svg>'

const navItems = [
  {
    label: '总览仪表盘',
    icon: dashboardIcon,
    to: '/',
    match: (path: string) => path === '/',
  },
  {
    label: '服务器管理',
    icon: serversIcon,
    to: '/servers',
    match: (path: string) => path.startsWith('/servers'),
  },
  {
    label: '远程终端',
    icon: commandsIcon,
    to: '/commands',
    match: (path: string) => path.startsWith('/commands'),
  },
  {
    label: '监控告警',
    icon: alertsIcon,
    to: '/alerts',
    match: (path: string) => path.startsWith('/alerts'),
  },
  {
    label: '用户管理',
    icon: usersIcon,
    to: '/users',
    match: (path: string) => path.startsWith('/users'),
  },
]

const currentTitle = computed(() => {
  if (typeof route.meta.title === 'string' && route.meta.title.trim()) {
    return route.meta.title
  }
  return 'HostDeck'
})

const activityCount = computed(() => notifications.value.length + activityItems.value.length)
const displayName = computed(() => currentUser.value?.username || '未登录')
const roleLabel = computed(() => {
  switch (currentUser.value?.role) {
    case 'admin': return '管理员'
    case 'operator': return '运维'
    case 'viewer': return '只读'
    default: return '访客'
  }
})

const userMenuOptions = computed(() => [
  { label: '用户设置', key: 'users' },
  { label: '退出登录', key: 'logout' },
])

async function loadShellData() {
  try {
    const [notificationResponse, activityResponse] = await Promise.all([
      getNotifications(6),
      getActivityFeed(6),
    ])
    notifications.value = notificationResponse.items
    activityItems.value = activityResponse.items
  } catch {
    notifications.value = []
    activityItems.value = []
  }
}

async function handleSearch() {
  const keyword = searchKeyword.value.trim()
  if (!keyword) {
    searchResults.alerts = []
    searchResults.commands = []
    searchResults.authEvents = []
    return
  }

  searchLoading.value = true
  try {
    const results = await searchWorkspace(keyword, 5)
    searchResults.alerts = results.alerts
    searchResults.commands = results.commands
    searchResults.authEvents = results.authEvents
  } catch (error) {
    message.error(error instanceof Error ? error.message : '搜索失败')
  } finally {
    searchLoading.value = false
  }
}

async function handleUserMenuSelect(key: string) {
  if (key === 'users') {
    await router.push('/users')
    return
  }
  if (key === 'logout') {
    await logoutSession()
    await router.replace('/login')
  }
}

function formatDateTime(value: string) {
  if (!value) return '未知时间'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

function toggleSidebar() {
  isSidebarCollapsed.value = !isSidebarCollapsed.value
}

onMounted(() => {
  void loadShellData()
})
</script>

<template>
  <div class="layout-shell">
    <aside class="sidebar" :class="{ 'collapsed': isSidebarCollapsed }">
      <div class="brand-block">
        <div class="brand-logo" aria-hidden="true">
          <svg xmlns="http://www.w3.org/2000/svg" width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="url(#hostdeck-grad-shell)" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
            <defs>
              <linearGradient id="hostdeck-grad-shell" x1="0%" y1="0%" x2="100%" y2="100%">
                <stop offset="0%" stop-color="#2dd4bf" />
                <stop offset="100%" stop-color="#34d399" />
              </linearGradient>
            </defs>
            <path d="M12 3 L 6 13 H 18 Z" />
            <path d="M 9.5 16 H 4 L 1.5 20 H 22.5 L 20 16 H 14.5" />
          </svg>
        </div>
        <div class="brand-text-wrapper">
          <strong class="brand-title">HostDeck</strong>
          <p class="brand-copy">轻量级运维面板</p>
        </div>
      </div>

      <nav class="nav-list" aria-label="主导航">
        <router-link
          v-for="item in navItems"
          :key="item.to"
          :to="item.to"
          class="nav-item"
          :class="{ active: item.match(route.path) }"
          :title="isSidebarCollapsed ? item.label : undefined"
        >
          <span class="nav-icon" v-html="item.icon"></span>
          <span class="nav-label">{{ item.label }}</span>
        </router-link>
      </nav>
    </aside>

    <main class="main-panel">
      <header class="topbar">
        <div class="topbar-leading">
          <div class="sidebar-toggle" @click="toggleSidebar" :title="isSidebarCollapsed ? '展开侧边栏' : '收起侧边栏'">
            <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="18" x="3" y="3" rx="2"/><path d="M9 3v18"/></svg>
          </div>
          <div class="topbar-route">
            <span class="topbar-route__brand">HostDeck</span>
            <span class="topbar-route__divider">/</span>
            <strong>{{ currentTitle }}</strong>
          </div>
        </div>

        <div class="topbar-tools">
          <div class="search-box">
            <n-input v-model:value="searchKeyword" clearable placeholder="Cmd+K 全局搜索..." class="dark-input" @keydown.enter.prevent="handleSearch">
              <template #prefix>🔍</template>
            </n-input>
            
            <div v-if="searchKeyword.trim()" class="search-results">
              <div v-if="searchResults.alerts.length" class="search-group">
                <strong>告警</strong>
                <div v-for="item in searchResults.alerts" :key="`${item.kind}-${item.title}-${item.createdAt}`" class="search-item">
                  <span>{{ item.title }}</span>
                  <small>{{ item.message }}</small>
                </div>
              </div>
              <div v-if="searchResults.commands.length" class="search-group">
                <strong>命令</strong>
                <div v-for="item in searchResults.commands" :key="`${item.kind}-${item.summary}-${item.createdAt}`" class="search-item">
                  <span>{{ item.serverName || item.title }}</span>
                  <small>{{ item.summary }}</small>
                </div>
              </div>
              <div v-if="searchResults.authEvents.length" class="search-group">
                <strong>认证</strong>
                <div v-for="item in searchResults.authEvents" :key="`${item.kind}-${item.summary}-${item.createdAt}`" class="search-item">
                  <span>{{ item.title }}</span>
                  <small>{{ item.summary }}</small>
                </div>
              </div>
            </div>
          </div>

          <div class="toolbar-actions">
            <!-- Notifications Popover -->
            <n-popover trigger="click" placement="bottom-end" :show-arrow="false" style="padding: 0; background: rgba(20,20,25,0.9); border: 1px solid rgba(255,255,255,0.1); border-radius: 16px; backdrop-filter: blur(20px);">
              <template #trigger>
                <div class="action-btn">
                  <n-badge :value="activityCount" :max="99" color="#f43f5e">
                    <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/></svg>
                  </n-badge>
                </div>
              </template>
              <div class="activity-preview">
                <div class="activity-head">
                  <strong>系统动态</strong>
                  <small>仅展示关键安全事件与命令执行</small>
                </div>
                <div v-if="!notifications.length && !activityItems.length" class="activity-empty">暂无最新动态</div>
                <div v-for="item in notifications.slice(0, 2)" :key="`${item.kind}-${item.title}-${item.createdAt}`" class="activity-item">
                  <span>{{ item.title }}</span>
                  <small>{{ item.message }}</small>
                </div>
                <div v-for="item in activityItems.slice(0, 2)" :key="`${item.kind}-${item.title}-${item.createdAt}`" class="activity-item">
                  <span>{{ item.title }}</span>
                  <small>{{ item.summary }} · {{ formatDateTime(item.createdAt) }}</small>
                </div>
              </div>
            </n-popover>

            <!-- User Dropdown -->
            <n-dropdown trigger="click" :options="userMenuOptions" @select="handleUserMenuSelect">
              <div class="user-chip">
                <div class="user-avatar">{{ displayName.charAt(0).toUpperCase() }}</div>
                <div class="user-info">
                  <span class="user-name">{{ displayName }}</span>
                  <span class="user-role">{{ roleLabel }}</span>
                </div>
              </div>
            </n-dropdown>
          </div>
        </div>
      </header>

      <section class="page-slot">
        <slot />
      </section>
    </main>
  </div>
</template>

<style scoped>
.layout-shell {
  display: flex;
  min-height: 100vh;
  background: transparent;
}

/* Sidebar */
.sidebar {
  position: relative;
  width: 240px;
  padding: 24px 16px;
  display: flex;
  flex-direction: column;
  gap: 32px;
  border-right: 1px solid rgba(255, 255, 255, 0.05);
  background: rgba(5, 5, 5, 0.3);
  backdrop-filter: blur(10px);
  z-index: 10;
  transition: width 0.3s ease;
}

.sidebar.collapsed {
  width: 80px;
  padding: 24px 12px;
}

.brand-block {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 8px;
  overflow: hidden;
}

.sidebar.collapsed .brand-block {
  padding: 0;
  justify-content: center;
}

.brand-logo {
  display: flex;
  align-items: center;
  justify-content: center;
  filter: drop-shadow(0 0 8px rgba(45, 212, 191, 0.6));
  flex: none;
}

.brand-text-wrapper {
  overflow: hidden;
  white-space: nowrap;
  transition: opacity 0.2s;
  opacity: 1;
}

.sidebar.collapsed .brand-text-wrapper {
  opacity: 0;
  width: 0;
  display: none;
}

.brand-title {
  display: block;
  font-size: 18px;
  font-weight: 700;
  letter-spacing: -0.5px;
  color: #fff;
}

.brand-copy {
  margin: 2px 0 0;
  color: #a1a1aa;
  font-size: 12px;
}

/* Navigation */
.nav-list {
  display: grid;
  gap: 8px;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  border-radius: 12px;
  color: #a1a1aa;
  text-decoration: none;
  transition: all 0.2s ease;
  font-weight: 500;
  overflow: hidden;
}

.sidebar.collapsed .nav-item {
  justify-content: center;
  padding: 12px;
}

.nav-item:hover {
  color: #fff;
  background: rgba(255, 255, 255, 0.05);
}

.nav-item.active {
  color: #10b981;
  background: rgba(16, 185, 129, 0.1);
}

.nav-icon {
  width: 20px;
  height: 20px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.nav-icon svg {
  width: 100%;
  height: 100%;
}

.nav-label {
  font-size: 14px;
  white-space: nowrap;
  opacity: 1;
  transition: opacity 0.2s;
}

.sidebar.collapsed .nav-label {
  opacity: 0;
  width: 0;
  display: none;
}

.sidebar-toggle {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 8px;
  color: #a1a1aa;
  cursor: pointer;
  transition: all 0.2s ease;
  margin-right: 16px;
}

.sidebar-toggle:hover {
  background: rgba(255, 255, 255, 0.05);
  color: #fff;
}

/* Main Panel */
.main-panel {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

/* Topbar */
.topbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 32px;
  height: 72px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
  background: rgba(5, 5, 5, 0.3);
  backdrop-filter: blur(10px);
  z-index: 5;
}

.topbar-leading {
  display: flex;
  align-items: center;
}

.topbar-route {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #a1a1aa;
}

.topbar-route__brand {
  color: #10b981;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.topbar-route__divider {
  color: #52525b;
}

.topbar-route strong {
  color: #fff;
  font-size: 15px;
}

/* Topbar Tools */
.topbar-tools {
  display: flex;
  align-items: center;
  gap: 24px;
}

.search-box {
  width: 280px;
  position: relative;
}

.search-results {
  position: absolute;
  top: 100%;
  left: 0;
  right: 0;
  margin-top: 8px;
  background: rgba(20, 20, 25, 0.9);
  border: 1px solid rgba(255, 255, 255, 0.1);
  backdrop-filter: blur(20px);
  border-radius: 12px;
  padding: 12px;
  display: grid;
  gap: 12px;
  max-height: 320px;
  overflow: auto;
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.5);
  z-index: 100;
}

.search-group {
  display: grid;
  gap: 8px;
}

.search-group strong {
  color: #f8fafc;
  font-size: 12px;
}

.search-item {
  display: grid;
  gap: 4px;
  padding: 8px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.03);
}

.search-item span {
  color: #ededed;
  font-size: 13px;
}

.search-item small {
  color: #a1a1aa;
  font-size: 12px;
  line-height: 1.5;
}

:deep(.dark-input .n-input__border) {
  border-color: rgba(255, 255, 255, 0.1) !important;
}
:deep(.dark-input) {
  background: rgba(255, 255, 255, 0.05) !important;
  color: #fff !important;
}

.toolbar-actions {
  display: flex;
  align-items: center;
  gap: 16px;
}

.action-btn {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #a1a1aa;
  cursor: pointer;
  transition: all 0.2s;
  background: rgba(255, 255, 255, 0.03);
  border: 1px solid rgba(255, 255, 255, 0.05);
}
.action-btn:hover {
  color: #fff;
  background: rgba(255, 255, 255, 0.08);
}

.user-chip {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 6px 12px 6px 6px;
  border-radius: 20px;
  cursor: pointer;
  transition: all 0.2s;
  border: 1px solid transparent;
}
.user-chip:hover {
  background: rgba(255, 255, 255, 0.05);
  border-color: rgba(255, 255, 255, 0.08);
}

.user-avatar {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: #3b82f6;
  color: white;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 600;
  font-size: 14px;
}

.user-info {
  display: flex;
  flex-direction: column;
}

.user-name {
  color: #fff;
  font-size: 13px;
  font-weight: 600;
  line-height: 1.2;
}

.user-role {
  color: #a1a1aa;
  font-size: 11px;
}

/* Activity Popover */
.activity-preview {
  width: min(320px, 80vw);
  display: grid;
  gap: 12px;
  padding: 16px;
}

.activity-head {
  display: grid;
  gap: 4px;
  color: #fff;
  font-size: 14px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
  padding-bottom: 12px;
}

.activity-head small {
  color: #a1a1aa;
  font-size: 12px;
  font-weight: 400;
}

.activity-item {
  display: grid;
  gap: 4px;
  padding: 8px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.03);
}
.activity-item span {
  color: #fff;
  font-size: 13px;
}
.activity-item small {
  color: #a1a1aa;
  font-size: 12px;
}
.activity-empty {
  color: #71717a;
  text-align: center;
  padding: 16px 0;
}

.page-slot {
  flex: 1;
  min-height: 0;
  display: flex;
}

@media (max-width: 960px) {
  .layout-shell {
    flex-direction: column;
  }
  .sidebar {
    width: auto;
    border-right: none;
    border-bottom: 1px solid rgba(255, 255, 255, 0.08);
    padding: 16px 20px;
    gap: 20px;
  }
  .topbar {
    padding: 0 20px;
    height: 60px;
  }
  .search-box {
    display: none; /* 移动端隐藏搜索 */
  }
}
</style>
