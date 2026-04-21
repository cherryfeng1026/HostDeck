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
  trustedHostKeyFingerprint: '',
  collectorMode: 'ssh_only',
  tags: [],
  purpose: '',
  remark: '',
  maintenanceStartAt: undefined,
  maintenanceEndAt: undefined,
  enabled: true,
})

const maintenanceRange = ref<[number, number] | null>(null)

const editing = computed(() => Boolean(props.server?.id))
const title = computed(() => (editing.value ? '编辑服务器' : '新增服务器'))
const submitText = computed(() => (editing.value ? '保存修改' : '创建服务器'))
const passwordHint = computed(() => {
  if (!editing.value) {
    return '首次创建服务器时必须填写 SSH 密码，密码会由后端加密保存。'
  }
  if (props.server?.passwordConfigured) {
    return '当前服务器已配置 SSH 密码，留空表示保持原密码不变。'
  }
  if (form.enabled === false) {
    return '当前服务器尚未配置 SSH 密码；如果这次仅用于停用服务器，可直接保存。'
  }
  return '当前服务器尚未配置 SSH 密码，本次保存时请补充密码。'
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
    if (!show) {
      return
    }
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
  form.authType = 'password'
  form.password = ''
  form.trustedHostKeyFingerprint = server?.trustedHostKeyFingerprint ?? ''
  form.collectorMode = server?.collectorMode ?? 'ssh_only'
  form.tags = server?.tags ? [...server.tags] : []
  form.purpose = server?.purpose ?? ''
  form.remark = server?.remark ?? ''
  form.maintenanceStartAt = server?.maintenanceStartAt
  form.maintenanceEndAt = server?.maintenanceEndAt
  maintenanceRange.value = server?.maintenanceStartAt && server?.maintenanceEndAt
    ? [Date.parse(server.maintenanceStartAt), Date.parse(server.maintenanceEndAt)]
    : null
  form.enabled = server?.enabled ?? true
}

function closeModal() {
  emit('update:show', false)
}

function handleMaintenanceRange(value: [number, number] | null) {
  maintenanceRange.value = value
  form.maintenanceStartAt = value ? new Date(value[0]).toISOString() : undefined
  form.maintenanceEndAt = value ? new Date(value[1]).toISOString() : undefined
}

async function handleSubmit() {
  if (saving.value) {
    return
  }

  if ((form.maintenanceStartAt && !form.maintenanceEndAt) || (!form.maintenanceStartAt && form.maintenanceEndAt)) {
    message.warning('维护窗口必须同时提供开始和结束时间')
    return
  }

  const password = form.password?.trim() ?? ''
  if (!editing.value && password === '') {
    message.warning('新增服务器时必须填写 SSH 密码')
    return
  }
  if (editing.value && props.server && !props.server.passwordConfigured && form.enabled !== false && password === '') {
    message.warning('当前服务器尚未配置 SSH 密码，如需保持启用请先补充密码')
    return
  }

  saving.value = true
  try {
    const payload: ServerPayload = {
      ...form,
      authType: 'password',
      password: password || undefined,
      trustedHostKeyFingerprint: form.trustedHostKeyFingerprint?.trim() || undefined,
      tags: [...form.tags],
      purpose: form.purpose?.trim() || undefined,
      remark: form.remark?.trim() || undefined,
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
    style="width: min(720px, calc(100vw - 32px))"
    @update:show="emit('update:show', $event)"
  >
    <div class="modal-copy">
      <strong>SSH 连接说明</strong>
      <n-text depth="3">{{ passwordHint }}</n-text>
    </div>

    <n-form label-placement="top">
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

      <div class="form-grid form-grid--two">
        <n-form-item label="SSH 密码">
          <n-input
            v-model:value="form.password"
            type="password"
            show-password-on="click"
            :placeholder="editing ? '留空表示不修改当前密码' : '请输入 SSH 密码'"
          />
        </n-form-item>
        <n-form-item label="采集模式">
          <n-select v-model:value="form.collectorMode" :options="collectorModeOptions" />
        </n-form-item>
      </div>

      <n-form-item label="已信任主机指纹">
        <n-input v-model:value="form.trustedHostKeyFingerprint" placeholder="首次探测并确认后会自动保存，也可手动粘贴" />
      </n-form-item>

      <div class="form-grid form-grid--two">
        <n-form-item label="标签">
          <n-input v-model:value="tagsText" placeholder="例如：prod, web, cn-hz" />
        </n-form-item>
        <n-form-item label="用途">
          <n-input v-model:value="form.purpose" placeholder="例如：Nginx 网关" />
        </n-form-item>
      </div>

      <n-form-item label="备注">
        <n-input
          v-model:value="form.remark"
          type="textarea"
          :autosize="{ minRows: 3, maxRows: 6 }"
          placeholder="补充这台服务器的职责、网络限制或维护说明"
        />
      </n-form-item>

      <n-form-item label="维护窗口">
        <n-date-picker
          v-model:value="maintenanceRange"
          type="datetimerange"
          clearable
          style="width: 100%"
          start-placeholder="维护开始时间"
          end-placeholder="维护结束时间"
          :actions="['clear', 'confirm']"
          :update-value-on-close="true"
          @update:value="handleMaintenanceRange"
        />
      </n-form-item>

      <n-form-item label="是否启用">
        <n-switch v-model:value="form.enabled" />
      </n-form-item>
    </n-form>

    <template #action>
      <div class="modal-actions">
        <n-button @click="closeModal">取消</n-button>
        <n-button type="primary" :loading="saving" :disabled="saving" @click="handleSubmit">
          {{ submitText }}
        </n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.modal-copy {
  display: grid;
  gap: 6px;
  margin-bottom: 18px;
  padding: 14px 16px;
  border-radius: var(--app-radius-md);
  background: rgba(148, 163, 184, 0.04);
  border: 1px solid var(--app-border);
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
  gap: 12px;
}

:deep(.server-modal .n-card) {
  background: var(--app-surface);
  border: 1px solid var(--app-border);
  border-radius: 20px;
}

:deep(.server-modal .n-card-header) {
  border-bottom: 1px solid var(--app-border);
  margin-bottom: 12px;
}

:deep(.server-modal .n-card-header__main) {
  color: #f8fafc;
}

:deep(.server-modal .n-form-item-label__text) {
  color: #cbd5e1;
}

:deep(.server-modal .n-input),
:deep(.server-modal .n-input-number),
:deep(.server-modal .n-base-selection) {
  --n-color: rgba(10, 16, 28, 0.76) !important;
  --n-color-focus: rgba(15, 23, 42, 0.96) !important;
}

@media (max-width: 720px) {
  .form-grid--two,
  .form-grid--three {
    grid-template-columns: 1fr;
  }
}
</style>
