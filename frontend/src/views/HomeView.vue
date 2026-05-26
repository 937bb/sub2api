<template>
  <div v-if="homeContent" class="min-h-screen">
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <div v-else v-html="homeContent"></div>
  </div>

  <div
    v-else
    class="global-home global-grid relative min-h-screen overflow-hidden bg-stone-50 text-zinc-950 dark:bg-zinc-950 dark:text-white"
  >
    <header
      class="sticky top-0 z-30 border-b border-zinc-200/70 bg-stone-50/85 px-5 py-4 backdrop-blur-xl dark:border-white/10 dark:bg-zinc-950/80"
    >
      <nav class="mx-auto flex max-w-7xl items-center justify-between gap-5">
        <router-link to="/home" class="flex min-w-0 items-center gap-3">
          <span
            class="flex h-9 w-9 shrink-0 items-center justify-center overflow-hidden rounded-md border border-zinc-200 bg-white text-sm font-semibold shadow-sm dark:border-white/10 dark:bg-white/10"
          >
            <img
              v-if="siteLogo"
              :src="siteLogo"
              alt=""
              class="h-full w-full object-contain"
            />
            <span v-else>{{ brandInitial }}</span>
          </span>
          <span class="truncate text-sm font-semibold">{{ siteName }}</span>
        </router-link>

        <div class="hidden items-center gap-8 text-sm text-zinc-500 dark:text-zinc-400 md:flex">
          <a href="#models" class="transition-colors hover:text-zinc-950 dark:hover:text-white">Models</a>
          <a href="#capabilities" class="transition-colors hover:text-zinc-950 dark:hover:text-white">Capabilities</a>
          <a href="#metrics" class="transition-colors hover:text-zinc-950 dark:hover:text-white">Metrics</a>
        </div>

        <div class="flex shrink-0 items-center gap-2">
          <LocaleSwitcher />
          <button
            type="button"
            class="rounded-md p-2 text-zinc-500 transition-colors hover:bg-zinc-100 hover:text-zinc-950 dark:text-zinc-400 dark:hover:bg-white/10 dark:hover:text-white"
            :title="isDark ? 'Switch to light mode' : 'Switch to dark mode'"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="hidden rounded-md p-2 text-zinc-500 transition-colors hover:bg-zinc-100 hover:text-zinc-950 dark:text-zinc-400 dark:hover:bg-white/10 dark:hover:text-white sm:inline-flex"
            title="Documentation"
          >
            <Icon name="book" size="md" />
          </a>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="inline-flex h-9 items-center justify-center rounded-full bg-zinc-950 px-4 text-sm font-medium text-white transition-colors hover:bg-zinc-800 dark:bg-white dark:text-zinc-950 dark:hover:bg-zinc-200"
          >
            {{ isAuthenticated ? 'Console' : 'Sign in' }}
          </router-link>
        </div>
      </nav>
    </header>

    <main class="relative z-10">
      <section class="mx-auto grid max-w-7xl gap-12 px-5 pb-14 pt-20 lg:grid-cols-[minmax(0,1.05fr)_minmax(360px,0.95fr)] lg:items-center lg:pb-20 lg:pt-24">
        <div>
          <div class="mb-8 flex items-center gap-3 text-sm text-zinc-500 dark:text-zinc-400">
            <span class="h-px w-8 bg-zinc-300 dark:bg-white/20"></span>
            <span>AI gateway for modern builders</span>
          </div>

          <h1 class="max-w-5xl text-5xl font-semibold leading-none text-zinc-950 dark:text-white md:text-7xl">
            One API.
            <span class="block">Every top AI model.</span>
          </h1>

          <p class="mt-7 max-w-2xl text-lg leading-8 text-zinc-600 dark:text-zinc-300 md:text-xl">
            {{ heroSubtitle }}
          </p>

          <div class="mt-9 flex flex-col gap-3 sm:flex-row">
            <router-link
              :to="isAuthenticated ? dashboardPath : '/login'"
              class="inline-flex h-12 items-center justify-center gap-2 rounded-full bg-zinc-950 px-6 text-sm font-semibold text-white transition-transform hover:-translate-y-0.5 hover:bg-zinc-800 dark:bg-white dark:text-zinc-950 dark:hover:bg-zinc-200"
            >
              {{ isAuthenticated ? 'Open console' : 'Start building' }}
              <Icon name="arrowRight" size="sm" :stroke-width="2" />
            </router-link>
            <a
              v-if="docUrl"
              :href="docUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="inline-flex h-12 items-center justify-center gap-2 rounded-full border border-zinc-300 bg-white/70 px-6 text-sm font-semibold text-zinc-900 transition-colors hover:bg-white dark:border-white/15 dark:bg-white/5 dark:text-white dark:hover:bg-white/10"
            >
              Read docs
              <Icon name="externalLink" size="sm" :stroke-width="2" />
            </a>
          </div>

          <div id="metrics" class="mt-12 grid max-w-2xl grid-cols-3 gap-3">
            <div
              v-for="metric in heroMetrics"
              :key="metric.label"
              class="border-l border-zinc-200 pl-4 dark:border-white/10"
            >
              <div class="text-2xl font-semibold text-zinc-950 dark:text-white">{{ metric.value }}</div>
              <div class="mt-1 text-sm leading-5 text-zinc-500 dark:text-zinc-400">{{ metric.label }}</div>
            </div>
          </div>
        </div>

        <div class="route-board relative min-h-[440px] overflow-hidden rounded-lg border border-zinc-200 shadow-sm dark:border-white/10">
          <div class="absolute inset-x-8 top-8 flex items-center justify-between text-xs text-zinc-400 dark:text-zinc-500">
            <span>Client</span>
            <span>Unified Gateway</span>
            <span>Providers</span>
          </div>

          <div class="absolute left-8 top-24 w-36 rounded-md border border-zinc-200 bg-white px-4 py-3 shadow-sm dark:border-white/10 dark:bg-zinc-900">
            <div class="text-xs text-zinc-500 dark:text-zinc-400">Request</div>
            <div class="mt-1 font-mono text-sm text-zinc-950 dark:text-white">/v1/chat</div>
          </div>

          <div class="absolute left-1/2 top-24 w-44 -translate-x-1/2 rounded-md border border-zinc-950 bg-zinc-950 px-4 py-4 text-white shadow-sm dark:border-white dark:bg-white dark:text-zinc-950">
            <div class="text-xs opacity-70">Policy layer</div>
            <div class="mt-1 text-lg font-semibold">route + meter</div>
          </div>

          <div class="absolute right-8 top-20 grid gap-3">
            <div
              v-for="provider in featuredProviders"
              :key="provider"
              class="w-36 rounded-md border border-zinc-200 bg-white px-4 py-2 text-sm font-medium shadow-sm dark:border-white/10 dark:bg-zinc-900"
            >
              {{ provider }}
            </div>
          </div>

          <div class="route-line route-line-a"></div>
          <div class="route-line route-line-b"></div>
          <div class="route-line route-line-c"></div>

          <div class="absolute bottom-8 left-8 right-8 grid gap-3 sm:grid-cols-3">
            <div
              v-for="item in routingStats"
              :key="item.label"
              class="rounded-md border border-zinc-200 bg-white/85 p-4 dark:border-white/10 dark:bg-zinc-900/90"
            >
              <div class="text-sm font-semibold text-zinc-950 dark:text-white">{{ item.value }}</div>
              <div class="mt-1 text-xs leading-5 text-zinc-500 dark:text-zinc-400">{{ item.label }}</div>
            </div>
          </div>
        </div>
      </section>

      <section id="models" class="border-y border-zinc-200/70 bg-white/55 px-5 py-8 dark:border-white/10 dark:bg-white/[0.03]">
        <div class="mx-auto flex max-w-7xl flex-col gap-5 md:flex-row md:items-center md:justify-between">
          <div>
            <p class="text-sm font-semibold text-zinc-950 dark:text-white">Model ecosystem</p>
            <p class="mt-1 text-sm text-zinc-500 dark:text-zinc-400">OpenAI-compatible access across leading providers.</p>
          </div>
          <div class="flex flex-wrap gap-2">
            <span
              v-for="provider in modelProviders"
              :key="provider"
              class="rounded-full border border-zinc-200 bg-stone-50 px-3 py-1.5 text-sm text-zinc-700 dark:border-white/10 dark:bg-white/5 dark:text-zinc-200"
            >
              {{ provider }}
            </span>
          </div>
        </div>
      </section>

      <section id="capabilities" class="mx-auto max-w-7xl px-5 py-16">
        <div class="max-w-2xl">
          <p class="text-sm font-semibold text-zinc-500 dark:text-zinc-400">Core capabilities</p>
          <h2 class="mt-3 text-3xl font-semibold text-zinc-950 dark:text-white md:text-4xl">
            A clear control layer for every model request.
          </h2>
        </div>

        <div class="mt-10 grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          <article
            v-for="feature in features"
            :key="feature.title"
            class="rounded-lg border border-zinc-200 bg-white/75 p-6 shadow-sm dark:border-white/10 dark:bg-white/[0.04]"
          >
            <div class="mb-5 flex h-10 w-10 items-center justify-center rounded-md bg-zinc-950 text-white dark:bg-white dark:text-zinc-950">
              <Icon :name="feature.icon" size="md" :stroke-width="2" />
            </div>
            <h3 class="text-base font-semibold text-zinc-950 dark:text-white">{{ feature.title }}</h3>
            <p class="mt-3 text-sm leading-6 text-zinc-600 dark:text-zinc-400">{{ feature.description }}</p>
          </article>
        </div>
      </section>
    </main>

    <footer class="relative z-10 border-t border-zinc-200/70 px-5 py-8 text-sm text-zinc-500 dark:border-white/10 dark:text-zinc-400">
      <div class="mx-auto flex max-w-7xl flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <span>&copy; {{ currentYear }} {{ siteName }}. All systems operational.</span>
        <div class="flex gap-5">
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="hover:text-zinc-950 dark:hover:text-white">Docs</a>
          <a :href="githubUrl" target="_blank" rel="noopener noreferrer" class="hover:text-zinc-950 dark:hover:text-white">GitHub</a>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'

