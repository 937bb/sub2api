import { describe, expect, it } from 'vitest'
import { codexStateScanKey, useCodexStateScanState } from '../useCodexStateScanState'

describe('useCodexStateScanState', () => {
  it('tracks every account and model independently', async () => {
    const state = useCodexStateScanState()
    let finishFirst!: () => void
    const first = state.run(41, 'gpt-5.5', () => new Promise<void>(resolve => { finishFirst = resolve }))

    expect(state.isScanning(41, 'gpt-5.5')).toBe(true)
    expect(state.isScanning(42, 'gpt-5.5')).toBe(false)
    expect(state.isScanning(41, 'gpt-5.6-sol')).toBe(false)

    await state.run(42, 'gpt-5.5', async () => undefined)
    expect(state.isScanning(41, 'gpt-5.5')).toBe(true)
    expect(state.isScanning(42, 'gpt-5.5')).toBe(false)

    finishFirst()
    await first
    expect(state.isScanning(41, 'gpt-5.5')).toBe(false)
  })

  it('normalizes model names without merging different accounts', () => {
    expect(codexStateScanKey(41, ' GPT-5.5 ')).toBe('41:gpt-5.5')
    expect(codexStateScanKey(42, 'gpt-5.5')).toBe('42:gpt-5.5')
  })

  it('keeps account-wide scans isolated by account', async () => {
    const state = useCodexStateScanState()
    let finish!: () => void
    const scan = state.run(41, '__all_target_models__', () => new Promise<void>(resolve => { finish = resolve }))

    expect(state.isScanning(41, '__all_target_models__')).toBe(true)
    expect(state.isScanning(42, '__all_target_models__')).toBe(false)

    finish()
    await scan
  })
})
