import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)

export const adminAuthFile = path.resolve(__dirname, '../../playwright/.auth/admin.json')
export const viewerAuthFile = path.resolve(__dirname, '../../playwright/.auth/viewer.json')

export const adminUsername = process.env.HOSTDECK_E2E_ADMIN_USERNAME || process.env.HOSTDECK_BOOTSTRAP_ADMIN_USERNAME || 'e2e_admin'
export const adminPassword = process.env.HOSTDECK_E2E_ADMIN_PASSWORD || process.env.HOSTDECK_BOOTSTRAP_ADMIN_PASSWORD || 'E2eAdmin123!'
export const viewerUsername = process.env.HOSTDECK_E2E_VIEWER_USERNAME || 'e2e_viewer'
export const viewerPassword = process.env.HOSTDECK_E2E_VIEWER_PASSWORD || 'E2eViewer123!'
