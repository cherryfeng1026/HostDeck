<script setup lang="ts">
import {
  NButton,
  NDatePicker,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NModal,
  NSelect,
  NSwitch,
  NText,
  useMessage,
} from 'naive-ui'
import { computed, reactive, ref, watch } from 'vue'
import { createServer, updateServer } from '../services/api'
import type { ServerAsset, ServerPayload } from '../types'

const props = defineProps<{
  show: boolean
  server?: ServerAsset | null
}>()

const emit = defineEmits<{
  (event: 'update:show', value: boolean): void
  (event: 'submitted'): void
}>()

const collectorModeOptions = [
  { label: '仅使用 SSH', value: 'ssh_only' },
]

const authTypeOptions = [
  { label: 'SSH 密码连接', value: 'password' },
  { label: 'SSH 密钥连接', value: 'private_key' },
]

const message = useMessage()
const saving = ref(false)

const form = reactive<ServerPayload>({
  name: '',
  hostname: '',
  ip: '',
  sshPort: 22,
  username: 'root',
  authType: 'password',
  password: '',
  privateKey: '',
  trustedHostKeyFingerprint: '',
  collectorMode: 'ssh_only',
  tags: [],
  purpose: '',
  remark: '',
  expiresAt: undefined,
  maintenanceStartAt: undefined,
  maintenanceEndAt: undefined,
  enabled: true,
})

const expiresAtValue = ref<number | null>(null)

const editing = computed(() => Boolean(props.server?.id))
const title = computed(() => (editing.value ? '编辑服务器' : '新增服务器'))
const submitText = computed(() => (editing.value ? '保存修改' : '创建服务器'))
const credentialHint = computed(() => {
  if (form.authType === 'private_key') {
    if (!editing.value) {
      return '新增服务器需要填写 SSH 私钥。'
    }
    if (props.server?.privateKeyConfigured) {
      return '留空表示保持当前 SSH 私钥不变。'
    }
    return '当前服务器未配置 SSH 私钥。'
  }
  if (!editing.value) {
    return '新增服务器需要填写 SSH 密码。'
  }
  if (props.server?.passwordConfigured) {
    return '留空表示保持当前 SSH 密码不变。'
  }
  if (form.enabled === false) {
    return '服务器未配置 SSH 密码；仅停用时可直接保存。'
  }
  return '服务器未配置 SSH 密码，保持启用时需要补充密码。'
})

const tagsText = computed({
  get: () => form.tags.join(', '),
  set: (value: string) => {
    form.tags = value
      .split(',')
      .map((tag) => tag.trim())
      .filter(Boolean)
  },
})

watch(
  () => [props.show, props.server] as const,
  ([show, server]) => {
    if (!show) return
    syncForm(server ?? null)
  },
  { immediate: true },
)

function syncForm(server: ServerAsset | null) {
  form.name = server?.name ?? ''
  form.hostname = server?.hostname ?? ''
  form.ip = server?.ip ?? ''
  form.sshPort = server?.sshPort ?? 22
  form.username = server?.username ?? 'root'
  form.authType = server?.authType === 'private_key' ? 'private_key' : 'password'
  form.password = ''
  form.privateKey = ''
  form.trustedHostKeyFingerprint = server?.trustedHostKeyFingerprint ?? ''
  form.collectorMode = server?.collectorMode ?? 'ssh_only'
  form.tags = server?.tags ? [...server.tags] : []
  form.purpose = server?.purpose ?? ''
  form.remark = server?.remark ?? ''
  form.expiresAt = server?.expiresAt
  form.maintenanceStartAt = server?.maintenanceStartAt
  form.maintenanceEndAt = server?.maintenanceEndAt
  expiresAtValue.value = server?.expiresAt ? Date.parse(server.expiresAt) : null
  form.enabled = server?.enabled ?? true
}

function closeModal() {
  emit('update:show', false)
}

function handleExpiresAt(value: number | null) {
  expiresAtValue.value = value
  form.expiresAt = value ? new Date(value).toISOString() : undefined
}

