<template>
  <BaseDialog presentation="sheet"
    :show="show"
    :title="t('admin.users.createUser')"
    width="normal"
    @close="$emit('close')"
  >
    <form id="create-user-form" @submit.prevent="submit" class="workbench-editor-form space-y-5 console-form">
      <DialogSection :title="t('console.formIdentity')">
<div class="workbench-field-row console-field">
        <label :for="`${fieldId}-email`" class="input-label">{{ t('admin.users.email') }}</label>
        <input :id="`${fieldId}-email`" v-model="form.email" type="email" required class="input" :placeholder="t('admin.users.enterEmail')" />
      </div>
      <div class="workbench-field-row ">
        <label :for="`${fieldId}-password`" class="input-label">{{ t('admin.users.password') }}</label>
        <div class="flex gap-2">
          <div class="relative flex-1">
            <input :id="`${fieldId}-password`" v-model="form.password" type="text" required class="input pr-10" :placeholder="t('admin.users.enterPassword')" />
          </div>
          <button type="button" @click="generateRandomPassword" :aria-label="t('admin.users.generatePassword')" class="btn btn-secondary px-3">
            <Icon name="refresh" size="md" />
          </button>
        </div>
      </div>
      <div class="workbench-field-row console-field">
        <label :for="`${fieldId}-username`" class="input-label">{{ t('admin.users.username') }}</label>
        <input :id="`${fieldId}-username`" v-model="form.username" type="text" class="input" :placeholder="t('admin.users.enterUsername')" />
      </div>
</DialogSection>
      <DialogSection :title="t('console.formAccess')">
<div class="workbench-field-row console-field">
        <label :for="`${fieldId}-role`" class="input-label">{{ t('admin.users.form.roleLabel') }}</label>
        <select :id="`${fieldId}-role`" v-model="form.role" class="input">
          <option value="user">{{ t('admin.users.roles.user') }}</option>
          <option value="admin">{{ t('admin.users.roles.admin') }}</option>
        </select>
      </div>
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div class="console-field">
          <label :for="`${fieldId}-balance`" class="input-label">{{ t('admin.users.columns.balance') }}</label>
          <input :id="`${fieldId}-balance`" v-model="form.balance" type="number" step="any" class="input" />
        </div>
        <div class="console-field">
          <label :for="`${fieldId}-concurrency`" class="input-label">{{ t('admin.users.columns.concurrency') }}</label>
          <input :id="`${fieldId}-concurrency`" v-model.number="form.concurrency" type="number" class="input" />
        </div>
      </div>
      <div class="workbench-field-row console-field">
        <label :for="`${fieldId}-rpm_limit`" class="input-label">{{ t('admin.users.form.rpmLimit') }}</label>
        <input
          :id="`${fieldId}-rpm_limit`" v-model.number="form.rpm_limit"
          type="number"
          min="0"
          step="1"
          class="input"
          :placeholder="t('admin.users.form.rpmLimitPlaceholder')"
        />
        <p class="input-hint">{{ t('admin.users.form.rpmLimitHint') }}</p>
      </div>
</DialogSection>
    </form>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button @click="$emit('close')" type="button" class="btn btn-secondary">{{ t('common.cancel') }}</button>
        <button type="submit" form="create-user-form" :disabled="loading" class="btn btn-primary">
          {{ loading ? t('admin.users.creating') : t('common.create') }}
        </button>
      </div>
    </template>
  </BaseDialog>

  <!-- Verify elevated access with TOTP before retrying administrator creation. -->
  <TotpStepUpDialog :controller="stepUp" />
</template>

<script setup lang="ts">
import DialogSection from '@/components/common/DialogSection.vue'
import { reactive, ref, watch, useId } from 'vue'
import { useI18n } from 'vue-i18n'; import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useStepUp, isStepUpBlocked, isStepUpCancelled, stepUpBlockReason } from '@/composables/useStepUp'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'

const fieldId = useId()
const props = defineProps<{ show: boolean }>()
const emit = defineEmits(['close', 'success']); const { t } = useI18n()
const appStore = useAppStore()

const form = reactive({ email: '', password: '', username: '', notes: '', role: 'user' as 'user' | 'admin', balance: '', concurrency: 1, rpm_limit: 0 })

const stepUp = useStepUp()
const loading = ref(false)

const submit = async () => {
  if (loading.value) return
  loading.value = true
  try {
    const { balance: rawBalance, ...rest } = { ...form }
    const balance = String(rawBalance).trim()
    const payload: typeof rest & { balance?: number } = { ...rest }
    if (balance !== '') {
      payload.balance = Number(balance)
    }
    // Complete step-up verification before retrying administrator creation.
    await stepUp.run(() => adminAPI.users.create(payload))
    appStore.showSuccess(t('admin.users.userCreated'))
    emit('success'); emit('close')
  } catch (e: any) {
    if (isStepUpCancelled(e)) {
      // Keep entered values when the user cancels step-up verification.
    } else if (isStepUpBlocked(e)) {
      appStore.showError(
        stepUpBlockReason(e) === 'STEP_UP_ADMIN_API_KEY_FORBIDDEN'
          ? t('stepUp.adminApiKeyForbidden')
          : t('stepUp.notEnabled')
      )
    } else {
      appStore.showError(e?.message || t('admin.users.failedToCreate'))
    }
  } finally { loading.value = false }
}

watch(() => props.show, (v) => { if(v) Object.assign(form, { email: '', password: '', username: '', notes: '', role: 'user', balance: '', concurrency: 1, rpm_limit: 0 }) })

const generateRandomPassword = () => {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789!@#$%^&*'
  let p = ''; for (let i = 0; i < 16; i++) p += chars.charAt(Math.floor(Math.random() * chars.length))
  form.password = p
}
</script>
