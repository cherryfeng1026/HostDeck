<script setup lang="ts">
import { NBadge, NButton, NDropdown, NInput, NPopover, useMessage } from 'naive-ui'
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getNotifications, markNotificationsRead, searchWorkspace } from '../services/api'
import { logoutSession, useSession } from '../session'
import type { ShellEventItem } from '../types'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const { currentUser } = useSession()
const notifications = ref<ShellEventItem[]>([])
const unreadNotificationCount = ref(0)
const searchKeyword = ref('')
const searchLoading = ref(false)
const searchResults = ref<ShellEventItem[]>([])
const isSidebarCollapsed = ref(false)

const dashboardIcon = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3.5" y="3.5" width="7" height="7" rx="1.5"/><rect x="13.5" y="3.5" width="7" height="7" rx="1.5"/><rect x="3.5" y="13.5" width="7" height="7" rx="1.5"/><rect x="13.5" y="13.5" width="7" height="7" rx="1.5"/></svg>'
const serversIcon = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="6" rx="2"/><rect x="3" y="14" width="18" height="6" rx="2"/><path d="M7 7h.01"/><path d="M7 17h.01"/></svg>'
const commandsIcon = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="5" width="18" height="14" rx="2"/><path d="m8 10 3 2-3 2"/><path d="M13 15h3"/></svg>'
const usersIcon = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M16 21v-2a4 4 0 0 0-4-4H7a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>'
const alertsIcon = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 8a6 6 0 0 0-12 0c0 6.5-3 8.5-3 8.5h18S18 14.5 18 8"/><path d="M10 20a2 2 0 0 0 4 0"/></svg>'

