<script setup lang="ts">
import {
  NButton,
  NCheckbox,
  NCheckboxGroup,
  NDataTable,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NModal,
  NPopconfirm,
  NScrollbar,
  NSelect,
  NSpace,
  NSwitch,
  NTabPane,
  NTabs,
  NTag,
  useMessage,
} from 'naive-ui'
import { computed, h, onMounted, ref, watch } from 'vue'
import { createAPIToken, createUser, getAPITokens, getUsers, resetUserPassword, revokeAPIToken, revokeUserSessions, updateUser } from '../services/api'
import { changeOwnPassword, useSession } from '../session'
import type { DataTableColumns } from 'naive-ui'
import type { APITokenItem, User } from '../types'

const message = useMessage()
const { currentUser, canManageUsers } = useSession()
const loading = ref(false)
const saving = ref(false)
const creating = ref(false)
const creatingToken = ref(false)
const activeTab = ref(canManageUsers.value ? 'members' : 'profile')
const users = ref<User[]>([])
const apiTokens = ref<APITokenItem[]>([])
const tokenValue = ref('')
const resetDialogVisible = ref(false)
const resetTarget = ref<User | null>(null)
const resetPasswordValue = ref('')

const tokenForm = ref({
  name: '',
  expiresInHours: 24,
  scopes: ['servers:read'],
})

const passwordForm = ref({
  currentPassword: '',
  newPassword: '',
  confirmPassword: '',
})

const createForm = ref({
  username: '',
  password: '',
  role: 'viewer',
})

const roleOptions = [
  { label: '管理员', value: 'admin' },
  { label: '运维', value: 'operator' },
  { label: '只读', value: 'viewer' },
]

const tokenScopeOptions = [
  { label: '全部权限', value: '*' },
  { label: '服务器读取', value: 'servers:read' },
  { label: '服务器写入', value: 'servers:write' },
  { label: '命令读取', value: 'commands:read' },
  { label: '命令执行', value: 'commands:execute' },
  { label: '命令模板写入', value: 'commands:templates:write' },
  { label: '告警读取', value: 'alerts:read' },
  { label: '告警写入', value: 'alerts:write' },
]

const tokenScopeLabelMap = Object.fromEntries(tokenScopeOptions.map((item) => [item.value, item.label])) as Record<string, string>
const roleLabelMap: Record<string, string> = {
  admin: '管理员',
  operator: '运维',
  viewer: '只读',
}

const currentUserRoleLabel = computed(() => roleLabel(currentUser.value?.role))

function roleLabel(role?: string) {
  if (!role) return '未知'
  return roleLabelMap[role] || role
}

function scopeLabel(scope: string) {
  return tokenScopeLabelMap[scope] || scope
}

function normalizeSelectedScopes(scopes: Array<string | number>) {
  const selected = scopes.map(String).filter((scope) => tokenScopeOptions.some((option) => option.value === scope))
  if (selected.includes('*')) {
    return ['*']
  }
  return Array.from(new Set(selected))
}

function updateTokenScopes(scopes: Array<string | number>) {
  tokenForm.value.scopes = normalizeSelectedScopes(scopes)
}

