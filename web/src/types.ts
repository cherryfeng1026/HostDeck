export interface OverviewResponse {
  totalServers: number
  onlineServers: number
  offlineServers: number
  activeAlerts: number
  sshFailures: number
  collectFailedServers: number
  collectStaleServers: number
}

export interface DashboardHeadline {
  totalServers: number
  onlineServers: number
  offlineServers: number
  activeAlerts: number
  sshFailures: number
  collectFailedServers: number
  collectStaleServers: number
  lastUpdatedAt: string
}

export interface DashboardTrendPoint {
  sampledAt: string
  avgCpuUsage: number
  avgMemoryUsage: number
  avgDiskUsage: number
  avgLoad1: number
  avgLoad5: number
  avgLoad15: number
  sampleCount: number
  fallback?: boolean
}

export type DashboardRange = '1h' | '6h' | '24h' | '7d'

export interface DashboardTopServer {
  id: number
  name: string
  hostname: string
  ip: string
  purpose: string
  online: boolean
  sshOk: boolean
  cpuUsage: number
  memoryUsage: number
  diskUsage: number
  load1: number
  lastReportAt: string
  rankReason: string
}

export interface DashboardResourceSummary {
  reportingServers: number
  unhealthyServers: number
  collectFailedServers: number
  collectStaleServers: number
  avgCpuUsage: number
  avgMemoryUsage: number
  avgDiskUsage: number
  peakCpuUsage: number
  peakMemoryUsage: number
  peakDiskUsage: number
}

export interface DashboardAlertSummary {
  total: number
  critical: number
  warning: number
  acknowledged: number
  muted: number
}

export interface DashboardOverviewResponse {
  headline: DashboardHeadline
  trends: DashboardTrendPoint[]
  topServers: DashboardTopServer[]
  resourceSummary: DashboardResourceSummary
  alertSummary: DashboardAlertSummary
}

export type ThemeMode = 'dark' | 'light'

export interface ServerAsset {
  id: number
  name: string
  hostname: string
  ip: string
  sshPort: number
  username: string
  authType: string
  passwordConfigured: boolean
  privateKeyConfigured: boolean
  trustedHostKeyFingerprint: string
  collectorMode: string
  tags: string[]
  purpose: string
  remark: string
  expiresAt?: string
  maintenanceStartAt?: string
  maintenanceEndAt?: string
  enabled: boolean
  createdAt: string
  updatedAt: string
}

export interface LiveServerItem extends ServerAsset {
  online: boolean
  sshOk: boolean
  cpuUsage: number
  memoryUsage: number
  diskUsage: number
  osVersion: string
  kernelVersion: string
  uptimeSeconds: number
  load1: number
  load5: number
  load15: number
  lastReportAt: string
  source: string
  collectStatus: string
  lastCollectStartedAt: string
  lastCollectFinishedAt: string
  lastSuccessAt: string
  lastCollectError: string
  collectFailureCount: number
  collectDurationMs: number
  stale: boolean
}

export type ServerListItem = LiveServerItem

export interface ServerStatusDetail {
  id: number
  name: string
  hostname: string
  ip: string
  sshPort: number
  username: string
  collectorMode: string
  trustedHostKeyFingerprint: string
  enabled: boolean
  online: boolean
  sshOk: boolean
  cpuUsage: number
  memoryUsage: number
  diskUsage: number
  osVersion: string
  kernelVersion: string
  uptimeSeconds: number
  load1: number
  load5: number
  load15: number
  lastReportAt: string
  source: string
  collectStatus: string
  lastCollectStartedAt: string
  lastCollectFinishedAt: string
  lastSuccessAt: string
  lastCollectError: string
  collectFailureCount: number
  collectDurationMs: number
  stale: boolean
}

export interface MetricPoint {
  sampledAt: string
  cpuUsage: number
  memoryUsage: number
  diskUsage: number
  load1: number
  load5: number
  load15: number
}

export interface MetricsResponse {
  points: MetricPoint[]
}

export interface TestSSHResponse {
  sshOk: boolean
  latencyMs?: number
  error?: string
  hostKeyFingerprint?: string
  trustedHostKeyFingerprint?: string
  fingerprintMismatch?: boolean
  trustRequired?: boolean
}

export interface ProbeResponse {
  snapshot: {
    online: boolean
    sshOk: boolean
    cpuUsage: number
    memoryUsage: number
    diskUsage: number
    osVersion: string
    kernelVersion: string
    uptimeSeconds: number
    load1: number
    load5: number
    load15: number
    collectDurationMs: number
    source: string
  }
}

export interface CommandResult {
  command: string
  stdout: string
  stderr: string
  exitCode: number
  durationMs: number
  executedAt: string
  source: string
  templateId?: string
  riskLevel: string
  riskConfirmed: boolean
}

export interface BatchCommandResult {
  serverId: number
  serverName: string
  success: boolean
  result: CommandResult
  error?: string
}

export interface BatchCommandResponse {
  results: BatchCommandResult[]
}