async function handleSubmit() {
  if (saving.value) return

  const password = form.password?.trim() ?? ''
  const privateKey = form.privateKey?.trim() ?? ''
  if (form.authType === 'password' && !editing.value && password === '') {
    message.warning('新增服务器时必须填写 SSH 密码')
    return
  }
  if (form.authType === 'password' && editing.value && props.server && !props.server.passwordConfigured && form.enabled !== false && password === '') {
    message.warning('当前服务器尚未配置 SSH 密码，如需保持启用请先补充密码')
    return
  }
  if (form.authType === 'private_key' && !editing.value && privateKey === '') {
    message.warning('新增服务器时必须填写 SSH 私钥')
    return
  }
  if (form.authType === 'private_key' && editing.value && props.server && !props.server.privateKeyConfigured && form.enabled !== false && privateKey === '') {
    message.warning('当前服务器尚未配置 SSH 私钥，如需保持启用请先补充私钥')
    return
  }

  saving.value = true
  try {
    const payload: ServerPayload = {
      ...form,
      authType: form.authType,
      password: password || undefined,
      privateKey: privateKey || undefined,
      trustedHostKeyFingerprint: form.trustedHostKeyFingerprint?.trim() || undefined,
      tags: [...form.tags],
      purpose: form.purpose?.trim() || undefined,
      remark: form.remark?.trim() || undefined,
      expiresAt: form.expiresAt || undefined,
      maintenanceStartAt: form.maintenanceStartAt || undefined,
      maintenanceEndAt: form.maintenanceEndAt || undefined,
    }

    if (props.server?.id) {
      await updateServer(props.server.id, payload)
      message.success('服务器信息已更新')
    } else {
      await createServer(payload)
      message.success('服务器已创建')
    }

    emit('submitted')
    closeModal()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '保存服务器失败')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    :title="title"
    class="server-modal"
    style="width: min(420px, 100vw); height: 100vh; max-height: 100vh"
    @update:show="emit('update:show', $event)"
  >
    <n-form label-placement="top">
      <section class="form-section">
        <div class="section-head">
          <strong>基础信息</strong>
          <n-switch v-model:value="form.enabled" />
        </div>
        <div class="form-grid form-grid--two">
          <n-form-item label="服务器名称">
            <n-input v-model:value="form.name" placeholder="例如：prod-web-01" />
          </n-form-item>
          <n-form-item label="主机名">
            <n-input v-model:value="form.hostname" placeholder="例如：prod-web-01" />
          </n-form-item>
        </div>
        <div class="form-grid form-grid--three">
          <n-form-item label="IP 地址">
            <n-input v-model:value="form.ip" placeholder="例如：10.0.0.21" />
          </n-form-item>
          <n-form-item label="SSH 端口">
            <n-input-number v-model:value="form.sshPort" :min="1" :max="65535" style="width: 100%" />
          </n-form-item>
          <n-form-item label="登录用户">
            <n-input v-model:value="form.username" placeholder="例如：root" />
          </n-form-item>
        </div>
      </section>

      <section class="form-section">
        <div class="section-head">
          <strong>SSH 连接</strong>
          <n-text depth="3">{{ credentialHint }}</n-text>
        </div>
        <div class="form-grid form-grid--two">
          <n-form-item label="连接方式">
            <n-select v-model:value="form.authType" :options="authTypeOptions" />
          </n-form-item>
          <n-form-item label="采集模式">
            <n-select v-model:value="form.collectorMode" :options="collectorModeOptions" />
          </n-form-item>
        </div>
        <div class="form-grid">
          <n-form-item label="SSH 密码">
            <n-input
              v-if="form.authType === 'password'"
              v-model:value="form.password"
              type="password"
              show-password-on="click"
              :placeholder="editing ? '留空表示不修改当前密码' : '请输入 SSH 密码'"
            />
            <n-input
              v-else
              v-model:value="form.privateKey"
              type="textarea"
              :autosize="{ minRows: 5, maxRows: 8 }"
              :placeholder="editing ? '留空表示不修改当前私钥' : '粘贴 OpenSSH 私钥'"
            />
          </n-form-item>
        </div>
        <n-form-item label="已信任主机指纹">
          <n-input v-model:value="form.trustedHostKeyFingerprint" placeholder="首次探测并确认后会自动保存，也可手动粘贴" />
        </n-form-item>
      </section>

      <section class="form-section">
        <div class="section-head">
          <strong>归属与过期</strong>
        </div>
        <div class="form-grid form-grid--two">
          <n-form-item label="标签">
            <n-input v-model:value="tagsText" placeholder="例如：prod, web, cn-hz" />
          </n-form-item>
          <n-form-item label="用途">
            <n-input v-model:value="form.purpose" placeholder="例如：Nginx 网关" />
          </n-form-item>
        </div>
        <n-form-item label="过期时间">
          <n-date-picker
            v-model:value="expiresAtValue"
            type="datetime"
            clearable
            :is-date-disabled="(ts: number) => ts < Date.now()"
            style="width: 100%"
            placeholder="选择过期时间"
            :actions="['clear', 'confirm']"
            :update-value-on-close="true"
            @update:value="handleExpiresAt"
          />
        </n-form-item>
        <n-form-item label="备注">
          <n-input
            v-model:value="form.remark"
            type="textarea"
            :autosize="{ minRows: 3, maxRows: 5 }"
            placeholder="补充职责、网络限制或备注说明"
          />
        </n-form-item>
      </section>
    </n-form>

    <template #action>
      <div class="modal-actions">
        <n-button ghost @click="closeModal">取消</n-button>
        <n-button type="primary" :loading="saving" :disabled="saving" @click="handleSubmit">
          {{ submitText }}
        </n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.form-section {
  padding: 16px 16px 14px;
  border: 1px solid rgba(93, 120, 162, 0.18);
  border-radius: 8px;
  background: rgba(17, 32, 52, 0.42);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.035);
}