type FeatureIcon = 'server' | 'shield' | 'chart' | 'globe'

const authStore = useAuthStore()
const appStore = useAppStore()

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || '')
const docUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const heroSubtitle = computed(() => (
  siteSubtitle.value
  || 'Route GPT, Claude, Gemini, image, video, and custom providers through one fast, observable, OpenAI-compatible endpoint.'
))

const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

const isDark = ref(document.documentElement.classList.contains('dark'))
const githubUrl = 'https://github.com/Wei-Shaw/sub2api'

const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')
const currentYear = computed(() => new Date().getFullYear())
const brandInitial = computed(() => siteName.value.trim().charAt(0).toUpperCase() || 'S')

const featuredProviders = ['OpenAI', 'Claude', 'Gemini', 'DeepSeek']
const modelProviders = ['OpenAI', 'Claude', 'Gemini', 'Codex', 'DeepSeek', 'Qwen', 'Mistral', 'Azure', 'Bedrock']
const heroMetrics = [
  { value: '30+', label: 'provider integrations' },
  { value: '24/7', label: 'usage and key monitoring' },
  { value: '1', label: 'control plane for teams' }
]
const routingStats = [
  { value: 'Smart failover', label: 'Route around unhealthy upstreams automatically.' },
  { value: 'Live metering', label: 'Track token spend close to the request path.' },
  { value: 'Policy aware', label: 'Apply groups, quotas, and access rules in one place.' }
]
const features: Array<{ title: string; description: string; icon: FeatureIcon }> = [
  {
    title: 'Unified routing',
    description: 'Use one OpenAI-compatible endpoint across frontier models and regional provider stacks.',
    icon: 'server'
  },
  {
    title: 'Operational guardrails',
    description: 'Keep quotas, group policies, rate limits, and access controls close to production traffic.',
    icon: 'shield'
  },
  {
    title: 'Usage observability',
    description: 'Monitor requests, latency, errors, consumption, and billing behaviour from one surface.',
    icon: 'chart'
  },
  {
    title: 'Global-ready delivery',
    description: 'Run a self-hosted control plane with flexible upstream connectivity across regions.',
    icon: 'globe'
  }
]

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

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
  authStore.checkAuth()

  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
