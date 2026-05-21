import type {
  ActivityListResponse,
  AlertEvent,
  AlertHistoryEvent,
  AlertNotificationDelivery,
  AlertNotificationRetryResponse,
  AlertNotificationSettings,
  AlertRule,
  AuthResponse,
  AuthStatusResponse,
  BatchCommandResponse,
  CommandHistoryQuery,
  CommandHistoryRecord,
  CommandResult,
  CommandTemplate,
  CommandTemplateVariable,
  CommandTemplateFavoriteResponse,
  CommandTemplateListResponse,
  CreateCommandTemplatePayload,
  LiveServerItem,
  MetricsResponse,
  NotificationListResponse,
  OverviewResponse,
  DashboardOverviewResponse,
  DashboardRange,
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

  const text = await response.text()
  if (!text.trim()) {
    return undefined as T
  }

  return JSON.parse(text) as T
}

type UnknownRecord = Record<string, unknown>

function asRecord(value: unknown): UnknownRecord | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return null
  }
  return value as UnknownRecord
}

function toStringValue(value: unknown, fallback = '') {
  if (typeof value === 'string') {
    return value
  }
  if (typeof value === 'number' || typeof value === 'boolean') {
    return String(value)
  }
  return fallback
}

function toOptionalStringValue(value: unknown) {
  const normalized = toStringValue(value).trim()
  return normalized ? normalized : undefined
}

function toNumberValue(value: unknown, fallback = 0) {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value
  }
  if (typeof value === 'string' && value.trim()) {
    const normalized = Number(value)
    if (Number.isFinite(normalized)) {
      return normalized
    }
  }
  return fallback
}

function toBooleanValue(value: unknown, fallback = false) {
  if (typeof value === 'boolean') {
    return value
  }
  if (typeof value === 'string') {
    if (value === 'true') return true
    if (value === 'false') return false
  }
  return fallback
}

interface CommandExecutionOptions {
  source?: string
  templateId?: string
  riskLevel?: string
  riskConfirmed?: boolean
}

export function normalizeCommandResult(value: unknown, fallback: Partial<CommandResult> = {}): CommandResult {
  const item = asRecord(value)
  const templateId = toOptionalStringValue(item?.templateId) ?? fallback.templateId
  return {
    command: toStringValue(item?.command, fallback.command ?? ''),
    stdout: toStringValue(item?.stdout, fallback.stdout ?? ''),
    stderr: toStringValue(item?.stderr, fallback.stderr ?? ''),
    exitCode: toNumberValue(item?.exitCode, fallback.exitCode ?? 0),
    durationMs: toNumberValue(item?.durationMs, fallback.durationMs ?? 0),
    executedAt: toStringValue(item?.executedAt, fallback.executedAt ?? ''),
    source: toStringValue(item?.source, fallback.source ?? 'custom'),
    ...(templateId ? { templateId } : {}),
    riskLevel: toStringValue(item?.riskLevel, fallback.riskLevel ?? 'normal'),
    riskConfirmed: toBooleanValue(item?.riskConfirmed, fallback.riskConfirmed ?? false),
  }
}

function normalizeCommandTemplateVariable(value: unknown): CommandTemplateVariable {
  const item = asRecord(value)
  return {
    name: toStringValue(item?.name),
    label: toStringValue(item?.label),
    placeholder: toOptionalStringValue(item?.placeholder),
    defaultValue: toOptionalStringValue(item?.defaultValue),
    required: toBooleanValue(item?.required, true),
  }
}

export function normalizeCommandTemplate(value: unknown): CommandTemplate {
  const item = asRecord(value)
  const rawVariables = Array.isArray(item?.variables) ? item.variables : []
  return {
    id: toStringValue(item?.id),
    name: toStringValue(item?.name),
    description: toStringValue(item?.description),
    command: toStringValue(item?.command),
    scope: toStringValue(item?.scope, 'personal'),
    riskLevel: toStringValue(item?.riskLevel, 'normal'),
    createdBy: toOptionalStringValue(item?.createdBy),
    isFavorite: toBooleanValue(item?.isFavorite, false),
    variables: rawVariables.map((variable) => normalizeCommandTemplateVariable(variable)).filter((variable) => variable.name),
  }
}

export function normalizeCommandHistoryRecord(value: unknown): CommandHistoryRecord {
  const item = asRecord(value)
  const templateId = toOptionalStringValue(item?.templateId)
  const requestId = toOptionalStringValue(item?.requestId)
  return {
    id: toNumberValue(item?.id),
    serverId: toNumberValue(item?.serverId),
    serverName: toStringValue(item?.serverName),
    serverIp: toStringValue(item?.serverIp),
    executorUsername: toStringValue(item?.executorUsername),
    executorAuthMethod: toStringValue(item?.executorAuthMethod),
    command: toStringValue(item?.command),
    stdout: toStringValue(item?.stdout),
    stderr: toStringValue(item?.stderr),
    exitCode: toNumberValue(item?.exitCode),
    durationMs: toNumberValue(item?.durationMs),
    executedAt: toStringValue(item?.executedAt),
    source: toStringValue(item?.source, 'custom'),
    ...(templateId ? { templateId } : {}),
    riskLevel: toStringValue(item?.riskLevel, 'normal'),
    riskConfirmed: toBooleanValue(item?.riskConfirmed, false),
    ...(requestId ? { requestId } : {}),
  }
}

