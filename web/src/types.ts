export interface OverviewResponse {
  totalServers: number
  onlineServers: number
  offlineServers: number
  activeAlerts: number
  sshFailures: number
}

export interface ServerAsset {
  id: number
  name: string
  hostname: string
  ip: string
  sshPort: number
  username: string
  authType: string
  passwordConfigured: boolean
  trustedHostKeyFingerprint: string
  collectorMode: string
  tags: string[]
  purpose: string
  remark: string
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
  executorUsername: string
  command: string
  stdout: string
  stderr: string
  exitCode: number
  durationMs: number
  executedAt: string
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
  variables: CommandTemplateVariable[]
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
}

export interface UserListResponse {
  items: User[]
}

export interface APITokenItem {
  id: number
  userId: number
  name: string
  prefix: string
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
  trustedHostKeyFingerprint?: string
  collectorMode: string
  tags: string[]
  purpose?: string
  remark?: string
  maintenanceStartAt?: string
  maintenanceEndAt?: string
  enabled?: boolean
}