.global-grid {
  background-image:
    linear-gradient(rgba(24, 24, 27, 0.07) 1px, transparent 1px),
    linear-gradient(90deg, rgba(24, 24, 27, 0.07) 1px, transparent 1px);
  background-size: 72px 72px;
}

:global(.dark) .global-grid {
  background-image:
    linear-gradient(rgba(255, 255, 255, 0.08) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 255, 255, 0.08) 1px, transparent 1px);
}

.route-board {
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.92), rgba(245, 245, 240, 0.9)),
    linear-gradient(rgba(24, 24, 27, 0.06) 1px, transparent 1px),
    linear-gradient(90deg, rgba(24, 24, 27, 0.06) 1px, transparent 1px);
  background-size: auto, 48px 48px, 48px 48px;
}

:global(.dark) .route-board {
  background:
    linear-gradient(180deg, rgba(24, 24, 27, 0.94), rgba(9, 9, 11, 0.94)),
    linear-gradient(rgba(255, 255, 255, 0.08) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 255, 255, 0.08) 1px, transparent 1px);
  background-size: auto, 48px 48px, 48px 48px;
}

.route-line {
  position: absolute;
  height: 1px;
  background: linear-gradient(90deg, rgba(24, 24, 27, 0.12), rgba(24, 24, 27, 0.5), rgba(24, 24, 27, 0.12));
  transform-origin: left center;
}

:global(.dark) .route-line {
  background: linear-gradient(90deg, rgba(255, 255, 255, 0.12), rgba(255, 255, 255, 0.55), rgba(255, 255, 255, 0.12));
}

.route-line-a {
  left: 180px;
  top: 142px;
  width: 185px;
}

.route-line-b {
  right: 178px;
  top: 142px;
  width: 190px;
}

.route-line-c {
  right: 178px;
  top: 194px;
  width: 190px;
  transform: rotate(13deg);
}

@media (max-width: 640px) {
  .route-line {
    display: none;
  }
}
</style>
