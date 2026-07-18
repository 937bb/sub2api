<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="homeContent" class="min-h-screen">
    <!-- iframe mode -->
    <iframe
      v-if="homeContentUrl"
      :src="homeContentUrl"
      class="h-screen w-full border-0"
      sandbox="allow-scripts"
      referrerpolicy="strict-origin-when-cross-origin"
      allowfullscreen
    ></iframe>
    <div v-else v-html="sanitizedHomeContent"></div>
  </div>

  <!-- Default Home Page -->
  <div
    v-else
    class="home-shell relative flex min-h-screen flex-col overflow-hidden bg-gradient-to-b from-gray-50 to-gray-100 dark:from-[#0c0d11] dark:to-[#08090c]"
  >
    <!-- Background Decorations:中性网格 + 单点克制蓝辉光 -->
    <div class="pointer-events-none absolute inset-0 overflow-hidden">
      <div
        class="absolute inset-0 bg-[linear-gradient(rgba(0,0,0,0.04)_1px,transparent_1px),linear-gradient(90deg,rgba(0,0,0,0.04)_1px,transparent_1px)] bg-[size:56px_56px] dark:bg-[linear-gradient(rgba(255,255,255,0.035)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.035)_1px,transparent_1px)]"
        style="mask-image: radial-gradient(120% 90% at 50% 0%, #000 0%, transparent 70%); -webkit-mask-image: radial-gradient(120% 90% at 50% 0%, #000 0%, transparent 70%);"
      ></div>
      <div
        class="absolute -top-48 -right-32 h-[36rem] w-[36rem] rounded-full bg-primary-500/10 blur-3xl dark:bg-primary-500/15"
      ></div>
      <div
        class="absolute -bottom-56 left-1/4 h-[30rem] w-[30rem] rounded-full bg-primary-500/[0.07] blur-3xl dark:bg-primary-500/10"
      ></div>
    </div>

    <!-- Sticky glass pill nav -->
    <header class="relative z-20 px-4 pt-4 sm:px-6">
      <nav class="mx-auto flex max-w-6xl items-center justify-between rounded-2xl border border-[var(--glass-border)] bg-[var(--glass-bg-float)] px-4 py-2.5 shadow-[inset_0_0.5px_0_0_rgba(255,255,255,0.6),0_16px_40px_-28px_rgba(0,0,0,0.4)] backdrop-blur-xl sm:px-5">
        <!-- Logo + name -->
        <div class="flex items-center gap-2.5">
          <div class="h-9 w-9 overflow-hidden rounded-xl shadow-sm ring-1 ring-black/5 dark:ring-white/10">
            <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <span class="font-display text-base font-bold tracking-tight text-gray-900 dark:text-white">{{ siteName }}</span>
        </div>

        <!-- Nav Actions -->
        <div class="flex items-center gap-1.5 sm:gap-2">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="hidden rounded-lg p-2 text-gray-500 transition-colors hover:bg-black/5 hover:text-gray-800 dark:text-dark-300 dark:hover:bg-white/10 dark:hover:text-white sm:block"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>
          <button
            @click="toggleTheme"
            class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-black/5 hover:text-gray-800 dark:text-dark-300 dark:hover:bg-white/10 dark:hover:text-white"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>
          <router-link
            v-if="isAuthenticated"
            :to="dashboardPath"
            class="inline-flex items-center gap-1.5 rounded-full bg-gray-900 py-1 pl-1 pr-3 transition-colors hover:bg-gray-800 dark:bg-white dark:hover:bg-gray-100"
          >
            <span class="flex h-5 w-5 items-center justify-center rounded-full bg-gradient-to-br from-primary-400 to-primary-600 text-[10px] font-semibold text-white">
              {{ userInitial }}
            </span>
            <span class="text-xs font-medium text-white dark:text-gray-900">{{ t('home.dashboard') }}</span>
          </router-link>
          <router-link
            v-else
            to="/login"
            class="inline-flex items-center rounded-full bg-gray-900 px-4 py-1.5 text-xs font-medium text-white transition-colors hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-100"
          >
            {{ t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <!-- Main Content -->
    <main class="relative z-10 flex-1 px-4 sm:px-6">
      <div class="mx-auto max-w-6xl">
        <!-- Hero:非对称编辑式布局 -->
        <section class="grid grid-cols-1 items-center gap-10 py-16 lg:grid-cols-[1.05fr_0.95fr] lg:gap-14 lg:py-24">
          <!-- Left: editorial text -->
          <div class="reveal text-center lg:text-left" style="--d: 0ms">
            <div class="mb-6 inline-flex items-center gap-2 rounded-full border border-[var(--glass-border)] bg-[var(--glass-bg-float)] px-3.5 py-1.5 text-xs font-medium text-gray-600 shadow-sm backdrop-blur-md dark:text-dark-200">
              <span class="h-1.5 w-1.5 rounded-full bg-primary-500 shadow-[0_0_8px_rgba(10,132,255,0.9)]"></span>
              AI API Gateway
            </div>
            <h1 class="font-display text-5xl font-extrabold leading-[0.98] tracking-tight text-gray-900 dark:text-white md:text-6xl lg:text-7xl">
              {{ siteName }}
            </h1>
            <p class="mx-auto mt-6 max-w-lg text-lg leading-relaxed text-gray-600 dark:text-dark-300 md:text-xl lg:mx-0">
              {{ siteSubtitle }}
            </p>

            <!-- CTA row -->
            <div class="mt-9 flex flex-wrap items-center justify-center gap-3 lg:justify-start">
              <router-link
                :to="isAuthenticated ? dashboardPath : '/login'"
                class="btn btn-primary px-7 py-3 text-base shadow-lg shadow-primary-500/30"
              >
                {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
                <Icon name="arrowRight" size="md" class="ml-2" :stroke-width="2" />
              </router-link>
              <a
                v-if="docUrl"
                :href="docUrl"
                target="_blank"
                rel="noopener noreferrer"
                class="inline-flex items-center gap-2 rounded-xl border border-[var(--glass-border)] bg-[var(--glass-bg-float)] px-6 py-3 text-base font-medium text-gray-700 shadow-sm backdrop-blur-md transition-colors hover:text-gray-900 dark:text-dark-200 dark:hover:text-white"
              >
                <Icon name="book" size="md" />
                {{ t('home.docs') }}
              </a>
            </div>

            <!-- Inline provider trust strip -->
            <div class="mt-10 flex flex-col items-center gap-3 lg:items-start">
              <span class="text-xs font-medium uppercase tracking-[0.16em] text-gray-400 dark:text-dark-400">
                {{ t('home.providers.title') }}
              </span>
              <div class="flex flex-wrap items-center justify-center gap-2.5 lg:justify-start">
                <span
                  v-for="p in heroProviders"
                  :key="p.key"
                  class="inline-flex items-center gap-2 rounded-full border border-[var(--glass-border)] bg-[var(--glass-bg-float)] py-1 pl-1 pr-3 text-xs font-medium text-gray-700 shadow-sm backdrop-blur-md dark:text-dark-200"
                >
                  <span :class="['flex h-6 w-6 items-center justify-center rounded-full text-white', p.color]">
                    <img v-if="p.img" :src="p.img" :alt="p.name" class="h-4 w-4" />
                    <svg v-else-if="p.path" viewBox="0 0 24 24" fill="currentColor" class="h-3.5 w-3.5"><path :d="p.path" /></svg>
                    <span v-else class="text-[11px] font-bold">{{ p.letter }}</span>
                  </span>
                  {{ p.name }}
                </span>
              </div>
            </div>
          </div>

          <!-- Right: terminal visual -->
          <div class="reveal flex justify-center lg:justify-end" style="--d: 120ms">
            <div class="terminal-container">
              <div class="terminal-window">
                <div class="terminal-header">
                  <div class="terminal-buttons">
                    <span class="btn-close"></span>
                    <span class="btn-minimize"></span>
                    <span class="btn-maximize"></span>
                  </div>
                  <span class="terminal-title">terminal</span>
                </div>
                <div class="terminal-body">
                  <div class="code-line line-1">
                    <span class="code-prompt">$</span>
                    <span class="code-cmd">curl</span>
                    <span class="code-flag">-X POST</span>
                    <span class="code-url">/v1/messages</span>
                  </div>
                  <div class="code-line line-2">
                    <span class="code-comment"># Routing to upstream...</span>
                  </div>
                  <div class="code-line line-3">
                    <span class="code-success">200 OK</span>
                    <span class="code-response">{ "content": "Hello!" }</span>
                  </div>
                  <div class="code-line line-4">
                    <span class="code-prompt">$</span>
                    <span class="cursor"></span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </section>

        <!-- Bento feature grid:大小错落,避免三卡雷同 -->
        <section class="reveal grid grid-cols-1 gap-4 pb-4 sm:grid-cols-2 lg:grid-cols-3 lg:auto-rows-[minmax(0,1fr)]" style="--d: 200ms">
          <!-- Feature 1: Unified Gateway (wide highlight) -->
          <article class="bento-cell group relative overflow-hidden sm:col-span-2">
            <div class="pointer-events-none absolute -right-16 -top-16 h-56 w-56 rounded-full bg-primary-500/10 blur-2xl transition-opacity duration-500 group-hover:opacity-80"></div>
            <div class="relative flex h-full flex-col">
              <div class="mb-5 flex h-12 w-12 items-center justify-center rounded-2xl bg-gradient-to-br from-primary-500 to-primary-600 shadow-lg shadow-primary-500/30">
                <Icon name="server" size="lg" class="text-white" />
              </div>
              <h3 class="font-display text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
                {{ t('home.features.unifiedGateway') }}
              </h3>
              <p class="mt-2 max-w-md text-sm leading-relaxed text-gray-600 dark:text-dark-300">
                {{ t('home.features.unifiedGatewayDesc') }}
              </p>
              <div class="mt-auto pt-6">
                <div class="inline-flex items-center gap-2 rounded-lg border border-[var(--glass-border)] bg-black/[0.03] px-3 py-1.5 font-mono text-xs text-gray-500 dark:bg-white/5 dark:text-dark-300">
                  <span class="text-primary-500">POST</span>
                  <span>/v1/messages</span>
                </div>
              </div>
            </div>
          </article>

          <!-- Vertical capability rail (tall) -->
          <aside class="bento-cell lg:row-span-2">
            <div class="flex h-full flex-col justify-center gap-5">
              <div v-for="cap in capabilityList" :key="cap.key" class="flex items-start gap-3">
                <div class="mt-0.5 flex h-9 w-9 flex-none items-center justify-center rounded-xl bg-primary-500/10 text-primary-500 dark:bg-primary-500/15">
                  <Icon :name="(cap.icon as any)" size="md" :stroke-width="2" />
                </div>
                <div>
                  <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ cap.label }}</p>
                </div>
              </div>
            </div>
          </aside>

          <!-- Feature 2: Account Pool -->
          <article class="bento-cell group">
            <div class="mb-4 flex h-11 w-11 items-center justify-center rounded-2xl bg-gradient-to-br from-primary-500 to-primary-600 shadow-lg shadow-primary-500/25">
              <Icon name="users" size="lg" class="text-white" :stroke-width="2" />
            </div>
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('home.features.multiAccount') }}
            </h3>
            <p class="mt-2 text-sm leading-relaxed text-gray-600 dark:text-dark-300">
              {{ t('home.features.multiAccountDesc') }}
            </p>
          </article>

          <!-- Feature 3: Billing & Quota -->
          <article class="bento-cell group">
            <div class="mb-4 flex h-11 w-11 items-center justify-center rounded-2xl bg-gradient-to-br from-primary-500 to-primary-600 shadow-lg shadow-primary-500/25">
              <Icon name="dollar" size="lg" class="text-white" :stroke-width="2" />
            </div>
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('home.features.balanceQuota') }}
            </h3>
            <p class="mt-2 text-sm leading-relaxed text-gray-600 dark:text-dark-300">
              {{ t('home.features.balanceQuotaDesc') }}
            </p>
          </article>
        </section>

        <!-- Providers strip -->
        <section class="reveal py-14" style="--d: 280ms">
          <div class="rounded-3xl border border-[var(--glass-border)] bg-[var(--glass-bg-content)] p-8 shadow-sm backdrop-blur-md sm:p-10">
            <div class="flex flex-col gap-6 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <h2 class="font-display text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
                  {{ t('home.providers.title') }}
                </h2>
                <p class="mt-1.5 text-sm text-gray-500 dark:text-dark-300">
                  {{ t('home.providers.description') }}
                </p>
              </div>
              <div class="flex flex-wrap items-center gap-2.5">
                <div
                  v-for="prov in providerList"
                  :key="prov.key"
                  :class="[
                    'flex items-center gap-2 rounded-xl border px-4 py-2.5 backdrop-blur-sm',
                    prov.soon
                      ? 'border-gray-200/60 bg-[var(--glass-bg-float)] opacity-60 dark:border-dark-700/60'
                      : 'border-primary-500/20 bg-[var(--glass-bg-float)] ring-1 ring-primary-500/10'
                  ]"
                >
                  <span :class="['flex h-7 w-7 items-center justify-center rounded-lg text-white', prov.color]">
                    <img v-if="prov.img" :src="prov.img" :alt="prov.name" class="h-5 w-5" />
                    <svg v-else-if="prov.path" viewBox="0 0 24 24" fill="currentColor" class="h-4 w-4"><path :d="prov.path" /></svg>
                    <span v-else class="text-[11px] font-bold">{{ prov.letter }}</span>
                  </span>
                  <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ prov.name }}</span>
                  <span
                    :class="[
                      'rounded px-1.5 py-0.5 text-[10px] font-medium',
                      prov.soon
                        ? 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-dark-300'
                        : 'bg-primary-100 text-primary-600 dark:bg-primary-900/30 dark:text-primary-400'
                    ]"
                  >{{ prov.soon ? t('home.providers.soon') : t('home.providers.supported') }}</span>
                </div>
              </div>
            </div>
          </div>
        </section>
      </div>
    </main>

    <!-- Footer -->
    <footer class="relative z-10 border-t border-gray-200/50 px-6 py-8 dark:border-dark-800/50">
      <div class="mx-auto flex max-w-6xl flex-col items-center justify-center gap-4 text-center sm:flex-row sm:justify-between sm:text-left">
        <p class="text-sm text-gray-500 dark:text-dark-400">
          &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
        </p>
        <div v-if="docUrl" class="flex items-center gap-4">
          <a
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-dark-400 dark:hover:text-white"
          >
            {{ t('home.docs') }}
          </a>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import { sanitizeHtml } from '@/utils/sanitize'
