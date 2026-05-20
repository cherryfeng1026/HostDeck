import { test as setup, expect, request } from '@playwright/test'
import {
  adminAuthFile,
  adminPassword,
  adminUsername,
  viewerAuthFile,
  viewerPassword,
  viewerUsername,
} from './env'

async function ensureSeedServer(adminApi: Awaited<ReturnType<typeof request.newContext>>) {
  const serversResponse = await adminApi.get('/api/servers')
  expect(
    serversResponse.ok(),
    `/api/servers => ${serversResponse.status()} ${await serversResponse.text()}`,
  ).toBeTruthy()

  const servers = (await serversResponse.json()) as Array<{ name: string }>
  if (servers.some((item) => item.name === 'E2E Demo Server')) {
    return
  }

  const createResponse = await adminApi.post('/api/servers', {
    data: {
      name: 'E2E Demo Server',
      hostname: 'e2e-demo.local',
      ip: '127.0.0.1',
      sshPort: 22,
      username: 'e2e',
      authType: 'password',
      password: 'E2eServer123!',
      collectorMode: 'push',
      tags: ['e2e', 'demo'],
      purpose: '浏览器测试固定样例',
      remark: 'Seeded by Playwright setup',
      enabled: true,
    },
  })
  expect(
    createResponse.ok(),
    `/api/servers create => ${createResponse.status()} ${await createResponse.text()}`,
  ).toBeTruthy()
}

setup('create auth states', async ({ baseURL }) => {
  const adminApi = await request.newContext({ baseURL })
  const adminLoginResponse = await adminApi.post('/api/auth/login', {
    data: {
      username: adminUsername,
      password: adminPassword,
    },
  })
  expect(
    adminLoginResponse.ok(),
    `/api/auth/login => ${adminLoginResponse.status()} ${await adminLoginResponse.text()}`,
  ).toBeTruthy()
  await adminApi.storageState({ path: adminAuthFile })

  const usersResponse = await adminApi.get('/api/users')
  expect(
    usersResponse.ok(),
    `/api/users => ${usersResponse.status()} ${await usersResponse.text()}`,
  ).toBeTruthy()
  const usersBody = (await usersResponse.json()) as {
    items: Array<{ id: number; username: string; role: string; enabled: boolean }>
  }
  const viewer = usersBody.items.find((item) => item.username === viewerUsername)

  let resetFailure: string | null = null

  if (!viewer) {
    const createResponse = await adminApi.post('/api/users', {
      data: {
        username: viewerUsername,
        password: viewerPassword,
        role: 'viewer',
      },
    })
    expect(
      createResponse.ok(),
      `/api/users create => ${createResponse.status()} ${await createResponse.text()}`,
    ).toBeTruthy()
  } else {
    const updateResponse = await adminApi.put(`/api/users/${viewer.id}`, {
      data: {
        role: 'viewer',
        enabled: true,
      },
    })
    expect(
      updateResponse.ok(),
      `/api/users/${viewer.id} update => ${updateResponse.status()} ${await updateResponse.text()}`,
    ).toBeTruthy()

    const resetResponse = await adminApi.post(`/api/users/${viewer.id}/reset-password`, {
      data: {
        newPassword: viewerPassword,
      },
    })
    if (!resetResponse.ok()) {
      resetFailure = `${resetResponse.status()} ${await resetResponse.text()}`
    }
  }

  const viewerApi = await request.newContext({ baseURL })
  const viewerLoginResponse = await viewerApi.post('/api/auth/login', {
    data: {
      username: viewerUsername,
      password: viewerPassword,
    },
  })
  expect(
    viewerLoginResponse.ok(),
    resetFailure
      ? `/api/auth/login viewer => ${viewerLoginResponse.status()} ${await viewerLoginResponse.text()}`
        + ` | reset failure: ${resetFailure}`
      : `/api/auth/login viewer => ${viewerLoginResponse.status()} ${await viewerLoginResponse.text()}`,
  ).toBeTruthy()
  await viewerApi.storageState({ path: viewerAuthFile })

  await ensureSeedServer(adminApi)

  await viewerApi.dispose()
  await adminApi.dispose()
})
