<template>
  <AppLayout>
    <div class="space-y-5">
      <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 pb-4 dark:border-dark-700">
        <div class="inline-flex rounded-md border border-gray-200 bg-white p-1 dark:border-dark-700 dark:bg-dark-900">
          <button v-for="item in tabs" :key="item.value" class="rounded px-4 py-2 text-sm font-medium" :class="tab === item.value ? 'bg-primary-600 text-white' : 'text-gray-600 hover:bg-gray-100 dark:text-dark-300 dark:hover:bg-dark-800'" @click="tab = item.value; onTabChange()">{{ item.label }}</button>
        </div>
        <button v-if="tab === 'config'" class="btn btn-primary" :disabled="saving || !config" @click="save">
          <Icon name="check" size="sm" />
          <span>{{ saving ? t('common.saving') : t('common.save') }}</span>
        </button>
      </div>

      <div v-if="loading" class="flex justify-center py-16"><Icon name="refresh" size="lg" class="animate-spin text-primary-500" /></div>

      <template v-else-if="tab === 'config' && config">
        <section class="border-b border-gray-200 pb-6 dark:border-dark-700">
          <div class="mb-4 flex items-center justify-between gap-4">
            <div><h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('growth.admin.checkinSettings') }}</h2><p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('growth.admin.checkinSettingsDescription') }}</p></div>
            <Toggle v-model="config.checkin_enabled" />
          </div>
          <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            <div><label class="input-label">{{ t('growth.admin.rewardMode') }}</label><Select v-model="config.checkin_reward_mode" :options="rewardModeOptions" /></div>
            <div v-if="config.checkin_reward_mode === 'fixed'"><label class="input-label">{{ t('growth.admin.fixedReward') }}</label><input v-model.number="config.checkin_fixed_reward" class="input" type="number" min="0" max="100" step="0.01" /></div>
            <template v-else>
              <div><label class="input-label">{{ t('growth.admin.minReward') }}</label><input v-model.number="config.checkin_min_reward" class="input" type="number" min="0" max="100" step="0.01" /></div>
              <div><label class="input-label">{{ t('growth.admin.maxReward') }}</label><input v-model.number="config.checkin_max_reward" class="input" type="number" min="0" max="100" step="0.01" /></div>
            </template>
          </div>
        </section>

        <section class="border-b border-gray-200 py-6 dark:border-dark-700">
          <div class="mb-4 flex items-center justify-between"><div><h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('growth.admin.streakRewards') }}</h2><p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('growth.admin.streakRewardsDescription') }}</p></div><button class="btn btn-secondary btn-sm" @click="addStreak"><Icon name="plus" size="sm" />{{ t('common.add') }}</button></div>
          <div v-if="config.checkin_streak_rewards.length" class="space-y-2">
            <div v-for="(reward, index) in config.checkin_streak_rewards" :key="index" class="grid grid-cols-[1fr_1fr_40px] gap-3">
              <input v-model.number="reward.days" class="input" type="number" min="2" max="365" :placeholder="t('growth.admin.days')" />
              <input v-model.number="reward.amount" class="input" type="number" min="0.01" max="100" step="0.01" :placeholder="t('growth.admin.rewardAmount')" />
              <button class="btn btn-secondary px-2" :title="t('common.delete')" @click="config.checkin_streak_rewards.splice(index, 1)"><Icon name="trash" size="sm" /></button>
            </div>
          </div>
          <p v-else class="text-sm text-gray-400 dark:text-dark-500">{{ t('common.noData') }}</p>
        </section>

        <section class="border-b border-gray-200 py-6 dark:border-dark-700">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('growth.admin.antiAbuse') }}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('growth.admin.antiAbuseDescription') }}</p>
          <div class="mt-4 grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            <div><label class="input-label">{{ t('growth.admin.accountAge') }}</label><input v-model.number="config.checkin_min_account_age_days" class="input" type="number" min="0" /></div>
            <div><label class="input-label">{{ t('growth.admin.spendWindow') }}</label><input v-model.number="config.checkin_recent_spend_days" class="input" type="number" min="1" max="365" /></div>
            <div><label class="input-label">{{ t('growth.admin.ipLimit') }}</label><input v-model.number="config.checkin_max_accounts_per_ip" class="input" type="number" min="1" /></div>
            <div><label class="input-label">{{ t('growth.admin.deviceLimit') }}</label><input v-model.number="config.checkin_max_accounts_per_device" class="input" type="number" min="1" /></div>
          </div>
        </section>

        <section class="pt-6">
          <div class="mb-4 flex items-center justify-between gap-4">
            <div><h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('growth.admin.leaderboardSettings') }}</h2><p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('growth.admin.leaderboardSettingsDescription') }}</p></div>
            <Toggle v-model="config.leaderboard_enabled" />
          </div>
          <div class="mb-6 max-w-sm">
            <div><label class="input-label">{{ t('growth.admin.displayLimit') }}</label><input v-model.number="config.leaderboard_display_limit" class="input" type="number" min="1" max="100" step="1" /><p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('growth.admin.displayLimitHint') }}</p></div>
          </div>
          <div class="mb-3"><h3 class="text-sm font-medium text-gray-700 dark:text-dark-300">{{ t('growth.admin.rewardRules') }}</h3><p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('growth.admin.rewardRulesDescription') }}</p></div>
          <section v-for="option in periodOptions" :key="option.value" class="border-t border-gray-200 py-4 first:border-t-0 dark:border-dark-700">
            <div class="mb-3 flex items-center justify-between gap-3">
              <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('growth.admin.periodRewards', { period: option.label }) }}</h4>
              <button class="btn btn-secondary btn-sm" @click="addRule(option.value)"><Icon name="plus" size="sm" />{{ t('growth.admin.addRankRange') }}</button>
            </div>
            <div class="hidden gap-2 px-1 pb-2 text-xs text-gray-500 dark:text-dark-400 md:grid md:grid-cols-[80px_1fr_1fr_1fr_40px]">
              <span class="text-center">{{ t('growth.admin.enabled') }}</span><span>{{ t('growth.admin.rankStart') }}</span><span>{{ t('growth.admin.rankEnd') }}</span><span>{{ t('growth.admin.rewardAmount') }}</span><span></span>
            </div>
            <div class="space-y-2">
              <div v-for="rule in rulesForPeriod(option.value)" :key="rule.id" class="grid gap-2 md:grid-cols-[80px_1fr_1fr_1fr_40px]">
                <label class="flex items-center gap-2 text-xs text-gray-500 md:justify-center"><input v-model="rule.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600" /><span class="md:hidden">{{ t('growth.admin.enabled') }}</span></label>
                <input v-model.number="rule.rank_start" class="input" type="number" min="1" max="100" :aria-label="t('growth.admin.rankStart')" :placeholder="t('growth.admin.rankStart')" />
                <input v-model.number="rule.rank_end" class="input" type="number" min="1" max="100" :aria-label="t('growth.admin.rankEnd')" :placeholder="t('growth.admin.rankEnd')" />
                <input v-model.number="rule.reward_amount" class="input" type="number" min="0.01" step="0.01" :aria-label="t('growth.admin.rewardAmount')" :placeholder="t('growth.admin.rewardAmount')" />
                <button class="btn btn-secondary px-2" :title="t('common.delete')" @click="removeRule(rule.id)"><Icon name="trash" size="sm" /></button>
              </div>
            </div>
          </section>
          <div class="mt-5 flex flex-wrap gap-2">
            <button v-for="option in periodOptions" :key="option.value" class="btn btn-secondary btn-sm" :disabled="settling" @click="settle(option.value)"><Icon name="badge" size="sm" />{{ t('growth.admin.settlePrevious', { period: option.label }) }}</button>
          </div>
        </section>
      </template>

      <div v-else-if="tab === 'rewards'" class="card overflow-x-auto">
        <table class="w-full min-w-[780px] text-left text-sm"><thead class="border-b border-gray-100 bg-gray-50 text-xs uppercase text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-400"><tr><th class="px-4 py-3">{{ t('growth.admin.user') }}</th><th class="px-4 py-3">{{ t('growth.admin.source') }}</th><th class="px-4 py-3 text-right">{{ t('growth.admin.amount') }}</th><th class="px-4 py-3 text-right">{{ t('growth.admin.balanceAfter') }}</th><th class="px-4 py-3">{{ t('growth.admin.time') }}</th></tr></thead><tbody><tr v-for="item in rewards" :key="item.id" class="border-b border-gray-100 dark:border-dark-800"><td class="px-4 py-3">{{ item.email || `#${item.user_id}` }}</td><td class="px-4 py-3">{{ item.source_type }}</td><td class="px-4 py-3 text-right font-medium text-emerald-600">+${{ item.amount.toFixed(2) }}</td><td class="px-4 py-3 text-right">${{ item.balance_after.toFixed(2) }}</td><td class="px-4 py-3 text-gray-500">{{ formatDateTime(item.created_at) }}</td></tr></tbody></table>
        <Pagination v-if="rewardTotal > 0" :page="rewardPage" :page-size="pageSize" :total="rewardTotal" :show-page-size-selector="false" @update:page="loadRewardPage" />
      </div>

      <div v-else class="space-y-4">
        <div class="inline-flex rounded-md border border-gray-200 bg-white p-1 dark:border-dark-700 dark:bg-dark-900">
          <button class="rounded px-3 py-1.5 text-sm font-medium" :class="riskView === 'accounts' ? 'bg-primary-600 text-white' : 'text-gray-600 hover:bg-gray-100 dark:text-dark-300 dark:hover:bg-dark-800'" @click="switchRiskView('accounts')">{{ t('growth.admin.riskAccounts') }}</button>
          <button class="rounded px-3 py-1.5 text-sm font-medium" :class="riskView === 'events' ? 'bg-primary-600 text-white' : 'text-gray-600 hover:bg-gray-100 dark:text-dark-300 dark:hover:bg-dark-800'" @click="switchRiskView('events')">{{ t('growth.admin.riskEvents') }}</button>
        </div>

        <div v-if="riskView === 'accounts'" class="card overflow-x-auto">
          <table class="w-full min-w-[980px] text-left text-sm">
            <thead class="border-b border-gray-100 bg-gray-50 text-xs uppercase text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-400"><tr><th class="px-4 py-3">{{ t('growth.admin.user') }}</th><th class="px-4 py-3">{{ t('growth.admin.status') }}</th><th class="px-4 py-3">{{ t('growth.admin.reason') }}</th><th class="px-4 py-3 text-right">{{ t('growth.admin.riskCount') }}</th><th class="px-4 py-3">{{ t('growth.admin.lastRiskTime') }}</th><th class="px-4 py-3">{{ t('growth.admin.adminAction') }}</th><th class="px-4 py-3 text-right">{{ t('growth.admin.actions') }}</th></tr></thead>
            <tbody>
              <tr v-for="item in riskAccounts" :key="item.user_id" class="border-b border-gray-100 dark:border-dark-800">
                <td class="px-4 py-3 font-medium">{{ item.email || `#${item.user_id}` }}</td>
                <td class="px-4 py-3"><span class="badge" :class="riskStatusClass(item.status)">{{ riskStatusLabel(item.status) }}</span></td>
                <td class="px-4 py-3">{{ riskReasonLabel(item.reason_code) }}</td>
                <td class="px-4 py-3 text-right tabular-nums">{{ item.event_count }}</td>
                <td class="px-4 py-3 text-gray-500">{{ formatDateTime(item.last_flagged_at) }}</td>
                <td class="max-w-[220px] px-4 py-3 text-gray-500"><div class="truncate" :title="item.action_note">{{ item.action_note || '-' }}</div><div v-if="item.action_at" class="mt-0.5 text-xs">{{ item.action_by_email || '-' }} | {{ formatDateTime(item.action_at) }}</div></td>
                <td class="px-4 py-3"><div class="flex justify-end gap-2">
                  <button v-if="item.status === 'flagged'" class="btn btn-secondary btn-sm" :disabled="riskActionSubmitting" @click="openRiskAction(item, 'clear')"><Icon name="checkCircle" size="sm" />{{ t('growth.admin.clearRisk') }}</button>
                  <button v-if="item.status !== 'whitelisted'" class="btn btn-primary btn-sm" :disabled="riskActionSubmitting" @click="openRiskAction(item, 'whitelist')"><Icon name="shield" size="sm" />{{ t('growth.admin.addWhitelist') }}</button>
                  <button v-else class="btn btn-secondary btn-sm" :disabled="riskActionSubmitting" @click="openRiskAction(item, 'remove_whitelist')"><Icon name="ban" size="sm" />{{ t('growth.admin.removeWhitelist') }}</button>
                </div></td>
              </tr>
              <tr v-if="!riskAccounts.length"><td colspan="7" class="px-4 py-12 text-center text-sm text-gray-400 dark:text-dark-500">{{ t('common.noData') }}</td></tr>
            </tbody>
          </table>
          <Pagination v-if="riskAccountTotal > 0" :page="riskAccountPage" :page-size="pageSize" :total="riskAccountTotal" :show-page-size-selector="false" @update:page="loadRiskAccountPage" />
        </div>

        <div v-else class="card overflow-x-auto">
          <table class="w-full min-w-[760px] text-left text-sm"><thead class="border-b border-gray-100 bg-gray-50 text-xs uppercase text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-400"><tr><th class="px-4 py-3">{{ t('growth.admin.user') }}</th><th class="px-4 py-3">{{ t('growth.admin.decision') }}</th><th class="px-4 py-3">{{ t('growth.admin.reason') }}</th><th class="px-4 py-3">{{ t('growth.admin.evidence') }}</th><th class="px-4 py-3">{{ t('growth.admin.time') }}</th></tr></thead><tbody><tr v-for="item in riskEvents" :key="item.id" class="border-b border-gray-100 dark:border-dark-800"><td class="px-4 py-3">{{ item.email || `#${item.user_id || '-'}` }}</td><td class="px-4 py-3"><span class="badge badge-error">{{ item.decision }}</span></td><td class="px-4 py-3 font-medium">{{ riskReasonLabel(item.reason_code) }}</td><td class="max-w-xs truncate px-4 py-3 text-gray-500" :title="JSON.stringify(item.evidence)">{{ JSON.stringify(item.evidence) }}</td><td class="px-4 py-3 text-gray-500">{{ formatDateTime(item.created_at) }}</td></tr></tbody></table>
          <Pagination v-if="riskTotal > 0" :page="riskPage" :page-size="pageSize" :total="riskTotal" :show-page-size-selector="false" @update:page="loadRiskPage" />
        </div>
      </div>
    </div>

    <ConfirmDialog :show="!!pendingRiskAction" :title="riskActionTitle" :message="riskActionMessage" :confirm-text="t('common.confirm')" :danger="pendingRiskAction?.action === 'remove_whitelist'" @confirm="submitRiskAction" @cancel="closeRiskAction">
      <div><label class="input-label">{{ t('growth.admin.actionNote') }}</label><input v-model.trim="riskActionNote" class="input" maxlength="500" :placeholder="t('growth.admin.actionNotePlaceholder')" /></div>
    </ConfirmDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Toggle from '@/components/common/Toggle.vue'
