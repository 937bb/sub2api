<template>
  <div class="console-shell console-auth">
    <section class="console-auth-identity">
      <router-link v-if="settingsLoaded" to="/home" class="console-auth-brand">
        <img :src="siteLogo || '/logo.svg'" alt="" />
        <span>{{ siteName }}</span>
      </router-link>
      <div class="console-auth-copy">
        <h1>{{ t('console.authTitle') }}</h1>
        <p>{{ displaySubtitle }}</p>
      </div>
      <footer>&copy; {{ currentYear }} {{ siteName }}. All rights reserved.</footer>
    </section>
    <main class="console-auth-content">
      <div class="console-auth-form">
        <slot />
        <div class="console-auth-footer"><slot name="footer" /></div>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'
import { resolveSiteName } from '@/utils/branding'

const { t } = useI18n()
const appStore = useAppStore()

const siteName = computed(() => resolveSiteName(appStore.siteName))
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle)
const displaySubtitle = computed(() => {
  const subtitle = siteSubtitle.value?.trim()
  return !subtitle || ['Subscription to API Conversion Platform', 'AI API Gateway Platform'].includes(subtitle)
    ? t('console.authDescription')
    : subtitle
})
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>
