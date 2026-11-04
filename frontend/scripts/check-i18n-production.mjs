import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { resolve } from 'node:path'
import { createContext, Script } from 'node:vm'
import { build, loadConfigFromFile } from 'vite'

// Compile real locale strings with the application's production resolver. Running
// the result with dynamic code generation disabled also checks strict CSP support.
const root = process.cwd()
const entry = '\0i18n-production-smoke'
const loaded = await loadConfigFromFile({ command: 'build', mode: 'production' }, resolve(root, 'vite.config.ts'))
assert.ok(loaded, 'Production Vite configuration must load')
const source = `
import { createI18n } from 'vue-i18n'
import zh from ${JSON.stringify(resolve(root, 'src/i18n/locales/zh/index.ts'))}
import en from ${JSON.stringify(resolve(root, 'src/i18n/locales/en/index.ts'))}
const i18n = createI18n({ legacy: false, locale: 'zh', messages: { zh, en }, warnHtmlMessage: false })
const cases = [
 ['admin.accounts.startingTestForAccount', { name: 'Account 65293' }],
 ['admin.accounts.testAccountTypeLabel', { type: 'OAuth' }],
 ['admin.accounts.usingModel', { model: 'gpt-5.6-sol' }],
 ['admin.accounts.autoRefreshCountdown', { seconds: 20 }],
 ['admin.accounts.selectedCount', { count: 3 }],
 ['admin.accounts.usageWindow.estimatedTotalCost', { cost: '12.34' }]
]
globalThis.__i18nSmoke = Object.fromEntries(['zh', 'en'].map(locale => {
 i18n.global.locale.value = locale
 return [locale, cases.map(([key, params]) => i18n.global.t(key, params))]
}))
`
const config = loaded.config
config.plugins = [...(config.plugins || []), {
  name: 'i18n-production-smoke',
  resolveId(id) { if (id === entry) return entry },
  load(id) { if (id === entry) return source }
}]
config.build = {
  write: false,
  emptyOutDir: false,
  minify: true,
  rollupOptions: { input: entry, output: { format: 'iife', inlineDynamicImports: true } }
}
const built = await build({ ...config, configFile: false, logLevel: 'warn' })
assert.ok(!Array.isArray(built) && 'output' in built)
const chunk = built.output.find(item => item.type === 'chunk' && item.isEntry)
assert.ok(chunk, 'Smoke bundle entry must exist')
const context = createContext({ console, setTimeout, clearTimeout }, { codeGeneration: { strings: false, wasm: false } })
new Script(chunk.code).runInContext(context, { timeout: 5000 })
assert.deepEqual(JSON.parse(JSON.stringify(context.__i18nSmoke)), {
  zh: ['开始测试账号：Account 65293', '账号类型：OAuth', '使用模型：gpt-5.6-sol', '自动刷新：20s', '已选 3', '预计总费用 $12.34'],
  en: ['Starting test for account: Account 65293', 'Account type: OAuth', 'Using model: gpt-5.6-sol', 'Auto refresh: 20s', '3 selected', 'Est. total $12.34']
})
console.log('Production i18n interpolation passed for zh/en with dynamic code generation disabled; Vue I18n', createRequire(import.meta.url)('vue-i18n/package.json').version)
