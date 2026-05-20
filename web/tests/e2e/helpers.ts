import { expect, type Page } from '@playwright/test'

interface RuntimeExpectationRule {
  path: RegExp
  statuses?: number[]
}

interface RuntimeCaptureOptions {
  allowedApiFailures?: RuntimeExpectationRule[]
}

function matchesAllowedRule(pathname: string, status: number | undefined, rules: RuntimeExpectationRule[] | undefined) {
  return rules?.some((rule) => {
    if (!rule.path.test(pathname)) {
      return false
    }
    if (!rule.statuses?.length || status == null) {
      return true
    }
    return rule.statuses.includes(status)
  })
}

export function captureRuntimeErrors(page: Page, options: RuntimeCaptureOptions = {}) {
  const pageErrors: string[] = []
  const consoleErrors: string[] = []
  const apiFailures: string[] = []

  page.on('pageerror', (error) => {
    pageErrors.push(String(error))
  })

  page.on('console', (message) => {
    if (message.type() !== 'error') {
      return
    }

    const location = message.location()
    if (location.url) {
      try {
        const url = new URL(location.url)
        if (url.pathname.startsWith('/api/') && matchesAllowedRule(url.pathname, undefined, options.allowedApiFailures)) {
          return
        }
      } catch {
      }
    }

    consoleErrors.push(message.text())
  })

  page.on('response', (response) => {
    const url = new URL(response.url())
    if (!url.pathname.startsWith('/api/') || response.status() < 400) {
      return
    }
    if (matchesAllowedRule(url.pathname, response.status(), options.allowedApiFailures)) {
      return
    }
    apiFailures.push(`${response.status()} ${url.pathname}`)
  })

  return {
    async expectClean() {
      expect.soft(pageErrors, `page errors: ${pageErrors.join('\n')}`).toEqual([])
      expect.soft(consoleErrors, `console errors: ${consoleErrors.join('\n')}`).toEqual([])
      expect.soft(apiFailures, `api failures: ${apiFailures.join('\n')}`).toEqual([])
    },
  }
}

export async function loginFromPage(page: Page, username: string, password: string) {
  await page.goto('/login')

  const loginForm = page.locator('.login-form')
  if (!(await loginForm.isVisible().catch(() => false))) {
    return
  }

  const usernameInput = loginForm.locator('input').first()
  const passwordInput = loginForm.locator('input[type="password"]').first()

  await usernameInput.fill(username)
  await passwordInput.fill(password)
  await loginForm.getByRole('button', { name: /登录/i }).click()

  await page.waitForURL((url) => !url.pathname.startsWith('/login'))
  await expect(page.getByRole('heading', { name: /欢迎回来/i })).toBeVisible()
}