import Select from '@/components/common/Select.vue'
import Pagination from '@/components/common/Pagination.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { getGrowthConfig, listGrowthRewards, listGrowthRiskAccounts, listGrowthRiskEvents, settleGrowthLeaderboard, updateGrowthConfig, updateGrowthRiskAccount, type GrowthConfig, type GrowthLeaderboardRewardRule, type GrowthPeriod, type GrowthRewardLedgerItem, type GrowthRiskAccount, type GrowthRiskAccountAction, type GrowthRiskAccountStatus, type GrowthRiskEvent } from '@/api/growth'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const tab = ref<'config' | 'rewards' | 'risk'>('config')
const config = ref<GrowthConfig | null>(null)
const rewards = ref<GrowthRewardLedgerItem[]>([])
const riskEvents = ref<GrowthRiskEvent[]>([])
const riskAccounts = ref<GrowthRiskAccount[]>([])
const riskView = ref<'accounts' | 'events'>('accounts')
const loading = ref(false)
const saving = ref(false)
const settling = ref(false)
const pageSize = 20
const rewardPage = ref(1)
const rewardTotal = ref(0)
const riskPage = ref(1)
const riskTotal = ref(0)
const riskAccountPage = ref(1)
const riskAccountTotal = ref(0)
const riskActionSubmitting = ref(false)
const riskActionNote = ref('')
const pendingRiskAction = ref<{ account: GrowthRiskAccount; action: GrowthRiskAccountAction } | null>(null)
const tabs = [{ value: 'config' as const, label: t('growth.admin.configTab') }, { value: 'rewards' as const, label: t('growth.admin.rewardsTab') }, { value: 'risk' as const, label: t('growth.admin.riskTab') }]
const rewardModeOptions = [{ value: 'fixed', label: t('growth.admin.fixed') }, { value: 'random', label: t('growth.admin.random') }]
const periodOptions = [{ value: 'daily' as const, label: t('growth.period.daily') }, { value: 'weekly' as const, label: t('growth.period.weekly') }, { value: 'monthly' as const, label: t('growth.period.monthly') }]

