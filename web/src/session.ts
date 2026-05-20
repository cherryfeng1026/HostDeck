import { computed, reactive } from 'vue'
import { APIError, changePassword, getAuthStatus, login, logout } from './services/api'
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

function clearSessionState() {
  state.user = null
  state.permissions = null
}

function applyStatusSnapshot(user?: User, permissions?: UserPermissions) {
  if (!user || !permissions) {
    clearSessionState()
    return null
  }
  state.user = user
  state.permissions = permissions
  state.systemInitialized = true
  return user
}

export async function refreshAuthStatus() {
  const response = await getAuthStatus()
  state.systemInitialized = response.initialized
  state.bootstrapEnabled = response.bootstrapEnabled
  if (response.authenticated) {
    applyStatusSnapshot(response.user, response.permissions)
  } else {
    clearSessionState()
  }
  state.initialized = true
  return response
}

export async function ensureSessionLoaded(force = false) {
  if (state.initialized && !force) {
    return state.user
  }
  if (pendingLoad && !force) {
    return pendingLoad
  }

  pendingLoad = refreshAuthStatus()
    .then((response) => (response.authenticated ? state.user : null))
    .finally(() => {
      pendingLoad = null
    })

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
  pendingLoad = null
  clearSessionState()
  state.initialized = true
  void logout().catch(() => undefined)
}

export async function changeOwnPassword(currentPassword: string, newPassword: string) {
  await changePassword(currentPassword, newPassword)
  clearSessionState()
  state.initialized = true
}

export const currentUser = computed(() => state.user)
export const currentPermissions = computed(() => state.permissions)
export const isAuthenticated = computed(() => !!state.user)
export const canManageInfrastructure = computed(() => !!state.permissions?.canManageInfrastructure)
export const canManageUsers = computed(() => !!state.permissions?.canManageUsers)

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
