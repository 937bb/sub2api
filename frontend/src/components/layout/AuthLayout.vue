<template>
  <div class="console-shell console-auth">
    <section class="console-auth-identity">
      <div v-if="settingsLoaded" class="console-auth-brand">
        <img v-if="siteLogo" :src="siteLogo" alt="" />
        <span>{{ siteName }}</span>
      </div>
      <div class="console-auth-copy">
        <h1>{{ t('console.authTitle') }}</h1>
        <p>{{ siteSubtitle || t('console.authDescription') }}</p>
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

const { t } = useI18n()
const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle)
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>
