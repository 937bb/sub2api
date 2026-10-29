<template>
  <div v-if="hasHomeContent" class="min-h-screen">
    <iframe v-if="isHomeContentUrl" :src="homeContent.trim()" class="h-screen w-full border-0" allowfullscreen></iframe>
    <div v-else v-html="homeContent"></div>
  </div>

  <div v-else-if="compactHomeEnabled" data-testid="compact-home" class="flex min-h-screen flex-col bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white">
    <header class="border-b border-gray-200 px-4 py-4 sm:px-6 dark:border-dark-800">
      <nav class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-3 sm:gap-4">
        <div class="flex min-w-0 flex-1 items-center gap-3">
          <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-9 w-9 shrink-0 rounded-lg object-contain" />
          <span class="min-w-0 truncate text-base font-semibold">{{ siteName }}</span>
        </div>
        <div class="flex max-w-full shrink-0 flex-wrap items-center justify-end gap-2">
          <LocaleSwitcher />
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800" :title="t('home.viewDocs')">
            <Icon name="book" size="md" />
          </a>
          <router-link v-if="showModelPlazaEntry" to="/model-plaza" class="flex h-10 shrink-0 items-center gap-1.5 rounded-lg px-2.5 text-sm font-medium text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white" :title="t('nav.modelPlaza')">
            <Icon name="grid" size="md" /><span class="hidden sm:inline">{{ t('nav.modelPlaza') }}</span>
          </router-link>
          <button class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800" :title="isDark ? t('home.switchToLight') : t('home.switchToDark')" @click="toggleTheme">
            <Icon v-if="isDark" name="sun" size="md" /><Icon v-else name="moon" size="md" />
          </button>
          <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="inline-flex min-h-10 shrink-0 items-center justify-center rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200">
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>
    <main class="flex min-w-0 flex-1 items-center justify-center px-4 py-16 sm:px-6">
      <div class="min-w-0 max-w-2xl text-center">
        <img :src="siteLogo || '/logo.svg'" alt="Logo" class="mx-auto mb-6 h-20 w-20 rounded-2xl object-contain" />
        <h1 class="[overflow-wrap:anywhere] text-3xl font-bold md:text-4xl">{{ siteName }}</h1>
        <p class="mt-4 whitespace-pre-wrap [overflow-wrap:anywhere] text-base text-gray-600 dark:text-dark-300">{{ siteSubtitle }}</p>
        <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="mt-8 inline-flex min-h-10 items-center justify-center rounded-lg bg-primary-600 px-5 py-2.5 text-sm font-medium text-white hover:bg-primary-700">
          {{ isAuthenticated ? t('home.goToDashboard') : t('home.login') }}
        </router-link>
      </div>
    </main>
    <footer class="min-w-0 border-t border-gray-200 px-4 py-5 text-center text-sm text-gray-500 [overflow-wrap:anywhere] sm:px-6 dark:border-dark-800 dark:text-dark-400">&copy; {{ currentYear }} {{ siteName }}</footer>
  </div>

  <div v-else data-testid="default-home" class="home-shell" :class="{ 'home-dark': isDark }">
    <header class="home-header">
      <nav class="home-nav" :aria-label="siteName">
        <router-link to="/" class="home-brand">
          <img :src="siteLogo || '/logo.svg'" alt="" class="home-brand-logo" /><span>{{ siteName }}</span>
        </router-link>
        <div class="home-actions">
          <LocaleSwitcher />
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="home-icon-button" :title="t('home.viewDocs')"><Icon name="book" size="md" /></a>
          <router-link v-if="showModelPlazaEntry" to="/model-plaza" class="home-icon-button" :title="t('nav.modelPlaza')"><Icon name="grid" size="md" /></router-link>
          <button type="button" class="home-icon-button" :title="isDark ? t('home.switchToLight') : t('home.switchToDark')" @click="toggleTheme"><Icon v-if="isDark" name="sun" size="md" /><Icon v-else name="moon" size="md" /></button>
          <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="home-nav-cta"><Icon :name="isAuthenticated ? 'grid' : 'login'" size="sm" /><span>{{ isAuthenticated ? t('home.dashboard') : t('home.login') }}</span></router-link>
        </div>
      </nav>
    </header>

    <main>
      <section class="home-hero" aria-labelledby="home-title">
        <div class="home-grid" aria-hidden="true"></div>
        <div class="home-signal-field" aria-hidden="true">
          <div class="signal-axis signal-axis-a"></div><div class="signal-axis signal-axis-b"></div><div class="signal-axis signal-axis-c"></div>
          <div class="signal-node signal-node-input"><span>POST</span><strong>/v1/messages</strong></div>
          <div class="signal-node signal-node-route"><span>ROUTE</span><strong>MODEL / 01</strong></div>
          <div class="signal-node signal-node-output"><span>HTTP</span><strong>200 / STREAM</strong></div>
          <span class="signal-pulse signal-pulse-a"></span><span class="signal-pulse signal-pulse-b"></span>
        </div>
        <div class="home-hero-content">
          <div class="home-index"><span>01</span><span>{{ currentYear }}</span></div>
          <img :src="siteLogo || '/logo.svg'" alt="" class="home-hero-logo" />
          <h1 id="home-title">{{ siteName }}</h1>
          <p>{{ siteSubtitle }}</p>
          <div class="home-hero-actions">
            <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="home-primary-action"><span>{{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}</span><Icon name="arrowRight" size="sm" /></router-link>
            <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="home-secondary-action"><Icon name="book" size="sm" /><span>{{ t('home.viewDocs') }}</span></a>
          </div>
        </div>
        <div class="home-hero-footer" aria-hidden="true"><span>API</span><span>ROUTING</span><span>USAGE</span><span>BILLING</span></div>
      </section>

      <section class="home-capabilities" :aria-label="t('home.features.unifiedGateway')">
        <article><span class="capability-number">01</span><Icon name="server" size="lg" /><h2>{{ t('home.features.unifiedGateway') }}</h2><p>{{ t('home.features.unifiedGatewayDesc') }}</p></article>
        <article><span class="capability-number">02</span><Icon name="users" size="lg" /><h2>{{ t('home.features.multiAccount') }}</h2><p>{{ t('home.features.multiAccountDesc') }}</p></article>
        <article><span class="capability-number">03</span><Icon name="chart" size="lg" /><h2>{{ t('home.features.balanceQuota') }}</h2><p>{{ t('home.features.balanceQuotaDesc') }}</p></article>
      </section>

      <section class="home-providers">
        <div><span class="home-section-index">02</span><h2>{{ t('home.providers.title') }}</h2><p>{{ t('home.providers.description') }}</p></div>
        <div class="provider-list">
          <div><span class="provider-mark provider-mark-claude">C</span><strong>{{ t('home.providers.claude') }}</strong><span>{{ t('home.providers.supported') }}</span></div>
          <div><span class="provider-mark provider-mark-gpt">G</span><strong>GPT</strong><span>{{ t('home.providers.supported') }}</span></div>
          <div><span class="provider-mark provider-mark-gemini">G</span><strong>Gemini</strong><span>{{ t('home.providers.supported') }}</span></div>
        </div>
      </section>
    </main>
    <footer class="home-footer"><span>&copy; {{ currentYear }} {{ siteName }}</span></footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore, useAuthStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import { sanitizeUrl } from '@/utils/url'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'AI API Gateway Platform')
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const hasHomeContent = computed(() => homeContent.value.trim().length > 0)
const compactHomeEnabled = computed(() => appStore.cachedPublicSettings?.compact_home_enabled === true)
const modelPlazaEnabled = computed(() => isFeatureFlagEnabled(FeatureFlags.modelPlaza))
const isHomeContentUrl = computed(() => { const content = homeContent.value.trim(); return content.startsWith('http://') || content.startsWith('https://') })
const isDark = ref(document.documentElement.classList.contains('dark'))
const isAuthenticated = computed(() => authStore.isAuthenticated)
const modelPlazaRequiresAuth = computed(() => appStore.cachedPublicSettings?.model_plaza_require_auth === true)
const showModelPlazaEntry = computed(() => modelPlazaEnabled.value && (isAuthenticated.value || !modelPlazaRequiresAuth.value))
const dashboardPath = computed(() => authStore.isAdmin ? '/admin/dashboard' : '/dashboard')
const currentYear = computed(() => new Date().getFullYear())