export interface CommandHistoryRecord {
  id: number
  serverId: number
  serverName: string
  serverIp: string
  executorUsername: string
  executorAuthMethod: string
  command: string
  stdout: string
  stderr: string
  exitCode: number
  durationMs: number
  executedAt: string
  source: string
  templateId?: string
  riskLevel: string
  riskConfirmed: boolean
  requestId?: string
}

export interface CommandHistoryQuery {
  limit?: number
  serverId?: number
  executorUsername?: string
  keyword?: string
  startTime?: string
  endTime?: string
}

export interface CommandTemplateVariable {
  name: string
  label: string
  placeholder?: string
  defaultValue?: string
  required: boolean
}

export interface CommandTemplate {
  id: string
  name: string
  description: string
  command: string
  scope: string
  riskLevel: string
  createdBy?: string
  isFavorite: boolean
  variables: CommandTemplateVariable[]
}

export interface CreateCommandTemplatePayload {
  name: string
  description: string
  command: string
  scope: string
  riskLevel: string
}

export interface SetCommandTemplateFavoritePayload {
  favorite: boolean
}

export interface CommandTemplateFavoriteResponse {
  favorite: boolean
}

export interface CommandTemplateListResponse {
  items: CommandTemplate[]
}

export interface AlertRule {
  id: number
  metric: string
  operator: string
  threshold: number
  durationSeconds: number
  enabled: boolean
  scopeType: string
  scopeValue: string
  createdAt: string
  updatedAt: string
}

export interface AlertEvent {
  id: number
  ruleId: number
  serverId: number
  serverName: string
  metric: string
  operator: string
  threshold: number
  currentValue: number
  severity: string
  message: string
  status: string
  triggeredAt: string
  lastTriggeredAt: string
  acknowledgedAt?: string
  acknowledgedBy?: string
  mutedUntil?: string
  durationSeconds: number
}

export interface AlertHistoryEvent {
  id: number
  alertId: number
  ruleId: number
  serverId: number
  serverName: string
  eventType: string
  metric: string
  operator: string
  threshold: number
  currentValue: number
  severity: string
  message: string
  status: string
  triggeredAt: string
  createdAt: string
  actorUsername?: string
  detail?: string
}

export interface AlertNotificationSettings {
  enabled: boolean
  webhookURL: string
  webhookConfigured?: boolean
  clearWebhookURL?: boolean
  webhookTimeoutSeconds: number
  createdAt?: string
  updatedAt?: string
}

export interface AlertNotificationDelivery {
  id: number
  eventType: string
  alertId: number
  ruleId: number
  serverId: number
  serverName: string
  status: string
  attemptCount: number
  nextAttemptAt?: string
  lastAttemptAt?: string
  lastError?: string
  occurredAt: string
  createdAt: string
  updatedAt: string
}

export interface AlertNotificationRetryResponse {
  retried: number
}

export interface User {
  id: number
  username: string
  role: string
  enabled: boolean
  lastLoginAt?: string
  createdAt: string
  updatedAt: string
}

export interface CreateUserPayload {
  username: string
  password: string
  role: string
}

export interface UpdateUserPayload {
  role: string
  enabled: boolean
}

export interface UserPermissions {
  canManageInfrastructure: boolean
  canManageUsers: boolean
  canChangeOwnPassword: boolean
}

export interface AuthResponse {
  user: User
  permissions: UserPermissions
}

export interface AuthStatusResponse {
  initialized: boolean
  bootstrapEnabled: boolean
  authenticated: boolean
  user?: User
  permissions?: UserPermissions
}

export interface UserListResponse {
  items: User[]
}

export interface APITokenItem {
  id: number
  userId: number
  name: string
  prefix: string
  scopes: string[]
  lastUsedAt?: string
  expiresAt?: string
  createdAt: string
  updatedAt: string
  revokedAt?: string
  isActive: boolean
}

export interface APITokenListResponse {
  items: APITokenItem[]
}

export interface CreateAPITokenPayload {
  name: string
  expiresInHours: number
  scopes?: string[]
}

export interface CreateAPITokenResponse {
  token: string
  item: APITokenItem
}

export interface ShellEventItem {
  kind: string
  severity: string
  title: string
  summary: string
  createdAt: string
  serverId?: number
  serverName?: string
  username?: string
  routePath?: string
  isRead?: boolean
}

export interface NotificationListResponse {
  items: ShellEventItem[]
  unreadCount: number
}

export interface ActivityListResponse {
  items: ShellEventItem[]
}

export interface SearchResults {
  items: ShellEventItem[]
}

export interface ServerPayload {
  name: string
  hostname: string
  ip: string
  sshPort: number
  username: string
  authType?: string
  password?: string
  privateKey?: string
  trustedHostKeyFingerprint?: string
  collectorMode: string
  tags: string[]
  purpose?: string
  remark?: string
  expiresAt?: string
  maintenanceStartAt?: string
  maintenanceEndAt?: string
  enabled?: boolean
}
