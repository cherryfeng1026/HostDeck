<script setup lang="ts">
import { NAlert, NButton, NCheckbox, NForm, NFormItem, NInput, useMessage } from 'naive-ui'
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ensureSessionLoaded, loginWithPassword, useSession } from '../session'

const router = useRouter()
const route = useRoute()
const message = useMessage()
const { state } = useSession()
const model = ref({
  username: '',
  password: '',
  remember: true,
})

const redirectTarget = computed(() => {
  const value = route.query.redirect
  return typeof value === 'string' && value.startsWith('/') ? value : '/'
})

onMounted(async () => {
  try {
    await ensureSessionLoaded(true)
    if (state.user) {
      await router.replace(redirectTarget.value)
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : '初始化登录状态失败')
  }
})

async function handleLogin() {
  if (state.authenticating) return
  if (!model.value.username.trim() || !model.value.password) {
    message.warning('请输入用户名和密码')
    return
  }

  try {
    await loginWithPassword(model.value.username.trim(), model.value.password)
    message.success('登录成功')
    await router.replace(redirectTarget.value)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '登录失败')
  }
}
</script>

<template>
  <div class="login-wrapper">
    <div class="login-grid"></div>

    <header class="top-nav">
      <div class="brand">
        <div class="brand-logo-small">
          <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="url(#hostdeck-grad-small)" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
            <defs>
              <linearGradient id="hostdeck-grad-small" x1="0%" y1="0%" x2="100%" y2="100%">
                <stop offset="0%" stop-color="#2dd4bf" />
                <stop offset="100%" stop-color="#34d399" />
              </linearGradient>
            </defs>
            <path d="M12 3 L 6 13 H 18 Z" />
            <path d="M 9.5 16 H 4 L 1.5 20 H 22.5 L 20 16 H 14.5" />
          </svg>
        </div>
        <span class="brand-text">HostDeck</span>
      </div>
    </header>

    <main class="login-container">
      <div class="login-card">
        <div class="login-header">
          <div class="brand-logo-large">
            <svg xmlns="http://www.w3.org/2000/svg" width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="url(#hostdeck-grad-large)" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <defs>
                <linearGradient id="hostdeck-grad-large" x1="0%" y1="0%" x2="100%" y2="100%">
                  <stop offset="0%" stop-color="#2dd4bf" />
                  <stop offset="100%" stop-color="#34d399" />
                </linearGradient>
              </defs>
              <path d="M12 3 L 6 13 H 18 Z" />
              <path d="M 9.5 16 H 4 L 1.5 20 H 22.5 L 20 16 H 14.5" />
            </svg>
          </div>
          <h2>欢迎登录 HostDeck</h2>
          <p>轻量级 Linux 运维管理平台</p>
        </div>

        <n-alert
          v-if="!state.systemInitialized"
          type="warning"
          title="系统尚未初始化"
          class="status-alert"
        >
          <template v-if="state.bootstrapEnabled">
            当前还没有管理员账号，请使用初始化令牌调用 `/api/auth/bootstrap-admin` 完成首次管理员创建。
          </template>
          <template v-else>
            当前还没有管理员账号，请在服务端配置 bootstrap 管理员信息或启用 bootstrap token 后完成初始化。
          </template>
        </n-alert>

        <n-form :model="model" size="large" class="login-form">
          <n-form-item path="username" label="用户名">
            <n-input
              v-model:value="model.username"
              placeholder=""
              class="dark-input"
              :disabled="!state.systemInitialized"
              @keyup.enter="handleLogin"
            />
          </n-form-item>

          <n-form-item path="password" label="密码">
            <n-input
              v-model:value="model.password"
              type="password"
              show-password-on="click"
              placeholder=""
              class="dark-input"
              :disabled="!state.systemInitialized"
              @keyup.enter="handleLogin"
            />
          </n-form-item>

          <div class="form-actions">
            <n-checkbox v-model:checked="model.remember" class="custom-checkbox" :disabled="!state.systemInitialized">保持登录状态</n-checkbox>
          </div>

          <n-button
            block
            size="large"
            class="login-btn solid-green-btn"
            :loading="state.authenticating"
            :disabled="!state.systemInitialized"
            @click="handleLogin"
          >
            进入系统
          </n-button>
        </n-form>
      </div>
    </main>
  </div>