import { sanitizeUrl } from '@/utils/url'

const { t } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

// Site settings - directly from appStore (already initialized from injected config)
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', {
    allowRelative: true,
    allowDataUrl: true
  })
)
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'AI API Gateway Platform')
const docUrl = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
)
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const homeContentUrl = computed(() => sanitizeUrl(homeContent.value))
const sanitizedHomeContent = computed(() => sanitizeHtml(homeContent.value))

// Theme
const isDark = ref(document.documentElement.classList.contains('dark'))

// Auth state
const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')
const userInitial = computed(() => {
  const user = authStore.user
  if (!user || !user.email) return ''
  return user.email.charAt(0).toUpperCase()
})

// Current year for footer
const currentYear = computed(() => new Date().getFullYear())

// Official provider brand marks (rendered white inside the brand-colored tile,
// so contrast holds in both light and dark themes).
const ANTHROPIC_LOGO = 'M17.3041 3.541h-3.6718l6.696 16.918H24Zm-10.6082 0L0 20.459h3.7442l1.3693-3.5527h7.0052l1.3693 3.5528h3.7442L10.5363 3.541Zm-.3712 10.223 2.2914-5.9456 2.2914 5.9456Z'
const OPENAI_LOGO = 'M22.2819 9.8211a5.9847 5.9847 0 0 0-.5157-4.9108 6.0462 6.0462 0 0 0-6.5098-2.9A6.0651 6.0651 0 0 0 4.9807 4.1818a5.9847 5.9847 0 0 0-3.9977 2.9 6.0462 6.0462 0 0 0 .7427 7.0966 5.98 5.98 0 0 0 .511 4.9107 6.051 6.051 0 0 0 6.5146 2.9001A5.9847 5.9847 0 0 0 13.2599 24a6.0557 6.0557 0 0 0 5.7718-4.2058 5.9894 5.9894 0 0 0 3.9977-2.9001 6.0557 6.0557 0 0 0-.7475-7.0729zm-9.022 12.6081a4.4755 4.4755 0 0 1-2.8764-1.0408l.1419-.0804 4.7783-2.7582a.7948.7948 0 0 0 .3927-.6813v-6.7369l2.02 1.1686a.071.071 0 0 1 .038.052v5.5826a4.504 4.504 0 0 1-4.4945 4.4944zm-9.6607-4.1254a4.4708 4.4708 0 0 1-.5346-3.0137l.1419.0852 4.783 2.7582a.7712.7712 0 0 0 .7806 0l5.8428-3.3685v2.3324a.0804.0804 0 0 1-.0332.0615L9.74 19.9502a4.4992 4.4992 0 0 1-6.1408-1.6464zM2.3408 7.8956a4.485 4.485 0 0 1 2.3655-1.9728V11.6a.7664.7664 0 0 0 .3879.6765l5.8144 3.3543-2.0201 1.1685a.0757.0757 0 0 1-.071 0l-4.8303-2.7865A4.504 4.504 0 0 1 2.3408 7.872zm16.5963 3.8558L13.1038 8.364 15.1192 7.2a.0757.0757 0 0 1 .071 0l4.8303 2.7913a4.4944 4.4944 0 0 1-.6765 8.1042v-5.6772a.79.79 0 0 0-.407-.667zm2.0107-3.0231l-.142-.0852-4.7735-2.7818a.7759.7759 0 0 0-.7854 0L9.409 9.2297V6.8974a.0662.0662 0 0 1 .0284-.0615l4.8303-2.7866a4.4992 4.4992 0 0 1 6.6802 4.66zM8.3065 12.863l-2.02-1.1638a.0804.0804 0 0 1-.038-.0567V6.0742a4.4992 4.4992 0 0 1 7.3757-3.4537l-.142.0805L8.704 5.459a.7948.7948 0 0 0-.3927.6813zm1.0976-2.3654l2.602-1.4998 2.6069 1.4998v2.9994l-2.5974 1.4997-2.6067-1.4997Z'
const GEMINI_LOGO = 'M12 24A14.304 14.304 0 0 0 0 12 14.304 14.304 0 0 0 12 0a14.305 14.305 0 0 0 12 12 14.305 14.305 0 0 0-12 12'

