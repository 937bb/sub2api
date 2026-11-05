<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import { isValidUpstreamRequestIdHeader } from '@/utils/upstreamRequestIdHeader'

const props = defineProps<{
  platform?: string
  type?: string
}>()
const model = defineModel<string>({ default: '' })
const { t } = useI18n()
const invalid = computed(() => !isValidUpstreamRequestIdHeader(model.value))

interface HeaderExample {
  label: string
  header: string
  note?: string
}

// Examples of response headers used for upstream request identifiers.
const OFFICIAL_REQUEST_ID_HEADERS: Record<string, { platform: string; header: string }> = {
  openai: { platform: 'OpenAI', header: 'x-request-id' },
  anthropic: { platform: 'Anthropic', header: 'request-id' },
  gemini: { platform: 'Gemini', header: 'x-goog-request-id' },
  antigravity: { platform: 'Antigravity', header: 'x-goog-request-id' },
  grok: { platform: 'Grok', header: 'xai-request-id' }
}

// API key accounts can also use relay-specific response headers.
const examples = computed<HeaderExample[]>(() => {
  const items: HeaderExample[] = []
  if (props.type === 'apikey') {
    items.push({
      label: 'sub2api',
      header: 'X-Client-Request-ID',
      note: t('admin.accounts.upstreamRequestIdHeaderHelp.sub2apiNote')
    })
    items.push({ label: 'new-api / one-api', header: 'X-Oneapi-Request-Id' })
  }
  const official = props.platform ? OFFICIAL_REQUEST_ID_HEADERS[props.platform] : undefined
  if (official) {
    items.push({
      label: t('admin.accounts.upstreamRequestIdHeaderHelp.official', { platform: official.platform }),
      header: official.header
    })
  }
  return items
})
</script>

<template>
  <div>
    <div class="flex items-center">
      <label class="input-label">{{ t('admin.accounts.upstreamRequestIdHeader') }}</label>
      <HelpTooltip width-class="w-80">
        <p>{{ t('admin.accounts.upstreamRequestIdHeaderHelp.intro') }}</p>
        <template v-if="examples.length">
          <p class="mt-2 font-medium">{{ t('admin.accounts.upstreamRequestIdHeaderHelp.examplesTitle') }}</p>
          <ul class="mt-1 space-y-1.5">
            <li v-for="item in examples" :key="item.label">
              <div class="flex items-center justify-between gap-3">
                <span class="text-gray-300">{{ item.label }}</span>
                <code class="select-all rounded bg-white/10 px-1.5 py-0.5 font-mono text-[11px] text-white">{{ item.header }}</code>
              </div>
              <p v-if="item.note" class="mt-0.5 text-gray-400">{{ item.note }}</p>
            </li>
          </ul>
        </template>
      </HelpTooltip>
    </div>
    <input
      v-model="model"
      data-testid="upstream-request-id-header"
      type="text"
      class="input"
      :aria-label="t('admin.accounts.upstreamRequestIdHeader')"
      :aria-invalid="invalid"
      :placeholder="t('admin.accounts.upstreamRequestIdHeaderPlaceholder')"
    />
    <p v-if="invalid" role="alert" class="mt-1 text-xs text-red-600 dark:text-red-400">
      {{ t('admin.accounts.upstreamRequestIdHeaderInvalid') }}
    </p>
    <p v-else class="mt-1 text-xs text-gray-500 dark:text-gray-400">
      {{ t('admin.accounts.upstreamRequestIdHeaderHint') }}
    </p>
  </div>
</template>