</template>

<style scoped>
.login-wrapper {
  min-height: 100vh;
  position: relative;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: #0b0e14;
  font-family: 'Inter', 'PingFang SC', sans-serif;
  color: #fff;
}

.login-grid {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgba(255, 255, 255, 0.02) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 255, 255, 0.02) 1px, transparent 1px);
  background-size: 60px 60px;
  z-index: 0;
  pointer-events: none;
}

.top-nav {
  position: absolute;
  top: 0;
  left: 0;
  padding: 32px 40px;
  z-index: 10;
}

.brand {
  display: flex;
  align-items: center;
  gap: 10px;
}

.brand-logo-small {
  display: flex;
  align-items: center;
  justify-content: center;
  filter: drop-shadow(0 0 8px rgba(45, 212, 191, 0.6));
}

.brand-text {
  font-size: 20px;
  font-weight: 700;
  letter-spacing: -0.5px;
  color: #fff;
  text-shadow: 0 0 10px rgba(255, 255, 255, 0.2);
}

.login-container {
  position: relative;
  z-index: 10;
  width: 100%;
  max-width: 420px;
  padding: 0 20px;
}

.login-card {
  background-color: #12151c;
  border: 1px solid rgba(255, 255, 255, 0.05);
  border-radius: 24px;
  padding: 48px 40px;
  box-shadow: 0 24px 48px rgba(0, 0, 0, 0.4);
}

.login-header {
  text-align: center;
  margin-bottom: 36px;
}

.brand-logo-large {
  margin: 0 auto 20px;
  display: flex;
  align-items: center;
  justify-content: center;
  filter: drop-shadow(0 0 24px rgba(45, 212, 191, 0.8));
}

.login-header h2 {
  margin: 0 0 12px;
  color: #fff;
  font-size: 24px;
  font-weight: 700;
  letter-spacing: -0.5px;
}

.login-header p {
  margin: 0;
  color: #71717a;
  font-size: 14px;
}

.status-alert {
  margin-bottom: 24px;
}

:deep(.n-form-item-label) {
  color: #ededed !important;
  font-weight: 600;
  font-size: 14px;
  padding-bottom: 8px !important;
}

:deep(.dark-input) {
  --n-color: #242936 !important;
  --n-color-focus: #2d3342 !important;
  --n-border: 1px solid transparent !important;
  --n-border-hover: 1px solid rgba(16, 185, 129, 0.5) !important;
  --n-border-focus: 1px solid #10b981 !important;
  --n-text-color: #fff !important;
  --n-caret-color: #10b981 !important;
  --n-border-radius: 8px !important;
  --n-height: 48px !important;
}

:deep(.dark-input .n-input__input-el:-webkit-autofill),
:deep(.dark-input .n-input__input-el:-webkit-autofill:hover),
:deep(.dark-input .n-input__input-el:-webkit-autofill:focus),
:deep(.dark-input .n-input__input-el:-webkit-autofill:active) {
  -webkit-transition-delay: 99999s;
  -webkit-transition: color 99999s ease-out, background-color 99999s ease-out;
}

.form-actions {
  display: flex;
  align-items: center;
  margin-bottom: 28px;
  margin-top: -4px;
}

:deep(.custom-checkbox) {
  --n-text-color: #a1a1aa !important;
  --n-font-size: 14px !important;
  --n-color-checked: #10b981 !important;
  --n-border-checked: 1px solid #10b981 !important;
}

.solid-green-btn {
  height: 48px;
  border-radius: 8px;
  font-size: 16px;
  font-weight: 600;
  background-color: #10b981 !important;
  border: none !important;
  color: #fff !important;
  transition: all 0.2s ease;
}

.solid-green-btn:hover {
  background-color: #0ea5e9 !important;
  transform: translateY(-1px);
}
</style>