// Unified provider list; `path` renders the real logo, `letter` is a fallback.
const providerList = computed(() => [
  { key: 'claude', name: t('home.providers.claude'), color: 'bg-gradient-to-br from-[#CC785C] to-[#B4593C]', path: ANTHROPIC_LOGO, soon: false },
  { key: 'gpt', name: 'GPT', color: 'bg-gradient-to-br from-[#10A37F] to-[#0B7D62]', path: OPENAI_LOGO, soon: false },
  { key: 'gemini', name: t('home.providers.gemini'), color: 'bg-gradient-to-br from-[#4E8CFF] to-[#1A73E8]', path: GEMINI_LOGO, soon: false },
  { key: 'antigravity', name: t('home.providers.antigravity'), color: 'bg-white ring-1 ring-black/10 dark:ring-white/15', img: '/logos/antigravity.svg', soon: false },
  { key: 'more', name: t('home.providers.more'), color: 'bg-gradient-to-br from-gray-400 to-gray-500', letter: '+', soon: true }
])

// Hero trust strip shows only the live providers (no "coming soon").
const heroProviders = computed(() => providerList.value.filter((p) => !p.soon))

// Capability rail (bento vertical cell)
const capabilityList = computed(() => [
  { key: 'sub', icon: 'swap', label: t('home.tags.subscriptionToApi') },
  { key: 'session', icon: 'shield', label: t('home.tags.stickySession') },
  { key: 'billing', icon: 'chart', label: t('home.tags.realtimeBilling') }
])

