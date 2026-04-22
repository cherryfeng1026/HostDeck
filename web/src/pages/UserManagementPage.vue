<script setup lang="ts">
import {
  NButton,
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
  NTag,
  useMessage,
} from 'naive-ui'
import { computed, h, onMounted, ref } from 'vue'
import { createAPIToken, createUser, getAPITokens, getUsers, resetUserPassword, revokeAPIToken, revokeUserSessions, updateUser } from '../services/api'
import { changeOwnPassword, useSession } from '../session'
import type { DataTableColumns } from 'naive-ui'
import type { APITokenItem, User } from '../types'

const message = useMessage()
const { currentUser, canManageUsers, canManageInfrastructure } = useSession()
const loading = ref(false)
const saving = ref(false)
const creating = ref(false)
const creatingToken = ref(false)
const users = ref<User[]>([])
const apiTokens = ref<APITokenItem[]>([])
const tokenValue = ref('')
const tokenForm = ref({
  name: '',
  expiresInHours: 24,
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
const resetDialogVisible = ref(false)
const resetTarget = ref<User | null>(null)
const resetPasswordValue = ref('')
const roleOptions = [
  { label: '管理员', value: 'admin' },
  { label: '运维', value: 'operator' },
  { label: '只读', value: 'viewer' },
]

const roleLabelMap: Record<string, string> = {
  admin: '管理员',
  operator: '运维',
  viewer: '只读',
}

const currentUserRoleLabel = computed(() => roleLabel(currentUser.value?.role))
const roleDescriptions: Record<string, string> = {
  admin: '可管理成员、角色、会话、密码与 API Token，并可执行全部运维操作。',
  operator: '可执行采集、命令、告警规则等运维操作，但不能管理成员与角色。',
  viewer: '仅可查看系统信息，不能执行命令、修改规则或管理成员。',
}
const currentRoleDescription = computed(() => roleDescription(currentUser.value?.role))
const managementRiskTips = computed(() => [
  '禁用账号会立即阻断该成员后续登录。',
  '修改角色或撤销会话后，目标账号现有登录状态会失效。',
  '重置密码后，目标账号需要使用新密码重新登录。',
])

function roleLabel(role?: string) {
  if (!role) return '未知'
  return roleLabelMap[role] || role
}

function roleDescription(role?: string) {
  if (!role) return '当前账号未分配角色。'
  return roleDescriptions[role] || '当前角色暂无说明。'
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
  try {
    const response = await getUsers()
    users.value = response.items
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载用户列表失败')
  }
}

async function loadAPITokens() {
  try {
    const response = await getAPITokens()
    apiTokens.value = response.items
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载 API Token 失败')
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
    passwordForm.value.currentPassword = ''
    passwordForm.value.newPassword = ''
    passwordForm.value.confirmPassword = ''
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
      username: createForm.value.username,
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
    message.success('密码已重置，目标账号需重新登录')
    resetDialogVisible.value = false
    resetTarget.value = null
    resetPasswordValue.value = ''
  } catch (error) {
    message.error(error instanceof Error ? error.message : '重置密码失败')
  }
}

async function handleRevokeSessions(row: User) {
  if (isSelf(row)) {
    message.warning('不能通过成员管理入口撤销当前登录账号的会话')
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
  creatingToken.value = true
  try {
    const response = await createAPIToken({
      name: tokenForm.value.name,
      expiresInHours: tokenForm.value.expiresInHours,
    })
    tokenValue.value = response.token
    tokenForm.value = { name: '', expiresInHours: 24 }
    message.success('API Token 已创建，请立即复制保存')
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

async function refreshPage() {
  loading.value = true
  try {
    await Promise.all([loadUsers(), loadAPITokens()])
  } finally {
    loading.value = false
  }
}

const columns: DataTableColumns<User> = [
  {
    title: '用户名',
    key: 'username',
    render: (row) => h('strong', { style: 'color: #fff' }, row.username),
  },
  {
    title: '角色',
    key: 'role',
    render: (row) =>
      canManageUsers.value
        ? h(NSelect, {
            value: row.role,
            options: roleOptions,
            disabled: isSelf(row),
            consistentMenuWidth: false,
            style: 'width: 120px',
            onUpdateValue: (value: string) => void changeUserRole(row, value),
          })
        : h(
            NTag,
            { size: 'small', bordered: false, type: row.role === 'admin' ? 'success' : row.role === 'operator' ? 'warning' : 'default' },
            { default: () => roleLabel(row.role) },
          ),
  },
  {
    title: '状态',
    key: 'enabled',
    render: (row) =>
      h(NSwitch, {
        value: row.enabled,
        disabled: isSelf(row),
        onUpdateValue: (value: boolean) => void toggleUserEnabled(row, value),
      }),
  },
  {
    title: '最近登录',
    key: 'lastLoginAt',
    render: (row) => h('span', { style: 'color: #a1a1aa' }, formatDateTime(row.lastLoginAt)),
  },
  {
    title: '创建时间',
    key: 'createdAt',
    render: (row) => h('span', { style: 'color: #a1a1aa' }, formatDateTime(row.createdAt)),
  },
  {
    title: '操作',
    key: 'actions',
    render: (row) =>
      h(NSpace, { size: 8 }, {
        default: () => [
          h(
            NButton,
            {
              size: 'small',
              ghost: true,
              disabled: !row.enabled || isSelf(row),
              onClick: () => openResetDialog(row),
            },
            { default: () => '重置密码' },
          ),
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

onMounted(() => {
  void refreshPage()
})
</script>

<template>
  <n-scrollbar class="content-scroll">
    <div class="page-container">
      <div class="page-header">
        <div class="header-text">
          <h1>用户管理</h1>
          <p>管理个人账号设置与系统成员。</p>
        </div>
        <div class="header-actions">
          <n-button ghost @click="refreshPage" :loading="loading" style="color: #a1a1aa; border-color: rgba(255,255,255,0.1)">
            刷新信息
          </n-button>
        </div>
      </div>

      <div class="layout-grid">
        <div class="left-col">
          <div class="bento-card profile-card">
            <div class="card-title-bar">
              <span class="card-title">我的账号</span>
            </div>

            <div class="profile-header">
              <div class="avatar-circle">
                {{ currentUser?.username?.charAt(0).toUpperCase() || 'U' }}
              </div>
              <div class="profile-title">
                <h2>{{ currentUser?.username || '未登录' }}</h2>
                <n-tag :type="currentUser?.role === 'admin' ? 'success' : 'default'" size="small" bordered>
                  {{ currentUserRoleLabel }}
                </n-tag>
              </div>
            </div>

            <div class="info-grid">
              <div class="info-item">
                <span class="info-label">最近登录</span>
                <span class="info-val">{{ formatDateTime(currentUser?.lastLoginAt) }}</span>
              </div>
              <div class="info-item">
                <span class="info-label">创建时间</span>
                <span class="info-val">{{ formatDateTime(currentUser?.createdAt) }}</span>
              </div>
            </div>

            <div class="role-hint-panel">
              <div class="role-hint-head">
                <span>当前角色权限</span>
                <n-tag size="small" bordered :type="currentUser?.role === 'admin' ? 'success' : currentUser?.role === 'operator' ? 'warning' : 'default'">
                  {{ currentUserRoleLabel }}
                </n-tag>
              </div>
              <p>{{ currentRoleDescription }}</p>
            </div>
          </div>

          <div class="bento-card password-card">
            <div class="card-title-bar">
              <span class="card-title">安全设置</span>
              <span style="font-size: 12px; color: #71717a">修改密码</span>
            </div>

            <n-form label-placement="top" class="password-form">
              <n-form-item label="当前密码">
                <n-input v-model:value="passwordForm.currentPassword" type="password" show-password-on="click" placeholder="验证当前身份" class="dark-input" />
              </n-form-item>
              <n-form-item label="新密码">
                <n-input v-model:value="passwordForm.newPassword" type="password" show-password-on="click" placeholder="至少 8 位安全密码" class="dark-input" />
              </n-form-item>
              <n-form-item label="确认新密码">
                <n-input v-model:value="passwordForm.confirmPassword" type="password" show-password-on="click" placeholder="再次输入新密码" class="dark-input" />
              </n-form-item>
              <div class="form-actions">
                <n-button type="primary" class="glow-btn" :loading="saving" @click="submitPasswordChange" style="width: 100%">
                  更新密码并重新登录
                </n-button>
              </div>
            </n-form>
          </div>

          <div v-if="canManageUsers" class="bento-card create-card">
            <div class="card-title-bar">
              <span class="card-title">新增成员</span>
            </div>

            <n-form label-placement="top">
              <n-form-item label="用户名">
                <n-input v-model:value="createForm.username" placeholder="例如 operator-1" class="dark-input" />
              </n-form-item>
              <n-form-item label="初始密码">
                <n-input v-model:value="createForm.password" type="password" show-password-on="click" placeholder="至少 8 位" class="dark-input" />
              </n-form-item>
              <n-form-item label="角色">
                <n-select v-model:value="createForm.role" :options="roleOptions" class="dark-select" />
              </n-form-item>
              <n-button type="primary" class="glow-btn" :loading="creating" @click="submitCreateUser" style="width: 100%">
                创建用户
              </n-button>
            </n-form>
          </div>
        </div>

        <div class="right-col">
          <div class="bento-card token-card">
            <div class="card-title-bar">
              <span class="card-title">API Token</span>
              <span style="font-size: 12px; color: #71717a">用于外部集成调用接口</span>
            </div>

            <n-form label-placement="top">
              <n-form-item label="Token 名称">
                <n-input v-model:value="tokenForm.name" placeholder="例如 grafana-readonly" class="dark-input" />
              </n-form-item>
              <n-form-item label="有效期（小时，0 为不过期）">
                <n-input-number v-model:value="tokenForm.expiresInHours" :min="0" :precision="0" style="width: 100%" />
              </n-form-item>
              <n-button type="primary" class="glow-btn" :loading="creatingToken" @click="submitCreateToken" style="width: 100%">
                创建 API Token
              </n-button>
            </n-form>

            <div v-if="tokenValue" class="token-output">
              <div class="token-output-header">
                <span>新建 Token</span>
                <n-button size="small" ghost @click="tokenValue = ''">关闭</n-button>
              </div>
              <div class="token-output-tip">该明文仅展示一次，请立即复制保存。</div>
              <div class="token-output-value">{{ tokenValue }}</div>
            </div>

            <div class="token-list-header">
              <span class="token-list-title">我的 Token</span>
              <n-tag type="default" style="color: #a1a1aa; background: transparent; border-color: rgba(255,255,255,0.1)">
                {{ apiTokens.length }} 个
              </n-tag>
            </div>

            <div v-if="apiTokens.length === 0" class="empty-state token-empty">
              暂无 API Token，可创建后用于脚本或第三方系统接入。
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
                  <div class="token-prefix">{{ item.prefix }}</div>
                  <div class="token-meta">
                    <span>创建于 {{ formatDateTime(item.createdAt) }}</span>
                    <span>最近使用 {{ formatDateTime(item.lastUsedAt) }}</span>
                    <span>过期时间 {{ item.expiresAt ? formatDateTime(item.expiresAt) : '永不过期' }}</span>
                  </div>
                </div>
                <n-popconfirm @positive-click="() => void handleRevokeToken(item)">
                  <template #trigger>
                    <n-button size="small" ghost :disabled="!item.isActive">撤销</n-button>
                  </template>
                  确认撤销 Token “{{ item.name }}”？
                </n-popconfirm>
              </div>
            </div>
          </div>

          <div class="bento-card users-card">
            <div class="card-title-bar">
              <span class="card-title">系统账号清单</span>
              <n-tag v-if="!canManageUsers" type="default" style="color: #a1a1aa; background: transparent; border-color: rgba(255,255,255,0.1)">仅管理员可见</n-tag>
              <n-tag v-else type="success" bordered>
                {{ users.length }} 个账号
              </n-tag>
            </div>

            <div class="role-matrix">
              <div class="role-matrix-item">
                <strong>管理员</strong>
                <span>成员与角色管理、密码重置、会话撤销、API Token、全部运维操作</span>
              </div>
              <div class="role-matrix-item">
                <strong>运维</strong>
                <span>服务器、采集、命令执行、告警规则等运维能力，不可管理成员</span>
              </div>
              <div class="role-matrix-item">
                <strong>只读</strong>
                <span>仅查看系统信息，不可执行命令、修改规则或管理成员</span>
              </div>
            </div>

            <div v-if="canManageUsers" class="risk-banner">
              <strong>高风险操作提醒</strong>
              <ul>
                <li v-for="tip in managementRiskTips" :key="tip">{{ tip }}</li>
              </ul>
            </div>
            <div v-else class="risk-banner muted">
              <strong>当前权限边界</strong>
              <p v-if="canManageInfrastructure">您当前是 {{ currentUserRoleLabel }}，可执行运维操作，但不能管理成员与角色。</p>
              <p v-else>您当前是 {{ currentUserRoleLabel }}，仅可查看系统信息，不能执行命令或管理成员。</p>
            </div>

            <div v-if="!canManageUsers" class="empty-state">
              您的账号角色 ({{ currentUserRoleLabel }}) 无权查看成员列表。
            </div>
            <div v-else>
              <n-data-table :columns="columns" :data="users" :loading="loading" :bordered="false" class="dark-table" />
            </div>
          </div>
        </div>
      </div>
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

.page-container {
  padding: 32px;
  max-width: 1600px;
  margin: 0 auto;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  margin-bottom: 32px;
}

.header-text h1 {
  font-size: 32px;
  font-weight: 600;
  margin: 0 0 8px 0;
  color: #fff;
  letter-spacing: -0.5px;
}

.header-text p {
  margin: 0;
  color: #a1a1aa;
  font-size: 16px;
}

.layout-grid {
  display: grid;
  grid-template-columns: minmax(360px, 400px) minmax(0, 1fr);
  gap: 24px;
  align-items: start;
}

.left-col {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.bento-card {
  background: rgba(20, 20, 25, 0.6);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 20px;
  padding: 24px;
  position: relative;
  overflow: hidden;
}

.card-title-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
  padding-bottom: 12px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.card-title {
  font-size: 16px;
  font-weight: 600;
  color: #fff;
}

.profile-header {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 24px;
}

.avatar-circle {
  width: 64px;
  height: 64px;
  border-radius: 50%;
  background: linear-gradient(135deg, #10b981, #059669);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 28px;
  font-weight: 700;
  box-shadow: 0 0 20px rgba(16, 185, 129, 0.3);
}

.profile-title h2 {
  margin: 0 0 8px 0;
  font-size: 20px;
  color: #fff;
}

.info-grid {
  display: grid;
  gap: 16px;
}

.info-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  background: rgba(255, 255, 255, 0.02);
  border: 1px solid rgba(255, 255, 255, 0.05);
  border-radius: 12px;
}

.info-label {
  color: #71717a;
  font-size: 13px;
}

.info-val {
  color: #d4d4d8;
  font-size: 14px;
  font-weight: 500;
}

.form-actions,
.modal-actions {
  margin-top: 8px;
}

.role-hint-panel {
  margin-top: 20px;
  padding: 16px;
  background: rgba(255, 255, 255, 0.03);
  border: 1px solid rgba(255, 255, 255, 0.05);
  border-radius: 14px;
}

.role-hint-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 10px;
}

.role-hint-head span {
  color: #f4f4f5;
  font-size: 14px;
  font-weight: 600;
}

.role-hint-panel p {
  margin: 0;
  color: #a1a1aa;
  font-size: 13px;
  line-height: 1.7;
}

.glow-btn {
  box-shadow: 0 0 20px rgba(16, 185, 129, 0.4);
  transition: all 0.3s ease;
  background-color: #10b981 !important;
  color: #fff !important;
  border: none;
}

.glow-btn:hover {
  box-shadow: 0 0 30px rgba(16, 185, 129, 0.6);
  transform: translateY(-1px);
}

:deep(.dark-input .n-input__border),
:deep(.dark-select .n-base-selection) {
  border-color: rgba(255, 255, 255, 0.1);
}

:deep(.dark-input .n-input__placeholder) {
  color: #71717a;
}

:deep(.n-form-item-label),
:deep(.n-modal .n-card-header__main) {
  color: #d4d4d8 !important;
}

.token-output {
  margin-top: 20px;
  padding: 16px;
  background: rgba(16, 185, 129, 0.08);
  border: 1px solid rgba(16, 185, 129, 0.18);
  border-radius: 14px;
}

.token-output-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
  color: #f4f4f5;
  font-size: 14px;
  font-weight: 600;
}

.token-output-tip {
  color: #a1a1aa;
  font-size: 12px;
  margin-bottom: 12px;
}

.token-output-value {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  font-size: 13px;
  color: #d4d4d8;
  word-break: break-all;
  line-height: 1.6;
}

.token-list-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 24px;
  margin-bottom: 16px;
}

.token-list-title {
  color: #fff;
  font-size: 14px;
  font-weight: 600;
}

.token-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.token-item {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
  padding: 16px;
  background: rgba(255, 255, 255, 0.02);
  border: 1px solid rgba(255, 255, 255, 0.05);
  border-radius: 14px;
}

.token-item-main {
  min-width: 0;
  flex: 1;
}

.token-item-top {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
  color: #fff;
}

.token-prefix {
  color: #10b981;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  font-size: 13px;
  margin-bottom: 8px;
}

.token-meta {
  display: flex;
  flex-direction: column;
  gap: 4px;
  color: #a1a1aa;
  font-size: 12px;
}

.token-empty {
  padding: 20px 0 8px;
}

.role-matrix {
  display: grid;
  gap: 12px;
  margin-bottom: 20px;
}

.role-matrix-item {
  display: grid;
  gap: 6px;
  padding: 14px 16px;
  background: rgba(255, 255, 255, 0.02);
  border: 1px solid rgba(255, 255, 255, 0.05);
  border-radius: 14px;
}

.role-matrix-item strong {
  color: #f4f4f5;
  font-size: 14px;
}

.role-matrix-item span {
  color: #a1a1aa;
  font-size: 13px;
  line-height: 1.6;
}

.risk-banner {
  margin-bottom: 20px;
  padding: 16px;
  background: rgba(245, 158, 11, 0.08);
  border: 1px solid rgba(245, 158, 11, 0.2);
  border-radius: 14px;
}

.risk-banner strong {
  display: block;
  margin-bottom: 10px;
  color: #fde68a;
  font-size: 14px;
}

.risk-banner ul {
  margin: 0;
  padding-left: 18px;
  color: #f4f4f5;
  display: grid;
  gap: 6px;
  font-size: 13px;
  line-height: 1.6;
}

.risk-banner p {
  margin: 0;
  color: #f4f4f5;
  font-size: 13px;
  line-height: 1.7;
}

.risk-banner.muted {
  background: rgba(59, 130, 246, 0.08);
  border-color: rgba(59, 130, 246, 0.2);
}

.risk-banner.muted strong {
  color: #bfdbfe;
}

.empty-state {
  padding: 40px 0;
  text-align: center;
  color: #71717a;
}

:deep(.dark-table) {
  --n-th-color: transparent !important;
  --n-td-color: transparent !important;
  --n-th-text-color: #a1a1aa !important;
  --n-td-text-color: #d4d4d8 !important;
  --n-border-color: rgba(255, 255, 255, 0.05) !important;
  --n-th-font-weight: 500 !important;
}

:deep(.n-data-table-th) {
  border-bottom: 1px solid rgba(255,255,255,0.05) !important;
}

:deep(.n-data-table-td) {
  border-bottom: 1px solid rgba(255,255,255,0.02) !important;
}

:deep(.n-data-table-tr:hover .n-data-table-td) {
  background-color: rgba(255, 255, 255, 0.02) !important;
}

@media (max-width: 960px) {
  .layout-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 640px) {
  .page-header {
    flex-direction: column;
    align-items: flex-start;
    gap: 16px;
  }
}
</style>
