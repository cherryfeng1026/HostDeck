import type {
  ActivityListResponse,
  AlertEvent,
  AlertRule,
  AuthResponse,
  BatchCommandResponse,
  CommandResult,
  LiveServerItem,
  MetricsResponse,
  NotificationListResponse,
  OverviewResponse,
  ProbeResponse,
  SearchResults,
  ServerAsset,
  ServerPayload,
  ServerStatusDetail,
  TestSSHResponse,
  UserListResponse,
} from '../types'

export class APIError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'APIError'
    this.status = status
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    credentials: 'same-origin',
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
    ...init,
  })

  if (!response.ok) {
    const fallback = `${response.status} ${response.statusText}`.trim()
    try {
      const errorBody = (await response.json()) as { error?: string }
      throw new APIError(response.status, errorBody.error || fallback)
    } catch (error) {
      if (error instanceof APIError) {
        throw error
      }
      throw new APIError(response.status, fallback)
    }
  }

  if (response.status === 204) {
    return undefined as T
  }

  return (await response.json()) as T
}

export function getOverview() {
  return request<OverviewResponse>('/api/overview')
}

export function getServers() {
  return request<ServerAsset[]>('/api/servers')
}

export function getLiveServers(keyword?: string) {
  const query = new URLSearchParams({ includeStatus: '1' })
  if (keyword?.trim()) {
    query.set('keyword', keyword.trim())
  }
  return request<LiveServerItem[]>(`/api/servers?${query.toString()}`)
}

export function getServerListWithStatus(keyword?: string) {
  return getLiveServers(keyword)
}

export function getServerStatus(id: number) {
  return request<ServerStatusDetail>(`/api/servers/${id}/status`)
}

export function getServerMetrics(id: number, range = '24h') {
  return request<MetricsResponse>(`/api/servers/${id}/metrics?range=${encodeURIComponent(range)}`)
}

export function testServerSSH(id: number) {
  return request<TestSSHResponse>(`/api/servers/${id}/test-ssh`, {
    method: 'POST',
  })
}

export function probeServer(id: number) {
  return request<ProbeResponse>(`/api/servers/${id}/probe`, {
    method: 'POST',
  })
}

export function createServer(payload: ServerPayload) {
  return request<void>('/api/servers', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function updateServer(id: number, payload: ServerPayload) {
  return request<void>(`/api/servers/${id}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export function executeCommand(serverId: number, command: string, timeoutSeconds = 15) {
  return request<CommandResult>(`/api/servers/${serverId}/commands/execute`, {
    method: 'POST',
    body: JSON.stringify({
      command,
      timeoutSeconds,
      source: 'custom',
    }),
  })
}

export function executeCommands(serverIds: number[], command: string, timeoutSeconds = 15) {
  return request<BatchCommandResponse>('/api/commands/execute', {
    method: 'POST',
    body: JSON.stringify({
      serverIds,
      command,
      timeoutSeconds,
    }),
  })
}

export function getAlerts() {
  return request<AlertEvent[]>('/api/alerts')
}

export function getAlertRules() {
  return request<AlertRule[]>('/api/alert-rules')
}

export function createAlertRule(payload: Omit<AlertRule, 'id' | 'createdAt' | 'updatedAt'>) {
  return request<void>('/api/alert-rules', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function updateAlertRule(id: number, payload: Omit<AlertRule, 'id' | 'createdAt' | 'updatedAt'>) {
  return request<void>(`/api/alert-rules/${id}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export function login(username: string, password: string) {
  return request<AuthResponse>('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
}

export function getCurrentUser() {
  return request<AuthResponse>('/api/auth/me')
}

export function logout() {
  return request<void>('/api/auth/logout', {
    method: 'POST',
  })
}

export function changePassword(currentPassword: string, newPassword: string) {
  return request<void>('/api/auth/change-password', {
    method: 'POST',
    body: JSON.stringify({ currentPassword, newPassword }),
  })
}

export function getUsers() {
  return request<UserListResponse>('/api/users')
}

export function getNotifications(limit = 20) {
  return request<NotificationListResponse>(`/api/notifications?limit=${limit}`)
}

export function getActivityFeed(limit = 20) {
  return request<ActivityListResponse>(`/api/activity-feed?limit=${limit}`)
}

export function searchWorkspace(query: string, limit = 10) {
  return request<SearchResults>(`/api/search?q=${encodeURIComponent(query)}&limit=${limit}`)
}
