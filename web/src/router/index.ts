import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { ensureSessionLoaded, useSession } from '../session'

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    component: () => import('../pages/LoginPage.vue'),
    meta: { title: '登录', public: true },
  },
  {
    path: '/',
    component: () => import('../pages/DashboardPage.vue'),
    meta: { title: '总览仪表盘' },
  },
  {
    path: '/servers',
    component: () => import('../pages/ServersPage.vue'),
    meta: { title: '服务器管理' },
  },
  {
    path: '/servers/:id',
    component: () => import('../pages/ServerDetailPage.vue'),
    meta: { title: '服务器详情' },
  },
  {
    path: '/commands',
    component: () => import('../pages/CommandPage.vue'),
    meta: { title: '命令执行' },
  },
  {
    path: '/users',
    component: () => import('../pages/UserManagementPage.vue'),
    meta: { title: '用户管理' },
  },
  {
    path: '/alerts',
    component: () => import('../pages/AlertPage.vue'),
    meta: { title: '告警中心' },
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/',
  },
]

export const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach(async (to) => {
  const { isAuthenticated } = useSession()

  try {
    await ensureSessionLoaded()
  } catch {
    if (to.meta.public) {
      return true
    }
    return {
      path: '/login',
      query: to.fullPath === '/' ? undefined : { redirect: to.fullPath },
    }
  }

  if (to.meta.public) {
    if (to.path === '/login' && isAuthenticated.value) {
      return '/'
    }
    return true
  }

  if (!isAuthenticated.value) {
    return {
      path: '/login',
      query: to.fullPath === '/' ? undefined : { redirect: to.fullPath },
    }
  }

  return true
})