export function normalizeBatchCommandResult(
  value: unknown,
  fallback: Partial<Pick<CommandResult, 'command'>> & { serverId?: number; serverName?: string } = {},
): BatchCommandResponse['results'][number] {
  const item = asRecord(value)
  const error = toOptionalStringValue(item?.error)
  return {
    serverId: toNumberValue(item?.serverId, fallback.serverId ?? 0),
    serverName: toStringValue(item?.serverName, fallback.serverName ?? ''),
    success: toBooleanValue(item?.success, !error),
    result: normalizeCommandResult(item?.result, { command: fallback.command ?? '' }),
    ...(error ? { error } : {}),
  }
}

function normalizeBatchCommandResponse(value: unknown, command: string): BatchCommandResponse {
  const item = asRecord(value)
  const rawResults = Array.isArray(item?.results) ? item.results : []
  return {
    results: rawResults.map((result) => normalizeBatchCommandResult(result, { command })),
  }
}

function normalizeCommandTemplateListResponse(value: unknown): CommandTemplateListResponse {
  const item = asRecord(value)
  const rawItems = Array.isArray(item?.items) ? item.items : []
  return {
    items: rawItems.map((template) => normalizeCommandTemplate(template)).filter((template) => template.id || template.name || template.command),
  }
}

export function getOverview() {
  return request<OverviewResponse>('/api/overview')
}

export function getDashboardOverview(range: DashboardRange = '24h') {
  return request<DashboardOverviewResponse>(`/api/overview/dashboard?range=${encodeURIComponent(range)}`)
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

export function deleteServer(id: number) {
  return request<void>(`/api/servers/${id}`, {
    method: 'DELETE',
  })
}

export async function executeCommand(serverId: number, command: string, timeoutSeconds = 15, options: CommandExecutionOptions = {}) {
  const response = await request<unknown>(`/api/servers/${serverId}/commands/execute`, {
    method: 'POST',
    body: JSON.stringify({
      command,
      timeoutSeconds,
      source: options.source ?? 'custom',
      templateId: options.templateId,
      riskLevel: options.riskLevel,
      riskConfirmed: options.riskConfirmed ?? false,
    }),
  })
  return normalizeCommandResult(response, {
    command,
    source: options.source ?? 'custom',
    templateId: options.templateId,
    riskLevel: options.riskLevel ?? 'normal',
    riskConfirmed: options.riskConfirmed ?? false,
  })
}

export async function executeCommands(serverIds: number[], command: string, timeoutSeconds = 15, options: CommandExecutionOptions = {}) {
  const response = await request<unknown>('/api/commands/execute', {
    method: 'POST',
    body: JSON.stringify({
      serverIds,
      command,
      timeoutSeconds,
      source: options.source ?? 'custom',
      templateId: options.templateId,
      riskLevel: options.riskLevel,
      riskConfirmed: options.riskConfirmed ?? false,
    }),
  })
  return normalizeBatchCommandResponse(response, command)
}

export async function getCommandTemplates() {
  const response = await request<unknown>('/api/commands/templates')
  return normalizeCommandTemplateListResponse(response)
}

export async function createCommandTemplate(payload: CreateCommandTemplatePayload) {
  const response = await request<unknown>('/api/commands/templates', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
  return normalizeCommandTemplate(response)
}

export function setCommandTemplateFavorite(id: string, favorite: boolean) {
  return request<CommandTemplateFavoriteResponse>(`/api/commands/templates/${encodeURIComponent(id)}/favorite`, {
    method: 'POST',
    body: JSON.stringify({ favorite }),
  })
}

export async function getCommandHistory(query: CommandHistoryQuery = {}) {
  const params = new URLSearchParams()
  if (query.limit) params.set('limit', String(query.limit))
  if (query.serverId) params.set('serverId', String(query.serverId))
  if (query.executorUsername?.trim()) params.set('executorUsername', query.executorUsername.trim())
  if (query.keyword?.trim()) params.set('keyword', query.keyword.trim())
  if (query.startTime?.trim()) params.set('startTime', query.startTime.trim())
  if (query.endTime?.trim()) params.set('endTime', query.endTime.trim())
  const suffix = params.toString() ? `?${params.toString()}` : ''
  const response = await request<unknown>(`/api/commands/history${suffix}`)
  const rawItems = Array.isArray(response) ? response : []
  return rawItems.map((item) => normalizeCommandHistoryRecord(item))
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

export function testAlertNotificationSettings() {
  return request<void>('/api/alert-notification-settings/test', { method: 'POST' })
}

export function getAlertNotificationDeliveries(limit = 20) {
  return request<AlertNotificationDelivery[]>(`/api/alert-notification-deliveries?limit=${limit}`)
}

export function retryAlertNotificationDeliveries(limit = 20) {
  return request<AlertNotificationRetryResponse>(`/api/alert-notification-deliveries/retry?limit=${limit}`, { method: 'POST' })
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

export function deleteAlertRule(id: number) {
  return request<void>(`/api/alert-rules/${id}`, {
    method: 'DELETE',
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
    keepalive: true,
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
