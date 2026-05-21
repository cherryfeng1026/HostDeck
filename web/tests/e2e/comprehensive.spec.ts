import { expect, test } from '@playwright/test'
import { adminAuthFile } from './env'
import { captureRuntimeErrors } from './helpers'

const pageReadyTimeout = 25000

test.describe('authenticated admin comprehensive smoke', () => {
  test.use({ storageState: adminAuthFile })

  test('loads primary workspaces without runtime errors', async ({ page }) => {
    const runtime = captureRuntimeErrors(page)

    await page.goto('/')
    await expect(page.locator('.dashboard-shell')).toBeVisible({ timeout: pageReadyTimeout })
    await expect(page.getByText('资源探索', { exact: true })).toBeVisible({ timeout: pageReadyTimeout })

    const dashboardShape = await page.evaluate(async () => {
      const response = await fetch('/api/overview/dashboard', { credentials: 'include' })
      const body = await response.json()
      return {
        ok: response.ok,
        trendsIsArray: Array.isArray(body.trends),
        topServersIsArray: Array.isArray(body.topServers),
      }
    })
    expect(dashboardShape).toEqual({
      ok: true,
      trendsIsArray: true,
      topServersIsArray: true,
    })

    await page.goto('/servers')
    await expect(page.getByText('E2E Demo Server')).toBeVisible({ timeout: pageReadyTimeout })

    await page.goto('/commands')
    await expect(page.getByRole('heading', { name: '远程终端' })).toBeVisible({ timeout: pageReadyTimeout })
    await expect(page.getByText('命令模板')).toBeVisible({ timeout: pageReadyTimeout })

    await page.goto('/alerts')
    await expect(page.getByText(/历史告警/)).toBeVisible({ timeout: pageReadyTimeout })

    await page.goto('/users')
    const tokenTab = page.locator('.n-tabs-tab').filter({ hasText: 'API Token' })
    await expect(tokenTab).toBeVisible({ timeout: pageReadyTimeout })
    await tokenTab.click()
    await expect(page.getByText('命令模板写入')).toBeVisible({ timeout: pageReadyTimeout })

    await runtime.expectClean()
  })
})
