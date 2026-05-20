import { spawn } from 'node:child_process'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const repoRoot = path.resolve(__dirname, '..', '..')
const serverDir = path.join(repoRoot, 'server')
const configPath = process.env.HOSTDECK_E2E_CONFIG || path.join(serverDir, 'config', 'config.e2e.yaml')
const e2eDBDSN = process.env.HOSTDECK_E2E_DB_DSN?.trim()

if (!e2eDBDSN) {
  console.error('HostDeck E2E server startup requires HOSTDECK_E2E_DB_DSN.')
  console.error('Use a dedicated E2E database. Do not rely on HOSTDECK_DB_DSN, DATABASE_URL, or config db_dsn for E2E.')
  process.exit(1)
}

const childEnv = {
  ...process.env,
  DATABASE_URL: '',
  HOSTDECK_ADDR: process.env.HOSTDECK_ADDR || ':28082',
  HOSTDECK_DB_DSN: e2eDBDSN,
  HOSTDECK_SESSION_COOKIE_NAME: process.env.HOSTDECK_SESSION_COOKIE_NAME || 'hostdeck_e2e_session',
  HOSTDECK_BOOTSTRAP_ADMIN_USERNAME: process.env.HOSTDECK_BOOTSTRAP_ADMIN_USERNAME || 'e2e_admin',
  HOSTDECK_BOOTSTRAP_ADMIN_PASSWORD: process.env.HOSTDECK_BOOTSTRAP_ADMIN_PASSWORD || 'E2eAdmin123!',
}

const child = spawn('go', ['run', './cmd/hostdeck', '--config', configPath], {
  cwd: serverDir,
  stdio: 'inherit',
  env: childEnv,
})

child.on('exit', (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal)
    return
  }
  process.exit(code ?? 0)
})

child.on('error', (error) => {
  console.error(error)
  process.exit(1)
})

for (const event of ['SIGINT', 'SIGTERM']) {
  process.on(event, () => {
    child.kill(event)
  })
}
