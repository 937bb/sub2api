<template>
  <section ref="panel" class="w-full min-w-0 rounded-lg border border-primary-200 bg-white p-3 text-left dark:border-primary-900 dark:bg-dark-900 sm:p-4" data-test="state-rules-panel" aria-labelledby="state-rules-title">
    <h2 id="state-rules-title" class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.turnState.rulesPanel.title') }}</h2>
    <p class="mt-1 text-xs leading-relaxed text-gray-500 dark:text-dark-400">{{ t('admin.ops.turnState.rulesPanel.hint') }}</p>
    <div v-if="loadError" role="alert" class="mt-3 flex flex-wrap items-center gap-3 text-xs text-red-600 dark:text-red-400">
      {{ t('admin.ops.turnState.scanSettings.loadFailed') }}
      <button type="button" class="btn btn-secondary" :disabled="loading" @click="load">{{ t('common.refresh') }}</button>
    </div>
    <form class="mt-3 space-y-3" @submit.prevent="save">
      <fieldset :disabled="loading || saving || !loaded" class="max-h-80 min-w-0 space-y-2 overflow-y-auto pr-1 disabled:opacity-60">
        <CodexStateRoutingGuard v-model="requireStateBeforeRouting" v-model:route-binding-required="requireRouteBinding" :plan-scan-enabled="planScanEnabled" class="max-w-4xl" />
        <CodexPlanScanSwitches v-model="planScanEnabled" class="max-w-4xl" test-prefix="inline-plan-scan-" />
        <p v-if="selectedRule" class="max-w-4xl break-words text-xs leading-relaxed text-primary-700 dark:text-primary-300" data-test="inline-rule-scope">{{ t('admin.ops.turnState.scanSettings.selectedScope', { plan: selectedRule.plan_type === '*' ? t('common.all') : selectedRule.plan_type.toUpperCase(), model: selectedRule.model }) }}</p>
        <div v-if="!rules.length" class="text-xs text-gray-400">{{ loading ? t('common.loading') : t('admin.ops.turnState.scanSettings.noRules') }}</div>
        <div v-for="(rule, index) in rules" :key="rule.key" :class="['grid min-w-0 max-w-4xl grid-cols-2 items-end gap-2 rounded-md p-2 sm:grid-cols-[148px_minmax(0,1fr)_180px_48px]', selectedRule?.key === rule.key ? 'bg-primary-50 dark:bg-primary-950/30' : 'bg-gray-50 dark:bg-dark-800']" data-test="inline-state-rule">
          <label class="min-w-0 text-[11px] text-gray-600 dark:text-dark-300">
            {{ t('admin.ops.turnState.rulesPanel.plan') }}
            <select v-model="rule.plan_type" class="input mt-1 px-2 py-1.5 text-xs" :aria-label="t('admin.ops.turnState.rulesPanel.plan')" data-test="inline-rule-plan">
              <option v-for="plan in plans" :key="plan" :value="plan">{{ plan === '*' ? t('common.all') : plan.toUpperCase() }}</option>
            </select>
          </label>
          <label class="min-w-0 text-[11px] text-gray-600 dark:text-dark-300">
            {{ t('admin.ops.turnState.columns.model') }}
            <input v-model="rule.model" list="inline-state-models" class="input mt-1 px-2 py-1.5 font-mono text-xs" :aria-label="t('admin.ops.turnState.columns.model')" autocomplete="off" spellcheck="false" placeholder="*" data-test="inline-rule-model" />
          </label>
          <label class="min-w-0 text-[11px] text-gray-600 dark:text-dark-300">
            {{ t('admin.ops.turnState.rulesPanel.values') }}
            <input v-model="rule.lengthsText" class="input mt-1 px-2 py-1.5 font-mono text-xs" :aria-label="t('admin.ops.turnState.rulesPanel.values')" autocomplete="off" placeholder="332, 292" data-test="inline-rule-lengths" />
          </label>
          <button type="button" class="mb-2 text-xs text-red-600 hover:text-red-700 dark:text-red-400" :aria-label="t('admin.ops.turnState.scanSettings.deleteRule', { index: index + 1 })" data-test="inline-delete-rule" @click="removeRule(index)">{{ t('common.delete') }}</button>
        </div>
        <datalist id="inline-state-models"><option v-for="model in modelSuggestions" :key="model" :value="model" /></datalist>
        <label class="flex max-w-4xl flex-wrap items-center gap-x-3 gap-y-1 pt-1 text-xs text-gray-600 dark:text-dark-300">
          {{ t('admin.ops.turnState.rulesPanel.defaultValues') }}
          <input v-model="lengthsText" class="input w-40 px-2 py-1.5 font-mono text-xs" :aria-label="t('admin.ops.turnState.rulesPanel.defaultValues')" autocomplete="off" placeholder="332, 292" data-test="inline-default-lengths" />
        </label>
      </fieldset>
      <p v-if="validationError && loaded" role="alert" class="text-xs text-red-600 dark:text-red-400">{{ validationError }}</p>
      <div class="flex flex-wrap items-center justify-start gap-2">
        <button type="button" class="btn btn-secondary px-3 py-1.5 text-xs" :disabled="loading || saving || !loaded || rules.length >= 128" data-test="inline-add-rule" @click="addRule">{{ t('admin.ops.turnState.scanSettings.addRule') }}</button>
        <button type="submit" class="btn btn-primary px-3 py-1.5 text-xs" :disabled="loading || saving || !loaded || !!validationError" data-test="inline-save-rules">{{ saving ? t('common.saving') : t('admin.ops.turnState.rulesPanel.save') }}</button>
        <span class="text-[11px] text-gray-400">{{ t('admin.ops.turnState.rulesPanel.priorityHint') }}</span>
      </div>
    </form>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { opsAPI, type CodexTurnStateLengthRule, type CodexTurnStateScanSettings } from '@/api/admin/ops'