function toggleTheme() { isDark.value = !isDark.value; document.documentElement.classList.toggle('dark', isDark.value); localStorage.setItem('theme', isDark.value ? 'dark' : 'light') }
function initTheme() { const savedTheme = localStorage.getItem('theme'); if (savedTheme === 'dark' || (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)) { isDark.value = true; document.documentElement.classList.add('dark') } }
onMounted(() => { initTheme(); authStore.checkAuth(); if (!appStore.publicSettingsLoaded) appStore.fetchPublicSettings() })
</script>

<style scoped>
.home-shell { --home-paper: #f4f6f3; --home-ink: #171a18; --home-muted: #67706a; --home-line: #cdd4cf; --home-accent: #087f68; --home-coral: #e34d38; min-height: 100vh; overflow-x: hidden; background: var(--home-paper); color: var(--home-ink); }
.home-shell.home-dark { --home-paper: #121513; --home-ink: #f1f5f2; --home-muted: #9aa49e; --home-line: #343a36; --home-accent: #37c8a4; --home-coral: #ff745f; }
.home-header { border-bottom: 1px solid var(--home-line); }
.home-nav { display: flex; min-height: 72px; align-items: center; justify-content: space-between; gap: 24px; max-width: 1440px; margin: 0 auto; padding: 0 40px; }
.home-brand { display: flex; min-width: 0; align-items: center; gap: 12px; color: inherit; font-size: 15px; font-weight: 700; }
.home-brand-logo { width: 34px; height: 34px; border-radius: 6px; object-fit: contain; }
.home-brand span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.home-actions { display: flex; align-items: center; gap: 8px; }
.home-icon-button { display: inline-flex; width: 38px; height: 38px; align-items: center; justify-content: center; border: 1px solid transparent; border-radius: 6px; color: var(--home-muted); }
.home-icon-button:hover { border-color: var(--home-line); color: var(--home-ink); }
.home-nav-cta, .home-primary-action, .home-secondary-action { display: inline-flex; min-height: 40px; align-items: center; justify-content: center; gap: 9px; border-radius: 6px; font-size: 13px; font-weight: 650; }
.home-nav-cta { margin-left: 4px; padding: 0 15px; background: var(--home-ink); color: var(--home-paper); }
.home-hero { position: relative; min-height: 680px; overflow: hidden; border-bottom: 1px solid var(--home-line); }
.home-grid { position: absolute; inset: 0; opacity: .65; background-size: 80px 80px; background-image: linear-gradient(var(--home-line) 1px, transparent 1px), linear-gradient(90deg, var(--home-line) 1px, transparent 1px); mask-image: linear-gradient(to right, transparent 0, black 28%, black 100%); }
.home-hero-content { position: relative; z-index: 3; display: flex; min-height: 620px; max-width: 1440px; margin: 0 auto; padding: 88px 40px 70px; flex-direction: column; align-items: flex-start; justify-content: center; }
.home-index { display: flex; width: 170px; justify-content: space-between; margin-bottom: 40px; border-top: 2px solid var(--home-ink); padding-top: 8px; color: var(--home-muted); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 11px; }
.home-hero-logo { width: 64px; height: 64px; margin-bottom: 22px; border-radius: 8px; object-fit: contain; }
.home-hero h1 { max-width: 780px; margin: 0; overflow-wrap: anywhere; font-size: 64px; line-height: 1.05; font-weight: 760; letter-spacing: 0; }
.home-hero-content > p { max-width: 620px; margin-top: 22px; white-space: pre-wrap; color: var(--home-muted); font-size: 18px; line-height: 1.75; overflow-wrap: anywhere; }
.home-hero-actions { display: flex; flex-wrap: wrap; gap: 10px; margin-top: 34px; }
.home-primary-action { padding: 0 18px; background: var(--home-accent); color: #fff; }
.home-primary-action:hover { filter: brightness(.92); }
.home-secondary-action { padding: 0 18px; border: 1px solid var(--home-line); color: var(--home-ink); }
.home-secondary-action:hover { border-color: var(--home-ink); }
.home-signal-field { position: absolute; z-index: 2; top: 0; right: 0; width: 58%; height: 100%; pointer-events: none; }
.signal-axis { position: absolute; height: 1px; background: var(--home-ink); opacity: .55; transform-origin: left; }
.signal-axis-a { top: 28%; left: 16%; width: 70%; transform: rotate(12deg); }.signal-axis-b { top: 48%; left: 4%; width: 82%; transform: rotate(-8deg); }.signal-axis-c { top: 67%; left: 25%; width: 60%; transform: rotate(17deg); }
.signal-node { position: absolute; display: grid; min-width: 158px; gap: 6px; border-left: 3px solid var(--home-coral); padding: 10px 0 10px 14px; background: color-mix(in srgb, var(--home-paper) 90%, transparent); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
.signal-node span { color: var(--home-muted); font-size: 10px; }.signal-node strong { font-size: 12px; font-weight: 700; }.signal-node-input { top: 20%; left: 20%; }.signal-node-route { top: 46%; left: 52%; border-color: #2563eb; }.signal-node-output { top: 70%; left: 25%; border-color: var(--home-accent); }
.signal-pulse { position: absolute; width: 9px; height: 9px; border: 2px solid var(--home-paper); border-radius: 50%; background: var(--home-coral); animation: signal-move 4s linear infinite; }.signal-pulse-a { top: 27%; left: 28%; }.signal-pulse-b { top: 47%; left: 20%; animation-delay: -2s; background: #2563eb; }
.home-hero-footer { position: absolute; z-index: 3; right: 40px; bottom: 24px; display: flex; gap: 28px; color: var(--home-muted); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 10px; }
.home-capabilities { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); max-width: 1440px; margin: 0 auto; border-inline: 1px solid var(--home-line); }
.home-capabilities article { position: relative; min-height: 310px; padding: 42px 38px; }.home-capabilities article + article { border-left: 1px solid var(--home-line); }.home-capabilities svg { color: var(--home-accent); }
.capability-number, .home-section-index { display: block; margin-bottom: 48px; color: var(--home-muted); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 11px; }
.home-capabilities h2 { margin-top: 28px; font-size: 18px; font-weight: 700; }.home-capabilities p { max-width: 360px; margin-top: 12px; color: var(--home-muted); font-size: 14px; line-height: 1.75; }
.home-providers { display: grid; grid-template-columns: minmax(0, .8fr) minmax(0, 1.2fr); gap: 64px; border-top: 1px solid var(--home-line); border-bottom: 1px solid var(--home-line); padding: 72px max(40px, calc((100vw - 1360px) / 2)); background: var(--home-ink); color: var(--home-paper); }
.home-providers h2 { font-size: 32px; font-weight: 720; }.home-providers p { max-width: 460px; margin-top: 14px; color: color-mix(in srgb, var(--home-paper) 66%, transparent); font-size: 14px; line-height: 1.75; }
.provider-list { border-top: 1px solid color-mix(in srgb, var(--home-paper) 28%, transparent); }.provider-list > div { display: grid; grid-template-columns: 34px 1fr auto; align-items: center; gap: 16px; min-height: 74px; border-bottom: 1px solid color-mix(in srgb, var(--home-paper) 28%, transparent); }.provider-list strong { font-size: 15px; }.provider-list > div > span:last-child { color: color-mix(in srgb, var(--home-paper) 58%, transparent); font-size: 11px; }
.provider-mark { display: inline-flex; width: 28px; height: 28px; align-items: center; justify-content: center; border-radius: 4px; color: #fff; font-size: 11px; font-weight: 800; }.provider-mark-claude { background: var(--home-coral); }.provider-mark-gpt { background: var(--home-accent); }.provider-mark-gemini { background: #2563eb; }
.home-footer { display: flex; justify-content: space-between; gap: 24px; padding: 24px 40px; color: var(--home-muted); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 10px; }
@keyframes signal-move { 0% { transform: translate(0, 0); } 50% { transform: translate(220px, 34px); } 100% { transform: translate(0, 0); } }
@media (max-width: 900px) {
  .home-nav { padding: 0 20px; }.home-brand span { max-width: 150px; }.home-hero { min-height: 720px; }.home-hero-content { min-height: 660px; padding: 72px 20px 180px; justify-content: flex-start; }.home-hero h1 { font-size: 46px; }.home-signal-field { top: auto; bottom: 0; width: 100%; height: 310px; opacity: .78; }.signal-node-route { left: 58%; }.home-hero-footer { right: 20px; bottom: 18px; gap: 15px; }.home-capabilities { grid-template-columns: 1fr; border-inline: 0; }.home-capabilities article { min-height: 250px; padding: 34px 20px; }.home-capabilities article + article { border-left: 0; border-top: 1px solid var(--home-line); }.capability-number { margin-bottom: 30px; }.home-providers { grid-template-columns: 1fr; gap: 38px; padding: 56px 20px; }
}
@media (max-width: 600px) {
  .home-header, .home-nav { min-height: 64px; }.home-nav { gap: 10px; }.home-brand-logo { width: 30px; height: 30px; }.home-brand span { display: none; }.home-actions { gap: 3px; }.home-icon-button { width: 34px; height: 34px; }.home-nav-cta { min-height: 36px; padding: 0 11px; }.home-nav-cta span { display: none; }.home-hero { min-height: 690px; }.home-hero-content { min-height: 625px; padding-top: 54px; }.home-index { margin-bottom: 28px; }.home-hero-logo { width: 52px; height: 52px; }.home-hero h1 { font-size: 38px; }.home-hero-content > p { font-size: 16px; line-height: 1.65; }.home-secondary-action { display: none; }.home-signal-field { height: 260px; }.signal-node { min-width: 130px; }.signal-node-route { left: 48%; }.home-hero-footer { display: none; }.home-providers h2 { font-size: 27px; }.home-footer { flex-direction: column; padding: 22px 20px; }
}
@media (prefers-reduced-motion: reduce) { .signal-pulse { animation: none; } }
</style>
