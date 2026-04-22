import type {
  ActivityListResponse,
  AlertEvent,
  AlertHistoryEvent,
  AlertNotificationSettings,
  AlertRule,
  AuthResponse,
  AuthStatusResponse,
  BatchCommandResponse,
  CommandHistoryQuery,
  CommandHistoryRecord,
  CommandResult,
  CommandTemplate,
  CommandTemplateFavoriteResponse,
  CommandTemplateListResponse,
  CreateCommandTemplatePayload,
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
  User,
  UserListResponse,
  CreateUserPayload,
  UpdateUserPayload,
  APITokenListResponse,
  CreateAPITokenPayload,
  CreateAPITokenResponse,
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

export function trustServerHostKey(id: number, fingerprint: string) {
  return request<{ trustedHostKeyFingerprint: string }>(`/api/servers/${id}/trust-host-key`, {
    method: 'POST',
    body: JSON.stringify({ fingerprint }),
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

export function getCommandTemplates() {
  return request<CommandTemplateListResponse>('/api/commands/templates')
}

export function createCommandTemplate(payload: CreateCommandTemplatePayload) {
  return request<CommandTemplate>('/api/commands/templates', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function setCommandTemplateFavorite(id: string, favorite: boolean) {
  return request<CommandTemplateFavoriteResponse>(`/api/commands/templates/${encodeURIComponent(id)}/favorite`, {
    method: 'POST',
    body: JSON.stringify({ favorite }),
  })
}

export function getCommandHistory(query: CommandHistoryQuery = {}) {
  const params = new URLSearchParams()
  if (query.limit) params.set('limit', String(query.limit))
  if (query.serverId) params.set('serverId', String(query.serverId))
  if (query.executorUsername?.trim()) params.set('executorUsername', query.executorUsername.trim())
  if (query.keyword?.trim()) params.set('keyword', query.keyword.trim())
  if (query.startTime?.trim()) params.set('startTime', query.startTime.trim())
  if (query.endTime?.trim()) params.set('endTime', query.endTime.trim())
  const suffix = params.toString() ? `?${params.toString()}` : ''
  return request<CommandHistoryRecord[]>(`/api/commands/history${suffix}`)
}

export function getAlerts() {
  return request<AlertEvent[]>('/api/alerts')
}

export function getAlertHistory(limit = 50) {
  return request<AlertHistoryEvent[]>(`/api/alert-history?limit=${limit}`)
}

export function getAlertNotificationSettings() {
  return request<AlertNotificationSettings>('/api/alert-notification-settings')
}

export function updateAlertNotificationSettings(payload: AlertNotificationSettings) {
  return request<AlertNotificationSettings>('/api/alert-notification-settings', {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export function ackAlert(id: number) {
  return request<AlertEvent>(`/api/alerts/${id}/ack`, {
    method: 'POST',
  })
}

export function muteAlert(id: number, durationMinutes = 30) {
  return request<AlertEvent>(`/api/alerts/${id}/mute`, {
    method: 'POST',
    body: JSON.stringify({ durationMinutes }),
  })
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

export function getAuthStatus() {
  return request<AuthStatusResponse>('/api/auth/status')
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

export function createUser(payload: CreateUserPayload) {
  return request<User>('/api/users', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function updateUser(id: number, payload: UpdateUserPayload) {
  return request<User>(`/api/users/${id}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export function resetUserPassword(id: number, newPassword: string) {
  return request<void>(`/api/users/${id}/reset-password`, {
    method: 'POST',
    body: JSON.stringify({ newPassword }),
  })
}

export function revokeUserSessions(id: number) {
  return request<void>(`/api/users/${id}/revoke-sessions`, {
    method: 'POST',
  })
}

export function getAPITokens() {
  return request<APITokenListResponse>('/api/auth/api-tokens')
}

export function createAPIToken(payload: CreateAPITokenPayload) {
  return request<CreateAPITokenResponse>('/api/auth/api-tokens', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function revokeAPIToken(id: number) {
  return request<void>(`/api/auth/api-tokens/${id}`, {
    method: 'DELETE',
  })
}

export function getNotifications(limit = 20) {
  return request<NotificationListResponse>(`/api/notifications?limit=${limit}`)
}

export function markNotificationsRead(readBefore: string) {
  return request<void>('/api/notifications/read', {
    method: 'POST',
    body: JSON.stringify({ readBefore }),
  })
}

export function getActivityFeed(limit = 20) {
  return request<ActivityListResponse>(`/api/activity-feed?limit=${limit}`)
}

export function searchWorkspace(query: string, limit = 10) {
  return request<SearchResults>(`/api/search?q=${encodeURIComponent(query)}&limit=${limit}`)
}
