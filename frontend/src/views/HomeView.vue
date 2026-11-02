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

  <div v-else ref="motionRoot" data-testid="default-home" class="console-shell home-shell" :class="{ 'motion-ready': motionReady, 'motion-paused': animationsPaused, 'page-hidden': pageHidden }">
    <header class="home-header">
      <nav class="home-nav" :aria-label="siteName">
        <router-link to="/" class="home-brand">
          <img :src="siteLogo || '/logo.svg'" alt="" class="home-brand-logo" /><span>{{ siteName }}</span>
        </router-link>
        <div class="home-section-nav">
          <a href="#standards">{{ t('home.brandPage.navStandards') }}</a>
          <a href="#models">{{ t('home.brandPage.navModels') }}</a>
          <a href="#connect">{{ t('home.brandPage.navConnect') }}</a>
        </div>
        <div class="home-actions">
          <LocaleSwitcher />
          <button type="button" class="home-icon-button home-motion-toggle" :title="t(animationsPaused ? 'home.brandPage.resumeMotion' : 'home.brandPage.pauseMotion')" :aria-label="t(animationsPaused ? 'home.brandPage.resumeMotion' : 'home.brandPage.pauseMotion')" :aria-pressed="animationsPaused" @click="animationsPaused = !animationsPaused"><svg viewBox="0 0 24 24" width="18" height="18" fill="none" aria-hidden="true"><path v-if="animationsPaused" d="M8 5L19 12L8 19Z" fill="currentColor" /><path v-else d="M8 5V19M16 5V19" stroke="currentColor" stroke-width="3" stroke-linecap="round" /></svg></button>
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="home-icon-button" :title="t('home.viewDocs')"><Icon name="book" size="md" /></a>
          <router-link v-if="showModelPlazaEntry" to="/model-plaza" class="home-icon-button" :title="t('nav.modelPlaza')"><Icon name="grid" size="md" /></router-link>
          <button type="button" class="home-icon-button" :title="isDark ? t('home.switchToLight') : t('home.switchToDark')" @click="toggleTheme"><Icon v-if="isDark" name="sun" size="md" /><Icon v-else name="moon" size="md" /></button>
          <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="home-nav-cta"><Icon :name="isAuthenticated ? 'grid' : 'login'" size="sm" /><span>{{ isAuthenticated ? t('home.dashboard') : t('home.login') }}</span></router-link>
        </div>
      </nav>
    </header>

    <main>
      <div class="home-hero-stage">
      <section class="home-hero" aria-labelledby="home-title">
        <div class="home-hero-content">
          <h1 id="home-title">{{ siteName }}</h1>
          <p class="home-hero-tagline"><span v-for="(line, index) in t('home.heroSubtitle').split('\n')" :key="index" class="hero-line"><span :style="{ '--line-index': index }">{{ line }}</span></span></p>
          <p>{{ heroDescription }}</p>
          <div class="home-hero-actions">
            <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="home-primary-action"><span>{{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}</span><Icon name="arrowRight" size="sm" /></router-link>
            <a href="#connect" class="home-secondary-action">{{ t('home.brandPage.seeExample') }}</a>
          </div>
          <p class="home-hero-note">{{ t('home.accessNote') }}</p>
        </div>
        <div class="home-navigation-demo">
          <div class="home-navigation-caption"><span>{{ t('home.brandPage.routeTitle') }}</span><span>{{ t('home.brandPage.illustration') }}</span></div>
          <div class="home-compass">
            <svg class="home-compass-chart" viewBox="0 0 500 500" fill="none" aria-hidden="true">
              <circle cx="250" cy="250" r="220" class="chart-outer" />
              <circle cx="250" cy="250" r="220" class="chart-sweep" />
              <circle cx="250" cy="250" r="190" class="chart-ticks" />
              <circle cx="250" cy="250" r="148" class="chart-orbit" />
              <path d="M250 18V36 M250 464V482 M18 250H36 M464 250H482" class="chart-cardinals" />
              <path d="M250 88V250 M250 250L390 331 M250 250L110 331" class="chart-routes" />
              <path :key="selectedRoute" :d="routePaths[selectedRoute]" class="chart-active-route" />
              <path :d="routePaths[selectedRoute]" pathLength="100" class="chart-packet" />
              <circle cx="250" cy="250" r="83" class="chart-core-ring" />
            </svg>
            <div class="home-compass-core"><img :src="siteLogo || '/logo.svg'" alt="" /><strong>{{ siteName }}</strong></div>
            <button v-for="route in routes" :key="route.id" type="button" :class="['home-route-node', `home-route-node-${route.id}`]" :aria-pressed="selectedRoute === route.id" @click="selectedRoute = route.id"><span class="route-node-light" aria-hidden="true"></span>{{ route.label }}</button>
          </div>
          <div class="home-route-detail" aria-live="polite"><div><span>{{ t('home.brandPage.selectedRoute') }}</span><strong>{{ routes.find(route => route.id === selectedRoute)?.label }}</strong></div><p>{{ t(`home.brandPage.providers.${selectedRoute}`) }}</p><span class="home-route-hint">{{ t('home.brandPage.switchRoute') }}</span></div>
        </div>
      </section>
      <div class="home-hero-bottom"><span>{{ t('home.brandPage.oneEntry') }}</span><a href="#standards">{{ t('home.brandPage.navStandards') }}<span aria-hidden="true">↓</span></a><span>{{ t('home.modelAvailability') }}</span></div>
      </div>

      <section id="standards" class="home-standards home-section" aria-labelledby="standards-title">
        <div class="home-section-heading"><h2 id="standards-title">{{ t('home.brandPage.standardsTitle') }}</h2><p>{{ t('home.brandPage.standardsDescription') }}</p></div>
        <div class="home-capabilities">
          <article><div class="home-feature-icon"><Icon name="key" size="lg" /></div><h3>{{ t('home.brandPage.modelTitle') }}</h3><p>{{ t('home.brandPage.modelDescription') }}</p><a href="#models">{{ t('home.brandPage.navModels') }} <Icon name="arrowRight" size="sm" /></a></article>
          <article><div class="home-feature-icon"><Icon name="chart" size="lg" /></div><h3>{{ t('home.usageTitle') }}</h3><p>{{ t('home.usageDescription') }}</p><span class="home-feature-note">{{ t('home.brandPage.usageNote') }}</span></article>
          <article><div class="home-feature-icon"><Icon name="shield" size="lg" /></div><h3>{{ t('home.quotaTitle') }}</h3><p>{{ t('home.quotaDescription') }}</p><span class="home-feature-note">{{ t('home.brandPage.quotaNote') }}</span></article>
        </div>
      </section>

      <section id="models" class="home-model-section home-section" aria-labelledby="models-title">
        <div class="home-model-intro"><h2 id="models-title">{{ t('home.brandPage.modelsTitle') }}</h2><p>{{ t('home.brandPage.modelsDescription') }}</p><router-link v-if="showModelPlazaEntry" to="/model-plaza" class="home-secondary-action">{{ t('nav.modelPlaza') }}<Icon name="arrowRight" size="sm" /></router-link><p class="home-model-disclaimer">{{ t('home.modelAvailability') }}</p></div>
        <div class="home-model-directory">
          <div v-for="provider in ['gpt', 'claude', 'gemini']" :key="provider" class="home-model-row"><span :class="['provider-mark', `provider-mark-${provider}`]">{{ provider === 'claude' ? 'C' : 'G' }}</span><div><h3>{{ provider === 'gpt' ? 'GPT' : provider === 'claude' ? 'Claude' : 'Gemini' }}</h3><p>{{ t(`home.brandPage.providers.${provider}`) }}</p></div></div>
          <p>{{ t('home.brandPage.modelsNote') }}</p>
        </div>
      </section>

      <section id="connect" class="home-connect home-section" aria-labelledby="connect-title">
        <div class="home-connect-guide"><h2 id="connect-title">{{ t('home.brandPage.connectTitle') }}</h2><p>{{ t('home.brandPage.connectDescription') }}</p><ol class="home-steps"><li v-for="step in ['key', 'configure', 'usage']" :key="step"><h3>{{ t(`home.brandPage.steps.${step}Title`) }}</h3><p>{{ t(`home.brandPage.steps.${step}Description`) }}</p></li></ol><a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="home-text-link"><Icon name="book" size="sm" />{{ t('home.viewDocs') }}</a></div>
        <div class="home-example">
          <div class="home-example-heading"><Icon name="terminal" size="md" /><h3>{{ t('home.requestExample') }}</h3><span>cURL</span></div>
          <div class="home-protocols" :aria-label="t('home.protocol')"><button v-for="protocol in protocols" :key="protocol.id" type="button" :aria-pressed="selectedProtocol === protocol.id" @click="selectedProtocol = protocol.id">{{ protocol.label }}</button></div>
          <pre class="home-code"><code :key="selectedProtocol"><span v-for="(line, index) in requestExample.split('\n')" :key="index" class="code-line" :style="{ '--code-delay': `${Math.min(index * 35, 490)}ms` }">{{ line }}{{ '\n' }}</span></code></pre>
          <p class="home-example-note"><Icon name="key" size="sm" />{{ t('home.exampleNote') }}</p>
        </div>
      </section>

      <section class="home-faq home-section" aria-labelledby="faq-title"><div><h2 id="faq-title">{{ t('home.brandPage.faqTitle') }}</h2><p>{{ t('home.brandPage.faqDescription') }}</p></div><div class="home-faq-list"><details v-for="question in ['models', 'price', 'client', 'start']" :key="question"><summary>{{ t(`home.brandPage.faq.${question}Question`) }}<span aria-hidden="true">+</span></summary><p>{{ t(`home.brandPage.faq.${question}Answer`) }}</p></details></div></section>

      <section class="home-closing home-section"><div><h2>{{ t('home.brandPage.closingTitle') }}</h2><p>{{ t('home.brandPage.closingDescription') }}</p></div><router-link :to="isAuthenticated ? dashboardPath : '/login'" class="home-primary-action">{{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}<Icon name="arrowRight" size="sm" /></router-link></section>
    </main>
    <footer class="home-footer"><div><strong>{{ siteName }}</strong><span>&copy; {{ currentYear }} {{ siteName }}</span></div><a href="#connect">{{ t('home.brandPage.navConnect') }}</a><a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">{{ t('home.viewDocs') }}</a></footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore, useAuthStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import { sanitizeUrl } from '@/utils/url'
import { resolveSiteName } from '@/utils/branding'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()
const siteName = computed(() => resolveSiteName(appStore.cachedPublicSettings?.site_name || appStore.siteName))
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
const motionRoot = ref<HTMLElement | null>(null)
const motionReady = ref(false)
const animationsPaused = ref(false)
const pageHidden = ref(document.hidden)

watch(motionRoot, (root, _previousRoot, onCleanup) => {
  if (!root || typeof IntersectionObserver === 'undefined') return

  const groups = [
    '.home-section-heading', '.home-capabilities article', '.home-model-intro',
    '.home-model-row', '.home-connect-guide > h2, .home-connect-guide > p',
    '.home-steps li', '.home-example', '.home-faq > div:first-child',
    '.home-faq-list details', '.home-closing', '.home-footer'
  ]
  groups.forEach((selector) => {
    root.querySelectorAll<HTMLElement>(selector).forEach((element, index) => {
      element.classList.add('motion-item')
      element.style.setProperty('--reveal-delay', `${Math.min(index * 90, 270)}ms`)
    })
  })

  const observer = new IntersectionObserver((entries) => {
    entries.forEach(({ target, isIntersecting }) => {
      target.classList.toggle('is-in-view', isIntersecting)
      if (isIntersecting) target.classList.add('is-revealed')
    })
  }, { threshold: 0, rootMargin: '0px 0px -30px 0px' })

  root.querySelectorAll('.motion-item, .home-hero-stage, .home-model-section').forEach((element) => observer.observe(element))
  motionReady.value = true
  onCleanup(() => {
    observer.disconnect()
    motionReady.value = false
  })
}, { flush: 'post' })

function updatePageVisibility() { pageHidden.value = document.hidden }
onMounted(() => document.addEventListener('visibilitychange', updatePageVisibility))
onUnmounted(() => document.removeEventListener('visibilitychange', updatePageVisibility))
const routes = [{ id: 'gpt', label: 'GPT' }, { id: 'claude', label: 'Claude' }, { id: 'gemini', label: 'Gemini' }] as const
const selectedRoute = ref<(typeof routes)[number]['id']>('gpt')
const routePaths = { gpt: 'M250 250V88', claude: 'M250 250L390 331', gemini: 'M250 250L110 331' }
const heroDescription = computed(() => {
  const subtitle = siteSubtitle.value.trim()
  return ['Subscription to API Conversion Platform', 'AI API Gateway Platform', ''].includes(subtitle)
    ? t('home.heroDescription')
    : subtitle
})
const protocols = [
  { id: 'responses', label: 'Responses' },
  { id: 'chat', label: 'Chat Completions' },
  { id: 'messages', label: 'Messages' },
] as const
const selectedProtocol = ref<(typeof protocols)[number]['id']>('responses')
const requestExample = computed(() => {
  const base = sanitizeUrl(appStore.cachedPublicSettings?.api_base_url || '') || window.location.origin
  const root = base.replace(/\/+$/, '').replace(/\/v1$/, '')
  const path = selectedProtocol.value === 'chat' ? 'chat/completions' : selectedProtocol.value
  const auth = selectedProtocol.value === 'messages'
    ? '  -H "x-api-key: YOUR_API_KEY" \\\n  -H "anthropic-version: 2023-06-01" \\\n'
    : '  -H "Authorization: Bearer YOUR_API_KEY" \\\n'
  const payload = selectedProtocol.value === 'responses'
    ? { model: 'YOUR_MODEL', input: 'Hello', stream: true }
    : { model: 'YOUR_MODEL', messages: [{ role: 'user', content: 'Hello' }], ...(selectedProtocol.value === 'messages' ? { max_tokens: 1024 } : {}), stream: true }
  return `curl "${root}/v1/${path}" \\\n${auth}  -H "Content-Type: application/json" \\\n  -d '${JSON.stringify(payload, null, 2)}'`
})

function toggleTheme() { isDark.value = !isDark.value; document.documentElement.classList.toggle('dark', isDark.value); localStorage.setItem('theme', isDark.value ? 'dark' : 'light') }
function initTheme() { const savedTheme = localStorage.getItem('theme'); if (savedTheme === 'dark' || (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)) { isDark.value = true; document.documentElement.classList.add('dark') } }
onMounted(() => { initTheme(); authStore.checkAuth(); if (!appStore.publicSettingsLoaded) appStore.fetchPublicSettings() })
</script>

<style scoped>
.home-shell { min-height: 100dvh; color: var(--console-ink); background: var(--console-surface); --home-blue: #32688b; --home-soft: #edf3f7; --home-navy: #203e54; font-family: "Avenir Next", "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif; }
:global(.dark .home-shell) { --home-soft: #192c3b; --home-navy: #bad4e6; }
.home-shell :is(a, button, summary):focus-visible { outline: 3px solid #6594b7; outline-offset: 5px; }
.home-header { border-bottom: 1px solid var(--console-line); }
.home-nav, .home-hero, .home-section, .home-footer { width: min(1200px, calc(100% - 80px)); margin-inline: auto; }
.home-nav { display: flex; min-height: 84px; align-items: center; justify-content: space-between; gap: 32px; }
.home-brand { display: flex; min-width: 0; align-items: center; gap: 10px; color: inherit; font-size: 19px; font-weight: 650; }
.home-brand-logo { width: 34px; height: 34px; border-radius: 9px; object-fit: contain; }
.home-brand span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.home-section-nav { display: flex; align-items: center; gap: 30px; margin-right: auto; margin-left: 36px; font-size: 13px; color: var(--console-muted); }
.home-section-nav a:hover, .home-footer a:hover { color: var(--home-blue); }
.home-actions { display: flex; flex-shrink: 0; align-items: center; gap: 8px; }
.home-icon-button { display: inline-flex; width: 38px; height: 38px; align-items: center; justify-content: center; border-radius: 8px; color: var(--console-muted); }
.home-icon-button:hover { background: var(--console-nav); color: var(--console-ink); }
.home-nav-cta, .home-primary-action, .home-secondary-action { display: inline-flex; min-height: 46px; align-items: center; justify-content: center; gap: 10px; border-radius: 8px; font-size: 14px; font-weight: 600; text-decoration: none; }
.home-nav-cta { min-height: 38px; margin-left: 4px; padding: 0 18px; border: 1px solid var(--console-line); color: var(--console-ink); }
.home-nav-cta:hover { background: var(--console-nav); }
.home-hero { display: grid; grid-template-columns: minmax(0,1.15fr) minmax(0,1fr); align-items: center; gap: 80px; padding-block: 88px 96px; }
.home-hero-content { min-width: 0; }
.home-hero h1 { margin: 0 0 26px; overflow-wrap: anywhere; color: var(--home-blue); font-size: 23px; line-height: 1.3; font-weight: 650; letter-spacing: -.025em; }
:global(.dark .home-hero h1) { color: #91b7d1; }
.home-hero-content > .home-hero-tagline { margin: 0; color: var(--console-ink); font-size: clamp(30px, 3.15vw, 46px); line-height: 1.45; font-weight: 650; letter-spacing: -.045em; white-space: pre-line; }
.home-hero-content > p { max-width: 490px; margin-top: 26px; color: var(--console-muted); font-size: 16px; line-height: 1.9; overflow-wrap: anywhere; }
.home-hero-actions { display: flex; flex-wrap: wrap; gap: 12px; margin-top: 34px; }
.home-primary-action { padding: 0 22px; background: var(--home-blue); color: #fff; }
.home-primary-action:hover { background: #285572; }
.home-secondary-action { padding: 0 20px; border: 1px solid var(--console-line); color: var(--console-ink); background: var(--console-surface); }
.home-secondary-action:hover { background: var(--console-nav); }
.home-hero-content > .home-hero-note { margin-top: 22px; font-size: 12px; }
.home-hero-stage { position: relative; isolation: isolate; overflow: hidden; background: #102b40; color: #f3f7fb; }
.home-hero-stage::before { content: ""; position: absolute; z-index: -1; width: 760px; height: 760px; right: -100px; top: -60px; border-radius: 50%; background: radial-gradient(circle, #245977 0%, #173a52 36%, transparent 69%); pointer-events: none; }
.home-hero-stage::after { content: "APIHelm"; position: absolute; z-index: -1; bottom: 15px; left: max(20px, calc((100% - 1200px) / 2)); color: rgb(190 222 246 / 3%); font-size: clamp(130px, 22vw, 310px); font-weight: 700; line-height: 1; letter-spacing: -.07em; pointer-events: none; }
.home-hero-stage .home-hero { gap: 50px; padding-block: 62px 56px; min-height: 690px; }
.home-hero-stage .home-hero h1 { color: #a2c6df; font-size: 25px; }
.home-hero-stage .home-hero-tagline { color: #f6f9fc; font-size: clamp(38px, 4.1vw, 58px); line-height: 1.5; font-weight: 650; letter-spacing: -.045em; }
.home-hero-stage .home-hero-content > p:not(.home-hero-tagline) { color: #b3c8d9; }
.home-hero-stage .home-primary-action { color: #102b40; background: #e5f1fa; box-shadow: 0 4px 24px rgb(148 201 238 / 14%); }
.home-hero-stage .home-primary-action:hover { background: #fff; }
.home-hero-stage .home-secondary-action { background: transparent; color: #e5f1fa; border-color: #557086; }
.home-hero-stage .home-secondary-action:hover { background: #234257; }
.home-navigation-demo { min-width: 0; width: 100%; max-width: 510px; justify-self: center; }
.home-navigation-caption { display: flex; justify-content: space-between; gap: 16px; color: #c1d3e0; font-size: 12px; padding: 0 12px; }
.home-navigation-caption > span:last-child { color: #98b4c8; font-size: 11px; }
.home-compass { position: relative; width: 100%; aspect-ratio: 1; }
.home-compass-chart { display: block; width: 100%; height: 100%; }
.chart-outer { stroke: #355b75; stroke-width: 1; }
.chart-ticks { stroke: #547e99; stroke-width: 5; stroke-dasharray: 1 11; }
.chart-orbit { stroke: #3a607a; stroke-dasharray: 4 6; }
.chart-cardinals { stroke: #a4c5dc; stroke-width: 2; }
.chart-routes { stroke: #52748b; stroke-width: 1; }
.chart-active-route { stroke: #b5e3ff; stroke-width: 2.5; stroke-dasharray: 180; stroke-dashoffset: 0; animation: route-arrive 800ms ease-out both; }
.chart-core-ring { stroke: #6a9cbb; }
.home-compass-core { position: absolute; left: 50%; top: 50%; transform: translate(-50%,-50%); display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 10px; width: 29%; aspect-ratio: 1; border: 1px solid #769bb3; border-radius: 50%; background: radial-gradient(circle at 30% 20%, #355e79, #16364e 72%); box-shadow: 0 0 0 10px rgb(150 197 227 / 3%), 0 18px 40px rgb(0 15 27 / 30%); }
.home-compass-core img { width: 38%; height: 38%; object-fit: contain; border-radius: 10px; }
.home-compass-core strong { max-width: 90%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: clamp(12px, 1.4vw, 19px); color: #f1f7fc; }
.home-route-node { position: absolute; transform: translate(-50%,-50%); display: inline-flex; align-items: center; justify-content: center; gap: 8px; min-height: 46px; min-width: 105px; padding: 10px 17px; border: 1px solid #648198; border-radius: 12px; background: #18384e; color: #c4d6e3; font-size: 14px; font-weight: 600; box-shadow: 0 8px 24px rgb(0 13 25 / 20%); transition: background 180ms, border-color 180ms, box-shadow 180ms; }
.home-route-node:hover { background: #2d5069; border-color: #a3c6de; }
.home-route-node[aria-pressed='true'] { color: #f3f9ff; border-color: #b5e3ff; background: #2b536e; box-shadow: 0 0 25px rgb(129 202 249 / 19%); }
.route-node-light { width: 6px; height: 6px; border-radius: 50%; background: #7690a2; }
.home-route-node[aria-pressed='true'] .route-node-light { background: #c0eaff; box-shadow: 0 0 10px #9cd6f7; }
.home-route-node-gpt { left: 50%; top: 17.6%; }
.home-route-node-claude { left: 78%; top: 66.2%; }
.home-route-node-gemini { left: 22%; top: 66.2%; }
.home-route-detail { position: relative; margin: -16px 12px 0; padding: 20px 22px; border: 1px solid #44657d; border-radius: 12px; background: #19384e; }
.home-route-detail > div { display: flex; align-items: center; justify-content: space-between; gap: 12px; font-size: 12px; color: #b7cddd; }
.home-route-detail strong { color: #eef7ff; font-size: 16px; }
.home-route-detail > p { margin-top: 7px; color: #d0e1ed; font-size: 13px; line-height: 1.8; }
.home-route-hint { display: block; margin-top: 12px; color: #a3bdcf; font-size: 10px; line-height: 1.8; }
.home-hero-bottom { width: min(1200px, calc(100% - 80px)); margin-inline: auto; padding: 24px 0; display: flex; align-items: center; justify-content: space-between; gap: 20px; border-top: 1px solid #395469; color: #a5bdce; font-size: 11px; }
.home-hero-bottom a { display: inline-flex; align-items: center; gap: 14px; color: #e2eef6; }
.home-hero-bottom a > span { font-size: 19px; }
@keyframes route-arrive { from { stroke-dashoffset: 180; opacity: .25; } to { stroke-dashoffset: 0; opacity: 1; } }
@media (prefers-reduced-motion: reduce) { .chart-active-route { animation: none; } .home-route-node { transition: none; } }
.home-section { scroll-margin-top: 30px; }
.home-section h2 { font-size: clamp(26px, 2.4vw, 34px); font-weight: 600; line-height: 1.5; letter-spacing: -.035em; white-space: pre-line; text-wrap: balance; }
.home-section-heading { display: flex; align-items: flex-end; justify-content: space-between; gap: 60px; }
.home-section-heading > p { max-width: 380px; font-size: 14px; line-height: 1.9; color: var(--console-muted); }
.home-standards { padding-block: 54px 72px; border-top: 1px solid var(--console-line); }
.home-capabilities { display: grid; grid-template-columns: repeat(3,minmax(0,1fr)); gap: 50px; margin-top: 38px; }
.home-capabilities article { min-width: 0; }
.home-feature-icon { display: inline-flex; align-items: center; justify-content: center; width: 40px; height: 40px; border-radius: 11px; background: var(--home-soft); color: var(--home-navy); margin-bottom: 20px; }
.home-capabilities h3 { font-size: 18px; font-weight: 600; }
.home-capabilities p { min-height: 54px; margin-top: 12px; color: var(--console-muted); font-size: 14px; line-height: 1.9; }
.home-capabilities article > a, .home-feature-note { display: inline-flex; align-items: center; gap: 8px; margin-top: 20px; font-size: 12px; color: var(--home-navy); }
.home-model-section { display: grid; grid-template-columns: minmax(0,1fr) minmax(0,1.1fr); gap: 70px; padding: 46px; background: var(--home-soft); border-radius: 18px; }
.home-model-intro > p { margin-block: 18px 24px; max-width: 340px; font-size: 14px; line-height: 1.9; color: var(--console-muted); }
.home-model-intro > .home-model-disclaimer { font-size: 12px; margin-bottom: 0; }
.home-model-directory { min-width: 0; }
.home-model-row { display: flex; align-items: center; gap: 16px; padding: 18px 0; border-bottom: 1px solid var(--console-line); }
.home-model-row:first-child { padding-top: 0; }
.home-model-row > div { min-width: 0; }
.home-model-row h3 { font-size: 19px; font-weight: 600; }
.home-model-row p { margin-top: 5px; color: var(--console-muted); font-size: 12px; line-height: 1.7; }
.home-model-directory > p { margin-top: 18px; color: var(--console-muted); font-size: 12px; line-height: 1.8; }
.provider-mark { display: inline-flex; flex-shrink: 0; width: 44px; height: 44px; align-items: center; justify-content: center; border: 1px solid var(--console-line); border-radius: 12px; background: var(--console-surface); font-size: 20px; font-weight: 600; }
.provider-mark-claude { color: #a5553d; }
.provider-mark-gpt { color: #27857a; }
.provider-mark-gemini { color: #42799b; }
.home-connect { display: grid; grid-template-columns: minmax(0,.9fr) minmax(0,1.1fr); align-items: start; gap: 80px; padding-block: 90px; }
.home-connect-guide > p { margin-top: 18px; max-width: 390px; color: var(--console-muted); font-size: 14px; line-height: 1.9; }
.home-steps { counter-reset: step; margin-top: 30px; display: grid; gap: 26px; list-style: none; }
.home-steps li { position: relative; padding-left: 44px; counter-increment: step; }
.home-steps li::before { content: counter(step); position: absolute; left: 0; top: 0; display: grid; place-items: center; width: 26px; height: 26px; border: 1px solid var(--console-line); border-radius: 50%; color: var(--home-navy); font-size: 12px; }
.home-steps h3 { font-size: 15px; font-weight: 600; line-height: 26px; }
.home-steps p { margin-top: 6px; color: var(--console-muted); font-size: 13px; line-height: 1.8; }
.home-text-link { display: inline-flex; align-items: center; gap: 8px; margin-top: 26px; color: var(--home-navy); font-size: 13px; }
.home-example { min-width: 0; overflow: hidden; border: 1px solid var(--console-line); border-radius: 12px; background: var(--console-bg); }
.home-example-heading { display: flex; align-items: center; gap: 9px; padding: 20px 24px; background: var(--console-surface); }
.home-example-heading > svg { color: var(--home-navy); }
.home-example-heading h3 { font-size: 14px; font-weight: 600; }
.home-example-heading > span { margin-left: auto; font-size: 12px; color: var(--console-muted); }
.home-protocols { display: flex; gap: 20px; padding-inline: 24px; border-bottom: 1px solid var(--console-line); background: var(--console-surface); overflow-x: auto; overflow-y: hidden; }
.home-protocols button { flex-shrink: 0; padding: 10px 0 13px; border-bottom: 2px solid transparent; color: var(--console-muted); font-size: 12px; }
.home-protocols button[aria-pressed='true'] { border-color: var(--home-blue); color: var(--home-navy); font-weight: 600; }
.home-code { min-height: 306px; max-height: 390px; margin: 0; padding: 24px; overflow: auto; color: var(--console-ink); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; line-height: 1.85; tab-size: 2; }
.home-example-note { display: flex; align-items: flex-start; gap: 8px; padding: 14px 24px; border-top: 1px solid var(--console-line); color: var(--console-muted); background: var(--console-surface); font-size: 12px; line-height: 1.7; }
.home-example-note svg { flex-shrink: 0; margin-top: 2px; }
.home-faq { display: grid; grid-template-columns: minmax(0,.65fr) minmax(0,1fr); gap: 100px; padding-block: 46px 72px; border-top: 1px solid var(--console-line); }
.home-faq > div > p { margin-top: 14px; color: var(--console-muted); font-size: 14px; line-height: 1.8; }
.home-faq-list details { border-bottom: 1px solid var(--console-line); }
.home-faq-list summary { display: flex; justify-content: space-between; align-items: center; gap: 24px; padding-block: 20px; cursor: pointer; font-size: 15px; line-height: 1.6; list-style: none; }
.home-faq-list summary::-webkit-details-marker { display: none; }
.home-faq-list summary > span { color: var(--console-muted); font-size: 22px; }
.home-faq-list details[open] summary > span { transform: rotate(45deg); }
.home-faq-list details > p { max-width: 560px; padding: 0 28px 20px 0; color: var(--console-muted); font-size: 13px; line-height: 1.9; }
.home-closing { display: flex; justify-content: space-between; align-items: center; gap: 32px; padding: 40px 46px; margin-bottom: 60px; background: var(--home-soft); border-radius: 14px; }
.home-closing h2 { font-size: 27px; }
.home-closing p { margin-top: 10px; font-size: 14px; color: var(--console-muted); line-height: 1.8; }
.home-closing > a { flex-shrink: 0; }
.home-footer { display: flex; align-items: center; gap: 28px; padding-block: 28px 34px; border-top: 1px solid var(--console-line); color: var(--console-muted); font-size: 12px; }
.home-footer > div { display: flex; align-items: center; gap: 22px; margin-right: auto; }
.home-footer strong { color: var(--console-ink); font-size: 16px; }
@media (max-width: 1050px) {
  .home-section-nav { margin-left: 0; gap: 18px; }
  .home-hero { gap: 36px; padding-block: 60px; }
  .home-capabilities { gap: 28px; }
  .home-model-section, .home-connect, .home-faq { gap: 36px; }
}
@media (max-width: 800px) {
  .home-nav, .home-hero, .home-section, .home-footer { width: calc(100% - 40px); }
  .home-section-nav { display: none; }
  .home-nav { min-height: 70px; gap: 12px; }
  .home-hero { grid-template-columns: minmax(0,1fr); gap: 40px; padding-block: 46px; }
  .home-hero-content > .home-hero-tagline { font-size: clamp(29px, 5.5vw, 44px); }
  .home-hero h1 { margin-bottom: 20px; font-size: 21px; }
  .home-hero-stage .home-hero { gap: 36px; min-height: 0; padding-block: 48px 36px; }
  .home-hero-stage .home-hero-tagline { font-size: clamp(34px, 6.3vw, 48px); }
  .home-navigation-demo { max-width: 480px; }
  .home-hero-bottom { width: calc(100% - 40px); flex-wrap: wrap; padding-block: 20px; }
  .home-hero-bottom > span:last-child { flex-basis: 100%; }
  .home-section-heading { display: block; }
  .home-section-heading > p { margin-top: 16px; }
  .home-standards { padding-block: 38px 46px; }
  .home-capabilities { grid-template-columns: minmax(0,1fr); gap: 30px; margin-top: 30px; }
  .home-capabilities article { position: relative; padding-left: 62px; }
  .home-feature-icon { position: absolute; top: 0; left: 0; }
  .home-capabilities p { min-height: 0; }
  .home-capabilities article > a, .home-feature-note { margin-top: 12px; }
  .home-model-section { grid-template-columns: minmax(0,1fr); padding: 28px; }
  .home-model-intro > p { max-width: none; }
  .home-connect { grid-template-columns: minmax(0,1fr); padding-block: 48px; }
  .home-faq { grid-template-columns: minmax(0,1fr); gap: 18px; padding-block: 32px 44px; }
  .home-closing { display: block; padding: 28px; margin-bottom: 36px; }
  .home-closing > a { margin-top: 24px; }
  .home-footer > div { flex-direction: column; align-items: flex-start; gap: 8px; }
}
@media (max-width: 440px) {
  .home-brand span { max-width: 94px; font-size: 15px; }
  .home-brand { gap: 7px; }
  .home-brand-logo { width: 28px; height: 28px; }
  .home-actions { gap: 0; }
  .home-icon-button { width: 32px; }
  .home-nav-cta { padding: 0 10px; gap: 5px; min-height: 36px; font-size: 12px; }
  .home-nav-cta > svg { display: none; }
  .home-hero-content > p { font-size: 14px; }
  .home-route-node { min-width: 85px; min-height: 44px; padding: 8px 10px; font-size: 12px; gap: 6px; }
  .home-route-detail { margin-inline: 0; padding: 18px; }
  .home-compass-core { gap: 6px; }
  .home-model-section { padding: 24px; }
  .home-protocols { gap: 16px; padding-inline: 16px; }
  .home-code { padding: 18px 16px; font-size: 11px; }
  .home-example-heading { padding: 16px; }
  .home-example-note { padding: 12px 16px; }
  .home-section h2 { font-size: 25px; }
  .home-footer { gap: 16px; }
}
/* Motion follows visibility; content remains readable without observers. */
.home-header { position: sticky; top: 0; z-index: 40; background: var(--console-surface); }
.home-section { scroll-margin-top: 108px; }
.home-section-nav a { position: relative; padding-block: 12px; }
.home-section-nav a::after { content: ""; position: absolute; inset: auto 0 4px; height: 2px; background: var(--home-blue); transform: scaleX(0); transform-origin: left; transition: transform 240ms ease; }
.home-section-nav a:is(:hover, :focus-visible)::after { transform: scaleX(1); }
.home-primary-action, .home-secondary-action, .home-nav-cta { transition: transform 200ms ease, box-shadow 200ms ease, background 200ms ease; }
.home-primary-action:hover, .home-secondary-action:hover { transform: translateY(-3px); box-shadow: 0 8px 20px rgb(32 62 84 / 15%); }
.home-primary-action:active, .home-secondary-action:active { transform: translateY(0); }
.home-primary-action > svg { transition: transform 220ms ease; }
.home-primary-action:hover > svg { transform: translateX(4px); }
.home-motion-toggle[aria-pressed='true'] { background: var(--home-soft); color: var(--home-navy); }
.hero-line { display: block; overflow: hidden; padding-block: 2px; }
.hero-line > span { display: block; animation: hero-line-enter 900ms cubic-bezier(.16,1,.3,1) both; animation-delay: calc(100ms + var(--line-index) * 150ms); }
.home-hero-content h1 { animation: content-enter 650ms ease both; }
.home-hero-content > p:not(.home-hero-tagline), .home-hero-actions { animation: content-enter 800ms 350ms ease both; }
.home-navigation-demo { animation: instrument-enter 1100ms cubic-bezier(.16,1,.3,1) both; }
.home-hero-stage::before { animation: sea-light 12s ease-in-out infinite alternate; }
.home-compass-core::after { content: ""; position: absolute; inset: -13px; border: 1px solid #83b1cf; border-radius: 50%; pointer-events: none; animation: core-pulse 4s ease-out infinite; }
.chart-ticks { transform-origin: 250px 250px; animation: compass-turn 100s linear infinite; }
.chart-sweep { stroke: #acdafa; stroke-width: 2; stroke-dasharray: 70 1313; stroke-linecap: round; transform-origin: 250px 250px; animation: compass-turn 18s linear infinite; }
.chart-packet { stroke: #fff; stroke-width: 4; stroke-linecap: round; stroke-dasharray: 6 94; animation: packet-travel 2.6s linear infinite; }
.home-hero-bottom a > span { animation: follow-down 2.4s ease-in-out infinite; }
.home-route-detail strong { display: inline-block; }
.motion-ready .motion-item { opacity: 0; transform: translateY(26px); transition: opacity 700ms ease, transform 850ms cubic-bezier(.16,1,.3,1); transition-delay: var(--reveal-delay, 0ms); }
.motion-ready .home-model-row.motion-item { transform: translateX(36px); }
.motion-ready .home-example.motion-item { transform: perspective(1200px) rotateY(-7deg) translateY(24px); transform-origin: left center; }
.motion-ready .motion-item.is-revealed, .motion-ready .motion-item:focus-within { opacity: 1; transform: none; }
.home-feature-icon, .provider-mark { transition: transform 350ms cubic-bezier(.2,.8,.2,1), box-shadow 350ms ease; }
.home-capabilities article:hover .home-feature-icon { transform: translateY(-5px) rotate(-8deg); box-shadow: 0 9px 18px rgb(32 62 84 / 12%); }
.home-capabilities article > a svg { transition: transform 250ms ease; }
.home-capabilities article > a:hover svg { transform: translateX(5px); }
.home-model-section { position: relative; isolation: isolate; overflow: hidden; }
.home-model-section::before, .home-model-section::after { content: ""; position: absolute; z-index: -1; width: 440px; height: 440px; left: -180px; bottom: -290px; border: 1px solid rgb(77 126 160 / 24%); border-radius: 50%; pointer-events: none; }
.home-model-section::after { width: 370px; height: 370px; left: -145px; bottom: -255px; border-style: dashed; animation: compass-turn 75s linear infinite; }
.home-model-row { position: relative; }
.home-model-row::after { content: ""; position: absolute; left: 0; right: 0; bottom: -1px; height: 1px; background: var(--home-blue); transform: scaleX(0); transform-origin: left; transition: transform 400ms ease; }
.home-model-row:hover::after { transform: scaleX(1); }
.home-model-row:hover .provider-mark { transform: rotate(-7deg) scale(1.08); box-shadow: 0 7px 18px rgb(32 62 84 / 12%); }
.home-steps li:not(:last-child)::after { content: ""; position: absolute; top: 32px; left: 12px; bottom: -20px; width: 1px; background: var(--console-line); transform-origin: top; transform: scaleY(0); transition: transform 650ms 250ms ease; }
.home-steps li.is-revealed::after { transform: scaleY(1); }
.home-steps li::before { transition: background 250ms, color 250ms, border-color 250ms; }
.home-steps li:hover::before { background: var(--home-blue); border-color: var(--home-blue); color: #fff; }
.home-example { position: relative; }
.home-example-heading::after { content: ""; width: 6px; height: 6px; border-radius: 50%; background: var(--home-blue); animation: terminal-pulse 3s ease-in-out infinite; }
.code-line { animation: code-line-enter 550ms ease both; animation-delay: var(--code-delay); }
.home-protocols button { position: relative; transition: color 180ms, background 180ms; }
.home-protocols button::after { content: ""; position: absolute; bottom: -2px; left: 0; width: 100%; height: 2px; background: var(--home-blue); transform: scaleX(0); transform-origin: left; transition: transform 300ms ease; }
.home-protocols button[aria-pressed='true'] { border-bottom-color: transparent; }
.home-protocols button[aria-pressed='true']::after { transform: scaleX(1); }
.home-faq-list summary { transition: color 200ms ease, padding-left 200ms ease; }
.home-faq-list summary:hover { color: var(--home-navy); padding-left: 6px; }
.home-faq-list summary > span { transition: transform 220ms ease; }
.home-faq-list details[open] > p { animation: answer-enter 320ms ease both; }
.home-closing { position: relative; isolation: isolate; overflow: hidden; }
.home-closing::before { content: ""; position: absolute; z-index: -1; top: -100%; left: -30%; width: 35%; height: 300%; background: linear-gradient(90deg, transparent, rgb(127 174 205 / 13%), transparent); transform: rotate(22deg); animation: closing-light 9s ease-in-out infinite; pointer-events: none; }
.home-footer a { transition: color 200ms; }
.home-footer strong { display: inline-block; transition: letter-spacing 300ms ease; }
.home-footer strong:hover { letter-spacing: .06em; }
.motion-ready .home-hero-stage:not(.is-in-view), .motion-ready .home-hero-stage:not(.is-in-view) *, .motion-ready .home-hero-stage:not(.is-in-view)::before, .motion-ready .home-hero-stage:not(.is-in-view) *::after,
.motion-ready .home-model-section:not(.is-in-view)::after, .motion-ready .home-example:not(.is-in-view) *, .motion-ready .home-example:not(.is-in-view) *::after, .motion-ready .home-closing:not(.is-in-view)::before,
.page-hidden *, .page-hidden *::before, .page-hidden *::after { animation-play-state: paused !important; }
.motion-paused *, .motion-paused *::before, .motion-paused *::after { animation: none !important; transition: none !important; }
.motion-paused .motion-item { opacity: 1 !important; transform: none !important; }
.motion-paused .home-steps li::after { transform: scaleY(1); }
@keyframes hero-line-enter { from { transform: translateY(108%); } to { transform: translateY(0); } }
@keyframes content-enter { from { opacity: 0; transform: translateY(14px); } to { opacity: 1; transform: translateY(0); } }
@keyframes instrument-enter { from { opacity: 0; transform: scale(.93) rotate(-5deg); } to { opacity: 1; transform: scale(1) rotate(0); } }
@keyframes sea-light { from { transform: translate(-35px, 20px) scale(.9); opacity: .7; } to { transform: translate(30px, -20px) scale(1.1); opacity: 1; } }
@keyframes compass-turn { to { transform: rotate(360deg); } }
@keyframes core-pulse { from { opacity: .5; transform: scale(.95); } to { opacity: 0; transform: scale(1.25); } }
@keyframes packet-travel { from { stroke-dashoffset: 100; } to { stroke-dashoffset: 0; } }
@keyframes follow-down { 0%, 100% { transform: translateY(-3px); } 50% { transform: translateY(4px); } }
@keyframes terminal-pulse { 0%, 100% { opacity: .45; } 50% { opacity: 1; } }
@keyframes code-line-enter { from { opacity: 0; } to { opacity: 1; } }
@keyframes answer-enter { from { opacity: 0; transform: translateY(-6px); } to { opacity: 1; transform: translateY(0); } }
@keyframes closing-light { 0%, 15% { transform: translateX(0) rotate(22deg); } 65%, 100% { transform: translateX(480%) rotate(22deg); } }
@media (prefers-reduced-motion: reduce) {
  .home-shell *, .home-shell *::before, .home-shell *::after { animation: none !important; transition: none !important; }
  .motion-ready .motion-item { opacity: 1 !important; transform: none !important; }
  .home-steps li::after { transform: scaleY(1); }
  .home-motion-toggle { display: none; }
}
@media print {
  .home-shell *, .home-shell *::before, .home-shell *::after { animation: none !important; transition: none !important; }
  .motion-ready .motion-item { opacity: 1 !important; transform: none !important; }
}
</style>
