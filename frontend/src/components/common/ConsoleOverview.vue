<template>
  <section class="console-overview">
    <div>
      <h2>{{ t(admin ? 'console.adminWorkspace' : 'console.userWorkspace') }}</h2>
      <p>{{ t(admin ? 'console.adminOverview' : 'console.userOverview') }}</p>
    </div>
    <div class="flex flex-wrap items-center gap-2">
      <router-link v-if="admin" to="/admin/accounts" class="btn btn-primary">
        <Icon name="server" size="sm" />{{ t('nav.accounts') }}
      </router-link>
      <router-link v-else to="/keys" class="btn btn-primary">
        <Icon name="key" size="sm" />{{ t('nav.apiKeys') }}
      </router-link>
      <router-link v-if="admin || !simple" :to="admin ? '/admin/usage' : '/usage'" class="btn btn-secondary">
        {{ t('nav.usage') }}<Icon name="chevronRight" size="sm" />
      </router-link>
      <slot name="actions" />
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
defineProps<{ admin?: boolean; simple?: boolean }>()
const { t } = useI18n()
</script>