function formatDateTime(value?: string) {
  if (!value) return '未记录'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

function isSelf(row: User) {
  return row.id === currentUser.value?.id
}

async function loadUsers() {
  if (!canManageUsers.value) {
    users.value = []
    return
  }
  const response = await getUsers()
  users.value = response.items
}

async function loadAPITokens() {
  const response = await getAPITokens()
  apiTokens.value = response.items
}

async function refreshPage() {
  loading.value = true
  try {
    await Promise.all([loadUsers(), loadAPITokens()])
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载用户管理数据失败')
  } finally {
    loading.value = false
  }
}

async function submitPasswordChange() {
  if (saving.value) return
  if (!passwordForm.value.currentPassword.trim()) {
    message.warning('请输入当前密码')
    return
  }
  if (passwordForm.value.newPassword.trim().length < 8) {
    message.warning('新密码至少需要 8 位')
    return
  }
  if (passwordForm.value.newPassword !== passwordForm.value.confirmPassword) {
    message.warning('两次输入的新密码不一致')
    return
  }

  saving.value = true
  try {
    await changeOwnPassword(passwordForm.value.currentPassword, passwordForm.value.newPassword)
    message.success('密码已修改，请重新登录')
    passwordForm.value = { currentPassword: '', newPassword: '', confirmPassword: '' }
    window.location.href = '/login'
  } catch (error) {
    message.error(error instanceof Error ? error.message : '修改密码失败')
  } finally {
    saving.value = false
  }
}

async function submitCreateUser() {
  if (creating.value) return
  if (!createForm.value.username.trim()) {
    message.warning('请输入用户名')
    return
  }
  if (createForm.value.password.trim().length < 8) {
    message.warning('初始密码至少需要 8 位')
    return
  }
  creating.value = true
  try {
    await createUser({
      username: createForm.value.username.trim(),
      password: createForm.value.password,
      role: createForm.value.role,
    })
    createForm.value = { username: '', password: '', role: 'viewer' }
    message.success('用户已创建')
    await loadUsers()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '创建用户失败')
  } finally {
    creating.value = false
  }
}

async function changeUserRole(row: User, role: string) {
  if (role === row.role) return
  try {
    await updateUser(row.id, { role, enabled: row.enabled })
    message.success('角色已更新')
    await loadUsers()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '更新角色失败')
  }
}

async function toggleUserEnabled(row: User, enabled: boolean) {
  if (enabled === row.enabled) return
  try {
    await updateUser(row.id, { role: row.role, enabled })
    message.success(enabled ? '账号已启用' : '账号已禁用')
    await loadUsers()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '更新账号状态失败')
  }
}

function openResetDialog(row: User) {
  resetTarget.value = row
  resetPasswordValue.value = ''
  resetDialogVisible.value = true
}

async function submitResetPassword() {
  if (!resetTarget.value) return
  if (isSelf(resetTarget.value)) {
    message.warning('请使用当前密码修改自己的密码')
    return
  }
  if (resetPasswordValue.value.trim().length < 8) {
    message.warning('新密码至少需要 8 位')
    return
  }
  try {
    await resetUserPassword(resetTarget.value.id, resetPasswordValue.value)
    message.success('密码已重置，目标账号需要重新登录')
    resetDialogVisible.value = false
    resetTarget.value = null
    resetPasswordValue.value = ''
  } catch (error) {
    message.error(error instanceof Error ? error.message : '重置密码失败')
  }
}

async function handleRevokeSessions(row: User) {
  if (isSelf(row)) {
    message.warning('不能通过成员管理撤销当前账号的会话')
    return
  }
  try {
    await revokeUserSessions(row.id)
    message.success('该账号所有会话已撤销')
    await loadUsers()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '撤销会话失败')
  }
}

async function submitCreateToken() {
  if (creatingToken.value) return
  if (!tokenForm.value.name.trim()) {
    message.warning('请输入 Token 名称')
    return
  }
  if (tokenForm.value.expiresInHours == null || tokenForm.value.expiresInHours < 0) {
    message.warning('请输入有效的过期小时数')
    return
  }
  const scopes = normalizeSelectedScopes(tokenForm.value.scopes)
  if (scopes.length === 0) {
    message.warning('请选择至少一个权限范围')
    return
  }
  creatingToken.value = true
  try {
    const response = await createAPIToken({
      name: tokenForm.value.name.trim(),
      expiresInHours: tokenForm.value.expiresInHours,
      scopes,
    })
    tokenValue.value = response.token
    tokenForm.value = { name: '', expiresInHours: 24, scopes: ['servers:read'] }
    message.success('API Token 已创建，请立即保存')
    await loadAPITokens()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '创建 API Token 失败')
  } finally {
    creatingToken.value = false
  }
}

async function handleRevokeToken(row: APITokenItem) {
  try {
    await revokeAPIToken(row.id)
    message.success('API Token 已撤销')
    await loadAPITokens()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '撤销 API Token 失败')
  }
}