// Toggle theme
function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

// Initialize theme
function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  ) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

onMounted(() => {
  initTheme()

  // Check auth state
  authStore.checkAuth()

  // Ensure public settings are loaded (will use cache if already loaded from injected config)
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
/* Bento glass cells */
.bento-cell {
  background-color: var(--glass-bg-content);
  border: 1px solid var(--glass-border);
  box-shadow: var(--glass-highlight);
  border-radius: 1.5rem;
  padding: 1.75rem;
  backdrop-filter: var(--glass-filter-sm);
  -webkit-backdrop-filter: var(--glass-filter-sm);
  transition: box-shadow 0.35s ease, transform 0.35s ease;
}

.bento-cell:hover {
  transform: translateY(-3px);
  box-shadow: var(--glass-highlight), 0 26px 55px -30px rgba(0, 0, 0, 0.4);
}

/* Staggered entrance */
.reveal {
  opacity: 0;
  transform: translateY(14px);
  animation: reveal-up 0.6s cubic-bezier(0.22, 1, 0.36, 1) forwards;
  animation-delay: var(--d, 0ms);
}

@keyframes reveal-up {
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (prefers-reduced-motion: reduce) {
  .reveal {
    animation: none;
    opacity: 1;
    transform: none;
  }
}

/* Terminal Container */
.terminal-container {
  position: relative;
  display: inline-block;
}

/* Terminal Window */
.terminal-window {
  width: 420px;
  max-width: 100%;
  background: linear-gradient(145deg, #27272a 0%, #18181b 100%);
  border-radius: 14px;
  box-shadow:
    0 25px 50px -12px rgba(0,0,0, 0.4),
    0 0 0 1px rgba(255, 255, 255, 0.1),
    inset 0 1px 0 rgba(255, 255, 255, 0.1);
  overflow: hidden;
  transform: perspective(1000px) rotateX(1.5deg) rotateY(-1.5deg);
  transition: transform 0.3s ease;
}

.terminal-window:hover {
  transform: perspective(1000px) rotateX(0deg) rotateY(0deg) translateY(-4px);
}

/* Terminal Header */
.terminal-header {
  display: flex;
  align-items: center;
  padding: 12px 16px;
  background: rgba(39, 39, 42, 0.8);
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.terminal-buttons {
  display: flex;
  gap: 8px;
}

.terminal-buttons span {
  width: 12px;
  height: 12px;
  border-radius: 50%;
}

.btn-close {
  background: #ef4444;
}
.btn-minimize {
  background: #eab308;
}
.btn-maximize {
  background: #22c55e;
}

.terminal-title {
  flex: 1;
  text-align: center;
  font-size: 12px;
  font-family: ui-monospace, monospace;
  color: #71717a;
  margin-right: 52px;
}

/* Terminal Body */
.terminal-body {
  padding: 20px 24px;
  font-family: ui-monospace, 'Fira Code', monospace;
  font-size: 14px;
  line-height: 2;
}

.code-line {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  opacity: 0;
  animation: line-appear 0.5s ease forwards;
}

.line-1 {
  animation-delay: 0.3s;
}
.line-2 {
  animation-delay: 1s;
}
.line-3 {
  animation-delay: 1.8s;
}
.line-4 {
  animation-delay: 2.5s;
}

@keyframes line-appear {
  from {
    opacity: 0;
    transform: translateY(5px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.code-prompt {
  color: #22c55e;
  font-weight: bold;
}
.code-cmd {
  color: #38bdf8;
}
.code-flag {
  color: #a78bfa;
}
.code-url {
  color: #0a84ff;
}
.code-comment {
  color: #71717a;
  font-style: italic;
}
.code-success {
  color: #22c55e;
  background: rgba(34, 197, 94, 0.15);
  padding: 2px 8px;
  border-radius: 4px;
  font-weight: 600;
}
.code-response {
  color: #fbbf24;
}

/* Blinking Cursor */
.cursor {
  display: inline-block;
  width: 8px;
  height: 16px;
  background: #22c55e;
  animation: blink 1s step-end infinite;
}

@keyframes blink {
  0%,
  50% {
    opacity: 1;
  }
  51%,
  100% {
    opacity: 0;
  }
}

/* Dark mode adjustments */
:deep(.dark) .terminal-window {
  box-shadow:
    0 25px 50px -12px rgba(0,0,0, 0.6),
    0 0 0 1px rgba(10, 132, 255, 0.25),
    0 0 40px rgba(10, 132, 255, 0.12),
    inset 0 1px 0 rgba(255, 255, 255, 0.1);
}
</style>