function addStreak(): void { config.value?.checkin_streak_rewards.push({ days: 7, amount: 1 }) }
function rulesForPeriod(period: GrowthPeriod): GrowthLeaderboardRewardRule[] { return config.value?.leaderboard_reward_rules.filter((rule) => rule.period === period) || [] }
function addRule(period: GrowthPeriod): void {
  if (!config.value) return
  const rules = rulesForPeriod(period)
  const nextRank = Math.max(0, ...rules.map((rule) => rule.rank_end)) + 1
  if (nextRank > 100) return
  config.value.leaderboard_reward_rules.push({ id: crypto.randomUUID(), enabled: true, period, rank_start: nextRank, rank_end: nextRank, reward_amount: 1 })
}
function removeRule(id: string): void { if (config.value) config.value.leaderboard_reward_rules = config.value.leaderboard_reward_rules.filter((rule) => rule.id !== id) }
function ensureFirstPlaceRules(value: GrowthConfig): GrowthConfig {
  for (const period of periodOptions.map((option) => option.value)) {
    if (!value.leaderboard_reward_rules.some((rule) => rule.period === period && rule.rank_start <= 1 && rule.rank_end >= 1)) {
      value.leaderboard_reward_rules.push({ id: crypto.randomUUID(), enabled: false, period, rank_start: 1, rank_end: 1, reward_amount: 1 })
    }
  }
  return value
}
function formatDateTime(value: string): string { return new Date(value).toLocaleString() }
function riskStatusLabel(status: GrowthRiskAccountStatus): string { return t(`growth.admin.riskStatus.${status}`) }
function riskStatusClass(status: GrowthRiskAccountStatus): string { return status === 'flagged' ? 'badge-error' : status === 'whitelisted' ? 'badge-success' : 'badge-warning' }
function riskReasonLabel(reason: string): string { return ['ip_account_limit', 'device_account_limit', 'missing_identity_signal'].includes(reason) ? t(`growth.admin.riskReason.${reason}`) : reason || '-' }
async function loadConfig(): Promise<void> { config.value = ensureFirstPlaceRules(await getGrowthConfig()) }
async function loadRewardPage(page: number): Promise<void> {
  const result = await listGrowthRewards(page, pageSize)
  rewardPage.value = result.page
  rewardTotal.value = result.total
  rewards.value = result.items
}
async function loadRiskPage(page: number): Promise<void> {
  const result = await listGrowthRiskEvents(page, pageSize)
  riskPage.value = result.page
  riskTotal.value = result.total
  riskEvents.value = result.items
}
async function loadRiskAccountPage(page: number): Promise<void> {
  const result = await listGrowthRiskAccounts(page, pageSize)
  riskAccountPage.value = result.page
  riskAccountTotal.value = result.total
  riskAccounts.value = result.items
}
async function switchRiskView(value: 'accounts' | 'events'): Promise<void> {
  riskView.value = value
  loading.value = true
  try { if (value === 'accounts') await loadRiskAccountPage(1); else await loadRiskPage(1) }
  catch (error) { appStore.showError(extractApiErrorMessage(error, t('growth.loadFailed'))) }
  finally { loading.value = false }
}
async function onTabChange(): Promise<void> {
  loading.value = true
  try {
    if (tab.value === 'config') await loadConfig()
    else if (tab.value === 'rewards') await loadRewardPage(1)
    else if (riskView.value === 'accounts') await loadRiskAccountPage(1)
    else await loadRiskPage(1)
  } catch (error) { appStore.showError(extractApiErrorMessage(error, t('growth.loadFailed'))) }
  finally { loading.value = false }
}
async function save(): Promise<void> {
  if (!config.value) return
  saving.value = true
  try { config.value = ensureFirstPlaceRules(await updateGrowthConfig(config.value)); appStore.showSuccess(t('common.saved')) }
  catch (error) { appStore.showError(extractApiErrorMessage(error, t('growth.admin.saveFailed'))) }
  finally { saving.value = false }
}
async function settle(period: GrowthPeriod): Promise<void> {
  settling.value = true
  try { const result = await settleGrowthLeaderboard(period); appStore.showSuccess(t('growth.admin.settleSuccess', { count: result.rewarded_users, amount: result.total_reward.toFixed(2) })) }
  catch (error) { appStore.showError(extractApiErrorMessage(error, t('growth.admin.settleFailed'))) }
  finally { settling.value = false }
}
function openRiskAction(account: GrowthRiskAccount, action: GrowthRiskAccountAction): void { pendingRiskAction.value = { account, action }; riskActionNote.value = '' }
function closeRiskAction(): void { if (!riskActionSubmitting.value) { pendingRiskAction.value = null; riskActionNote.value = '' } }
const riskActionTitle = computed(() => pendingRiskAction.value ? t(`growth.admin.riskAction.${pendingRiskAction.value.action}Title`) : '')
const riskActionMessage = computed(() => pendingRiskAction.value ? t(`growth.admin.riskAction.${pendingRiskAction.value.action}Message`, { email: pendingRiskAction.value.account.email || `#${pendingRiskAction.value.account.user_id}` }) : '')
async function submitRiskAction(): Promise<void> {
  if (!pendingRiskAction.value || riskActionSubmitting.value) return
  riskActionSubmitting.value = true
  try {
    await updateGrowthRiskAccount(pendingRiskAction.value.account.user_id, pendingRiskAction.value.action, riskActionNote.value)
    appStore.showSuccess(t('growth.admin.riskActionSuccess'))
    pendingRiskAction.value = null
    riskActionNote.value = ''
    await loadRiskAccountPage(riskAccountPage.value)
  } catch (error) { appStore.showError(extractApiErrorMessage(error, t('growth.admin.riskActionFailed'))) }
  finally { riskActionSubmitting.value = false }
}
onMounted(() => void onTabChange())
</script>