const columns: DataTableColumns<User> = [
  {
    title: '用户名',
    key: 'username',
    width: 132,
    render: (row) =>
      h('strong', {
        style: 'display: inline-block; color: #f3f7ff; white-space: nowrap; word-break: normal;',
      }, row.username),
  },
  {
    title: '角色',
    key: 'role',
    width: 122,
    render: (row) =>
      canManageUsers.value
        ? h(NSelect, {
            value: row.role,
            options: roleOptions,
            disabled: isSelf(row),
            consistentMenuWidth: false,
            class: 'dark-input compact-select',
            style: 'width: 120px',
            onUpdateValue: (value: string) => void changeUserRole(row, value),
          })
        : h(NTag, { size: 'small', bordered: false }, { default: () => roleLabel(row.role) }),
  },
  {
    title: '状态',
    key: 'enabled',
    width: 78,
    render: (row) =>
      h(NSwitch, {
        value: row.enabled,
        disabled: isSelf(row),
        onUpdateValue: (value: boolean) => void toggleUserEnabled(row, value),
      }),
  },
  {
    title: '时间',
    key: 'timestamps',
    width: 174,
    render: (row) =>
      h('div', { class: 'member-time-stack' }, [
        h('span', `登录 ${formatDateTime(row.lastLoginAt)}`),
        h('span', `创建 ${formatDateTime(row.createdAt)}`),
      ]),
  },
  {
    title: '操作',
    key: 'actions',
    width: 96,
    render: (row) =>
      h(NSpace, { size: 6, vertical: true }, {
        default: () => [
          h(NButton, { size: 'small', ghost: true, disabled: !row.enabled || isSelf(row), onClick: () => openResetDialog(row) }, { default: () => '重置密码' }),
          h(
            NPopconfirm,
            { onPositiveClick: () => void handleRevokeSessions(row) },
            {
              trigger: () => h(NButton, { size: 'small', ghost: true, disabled: isSelf(row) }, { default: () => '撤销会话' }),
              default: () => `确认撤销 ${row.username} 的所有登录会话？`,
            },
          ),
        ],
      }),
  },
]

watch(canManageUsers, (canManage) => {
  if (canManage && activeTab.value === 'profile') {
    activeTab.value = 'members'
  }
}, { immediate: true })

onMounted(() => {
  void refreshPage()
})
</script>

