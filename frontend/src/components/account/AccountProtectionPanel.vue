<template>
  <section v-if="eligibleAccount" class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div class="min-w-0">
        <div class="flex items-center gap-2">
          <Icon name="shield" size="sm" class="text-primary-600 dark:text-primary-400" />
          <label class="input-label mb-0">Codex 账号保护</label>
          <span :class="statusClass" class="rounded px-2 py-0.5 text-xs font-medium">{{ statusLabel }}</span>
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          显式启用账号级身份、传输模板与请求语义检查；不会改变账号代理池或降低现有并发。
        </p>
      </div>
      <button v-if="enabled" type="button" class="btn btn-secondary btn-sm" :disabled="busy" @click="confirmRevert = true">
        <Icon name="refresh" size="xs" />
        还原
      </button>
    </div>

    <div class="mt-3 grid gap-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-end">
      <div>
        <label class="input-label">保护策略</label>
        <select v-model="selectedMode" class="input" :disabled="busy">
          <option v-for="strategy in visibleStrategies" :key="strategy.id" :value="strategy.id">
            {{ strategy.name }}{{ strategy.diagnostic_only ? '（诊断）' : '' }}
          </option>
        </select>
      </div>
      <button type="button" class="btn btn-primary" :disabled="busy || !preview?.eligible" @click="applySelected">
        <Icon :name="busy ? 'refresh' : 'check'" size="sm" />
        {{ enabled ? '切换策略' : '应用保护' }}
      </button>
    </div>

    <label class="mt-2 flex items-center gap-2 text-xs text-gray-600 dark:text-gray-300">
      <input v-model="showDiagnostics" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600" />
      显示诊断策略
    </label>

    <div v-if="preview" class="mt-3 space-y-1 text-xs text-gray-600 dark:text-gray-300">
      <p>{{ preview.reason || selectedStrategy?.description }}</p>
      <p v-if="preview.runtime">
        身份 {{ preview.runtime.identity_mode }} · TLS {{ preview.runtime.effective_tls }} ·
        完整性 {{ preview.runtime.integrity_mode || 'off' }} · 代理 {{ preview.runtime.proxy_mode }}
      </p>
      <p v-if="preview.issues?.length" class="text-red-600 dark:text-red-400">{{ preview.issues.join('；') }}</p>
    </div>

    <ConfirmDialog
      :show="confirmRevert"
      title="还原账号保护？"
      message="将恢复策略启用前的身份与 TLS 配置；当前并发和 Codex 代理池保持不变。"
      confirm-text="确认还原"
      cancel-text="取消"
      danger
      @confirm="revertProtection"
      @cancel="confirmRevert = false"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import type { Account } from '@/types'
import {
  applyAntiDegrade,
  listAntiDegradeStrategies,
  previewAntiDegrade,
  revertAntiDegrade,
  type AntiDegradeMode,
  type AntiDegradePreview,
  type AntiDegradeStrategyProfile
} from '@/api/admin/accounts'
import { DEFAULT_ANTI_DEGRADE_MODE } from '@/utils/accountProtection'
import { useAppStore } from '@/stores/app'
import Icon from '@/components/icons/Icon.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'

const props = defineProps<{ account: Account }>()
const emit = defineEmits<{ updated: [account: Account] }>()
const appStore = useAppStore()
const current = ref<Account>(props.account)
const strategies = ref<AntiDegradeStrategyProfile[]>([])
const selectedMode = ref<AntiDegradeMode>(DEFAULT_ANTI_DEGRADE_MODE)
const preview = ref<AntiDegradePreview | null>(null)
const showDiagnostics = ref(false)
const busy = ref(false)
const confirmRevert = ref(false)

const eligibleAccount = computed(() => current.value.platform === 'openai' && ['oauth', 'setup-token'].includes(current.value.type) && !current.value.parent_account_id)
const enabled = computed(() => current.value.anti_degradation === true || (current.value.extra?.anti_degrade as { enabled?: boolean } | undefined)?.enabled === true)
const activeMode = computed(() => current.value.protection_mode || (current.value.extra?.anti_degrade as { mode?: string } | undefined)?.mode || '')
const visibleStrategies = computed(() => strategies.value.filter(strategy => strategy.apply_supported && (showDiagnostics.value || !strategy.diagnostic_only)))
const selectedStrategy = computed(() => strategies.value.find(strategy => strategy.id === selectedMode.value))
const statusLabel = computed(() => enabled.value ? `已启用${activeMode.value ? ` · ${activeMode.value}` : ''}` : '未启用')
const statusClass = computed(() => enabled.value
  ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
  : 'bg-gray-100 text-gray-600 dark:bg-dark-600 dark:text-gray-300')

async function loadPreview() {
  if (!eligibleAccount.value) return
  try {
    preview.value = await previewAntiDegrade(current.value.id, selectedMode.value)
  } catch (error: any) {
    preview.value = null
    appStore.showError(error?.message || '读取账号保护状态失败')
  }
}

async function applySelected() {
  if (busy.value || !preview.value?.eligible) return
  busy.value = true
  try {
    current.value = await applyAntiDegrade(current.value.id, selectedMode.value)
    emit('updated', current.value)
    appStore.showSuccess('账号保护已应用')
    await loadPreview()
  } catch (error: any) {
    appStore.showError(error?.message || '应用账号保护失败')
  } finally {
    busy.value = false
  }
}

async function revertProtection() {
  if (busy.value) return
  confirmRevert.value = false
  busy.value = true
  try {
    current.value = await revertAntiDegrade(current.value.id)
    emit('updated', current.value)
    appStore.showSuccess('账号保护已还原')
    await loadPreview()
  } catch (error: any) {
    appStore.showError(error?.message || '还原账号保护失败')
  } finally {
    busy.value = false
  }
}

watch(() => props.account, value => { current.value = value }, { deep: true })
watch(selectedMode, () => { void loadPreview() })
onMounted(async () => {
  try {
    strategies.value = await listAntiDegradeStrategies()
    if (enabled.value && strategies.value.some(strategy => strategy.id === activeMode.value)) {
      selectedMode.value = activeMode.value as AntiDegradeMode
    }
    await loadPreview()
  } catch (error: any) {
    appStore.showError(error?.message || '读取账号保护策略失败')
  }
})
</script>
