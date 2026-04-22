import { computed, reactive } from 'vue'
import { APIError, changePassword, getAuthStatus, getCurrentUser, login, logout } from './services/api'
import type { User, UserPermissions } from './types'

const state = reactive({
  initialized: false,
  loading: false,
  authenticating: false,
  user: null as User | null,
  permissions: null as UserPermissions | null,
  systemInitialized: true,
  bootstrapEnabled: false,
})

let pendingLoad: Promise<User | null> | null = null

async function loadCurrentUser(): Promise<User | null> {
  state.loading = true
  try {
    const response = await getCurrentUser()
    state.user = response.user
    state.permissions = response.permissions
    state.systemInitialized = true
    return response.user
  } catch (error) {
    if (error instanceof APIError && error.status === 401) {
      state.user = null
      state.permissions = null
      return null
    }
    throw error
  } finally {
    state.initialized = true
    state.loading = false
    pendingLoad = null
  }
}

export async function refreshAuthStatus() {
  const response = await getAuthStatus()
  state.systemInitialized = response.initialized
  state.bootstrapEnabled = response.bootstrapEnabled
  return response
}

export async function ensureSessionLoaded(force = false) {
  if (!force && state.initialized) {
    return Promise.resolve(state.user)
  }
  if (!force && pendingLoad) {
    return pendingLoad
  }
  pendingLoad = (async () => {
    try {
      await refreshAuthStatus()
      return await loadCurrentUser()
    } finally {
      pendingLoad = null
    }
  })()
  return pendingLoad
}

export async function loginWithPassword(username: string, password: string) {
  state.authenticating = true
  try {
    const response = await login(username, password)
    state.user = response.user
    state.permissions = response.permissions
    state.systemInitialized = true
    state.initialized = true
    return response.user
  } catch (error) {
    if (error instanceof APIError && error.status === 412) {
      state.systemInitialized = false
    }
    throw error
  } finally {
    state.authenticating = false
  }
}

export async function logoutSession() {
  try {
    await logout()
  } finally {
    state.user = null
    state.permissions = null
    state.initialized = true
  }
}

export async function changeOwnPassword(currentPassword: string, newPassword: string) {
  await changePassword(currentPassword, newPassword)
  state.user = null
  state.permissions = null
  state.initialized = true
}

export const currentUser = computed(() => state.user)
export const currentPermissions = computed(() => state.permissions)
export const isAuthenticated = computed(() => Boolean(state.user))
export const canManageInfrastructure = computed(() => state.permissions?.canManageInfrastructure ?? false)
export const canManageUsers = computed(() => state.permissions?.canManageUsers ?? false)

export function useSession() {
  return {
    state,
    currentUser,
    currentPermissions,
    isAuthenticated,
    canManageInfrastructure,
    canManageUsers,
    ensureSessionLoaded,
    refreshAuthStatus,
    loginWithPassword,
    logoutSession,
    changeOwnPassword,
  }
}