<template>
  <n-scrollbar class="content-scroll">
    <div class="page-container users-page">
      <div class="page-header">
        <div class="header-text">
          <h1>用户管理</h1>
        </div>
        <div class="header-actions">
          <n-button ghost @click="refreshPage" :loading="loading">刷新</n-button>
        </div>
      </div>

      <n-tabs v-model:value="activeTab" type="line" animated class="management-tabs">
        <n-tab-pane name="profile" tab="我的账号">
          <div class="account-grid">
            <section class="panel profile-panel">
              <div class="profile-header">
                <div class="avatar-circle">
                  {{ currentUser?.username?.charAt(0).toUpperCase() || 'U' }}
                </div>
                <div>
                  <h2>{{ currentUser?.username || '未登录' }}</h2>
                  <n-tag :type="currentUser?.role === 'admin' ? 'success' : 'default'" size="small" bordered>
                    {{ currentUserRoleLabel }}
                  </n-tag>
                </div>
              </div>
              <div class="info-grid">
                <div class="info-item">
                  <span>最近登录</span>
                  <strong>{{ formatDateTime(currentUser?.lastLoginAt) }}</strong>
                </div>
                <div class="info-item">
                  <span>创建时间</span>
                  <strong>{{ formatDateTime(currentUser?.createdAt) }}</strong>
                </div>
              </div>
            </section>

            <section class="panel">
              <div class="panel-title">安全设置</div>
              <n-form label-placement="top" class="form-grid">
                <n-form-item label="当前密码">
                  <n-input v-model:value="passwordForm.currentPassword" type="password" show-password-on="click" placeholder="验证当前身份" class="dark-input" />
                </n-form-item>
                <n-form-item label="新密码">
                  <n-input v-model:value="passwordForm.newPassword" type="password" show-password-on="click" placeholder="至少 8 位" class="dark-input" />
                </n-form-item>
                <n-form-item label="确认新密码">
                  <n-input v-model:value="passwordForm.confirmPassword" type="password" show-password-on="click" placeholder="再次输入新密码" class="dark-input" />
                </n-form-item>
                <n-button type="primary" :loading="saving" @click="submitPasswordChange">更新密码并重新登录</n-button>
              </n-form>
            </section>
          </div>
        </n-tab-pane>

        <n-tab-pane name="members" tab="成员管理">
          <div class="member-layout">
            <section v-if="canManageUsers" class="panel member-create">
              <div class="panel-title">新增成员</div>
              <n-form label-placement="top" class="form-grid">
                <n-form-item label="用户名">
                  <n-input v-model:value="createForm.username" placeholder="例如 operator-1" class="dark-input" />
                </n-form-item>
                <n-form-item label="初始密码">
                  <n-input v-model:value="createForm.password" type="password" show-password-on="click" placeholder="至少 8 位" class="dark-input" />
                </n-form-item>
                <n-form-item label="角色">
                  <n-select v-model:value="createForm.role" :options="roleOptions" class="dark-input" />
                </n-form-item>
                <n-button type="primary" :loading="creating" @click="submitCreateUser">创建用户</n-button>
              </n-form>
            </section>

            <section class="panel member-table">
              <div class="panel-head">
                <div class="panel-title">系统账号</div>
                <n-tag v-if="canManageUsers" size="small" type="success" bordered>{{ users.length }} 个账号</n-tag>
                <n-tag v-else size="small" bordered>仅管理员可见</n-tag>
              </div>
              <div v-if="!canManageUsers" class="empty-state">
                当前账号是 {{ currentUserRoleLabel }}，无权查看成员列表。
              </div>
              <n-data-table v-else :columns="columns" :data="users" :loading="loading" :bordered="false" class="dark-table" />
            </section>
          </div>
        </n-tab-pane>

        <n-tab-pane name="tokens" tab="API Token">
          <div class="token-layout">
            <section class="panel token-create">
              <div class="panel-title">创建 Token</div>
              <n-form label-placement="top" class="form-grid">
                <n-form-item label="Token 名称">
                  <n-input v-model:value="tokenForm.name" placeholder="例如 grafana-readonly" class="dark-input" />
                </n-form-item>
                <n-form-item label="有效期（小时，0 为不过期）">
                  <n-input-number v-model:value="tokenForm.expiresInHours" :min="0" :precision="0" style="width: 100%" class="dark-input" />
                </n-form-item>
                <n-form-item label="权限范围">
                  <n-checkbox-group :value="tokenForm.scopes" @update:value="updateTokenScopes">
                    <div class="token-scope-options">
                      <n-checkbox v-for="option in tokenScopeOptions" :key="option.value" :value="option.value">
                        {{ option.label }}
                      </n-checkbox>
                    </div>
                  </n-checkbox-group>
                </n-form-item>
                <n-button type="primary" :loading="creatingToken" @click="submitCreateToken">创建 API Token</n-button>
              </n-form>

              <div v-if="tokenValue" class="token-output">
                <div class="token-output-header">
                  <strong>新建 Token</strong>
                  <n-button size="small" ghost @click="tokenValue = ''">关闭</n-button>
                </div>
                <div class="token-output-value">{{ tokenValue }}</div>
              </div>
            </section>

            <section class="panel token-list-panel">
              <div class="panel-head">
                <div class="panel-title">我的 Token</div>
                <n-tag size="small" bordered>{{ apiTokens.length }} 个</n-tag>
              </div>
              <div v-if="apiTokens.length === 0" class="empty-state">
                暂无 API Token。
              </div>
              <div v-else class="token-list">
                <div v-for="item in apiTokens" :key="item.id" class="token-item">
                  <div class="token-item-main">
                    <div class="token-item-top">
                      <strong>{{ item.name }}</strong>
                      <n-tag :type="item.isActive ? 'success' : 'default'" size="small" bordered>
                        {{ item.isActive ? '生效中' : '已停用' }}
                      </n-tag>
                    </div>
                    <code>{{ item.prefix }}</code>
                    <div class="token-meta">
                      <span>创建 {{ formatDateTime(item.createdAt) }}</span>
                      <span>最近使用 {{ formatDateTime(item.lastUsedAt) }}</span>
                      <span>过期 {{ item.expiresAt ? formatDateTime(item.expiresAt) : '永不过期' }}</span>
                    </div>
                    <div class="token-scopes">
                      <n-tag v-for="scope in item.scopes" :key="scope" size="small" :bordered="false" :type="scope === '*' ? 'warning' : 'info'">
                        {{ scopeLabel(scope) }}
                      </n-tag>
                    </div>
                  </div>
                  <n-popconfirm @positive-click="() => void handleRevokeToken(item)">
                    <template #trigger>
                      <n-button size="small" ghost type="error" :disabled="!item.isActive">撤销</n-button>
                    </template>
                    确认撤销 Token "{{ item.name }}"？
                  </n-popconfirm>
                </div>
              </div>
            </section>
          </div>
        </n-tab-pane>
      </n-tabs>
    </div>
  </n-scrollbar>

  <n-modal v-model:show="resetDialogVisible" preset="card" title="重置用户密码" style="max-width: 420px">
    <n-form label-placement="top">
      <n-form-item :label="`为 ${resetTarget?.username || ''} 设置新密码`">
        <n-input v-model:value="resetPasswordValue" type="password" show-password-on="click" placeholder="至少 8 位" class="dark-input" />
      </n-form-item>
      <div class="modal-actions">
        <n-space justify="end">
          <n-button ghost @click="resetDialogVisible = false">取消</n-button>
          <n-button type="primary" @click="submitResetPassword">确认重置</n-button>
        </n-space>
      </div>
    </n-form>
  </n-modal>
