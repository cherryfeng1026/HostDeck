import { expect, test, type Page } from '@playwright/test'
import { adminAuthFile, viewerAuthFile, viewerPassword, viewerUsername } from './env'
import { captureRuntimeErrors, loginFromPage } from './helpers'

const pageReadyTimeout = 20000

async function expectCommandPageStable(page: Page) {
  await expect(page.getByRole('heading', { name: '远程终端' })).toBeVisible({ timeout: pageReadyTimeout })
  await expect(page.getByText('命令模板')).toBeVisible({ timeout: pageReadyTimeout })
  await expect(page.getByText('执行结果')).toBeVisible({ timeout: pageReadyTimeout })
  await expect(page.getByRole('heading', { name: '命令历史' })).toBeVisible({ timeout: pageReadyTimeout })
}

async function resolvePresetServerId(page: Page) {
  return page.evaluate(async () => {
    const response = await fetch('/api/servers', { credentials: 'same-origin' })
    if (!response.ok) {
      return null
    }

    const body = (await response.json()) as Array<{ id?: number; enabled?: boolean }>
    const preferred = body.find((item) => typeof item.id === 'number' && item.enabled)
    if (preferred?.id) {
      return preferred.id
    }

    const fallback = body.find((item) => typeof item.id === 'number')
    return fallback?.id ?? null
  })
}

interface NotificationState {
  items: Array<{ isRead?: boolean }>
  unreadCount: number
}

async function getNotificationState(page: Page) {
  const response = await page.request.get('/api/notifications?limit=6')
  if (!response.ok()) {
    expect(response.ok(), `/api/notifications => ${response.status()} ${await response.text()}`).toBeTruthy()
  }
  return response.json() as Promise<NotificationState>
}

async function markNotificationsReadForTest(page: Page, readBefore = new Date().toISOString()) {
  const response = await page.request.post('/api/notifications/read', {
    data: { readBefore },
  })
  expect(response.ok(), `/api/notifications/read => ${response.status()} ${await response.text()}`).toBeTruthy()
}

async function createUnreadNotification(page: Page) {
  await markNotificationsReadForTest(page)
  await page.waitForTimeout(1100)

  const response = await page.request.post('/api/auth/login', {
    data: {
      username: `missing-user-${Date.now()}`,
      password: 'WrongPassword123!',
    },
  })
  expect(response.status()).toBe(401)

  await expect.poll(async () => (await getNotificationState(page)).unreadCount, {
    timeout: pageReadyTimeout,
  }).toBeGreaterThan(0)
}

test('redirects unauthenticated requests to login', async ({ page }) => {
  const runtime = captureRuntimeErrors(page)

  await page.goto('/commands')
  await page.waitForURL(/\/login/)
  await expect(page.locator('.login-form')).toBeVisible()

  await runtime.expectClean()
})

test('loads login page without protected api failures', async ({ page }) => {
  const runtime = captureRuntimeErrors(page)

  await page.goto('/login')
  await expect(page.locator('.login-form')).toBeVisible()
  await page.waitForLoadState('networkidle')

  await runtime.expectClean()
})

test.describe('authenticated admin smoke', () => {
  test.use({ storageState: adminAuthFile })

  test('renders dashboard overview and keeps shell stable after reload', async ({ page }) => {
    const runtime = captureRuntimeErrors(page)

    await page.goto('/')
    await expect(page.getByRole('heading', { name: /运维概览/i })).toBeVisible({ timeout: pageReadyTimeout })
    await expect(page.getByText('服务实例', { exact: true })).toBeVisible({ timeout: pageReadyTimeout })
    await expect(page.getByText('资源探索', { exact: true })).toBeVisible({ timeout: pageReadyTimeout })
    await expect(page.getByText('快捷操作', { exact: true })).toBeVisible({ timeout: pageReadyTimeout })
    await expect(page.getByText('最近活动', { exact: true })).toBeVisible({ timeout: pageReadyTimeout })

    await page.reload()
    await expect(page.getByRole('heading', { name: /运维概览/i })).toBeVisible({ timeout: pageReadyTimeout })
    await expect(page.getByText('资源探索')).toBeVisible({ timeout: pageReadyTimeout })

    await runtime.expectClean()
  })

  test('keeps commands page stable with and without preset server', async ({ page }) => {
    const runtime = captureRuntimeErrors(page)

    await page.goto('/commands')
    await expectCommandPageStable(page)

    const presetServerId = (await resolvePresetServerId(page)) ?? 999999
    await page.goto(`/commands?serverId=${presetServerId}`)
    await expectCommandPageStable(page)

    await runtime.expectClean()
  })

  test('clears unread notifications from the shell popover', async ({ page }) => {
    const runtime = captureRuntimeErrors(page)

    await createUnreadNotification(page)
    await page.goto('/')
    await expect(page.locator('.layout-shell')).toBeVisible({ timeout: pageReadyTimeout })

    const notificationTrigger = page.locator('.toolbar-actions .action-btn').first()
    await expect(notificationTrigger.locator('.n-badge-sup')).toBeVisible({ timeout: pageReadyTimeout })

    await notificationTrigger.click()
    const preview = page.locator('.activity-preview')
    await expect(preview).toBeVisible({ timeout: pageReadyTimeout })

    const markAllButton = preview.locator('.activity-head .n-button')
    await expect(markAllButton).toBeVisible({ timeout: pageReadyTimeout })

    const readResponse = page.waitForResponse((response) => (
      response.url().includes('/api/notifications/read')
      && response.request().method() === 'POST'
    ))
    await markAllButton.click()
    expect((await readResponse).status()).toBe(204)

    await expect.poll(async () => (await getNotificationState(page)).unreadCount, {
      timeout: pageReadyTimeout,
    }).toBe(0)
    await expect(markAllButton).toHaveCount(0)
    await expect(notificationTrigger.locator('.n-badge-sup')).toHaveCount(0)

    await runtime.expectClean()
  })
})

test.describe('viewer route guard', () => {
  test.use({ storageState: viewerAuthFile })

  test('blocks viewer from commands and keeps dashboard accessible', async ({ page }) => {
    const runtime = captureRuntimeErrors(page)

    await page.goto('/')
    if (/\/login/.test(page.url())) {
      await loginFromPage(page, viewerUsername, viewerPassword)
    }

    await page.goto('/commands')
    await page.waitForURL(/\/$/, { timeout: pageReadyTimeout })
    await expect(page.getByRole('heading', { name: /运维概览/i })).toBeVisible({ timeout: pageReadyTimeout })
    await expect(page.getByRole('link', { name: '远程终端' })).toHaveCount(0)

    await runtime.expectClean()
  })
})