const navItems = computed(() => {
  const items = [
    {
      label: '概览',
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
  ]
  if (currentUser.value && currentUser.value.role !== 'viewer') {
    items.push({
      label: '远程终端',
      icon: commandsIcon,
      to: '/commands',
      match: (path: string) => path.startsWith('/commands'),
    })
  }
  items.push(
    {
      label: '系统告警',
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
  )
  return items
})

const currentTitle = computed(() => {
  if (typeof route.meta.title === 'string' && route.meta.title.trim()) {
    return route.meta.title
  }
  return '概览'
})

const activityCount = computed(() => unreadNotificationCount.value)
const visibleNotifications = computed(() => notifications.value.slice(0, 6))
const notificationSummary = computed(() => (
  unreadNotificationCount.value > 0 ? `${unreadNotificationCount.value} 条未读通知` : '当前无未读通知'
))
const displayName = computed(() => currentUser.value?.username || '未登录')
const roleLabel = computed(() => {
  switch (currentUser.value?.role) {
    case 'admin':
      return '管理员'
    case 'operator':
      return '运维'
    case 'viewer':
      return '只读'
    default:
      return '访客'
  }
})

const userMenuOptions = computed(() => [
  { label: '用户设置', key: 'users' },
  { label: '退出登录', key: 'logout' },
])
const userMenuProps = () => ({ class: 'shell-user-menu' })

async function loadShellData() {
  try {
    const response = await getNotifications(20)
    notifications.value = response.items
    unreadNotificationCount.value = response.unreadCount
  } catch {
    notifications.value = []
    unreadNotificationCount.value = 0
  }
}

async function handleNotificationsVisible(show: boolean) {
  if (!show) {
    return
  }
  await loadShellData()
}

async function markAllNotificationsRead() {
  try {
    await markNotificationsRead(new Date().toISOString())
    unreadNotificationCount.value = 0
    notifications.value = notifications.value.map((item) => ({ ...item, isRead: true }))
    await loadShellData()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '标记通知失败')
  }
}

async function handleSearch() {
  const keyword = searchKeyword.value.trim()
  if (!keyword) {
    searchResults.value = []
    return
  }

  searchLoading.value = true
  try {
    const results = await searchWorkspace(keyword, 6)
    searchResults.value = results.items
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

async function openShellItem(item: ShellEventItem) {
  if (!item.routePath) {
    return
  }
  searchResults.value = []
  await router.push(item.routePath)
}

function openShellItemFromKeyboard(event: KeyboardEvent, item: ShellEventItem) {
  if (event.key !== 'Enter' && event.key !== ' ') {
    return
  }
  event.preventDefault()
  void openShellItem(item)
}

function activateFromKeyboard(event: KeyboardEvent, action: () => void) {
  if (event.key !== 'Enter' && event.key !== ' ') {
    return
  }
  event.preventDefault()
  action()
}

function triggerClickFromKeyboard(event: KeyboardEvent) {
  if (event.key !== 'Enter' && event.key !== ' ') {
    return
  }
  event.preventDefault()
  ;(event.currentTarget as HTMLElement | null)?.click()
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
    <aside class="sidebar" :class="{ collapsed: isSidebarCollapsed }">
      <div class="brand-block">
        <div class="brand-mark" aria-hidden="true">
          <svg xmlns="http://www.w3.org/2000/svg" width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="url(#hostdeck-grad-shell)" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
            <defs>
              <linearGradient id="hostdeck-grad-shell" x1="0%" y1="0%" x2="100%" y2="100%">
                <stop offset="0%" stop-color="#2dd4bf" />
                <stop offset="100%" stop-color="var(--app-accent)" />
              </linearGradient>
            </defs>
            <path d="M12 3 L 6 13 H 18 Z" />
            <path d="M 9.5 16 H 4 L 1.5 20 H 22.5 L 20 16 H 14.5" />
          </svg>
        </div>
        <div v-if="!isSidebarCollapsed" class="brand-copy">
          <strong>HostDeck</strong>
        </div>
      </div>

      <nav class="nav-list" aria-label="主导航">
        <router-link
          v-for="item in navItems"
          :key="item.label"
          :to="item.to"
          class="nav-item"
          :class="{ active: item.match(route.path) }"
          :title="isSidebarCollapsed ? item.label : undefined"
          :aria-label="item.label"
        >
          <span class="nav-icon" v-html="item.icon" />
          <span v-if="!isSidebarCollapsed" class="nav-label">{{ item.label }}</span>
        </router-link>
      </nav>

      <div class="sidebar-fill" />

      <NDropdown trigger="click" :options="userMenuOptions" :menu-props="userMenuProps" @select="handleUserMenuSelect">
        <div
          class="sidebar-footer"
          :class="{ collapsed: isSidebarCollapsed }"
          role="button"
          tabindex="0"
          aria-label="打开用户菜单"
          @keydown="triggerClickFromKeyboard"
        >
          <div class="sidebar-user-avatar">{{ displayName.charAt(0).toUpperCase() }}</div>
          <div v-if="!isSidebarCollapsed" class="sidebar-user-copy">
            <strong>{{ displayName }}</strong>
            <span>{{ roleLabel }}</span>
          </div>
        </div>
      </NDropdown>
    </aside>

    <main class="main-panel">
      <header class="topbar">
        <div class="topbar-leading">
          <div
            class="sidebar-toggle"
            role="button"
            tabindex="0"
            :title="isSidebarCollapsed ? '展开侧边栏' : '收起侧边栏'"
            :aria-label="isSidebarCollapsed ? '展开侧边栏' : '收起侧边栏'"
            @click="toggleSidebar"
            @keydown="activateFromKeyboard($event, toggleSidebar)"
          >
            <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="4" y1="7" x2="20" y2="7" />
              <line x1="4" y1="12" x2="16" y2="12" />
              <line x1="4" y1="17" x2="20" y2="17" />
            </svg>
          </div>

          <div class="topbar-route">
            <span class="topbar-route__item muted">首页</span>
            <span class="topbar-route__divider">/</span>
            <span class="topbar-route__item accent">HOSTDECK</span>
            <span class="topbar-route__divider">/</span>
            <strong>{{ currentTitle }}</strong>
          </div>
        </div>

        <div class="topbar-tools">
          <div class="search-box">
            <NInput
              v-model:value="searchKeyword"
              clearable
              placeholder="搜索服务、实例、IP ..."
              class="dark-input"
              @keydown.enter.prevent="handleSearch"
            >
              <template #prefix>
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>
              </template>
              <template #suffix>
                <span class="search-shortcut">Ctrl + K</span>
              </template>
            </NInput>

            <div v-if="searchKeyword.trim()" class="search-results">
              <div v-if="searchLoading" class="activity-empty">搜索中...</div>
              <div v-else-if="!searchResults.length" class="activity-empty">暂无匹配结果</div>
              <div
                v-for="item in searchResults"
                :key="`${item.kind}-${item.title}-${item.createdAt}`"
                class="search-item"
                :class="{ clickable: !!item.routePath }"
                :role="item.routePath ? 'button' : undefined"
                :tabindex="item.routePath ? 0 : undefined"
                @click="openShellItem(item)"
                @keydown="openShellItemFromKeyboard($event, item)"
              >
                <span>{{ item.title }}</span>
                <small>{{ item.summary }}</small>
              </div>
            </div>
          </div>

          <div class="toolbar-actions">
            <NPopover
              trigger="click"
              placement="bottom-end"
              :show-arrow="false"
              style="padding: 0; background: rgba(10, 19, 33, 0.9); border: 1px solid rgba(93,120,162,0.22); border-radius: 8px; backdrop-filter: blur(18px);"
              @update:show="handleNotificationsVisible"
            >
              <template #trigger>
                <div class="action-btn" role="button" tabindex="0" aria-label="打开系统动态通知" @keydown="triggerClickFromKeyboard">
                  <NBadge :value="activityCount" :max="99" color="#ff6b7d">
                    <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/></svg>
                  </NBadge>
                </div>
              </template>
              <div class="activity-preview">
                <div class="activity-head">
                  <div>
                    <strong>系统通知</strong>
                    <small>{{ notificationSummary }}</small>
                  </div>
                  <NButton v-if="unreadNotificationCount > 0" size="tiny" tertiary @click="markAllNotificationsRead">全部已读</NButton>
                </div>
                <div v-if="!notifications.length" class="activity-empty">暂无通知</div>
                <div
                  v-for="item in visibleNotifications"
                  :key="`${item.kind}-${item.title}-${item.createdAt}`"
                  class="activity-item"
                  :class="{ clickable: !!item.routePath, unread: item.isRead === false }"
                  :role="item.routePath ? 'button' : undefined"
                  :tabindex="item.routePath ? 0 : undefined"
                  @click="openShellItem(item)"
                  @keydown="openShellItemFromKeyboard($event, item)"
                >
                  <span>{{ item.title }}</span>
                  <small>{{ item.summary }}</small>
                </div>
              </div>
            </NPopover>

            <NDropdown trigger="click" :options="userMenuOptions" :menu-props="userMenuProps" @select="handleUserMenuSelect">
              <div class="user-chip" role="button" tabindex="0" aria-label="打开用户菜单" @keydown="triggerClickFromKeyboard">
                <div class="user-avatar">{{ displayName.charAt(0).toUpperCase() }}</div>
                <div class="user-info">
                  <span class="user-name">{{ displayName }}</span>
                  <span class="user-role">{{ roleLabel }}</span>
                </div>
              </div>
            </NDropdown>
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
  height: 100vh;
  overflow: hidden;
  background: transparent;
}

.sidebar {
  position: relative;
  width: 272px;
  padding: 20px 18px 18px;
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  border-right: 1px solid rgba(93, 120, 162, 0.16);
  background:
    linear-gradient(180deg, rgba(13, 25, 44, 0.72), rgba(8, 17, 31, 0.82)),
    rgba(9, 18, 32, 0.52);
  box-shadow: inset -1px 0 0 rgba(255, 255, 255, 0.035);
  backdrop-filter: saturate(120%);
  transition: width 0.24s ease, padding 0.24s ease;
}

.sidebar::before {
  content: '';
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgba(99, 146, 205, 0.045) 1px, transparent 1px),
    linear-gradient(90deg, rgba(99, 146, 205, 0.045) 1px, transparent 1px);
  background-size: 28px 28px;
  opacity: 0.1;
  pointer-events: none;
}

.sidebar.collapsed {
  width: 92px;
  padding-inline: 12px;
}

.brand-block,
.nav-list,
.sidebar-footer {
  position: relative;
  z-index: 1;
}

.brand-block {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 56px;
}

.brand-mark {
  width: 34px;
  height: 34px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  filter: drop-shadow(0 0 10px rgba(32, 212, 255, 0.56));
}

.brand-mark svg {
  width: 28px;
  height: 28px;
}

.brand-copy strong {
  display: block;
  color: var(--app-text);
  font-size: 18px;
  font-weight: 700;
}

.brand-copy span {
  display: block;
  margin-top: 4px;
  color: var(--app-text-soft);
  font-size: 12px;
}

.nav-list {
  display: grid;
  gap: 8px;
  margin-top: 20px;
}

.nav-item {
  position: relative;
  display: flex;
  align-items: center;
  gap: 14px;
  min-height: 46px;
  padding: 0 14px;
  border-radius: 8px;
  color: var(--app-text-soft);
  text-decoration: none;
  transition: background-color 0.2s ease, color 0.2s ease, transform 0.2s ease;
}

.sidebar.collapsed .nav-item {
  justify-content: center;
  padding-inline: 0;
}

.nav-item:hover {
  color: var(--app-text);
  background: rgba(79, 131, 255, 0.08);
}

.nav-item.active {
  color: #62a8ff;
  background: linear-gradient(90deg, rgba(79, 131, 255, 0.18), rgba(79, 131, 255, 0.08));
}

.nav-item.active::before {
  content: '';
  position: absolute;
  left: 0;
  top: 10px;
  bottom: 10px;
  width: 3px;
  border-radius: 0 999px 999px 0;
  background: #4f83ff;
  box-shadow: 0 0 14px rgba(79, 131, 255, 0.72);
}

.nav-icon {
  width: 22px;
  height: 22px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.nav-icon :deep(svg) {
  width: 20px;
  height: 20px;
  stroke-width: 1.9;
}

.nav-label {
  white-space: nowrap;
  font-size: 15px;
}

.sidebar-fill {
  flex: 1;
}

.sidebar-footer {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 68px;
  padding: 10px 12px;
  border-radius: 8px;
  background: rgba(20, 35, 58, 0.5);
  border: 1px solid rgba(93, 120, 162, 0.18);
  cursor: pointer;
  transition: background-color 0.2s ease, border-color 0.2s ease;
}

.sidebar-footer:hover {
  background: rgba(31, 51, 82, 0.64);
  border-color: rgba(93, 120, 162, 0.32);
}

.sidebar-footer.collapsed {
  justify-content: center;
  padding-inline: 0;
}

.sidebar-user-avatar {
  width: 40px;
  height: 40px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: rgba(79, 131, 255, 0.2);
  border: 1px solid rgba(79, 131, 255, 0.32);
  color: #e4efff;
  font-weight: 700;
}

.sidebar-user-copy strong {
  display: block;
  color: var(--app-text);
  font-size: 14px;
}

.sidebar-user-copy span {
  display: block;
  margin-top: 4px;
  color: var(--app-text-soft);
  font-size: 12px;
}

.main-panel {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.topbar {
  position: sticky;
  top: 0;
  z-index: 5;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  min-height: 64px;
  padding: 0 20px;
  border-bottom: 1px solid rgba(93, 120, 162, 0.18);
  box-shadow: inset 0 -1px 0 rgba(255, 255, 255, 0.03);
  background: rgba(8, 17, 31, 0.58);
  backdrop-filter: saturate(125%);
}

.topbar-leading {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.sidebar-toggle {
  width: 34px;
  height: 34px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  color: var(--app-text-soft);
  cursor: pointer;
  transition: background-color 0.2s ease, color 0.2s ease;
}

.sidebar-toggle:hover {
  color: var(--app-text);
  background: rgba(79, 131, 255, 0.1);
}

.topbar-route {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.topbar-route__item,
.topbar-route strong {
  white-space: nowrap;
}

.topbar-route__item {
  color: #cfd8ea;
  font-size: 13px;
}

.topbar-route__item.muted {
  color: var(--app-text-soft);
}

.topbar-route__item.accent {
  color: #62a8ff;
  font-weight: 700;
  letter-spacing: 0.1em;
}

.topbar-route__divider {
  color: #607089;
}

.topbar-route strong {
  color: var(--app-text);
  font-size: 15px;
}

.topbar-tools {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.search-box {
  position: relative;
  width: min(380px, 38vw);
  flex-shrink: 1;
}

.search-results {
  position: absolute;
  top: calc(100% + 10px);
  left: 0;
  right: 0;
  display: grid;
  gap: 10px;
  padding: 14px;
  border-radius: 8px;
  background: rgba(10, 19, 33, 0.96);
  border: 1px solid rgba(93, 120, 162, 0.22);
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.35);
}

.search-item,
.activity-item {
  display: grid;
  gap: 4px;
  padding: 10px 12px;
  border-radius: 8px;
  background: rgba(20, 35, 58, 0.5);
}

.search-item.clickable,
.activity-item.clickable {
  cursor: pointer;
}

.search-item.clickable:hover,
.activity-item.clickable:hover {
  background: rgba(255, 255, 255, 0.08);
}

.search-item span,
.activity-item span {
  color: var(--app-text);
  font-size: 13px;
}

.search-item small,
.activity-item small {
  color: var(--app-text-soft);
  font-size: 12px;
  line-height: 1.5;
}

:deep(.dark-input .n-input-wrapper) {
  min-height: 42px;
  border-radius: 8px !important;
  background: linear-gradient(180deg, rgba(16, 29, 49, 0.68), rgba(10, 19, 33, 0.76)) !important;
  border: 1px solid rgba(93, 120, 162, 0.22);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.05), 0 8px 18px rgba(0, 8, 22, 0.18);
}

:deep(.dark-input .n-input__input-el),
:deep(.dark-input .n-input__placeholder) {
  color: var(--app-text) !important;
}

:deep(.dark-input .n-input__suffix),
:deep(.dark-input .n-input__prefix) {
  color: var(--app-text-soft);
}

:deep(.dark-input .n-input__border),
:deep(.dark-input .n-input__state-border) {
  display: none !important;
}

.search-shortcut {
  font-size: 11px;
  color: var(--app-text-soft);
  white-space: nowrap;
}

.toolbar-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.action-btn {
  width: 34px;
  height: 34px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: transparent;
  border: 1px solid transparent;
  color: #e8f0ff;
  cursor: pointer;
}

.action-btn:hover {
  background: rgba(79, 131, 255, 0.1);
  border-color: rgba(93, 120, 162, 0.24);
  color: #fff;
}

.user-chip {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 4px 10px 4px 4px;
  border-radius: 8px;
  background: rgba(20, 35, 58, 0.46);
  border: 1px solid rgba(93, 120, 162, 0.2);
  cursor: pointer;
}

.user-avatar {
  width: 32px;
  height: 32px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: rgba(79, 131, 255, 0.22);
  border: 1px solid rgba(79, 131, 255, 0.34);
  color: #e4efff;
  font-weight: 700;
}

.user-info {
  display: flex;
  flex-direction: column;
}

.user-name {
  color: var(--app-text);
  font-size: 12px;
  font-weight: 700;
}

.user-role {
  color: var(--app-text-soft);
  font-size: 10px;
}

.activity-preview {
  width: min(340px, 82vw);
  display: grid;
  gap: 10px;
  padding: 16px;
  max-height: min(520px, 78vh);
  overflow: auto;
}

.activity-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding-bottom: 10px;
  border-bottom: 1px solid rgba(93, 120, 162, 0.18);
}

.activity-head > div {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.activity-head strong {
  color: var(--app-text);
}

.activity-head small {
  display: block;
}

.activity-head small,
.activity-empty {
  color: var(--app-text-soft);
  font-size: 12px;
}

.activity-item.unread {
  border: 1px solid rgba(255, 107, 125, 0.28);
  background: rgba(255, 107, 125, 0.1);
}

.page-slot {
  flex: 1;
  min-height: 0;
  display: flex;
  overflow: hidden;
}

@media (max-width: 1080px) {
  .topbar {
    padding-inline: 18px;
  }

  .search-box {
    width: min(300px, 34vw);
  }
}

@media (max-width: 1180px) {
  .topbar-route__item,
  .topbar-route__divider {
    display: none;
  }

  .topbar-route strong {
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .search-box {
    width: clamp(180px, 28vw, 260px);
  }
}

@media (max-width: 920px) {
  .layout-shell {
    flex-direction: column;
    height: auto;
    min-height: 100vh;
  }

  .sidebar,
  .sidebar.collapsed {
    width: 100%;
    padding: 14px 16px;
  }

  .topbar {
    flex-direction: column;
    align-items: stretch;
    padding: 14px 16px;
  }

  .topbar-tools {
    justify-content: space-between;
  }

  .search-box {
    width: 100%;
  }
}

@media (max-width: 640px) {
  .user-info,
  .topbar-route {
    display: none;
  }

  .toolbar-actions {
    gap: 10px;
  }
}
</style>
