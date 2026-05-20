import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig, devices } from '@playwright/test'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const baseURL = process.env.HOSTDECK_E2E_BASE_URL || 'http://127.0.0.1:28082'
const ci = Boolean(process.env.CI)
const repoRoot = path.resolve(__dirname, '..')
const serverDir = path.resolve(repoRoot, 'server')
const selectedConfigPath = process.env.HOSTDECK_E2E_CONFIG || path.join(serverDir, 'config', 'config.e2e.yaml')
const configValues = readConfigValues(selectedConfigPath)

process.env.HOSTDECK_E2E_CONFIG ||= selectedConfigPath
process.env.HOSTDECK_E2E_ADMIN_USERNAME ||=
  process.env.HOSTDECK_BOOTSTRAP_ADMIN_USERNAME || configValues.bootstrap_admin_username || 'e2e_admin'
process.env.HOSTDECK_E2E_ADMIN_PASSWORD ||=
  process.env.HOSTDECK_BOOTSTRAP_ADMIN_PASSWORD || configValues.bootstrap_admin_password || 'E2eAdmin123!'

function readConfigValues(filePath: string): Record<string, string> {
  try {
    const content = fs.readFileSync(filePath, 'utf8')
    return Object.fromEntries(
      content
        .split(/\r?\n/)
        .map((line) => line.match(/^([a-zA-Z0-9_]+):\s*"?(.*?)"?\s*$/))
        .filter((match): match is RegExpMatchArray => Boolean(match))
        .map((match) => [match[1], match[2]]),
    )
  } catch {
    return {}
  }
}

export default defineConfig({
  testDir: './tests/e2e',
  testIgnore: ['**/*.js'],
  fullyParallel: false,
  forbidOnly: ci,
  retries: ci ? 2 : 0,
  workers: 1,
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'setup',
      testMatch: /auth\.setup\.ts/,
      use: {
        ...devices['Desktop Chrome'],
        channel: process.env.HOSTDECK_E2E_BROWSER_CHANNEL || 'msedge',
      },
    },
    {
      name: 'edge',
      use: {
        ...devices['Desktop Chrome'],
        channel: process.env.HOSTDECK_E2E_BROWSER_CHANNEL || 'msedge',
      },
      dependencies: ['setup'],
    },
  ],
  webServer: {
    command: 'npm run e2e:serve',
    url: `${baseURL}/api/healthz`,
    reuseExistingServer: !ci,
    timeout: 120000,
  },
})