import { useAppStore } from '@/stores/app'
import CodexPlanScanSwitches from './CodexPlanScanSwitches.vue'
import CodexStateRoutingGuard from './CodexStateRoutingGuard.vue'

interface RuleRow { key: number; plan_type: string; model: string; lengthsText: string }
const emit = defineEmits<{ saved: [settings: CodexTurnStateScanSettings] }>()
const { t } = useI18n()
const appStore = useAppStore()
const panel = ref<HTMLElement | null>(null)
const loading = ref(false)
const saving = ref(false)
const loaded = ref(false)
const loadError = ref(false)
const lengthsText = ref('')
const rules = ref<RuleRow[]>([])
const selectedRuleKey = ref<number | null>(null)
const selectedRule = computed(() => rules.value.find(rule => rule.key === selectedRuleKey.value))
const plans = ['*', 'pro', 'team', 'plus', 'free', 'enterprise']
const switchablePlans = plans.filter(plan => plan !== '*')
const planScanEnabled = ref<Record<string, boolean>>(Object.fromEntries(switchablePlans.map(plan => [plan, true])))
const requireStateBeforeRouting = ref(true)
const requireRouteBinding = ref(false)
let nextKey = 0
const modelSuggestions = computed(() => [...new Set(['*', 'gpt-5.6-terra', 'gpt-6-astra', ...rules.value.map(rule => rule.model.trim().toLowerCase()).filter(Boolean)])])
const parseLengths = (value: string) => value.trim().split(/[\s,，]+/).filter(Boolean).map(Number)
const validLengths = (values: number[]) => values.length >= 1 && values.length <= 16 && values.every(value => Number.isInteger(value) && value >= 64 && value <= 4096) && new Set(values).size === values.length
const targetLengths = computed(() => parseLengths(lengthsText.value))
const normalizedRules = computed<CodexTurnStateLengthRule[]>(() => rules.value.map(rule => ({ plan_type: rule.plan_type, model: rule.model.trim().toLowerCase(), target_lengths: parseLengths(rule.lengthsText) })))
const validationError = computed(() => {
  if (!validLengths(targetLengths.value)) return t('admin.ops.turnState.scanSettings.invalidLengths')
  if (rules.value.length > 128) return t('admin.ops.turnState.scanSettings.tooManyRules')
  const seen = new Set<string>()
  for (const rule of normalizedRules.value) {
    if (!plans.includes(rule.plan_type) || !rule.model || /\s/.test(rule.model) || rule.model.length > 200 || (rule.model.includes('*') && rule.model !== '*')) return t('admin.ops.turnState.scanSettings.invalidRuleModel')
    if (!validLengths(rule.target_lengths)) return t('admin.ops.turnState.scanSettings.invalidRuleLengths')
    const key = `${rule.plan_type}/${rule.model}`
    if (seen.has(key)) return t('admin.ops.turnState.scanSettings.duplicateRule')
    seen.add(key)
  }
  return ''
})