.form-section + .form-section {
  margin-top: 14px;
}

.section-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  margin-bottom: 16px;
}

.section-head strong {
  color: var(--app-text);
  font-size: 15px;
}

.section-head :deep(.n-text) {
  max-width: 460px;
  text-align: right;
  font-size: 12px;
  color: var(--app-text-soft) !important;
}

.form-grid {
  display: grid;
  gap: 14px;
}

.form-grid--two {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.form-grid--three {
  grid-template-columns: minmax(0, 1.3fr) 140px minmax(0, 1fr);
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

:deep(.server-modal.n-card),
:deep(.server-modal .n-card) {
  display: flex;
  flex-direction: column;
  overflow: hidden !important;
  background: linear-gradient(180deg, rgba(14, 28, 48, 0.98), rgba(8, 18, 32, 0.98));
  border: 1px solid rgba(93, 120, 162, 0.24);
  border-radius: 0;
}

:deep(.server-modal.n-card > .n-card-header),
:deep(.server-modal .n-card-header),
:deep(.server-modal.n-card > .n-card__action),
:deep(.server-modal .n-card__action) {
  flex-shrink: 0;
}

:deep(.server-modal.n-card > .n-card-content),
:deep(.server-modal .n-card-content),
:deep(.server-modal.n-card > .n-card__content),
:deep(.server-modal .n-card__content) {
  flex: 1 1 auto;
  min-height: 0;
  overflow: auto !important;
}

:deep(.server-modal.n-card > .n-card__action),
:deep(.server-modal .n-card__action) {
  border-top: 1px solid rgba(93, 120, 162, 0.18);
  background: rgba(8, 18, 32, 0.96);
}

:deep(.server-modal .n-card-header) {
  border-bottom: 1px solid rgba(93, 120, 162, 0.18);
  margin-bottom: 12px;
  background: rgba(8, 18, 32, 0.86);
}

:deep(.server-modal .n-card-header__main) {
  color: var(--app-text);
}

:deep(.server-modal .n-form-item-label__text) {
  color: var(--app-text-soft);
}

:deep(.server-modal .n-input),
:deep(.server-modal .n-input-number),
:deep(.server-modal .n-base-selection) {
  --n-color: rgba(12, 24, 42, 0.72) !important;
  --n-color-focus: rgba(16, 31, 53, 0.94) !important;
  --n-border: 1px solid rgba(93, 120, 162, 0.24) !important;
  --n-border-hover: 1px solid rgba(79, 131, 255, 0.42) !important;
  --n-border-focus: 1px solid rgba(79, 131, 255, 0.58) !important;
}

:global(.server-modal.n-card) {
  position: fixed !important;
  top: 0 !important;
  right: 0 !important;
  bottom: 0 !important;
  margin: 0 !important;
  display: flex !important;
  flex-direction: column !important;
  overflow: hidden !important;
  background: linear-gradient(180deg, rgba(14, 28, 48, 0.98), rgba(8, 18, 32, 0.98)) !important;
  border: 0 !important;
  border-left: 1px solid rgba(93, 120, 162, 0.32) !important;
  border-radius: 0 !important;
  box-shadow: -18px 0 44px rgba(0, 8, 22, 0.34) !important;
}

:global(.server-modal.n-card > .n-card-header),
:global(.server-modal.n-card > .n-card__action) {
  flex-shrink: 0 !important;
}

:global(.server-modal.n-card > .n-card-header) {
  border-bottom: 1px solid rgba(93, 120, 162, 0.18);
  margin-bottom: 0;
  background: rgba(8, 18, 32, 0.86);
}

:global(.server-modal.n-card > .n-card-content) {
  flex: 1 1 auto !important;
  min-height: 0 !important;
  overflow: auto !important;
}

:global(.server-modal.n-card > .n-card__action) {
  border-top: 1px solid rgba(93, 120, 162, 0.18);
  background: rgba(8, 18, 32, 0.96);
}

:global(.server-modal.n-card .n-card-header__main) {
  color: var(--app-text);
}

:global(.server-modal.n-card .n-form-item-label__text) {
  color: var(--app-text-soft);
}

:global(.server-modal.n-card .n-input),
:global(.server-modal.n-card .n-input-number),
:global(.server-modal.n-card .n-base-selection) {
  --n-color: rgba(12, 24, 42, 0.72) !important;
  --n-color-focus: rgba(16, 31, 53, 0.94) !important;
  --n-border: 1px solid rgba(93, 120, 162, 0.24) !important;
  --n-border-hover: 1px solid rgba(79, 131, 255, 0.42) !important;
  --n-border-focus: 1px solid rgba(79, 131, 255, 0.58) !important;
}

@media (max-width: 720px) {
  .form-grid--two,
  .form-grid--three {
    grid-template-columns: 1fr;
  }

  .section-head {
    align-items: flex-start;
    flex-direction: column;
  }

  .section-head :deep(.n-text) {
    text-align: left;
  }
}
</style>
