<script setup lang="ts">
import { NAlert, NButton, NCheckbox, NForm, NFormItem, NInput, useMessage } from 'naive-ui'
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { loginWithPassword, useSession } from '../session'

const router = useRouter()
const route = useRoute()
const message = useMessage()
const { state } = useSession()
const model = ref({
  username: '',
  password: '',
  remember: true,
})

const loginInputInlineStyle = [
  'font-family: Inter, "Segoe UI", "PingFang SC", sans-serif',
  'font-size: 16px',
  'font-weight: 500',
  'line-height: 48px',
  'height: 48px',
  'letter-spacing: 0',
  '-webkit-text-size-adjust: 100%',
  'text-size-adjust: 100%',
].join('; ')

const usernameInputProps = {
  autocomplete: 'username',
  autocapitalize: 'none',
  spellcheck: 'false',
  style: loginInputInlineStyle,
}

const passwordInputProps = {
  autocomplete: 'current-password',
  autocapitalize: 'none',
  spellcheck: 'false',
  style: loginInputInlineStyle,
}

const redirectTarget = computed(() => {
  const value = route.query.redirect
  return typeof value === 'string' && value.startsWith('/') ? value : '/'
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
                <stop offset="100%" stop-color="var(--app-accent)" />
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
                  <stop offset="100%" stop-color="var(--app-accent)" />
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
              :input-props="usernameInputProps"
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
              :input-props="passwordInputProps"
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
            class="login-btn solid-blue-btn"
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
  justify-content: flex-end;
  background: #020811;
  font-family: 'Inter', 'PingFang SC', sans-serif;
  color: var(--app-text);
  -webkit-text-size-adjust: 100%;
}

.login-wrapper::before {
  content: '';
  position: absolute;
  inset: 0;
  background-image: var(--app-background-overlay), var(--app-background-image);
  background-size: var(--app-background-size);
  background-position: var(--app-background-position);
  background-repeat: var(--app-background-repeat);
}

.login-wrapper::after {
  content: '';
  position: absolute;
  inset: 0;
  background:
    radial-gradient(circle at 11% 64%, rgba(28, 230, 229, 0.1), transparent 18%),
    radial-gradient(circle at 79% 78%, rgba(28, 230, 229, 0.08), transparent 20%),
    linear-gradient(120deg, transparent 0 42%, rgba(36, 214, 255, 0.055) 48%, transparent 54% 100%);
  opacity: 0.54;
  animation: light-sweep 8s ease-in-out infinite;
  pointer-events: none;
}

.login-grid {
  position: absolute;
  inset: 0;
  background:
    radial-gradient(circle, rgba(30, 230, 230, 0.46) 0 1.5px, transparent 2px),
    linear-gradient(rgba(62, 111, 160, 0.05) 1px, transparent 1px),
    linear-gradient(90deg, rgba(62, 111, 160, 0.05) 1px, transparent 1px);
  background-size: 360px 220px, 58px 58px, 58px 58px;
  background-position: 0 0, center, center;
  opacity: 0.28;
  z-index: 1;
  pointer-events: none;
  animation: particle-drift 14s linear infinite;
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
  filter: drop-shadow(0 0 10px rgba(32, 212, 255, 0.65));
}

.brand-text {
  font-size: 20px;
  font-weight: 700;
  letter-spacing: 0;
  color: var(--app-text);
  text-shadow: 0 0 14px rgba(32, 212, 255, 0.2);
}

.login-container {
  position: relative;
  z-index: 10;
  width: 100%;
  max-width: 430px;
  margin-right: clamp(32px, 9vw, 154px);
  padding: 0 20px;
}

.login-card {
  background: linear-gradient(180deg, rgba(12, 25, 43, 0.72), rgba(7, 17, 31, 0.82));
  border: 1px solid rgba(78, 119, 170, 0.26);
  border-radius: 8px;
  padding: 42px 38px;
  box-shadow: 0 24px 60px rgba(0, 8, 22, 0.54), inset 0 1px 0 rgba(255, 255, 255, 0.07);
  backdrop-filter: saturate(125%);
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
  filter: drop-shadow(0 0 26px rgba(32, 212, 255, 0.72));
}

.login-header h2 {
  margin: 0 0 12px;
  color: var(--app-text);
  font-size: 24px;
  font-weight: 700;
  letter-spacing: 0;
}

.login-header p {
  margin: 0;
  color: var(--app-text-faint);
  font-size: 14px;
}

.status-alert {
  margin-bottom: 24px;
}

:deep(.n-form-item-label) {
  color: var(--app-text) !important;
  font-weight: 600;
  font-size: 14px;
  padding-bottom: 8px !important;
}

:deep(.dark-input) {
  --n-color: rgba(12, 24, 42, 0.9) !important;
  --n-color-focus: rgba(15, 32, 55, 0.96) !important;
  --n-border: 1px solid rgba(83, 112, 153, 0.26) !important;
  --n-border-hover: 1px solid rgba(79, 131, 255, 0.58) !important;
  --n-border-focus: 1px solid var(--app-accent-strong) !important;
  --n-text-color: var(--app-text) !important;
  --n-caret-color: var(--app-accent-strong) !important;
  --n-border-radius: var(--app-radius-sm) !important;
  --n-font-size: 16px !important;
  --n-font-weight: 500 !important;
  --n-height: 48px !important;
  --n-padding-left: 16px !important;
  --n-padding-right: 16px !important;
  font-size: 16px !important;
  font-weight: 500 !important;
  line-height: 48px !important;
  font-family: 'Inter', 'Segoe UI', 'PingFang SC', sans-serif !important;
  -webkit-text-size-adjust: 100%;
  text-size-adjust: 100%;
}

:deep(.login-form .n-input),
:deep(.login-form .n-input--focus),
:deep(.login-form .n-input--active) {
  font-size: 16px !important;
  font-weight: 500 !important;
  line-height: 48px !important;
  font-family: 'Inter', 'Segoe UI', 'PingFang SC', sans-serif !important;
}

:deep(.dark-input .n-input-wrapper) {
  min-height: 48px !important;
  height: 48px !important;
  transition: background-color 0.18s ease, border-color 0.18s ease, box-shadow 0.18s ease !important;
}

:deep(.dark-input .n-input__input),
:deep(.dark-input .n-input__input-el),
:deep(.dark-input .n-input__placeholder) {
  height: 48px !important;
  min-height: 48px !important;
  font-size: 16px !important;
  font-weight: 500 !important;
  line-height: 48px !important;
  font-family: 'Inter', 'Segoe UI', 'PingFang SC', sans-serif !important;
  transform: none !important;
  letter-spacing: 0 !important;
  -webkit-text-size-adjust: 100%;
  text-size-adjust: 100%;
}

:deep(.dark-input .n-input__input-el),
:deep(.dark-input .n-input__input-el:hover),
:deep(.dark-input .n-input__input-el:focus),
:deep(.dark-input .n-input__input-el:active),
:deep(.dark-input .n-input__input-el:disabled) {
  font-size: 16px !important;
  font-weight: 500 !important;
  line-height: 48px !important;
  font-family: 'Inter', 'Segoe UI', 'PingFang SC', sans-serif !important;
  transform: none !important;
}

:deep(.dark-input .n-input__input-el::first-line),
:deep(.dark-input .n-input__input-el:focus::first-line),
:deep(.dark-input .n-input__input-el:-webkit-autofill::first-line),
:deep(.dark-input .n-input__input-el:-webkit-autofill:hover::first-line),
:deep(.dark-input .n-input__input-el:-webkit-autofill:focus::first-line),
:deep(.dark-input .n-input__input-el:-webkit-autofill:active::first-line) {
  font-family: 'Inter', 'Segoe UI', 'PingFang SC', sans-serif !important;
  font-size: 16px !important;
  font-weight: 500 !important;
  line-height: 48px !important;
}

:deep(.dark-input .n-input__input-el:-webkit-autofill),
:deep(.dark-input .n-input__input-el:-webkit-autofill:hover),
:deep(.dark-input .n-input__input-el:-webkit-autofill:focus),
:deep(.dark-input .n-input__input-el:-webkit-autofill:active) {
  -webkit-transition-delay: 99999s;
  -webkit-transition: color 99999s ease-out, background-color 99999s ease-out;
  -webkit-text-fill-color: var(--app-text) !important;
  caret-color: var(--app-accent-strong) !important;
  font-family: 'Inter', 'Segoe UI', 'PingFang SC', sans-serif !important;
  font-size: 16px !important;
  font-weight: 500 !important;
  line-height: 48px !important;
}

.form-actions {
  display: flex;
  align-items: center;
  margin-bottom: 28px;
  margin-top: -4px;
}

:deep(.custom-checkbox) {
  --n-text-color: var(--app-text-soft) !important;
  --n-font-size: 14px !important;
  --n-color-checked: var(--app-accent-strong) !important;
  --n-border-checked: 1px solid var(--app-accent-strong) !important;
}

.solid-blue-btn {
  height: 48px;
  border-radius: var(--app-radius-sm);
  font-size: 16px;
  font-weight: 600;
  background-color: var(--app-accent-strong) !important;
  border: none !important;
  color: var(--app-text) !important;
  transition: all 0.2s ease;
}

.solid-blue-btn:hover {
  background-color: #356fe8 !important;
  transform: translateY(-1px);
}

@keyframes particle-drift {
  from {
    background-position: 0 0, center, center;
  }
  to {
    background-position: 360px 220px, calc(50% + 18px) calc(50% + 12px), calc(50% + 18px) calc(50% + 12px);
  }
}

@keyframes light-sweep {
  0%,
  100% {
    opacity: 0.48;
  }
  50% {
    opacity: 0.78;
  }
}

@media (prefers-reduced-motion: reduce) {
  .login-wrapper::after,
  .login-grid {
    animation: none;
  }
}

@media (max-width: 900px) {
  .login-wrapper {
    justify-content: center;
  }

  .login-container {
    margin-right: 0;
    margin-top: 72px;
  }
}

@media (max-width: 560px) {
  .top-nav {
    padding: 22px 24px;
  }

  .login-card {
    padding: 34px 24px;
  }
}
</style>
