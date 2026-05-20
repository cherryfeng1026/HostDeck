import { reactive } from 'vue'
import {
  getActivityFeed,
  getAlertHistory,
  getAlertNotificationDeliveries,
  getAlertNotificationSettings,
  getAlertRules,
  getAlerts,
  getDashboardOverview,
  getServerListWithStatus,
  getServerMetrics,
  getServerStatus,
} from './services/api'
import type {
  AlertEvent,
  AlertHistoryEvent,
  AlertNotificationDelivery,
  AlertNotificationSettings,
  AlertRule,
  DashboardOverviewResponse,
  DashboardRange,
  MetricPoint,
  ServerListItem,
  ServerStatusDetail,
  ShellEventItem,
} from './types'

interface CacheOptions {
  force?: boolean
  silent?: boolean
  range?: DashboardRange
}

export interface DashboardBundle {
  dashboard: DashboardOverviewResponse
  servers: ServerListItem[]
  activityItems: ShellEventItem[]
}

export interface ServerDetailBundle {
  status: ServerStatusDetail
  metrics: MetricPoint[]
}

export interface AlertBundle {
  alerts: AlertEvent[]
  historyItems: AlertHistoryEvent[]
  rules: AlertRule[]
  notificationSettings: AlertNotificationSettings | null
  notificationDeliveries: AlertNotificationDelivery[]
}

interface ServerDetailEntry {
  initialized: boolean
  loading: boolean
  status: ServerStatusDetail | null
  metrics: MetricPoint[]
}

const dashboardState = reactive({
  initialized: false,
  loading: false,
  range: '24h' as DashboardRange,
  dashboard: createEmptyDashboard(),
  servers: [] as ServerListItem[],
  activityItems: [] as ShellEventItem[],
})

const alertState = reactive({
  initialized: false,
  loading: false,
  alerts: [] as AlertEvent[],
  historyItems: [] as AlertHistoryEvent[],
  rules: [] as AlertRule[],
  notificationSettings: null as AlertNotificationSettings | null,
  notificationDeliveries: [] as AlertNotificationDelivery[],
})

const serverDetailEntries = reactive(new Map<number, ServerDetailEntry>())
let pendingDashboard: Promise<DashboardBundle> | null = null
let pendingDashboardRange: DashboardRange | null = null
let dashboardRequestID = 0
let pendingAlerts: Promise<AlertBundle> | null = null
let pendingAlertsCanLoadNotificationSettings = false
let alertRequestID = 0
const pendingServerDetails = new Map<number, Promise<ServerDetailBundle>>()
const serverDetailRequestIDs = new Map<number, number>()

export function useDashboardCache() {
  return dashboardState
}

export function useAlertCache() {
  return alertState
}

export function getServerDetailCache(serverId: number) {
  return ensureServerDetailEntry(serverId)
}

export async function loadDashboard(options: CacheOptions = {}) {
  const range = options.range ?? dashboardState.range
  if (pendingDashboard && pendingDashboardRange === range && !options.force) {
    return pendingDashboard
  }

  if (!options.silent || !dashboardState.initialized || dashboardState.range !== range) {
    dashboardState.loading = true
  }

  const requestID = ++dashboardRequestID
  const request = Promise.all([
    getDashboardOverview(range),
    getServerListWithStatus(),
    getActivityFeed(6),
  ])
    .then(([dashboard, servers, activity]) => {
      const bundle: DashboardBundle = {
        dashboard,
        servers,
        activityItems: activity.items,
      }
      if (requestID === dashboardRequestID) {
        dashboardState.range = range
        dashboardState.dashboard = bundle.dashboard
        dashboardState.servers = bundle.servers
        dashboardState.activityItems = bundle.activityItems
        dashboardState.initialized = true
      }
      return bundle
    })
    .finally(() => {
      if (pendingDashboard === request) {
        pendingDashboard = null
        pendingDashboardRange = null
        dashboardState.loading = false
      }
    })

  pendingDashboard = request
  pendingDashboardRange = range
  return request
}