</template>

<style scoped>
.content-scroll {
  flex: 1;
}

.users-page {
  max-width: 100%;
  margin: 0;
}

.management-tabs {
  padding: 0;
  border: 0;
  border-radius: 8px;
  background: transparent;
}

.management-tabs :deep(.n-tabs-nav) {
  padding: 0 2px 12px;
  border-bottom: 1px solid rgba(93, 120, 162, 0.18);
}

.management-tabs :deep(.n-tabs-tab) {
  padding-inline: 0;
}

.account-grid,
.member-layout,
.token-layout {
  display: grid;
  grid-template-columns: minmax(320px, 420px) minmax(0, 1fr);
  gap: 14px;
  align-items: start;
  margin-top: 14px;
}

.member-layout {
  grid-template-columns: minmax(300px, 360px) minmax(0, 1fr);
}

.panel {
  min-width: 0;
  padding: 14px;
  border-radius: 8px;
  border: 1px solid rgba(93, 120, 162, 0.22);
  background: var(--app-panel);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.035);
}

.panel-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
}

.panel-title {
  margin-bottom: 12px;
  color: var(--app-text);
  font-size: 16px;
  font-weight: 700;
}

.panel-head .panel-title {
  margin-bottom: 0;
}

.profile-header {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-bottom: 18px;
}

.profile-header h2 {
  margin: 0 0 8px;
  color: var(--app-text);
  font-size: 20px;
}

.avatar-circle {
  width: 54px;
  height: 54px;
  border-radius: 8px;
  background: rgba(79, 131, 255, 0.18);
  border: 1px solid rgba(79, 131, 255, 0.34);
  color: #e4efff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 24px;
  font-weight: 800;
}

.info-grid {
  display: grid;
  gap: 10px;
}

.info-item {
  display: grid;
  gap: 4px;
  padding: 12px;
  border: 1px solid rgba(93, 120, 162, 0.16);
  background: rgba(17, 32, 52, 0.46);
  border-radius: 8px;
}

