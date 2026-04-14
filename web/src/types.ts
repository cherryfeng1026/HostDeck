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
  collectorMode: string
  tags: string[]
  purpose: string
  remark: string
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
  latencyMs: number
  error?: string
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
  ruleId: number
  serverId: number
  serverName: string
  metric: string
  operator: string
  threshold: number
  currentValue: number
  severity: string
  message: string
  triggeredAt: string
  durationSeconds: number
}

export interface User {
  id: number
  username: string
  role: string
  lastLoginAt?: string
  createdAt: string
  updatedAt: string
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

export interface UserListResponse {
  items: User[]
}

export interface NotificationItem {
  kind: string
  severity: string
  title: string
  message: string
  createdAt: string
}

export interface ActivityItem {
  kind: string
  title: string
  summary: string
  createdAt: string
  serverId?: number
  serverName?: string
  username?: string
}

export interface NotificationListResponse {
  items: NotificationItem[]
}

export interface ActivityListResponse {
  items: ActivityItem[]
}

export interface SearchResults {
  alerts: NotificationItem[]
  commands: ActivityItem[]
  authEvents: ActivityItem[]
}

export interface ServerPayload {
  name: string
  hostname: string
  ip: string
  sshPort: number
  username: string
  authType?: string
  password?: string
  collectorMode: string
  tags: string[]
  purpose?: string
  remark?: string
  enabled?: boolean
}
