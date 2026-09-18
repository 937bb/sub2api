import { reactive } from 'vue'

export function codexStateScanKey(accountID: number, model: string): string {
  return `${accountID}:${model.trim().toLowerCase() || 'gpt-5.5'}`
}

export function useCodexStateScanState() {
  const scanningKeys = reactive(new Set<string>())

  const isScanning = (accountID: number, model: string) =>
    scanningKeys.has(codexStateScanKey(accountID, model))

  const run = async <T>(accountID: number, model: string, operation: () => Promise<T>): Promise<T> => {
    const key = codexStateScanKey(accountID, model)
    scanningKeys.add(key)
    try {
      return await operation()
    } finally {
      scanningKeys.delete(key)
    }
  }

  return {
    scanningKeys,
    isScanning,
    run
  }
}