.info-item span,
.token-meta {
  color: var(--app-text-soft);
  font-size: 12px;
}

.info-item strong {
  color: #dce7f6;
  font-size: 13px;
}

.form-grid {
  display: grid;
  gap: 0;
}

.member-table,
.token-list-panel {
  overflow: hidden;
}

.member-table :deep(.n-data-table-th) {
  padding: 10px 12px !important;
}

.member-table :deep(.n-data-table-td) {
  padding: 10px 12px !important;
}

.member-time-stack {
  display: grid;
  gap: 3px;
  color: var(--app-text-soft);
  font-size: 12px;
  line-height: 1.35;
  white-space: nowrap;
}

.token-scope-options {
  display: flex;
  flex-wrap: wrap;
  gap: 10px 14px;
}

.token-output {
  margin-top: 16px;
  padding: 14px;
  border: 1px solid rgba(32, 212, 255, 0.26);
  border-radius: 8px;
  background: rgba(32, 212, 255, 0.08);
}

.token-output-header,
.token-item-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.token-output-value,
.token-item code {
  color: #dce7f6;
  font-family: var(--app-font-mono);
  font-size: 12px;
  line-height: 1.6;
  word-break: break-all;
}

.token-list {
  display: grid;
  gap: 12px;
}

.token-item {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  padding: 14px;
  border: 1px solid rgba(93, 120, 162, 0.16);
  border-radius: 8px;
  background: rgba(17, 32, 52, 0.42);
}

.token-item-main {
  min-width: 0;
}

.token-item-top {
  justify-content: flex-start;
  margin-bottom: 8px;
  color: var(--app-text);
}

.token-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 14px;
  margin-top: 8px;
}

.token-scopes {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 10px;
}

.empty-state {
  padding: 36px 0;
  color: var(--app-text-faint);
  text-align: center;
}

:deep(.dark-input .n-input-wrapper),
:deep(.dark-input .n-base-selection),
:deep(.dark-input .n-input-number) {
  border-radius: 8px !important;
  background: rgba(9, 20, 36, 0.88) !important;
  background-color: rgba(9, 20, 36, 0.88) !important;
  border: 1px solid rgba(93, 120, 162, 0.24) !important;
}

:deep(.dark-input .n-input__input),
:deep(.dark-input .n-input__textarea),
:deep(.dark-input .n-input__input-el),
:deep(.dark-input .n-input__textarea-el),
:deep(.dark-input .n-input-number-input__input) {
  background: transparent !important;
  background-color: transparent !important;
  color: var(--app-text) !important;
  color-scheme: dark !important;
  font-size: 13px !important;
  line-height: 1.45 !important;
}

:deep(.dark-input .n-input__input-el:-webkit-autofill),
:deep(.dark-input .n-input__input-el:-webkit-autofill:hover),
:deep(.dark-input .n-input__input-el:-webkit-autofill:focus),
:deep(.dark-input .n-input__input-el:-webkit-autofill:active) {
  -webkit-text-fill-color: var(--app-text) !important;
  background-color: rgba(9, 20, 36, 0.88) !important;
  box-shadow: 0 0 0 1000px rgba(9, 20, 36, 0.88) inset !important;
  caret-color: var(--app-accent) !important;
}

:deep(.dark-input .n-input__border),
:deep(.dark-input .n-input__state-border),
:deep(.dark-input .n-base-selection__border),
:deep(.dark-input .n-base-selection__state-border) {
  display: none !important;
}

:deep(.dark-table) {
  --n-th-color: transparent !important;
  --n-td-color: transparent !important;
  --n-th-text-color: var(--app-text-soft) !important;
  --n-td-text-color: #dce7f6 !important;
  --n-border-color: rgba(93, 120, 162, 0.12) !important;
}

@media (max-width: 980px) {
  .account-grid,
  .member-layout,
  .token-layout {
    grid-template-columns: 1fr;
  }
}
</style>