function apply(settings: CodexTurnStateScanSettings) {
  lengthsText.value = settings.target_lengths.join(', ')
  planScanEnabled.value = Object.fromEntries(switchablePlans.map(plan => [plan, settings.plan_scan_enabled?.[plan] ?? true]))
  requireStateBeforeRouting.value = settings.require_state_before_routing ?? true
  requireRouteBinding.value = settings.require_route_binding ?? false
  rules.value = (settings.rules ?? []).map(rule => ({ key: nextKey++, plan_type: rule.plan_type, model: rule.model, lengthsText: rule.target_lengths.join(', ') }))
  selectedRuleKey.value = null
}
async function load() {
  if (loading.value || saving.value) return
  loading.value = true
  loadError.value = false
  loaded.value = false
  try { apply(await opsAPI.getCodexTurnStateScanSettings()); loaded.value = true }
  catch { loadError.value = true }
  finally { loading.value = false }
}
function addRule() {
  if (rules.value.length >= 128) return
  rules.value.push({ key: nextKey++, plan_type: '*', model: '', lengthsText: lengthsText.value })
}
function removeRule(index: number) { rules.value.splice(index, 1) }
async function editRule(selection: CodexTurnStateLengthRule) {
  if (!loaded.value || loading.value || saving.value) return
  const plan = plans.includes(selection.plan_type) ? selection.plan_type : '*'
  const model = selection.model.trim().toLowerCase()
  let rule = rules.value.find(rule => rule.plan_type === plan && rule.model.trim().toLowerCase() === model)
  if (!rule) {
    if (rules.value.length >= 128) { appStore.showError(t('admin.ops.turnState.scanSettings.tooManyRules')); return }
    rule = { key: nextKey++, plan_type: plan, model, lengthsText: selection.target_lengths.join(', ') }
    rules.value.unshift(rule)
  }
  selectedRuleKey.value = rule.key
  await nextTick()
  panel.value?.scrollIntoView?.({ behavior: 'smooth', block: 'start' })
  const index = rules.value.findIndex(item => item.key === rule.key)
  const input = panel.value?.querySelectorAll<HTMLInputElement>('[data-test="inline-rule-lengths"]')[index]
  input?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' })
  input?.focus({ preventScroll: true })
}
async function save() {
  if (!loaded.value || loading.value || saving.value || validationError.value) return
  saving.value = true
  try {
    const latest = await opsAPI.getCodexTurnStateScanSettings()
    const result = await opsAPI.updateCodexTurnStateScanSettings({ ...latest, target_lengths: [...targetLengths.value], rules: normalizedRules.value, plan_scan_enabled: { ...planScanEnabled.value }, require_state_before_routing: requireStateBeforeRouting.value, require_route_binding: requireRouteBinding.value })
    apply(result)
    appStore.showSuccess(t('admin.ops.turnState.scanSettings.saved'))
    emit('saved', result)
  } catch { appStore.showError(t('admin.ops.turnState.scanSettings.saveFailed')) }
  finally { saving.value = false }
}
defineExpose({ editRule, load })
onMounted(load)
</script>
