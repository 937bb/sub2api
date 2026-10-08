<template>
  <BaseDialog :show="show" :title="t(account ? 'siwc.reauthTitle' : 'siwc.title')" @close="close">
    <form class="space-y-4" @submit.prevent="finish">
      <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('siwc.description') }}</p>
      <label v-if="!account" class="block">{{ t('siwc.name') }}
        <input v-model="name" class="input mt-1" autocomplete="off" :disabled="busy" />
      </label>
      <label v-if="!account" class="block">{{ t('siwc.proxy') }}
        <select v-model="proxyId" class="input mt-1" :disabled="busy || !!authorization">
          <option :value="null">{{ t('siwc.direct') }}</option>
          <option v-for="proxy in proxies" :key="proxy.id" :value="proxy.id">{{ proxy.name }}</option>
        </select>
      </label>
      <label v-if="!account" class="block">{{ t('siwc.groups') }}
        <select v-model="groupIds" multiple class="input mt-1" :disabled="busy">
          <option v-for="group in groups.filter(g => g.platform === 'openai')" :key="group.id" :value="group.id">{{ group.name }}</option>
        </select>
      </label>
      <p class="text-xs text-gray-500">{{ t(account ? 'siwc.reauthHelp' : 'siwc.unbound') }}</p>
      <label v-if="!account" class="block">{{ t('siwc.concurrency') }}
        <input v-model.number="concurrency" type="number" min="1" max="100" required class="input mt-1" :disabled="busy" />
      </label>
      <button v-if="!authorization" type="button" class="btn btn-primary" :disabled="busy" @click="start">{{ t('siwc.start') }}</button>
      <template v-else>
        <a :href="authorization.auth_url" target="_blank" rel="noopener noreferrer" referrerpolicy="no-referrer" class="btn btn-secondary">{{ t('siwc.open') }}</a>
        <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('siwc.callbackHelp') }}</p>
        <label class="block">{{ t('siwc.callback') }}
          <textarea v-model="callback" class="input mt-1 font-mono text-xs" rows="3" autocomplete="off" :spellcheck="false" :disabled="busy" />
        </label>
        <button type="submit" class="btn btn-primary" :disabled="busy || !callback.trim()">{{ t('siwc.save') }}</button>
        <button type="button" class="btn btn-secondary ml-2" :disabled="busy" @click="restart">{{ t('siwc.restart') }}</button>
      </template>
      <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
    </form>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { startSIWCAuthorization, createSIWCAccount, type SIWCAuthorization } from '@/api/admin/siwc'

const props = defineProps<{
  show: boolean
  account?: { id: number } | null
  proxies: { id: number; name: string }[]
  groups: { id: number; name: string; platform: string }[]
  initialValues?: { name: string; proxy_id: number | null; group_ids: number[]; concurrency: number }
}>()
const emit = defineEmits<{ close: []; created: [] }>()
const { t } = useI18n()
const name = ref('')
const proxyId = ref<number | null>(null)
const groupIds = ref<number[]>([])
const concurrency = ref(1)
const authorization = ref<SIWCAuthorization>()
const callback = ref('')
const error = ref('')
const busy = ref(false)

function restart() { authorization.value = undefined; callback.value = ''; error.value = '' }
function close() { if (!busy.value) { restart(); emit('close') } }
watch(() => props.show, () => {
  restart()
  const initial = props.account ? undefined : props.initialValues
  name.value = initial?.name ?? ''
  proxyId.value = initial?.proxy_id ?? null
  groupIds.value = initial?.group_ids.filter(id => props.groups.some(group => group.id === id && group.platform === 'openai')) ?? []
  concurrency.value = initial?.concurrency ?? 1
}, { immediate: true })
function failure(cause: unknown) {
  const data = (cause as { response?: { data?: { message?: string } } })?.response?.data
  error.value = data?.message || t('siwc.failed')
}
async function start() {
  busy.value = true; error.value = ''
  try {
    let hostId: string | undefined
    try { hostId = localStorage.getItem('sub2api-siwc-host-id') || undefined } catch { /* Storage is optional. */ }
    authorization.value = await startSIWCAuthorization(proxyId.value, props.account ? undefined : hostId, props.account?.id)
    if (!props.account) {
      try { localStorage.setItem('sub2api-siwc-host-id', authorization.value.host_id) } catch { /* Credentials remain server-side. */ }
    }
  } catch (cause) { failure(cause) } finally { busy.value = false }
}
async function finish() {
  if (!authorization.value || busy.value) return
  busy.value = true; error.value = ''
  try {
    await createSIWCAccount({ session_id: authorization.value.session_id, callback_url: callback.value.trim(), name: name.value, concurrency: concurrency.value, group_ids: groupIds.value, account_id: props.account?.id })
    restart(); emit('created'); emit('close')
  } catch (cause) { failure(cause) } finally { busy.value = false }
}
</script>
