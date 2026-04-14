<script setup lang="ts">
import { NButton, NDataTable, NForm, NFormItem, NInput, NScrollbar, NTag, useMessage } from 'naive-ui'
import { computed, h, onMounted, ref } from 'vue'
import { getUsers } from '../services/api'
import { changeOwnPassword, useSession } from '../session'
import type { DataTableColumns } from 'naive-ui'
import type { User } from '../types'

const message = useMessage()
const { currentUser, canManageUsers } = useSession()
const loading = ref(false)
const saving = ref(false)
const users = ref<User[]>([])
const form = ref({
  currentPassword: '',
  newPassword: '',
  confirmPassword: '',
})

const roleLabelMap: Record<string, string> = {
  admin: '管理员',
  operator: '运维',
  viewer: '只读',
}

const columns: DataTableColumns<User> = [
  {
    title: '用户名',
    key: 'username',
    render: (row) => h('strong', { style: 'color: #fff' }, row.username)
  },
  {
    title: '角色',
    key: 'role',
    render: (row) =>
      h(
        NTag,
        { size: 'small', bordered: false, type: row.role === 'admin' ? 'success' : row.role === 'operator' ? 'warning' : 'default' },
        { default: () => roleLabel(row.role) },
      ),
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
]

const currentUserRoleLabel = computed(() => roleLabel(currentUser.value?.role))

function roleLabel(role?: string) {
  if (!role) return '未知'
  return roleLabelMap[role] || role
}

function formatDateTime(value?: string) {
  if (!value) return '未记录'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
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

async function submitPasswordChange() {
  if (saving.value) return
  if (!form.value.currentPassword.trim()) {
    message.warning('请输入当前密码')
    return
  }
  if (form.value.newPassword.trim().length < 8) {
    message.warning('新密码至少需要 8 位')
    return
  }
  if (form.value.newPassword !== form.value.confirmPassword) {
    message.warning('两次输入的新密码不一致')
    return
  }

  saving.value = true
  try {
    await changeOwnPassword(form.value.currentPassword, form.value.newPassword)
    message.success('密码已修改，请重新登录')
    form.value.currentPassword = ''
    form.value.newPassword = ''
    form.value.confirmPassword = ''
    window.location.href = '/login'
  } catch (error) {
    message.error(error instanceof Error ? error.message : '修改密码失败')
  } finally {
    saving.value = false
  }
}

async function refreshPage() {
  loading.value = true
  try {
    await loadUsers()
  } finally {
    loading.value = false
  }
}

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
          <!-- 个人信息卡片 -->
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
          </div>

          <!-- 修改密码卡片 -->
          <div class="bento-card password-card">
            <div class="card-title-bar">
              <span class="card-title">安全设置</span>
              <span style="font-size: 12px; color: #71717a">修改密码</span>
            </div>
            
            <n-form label-placement="top" class="password-form">
              <n-form-item label="当前密码">
                <n-input v-model:value="form.currentPassword" type="password" show-password-on="click" placeholder="验证当前身份" class="dark-input" />
              </n-form-item>
              <n-form-item label="新密码">
                <n-input v-model:value="form.newPassword" type="password" show-password-on="click" placeholder="至少 8 位安全密码" class="dark-input" />
              </n-form-item>
              <n-form-item label="确认新密码">
                <n-input v-model:value="form.confirmPassword" type="password" show-password-on="click" placeholder="再次输入新密码" class="dark-input" />
              </n-form-item>
              <div class="form-actions">
                <n-button type="primary" class="glow-btn" :loading="saving" @click="submitPasswordChange" style="width: 100%">
                  更新密码并重新登录
                </n-button>
              </div>
            </n-form>
          </div>
        </div>

        <div class="right-col">
          <!-- 用户列表卡片 -->
          <div class="bento-card users-card">
            <div class="card-title-bar">
              <span class="card-title">系统账号清单</span>
              <n-tag v-if="!canManageUsers" type="default" style="color: #a1a1aa; background: transparent; border-color: rgba(255,255,255,0.1)">仅管理员可见</n-tag>
              <n-button v-else size="small" ghost type="primary">邀请成员</n-button>
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

/* 个人信息 */
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

/* 修改密码 */
.form-actions {
  margin-top: 8px;
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

:deep(.dark-input .n-input__border) {
  border-color: rgba(255, 255, 255, 0.1);
}
:deep(.dark-input .n-input__placeholder) {
  color: #71717a;
}
:deep(.n-form-item-label) {
  color: #d4d4d8 !important;
}

/* 用户列表 */
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