export async function loadServerDetail(serverId: number, options: CacheOptions = {}) {
  const entry = ensureServerDetailEntry(serverId)
  const pending = pendingServerDetails.get(serverId)
  if (pending && !options.force) {
    return pending
  }

  if (!options.silent || !entry.initialized) {
    entry.loading = true
  }

  const requestID = (serverDetailRequestIDs.get(serverId) ?? 0) + 1
  serverDetailRequestIDs.set(serverId, requestID)
  const request = Promise.all([
    getServerStatus(serverId),
    getServerMetrics(serverId, '24h'),
  ])
    .then(([status, metrics]) => {
      const bundle: ServerDetailBundle = {
        status,
        metrics: metrics.points,
      }
      if (serverDetailRequestIDs.get(serverId) === requestID) {
        entry.status = bundle.status
        entry.metrics = bundle.metrics
        entry.initialized = true
      }
      return bundle
    })
    .finally(() => {
      if (pendingServerDetails.get(serverId) === request) {
        pendingServerDetails.delete(serverId)
        entry.loading = false
      }
    })

  pendingServerDetails.set(serverId, request)
  return request
}

export async function loadAlerts(canLoadNotificationSettings: boolean, options: CacheOptions = {}) {
  if (!canLoadNotificationSettings) {
    alertState.notificationSettings = null
    alertState.notificationDeliveries = []
  }
  if (pendingAlerts && !options.force && pendingAlertsCanLoadNotificationSettings === canLoadNotificationSettings) {
    return pendingAlerts
  }

  if (!options.silent || !alertState.initialized) {
    alertState.loading = true
  }

  const requestID = ++alertRequestID
  const request = Promise.all([
    getAlerts(),
    getAlertHistory(),
    getAlertRules(),
    canLoadNotificationSettings ? getAlertNotificationSettings() : Promise.resolve(null),
    canLoadNotificationSettings ? getAlertNotificationDeliveries(20) : Promise.resolve([]),
  ])
    .then(([alerts, historyItems, rules, notificationSettings, notificationDeliveries]) => {
      const bundle: AlertBundle = {
        alerts,
        historyItems,
        rules,
        notificationSettings,
        notificationDeliveries,
      }
      if (requestID === alertRequestID) {
        alertState.alerts = bundle.alerts
        alertState.historyItems = bundle.historyItems
        alertState.rules = bundle.rules
        alertState.notificationSettings = canLoadNotificationSettings ? bundle.notificationSettings : null
        alertState.notificationDeliveries = canLoadNotificationSettings ? bundle.notificationDeliveries : []
        alertState.initialized = true
      }
      return bundle
    })
    .finally(() => {
      if (pendingAlerts === request) {
        pendingAlerts = null
        alertState.loading = false
      }
    })

  pendingAlerts = request
  pendingAlertsCanLoadNotificationSettings = canLoadNotificationSettings
  return request
}

export function updateCachedAlert(nextAlert: AlertEvent) {
  const index = alertState.alerts.findIndex((item) => item.id === nextAlert.id)
  if (index === -1) {
    alertState.alerts.unshift(nextAlert)
    return
  }
  alertState.alerts.splice(index, 1, nextAlert)
}

export function removeCachedAlert(alertId: number) {
  alertState.alerts = alertState.alerts.filter((item) => item.id !== alertId)
}

export function prependCachedAlertHistory(item: AlertHistoryEvent) {
  alertState.historyItems = [item, ...alertState.historyItems].slice(0, 50)
}

export function setCachedAlertNotificationSettings(settings: AlertNotificationSettings) {
  alertState.notificationSettings = {
    ...settings,
    webhookConfigured: !!settings.webhookConfigured,
    clearWebhookURL: false,
  }
}

function ensureServerDetailEntry(serverId: number) {
  let entry = serverDetailEntries.get(serverId)
  if (!entry) {
    entry = reactive({
      initialized: false,
      loading: false,
      status: null,
      metrics: [] as MetricPoint[],
    })
    serverDetailEntries.set(serverId, entry)
  }
  return entry
}

function createEmptyDashboard(): DashboardOverviewResponse {
  return {
    headline: {
      totalServers: 0,
      onlineServers: 0,
      offlineServers: 0,
      activeAlerts: 0,
      sshFailures: 0,
      collectFailedServers: 0,
      collectStaleServers: 0,
      lastUpdatedAt: '',
    },
    trends: [],
    topServers: [],
    resourceSummary: {
      reportingServers: 0,
      unhealthyServers: 0,
      collectFailedServers: 0,
      collectStaleServers: 0,
      avgCpuUsage: 0,
      avgMemoryUsage: 0,
      avgDiskUsage: 0,
      peakCpuUsage: 0,
      peakMemoryUsage: 0,
      peakDiskUsage: 0,
    },
    alertSummary: {
      total: 0,
      critical: 0,
      warning: 0,
      acknowledged: 0,
      muted: 0,
    },
  }
}
