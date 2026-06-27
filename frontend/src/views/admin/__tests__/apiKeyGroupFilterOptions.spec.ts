import { describe, expect, it } from 'vitest'
import { buildApiKeyGroupFilterOptions } from '../apiKeyGroupFilterOptions'
import type { AdminGroup } from '@/types'

const labels = {
  all: 'All',
  exclusive: 'Exclusive',
  public: 'Public',
  subscription: 'Subscription',
  disabled: 'Disabled'
}

function group(partial: Partial<AdminGroup>): AdminGroup {
  return {
    id: 0,
    name: '',
    status: 'active',
    is_exclusive: false,
    subscription_type: 'standard',
    ...partial
  } as AdminGroup
}

describe('buildApiKeyGroupFilterOptions', () => {
  it('partitions active groups and renders section headers', () => {
    const groups = [
      group({ id: 1, name: 'Exclusive', is_exclusive: true }),
      group({ id: 2, name: 'Public' }),
      group({ id: 3, name: 'Subscription', subscription_type: 'subscription' })
    ]

    expect(buildApiKeyGroupFilterOptions(groups, labels)).toEqual([
      { value: null, label: 'All' },
      { value: -1, label: 'Exclusive', kind: 'group', disabled: true },
      { value: 1, label: 'Exclusive' },
      { value: -2, label: 'Public', kind: 'group', disabled: true },
      { value: 2, label: 'Public' },
      { value: -3, label: 'Subscription', kind: 'group', disabled: true },
      { value: 3, label: 'Subscription' }
    ])
  })

  it('puts inactive groups in a disabled section without omitting them', () => {
    const groups = [
      group({ id: 1, name: 'Active', is_exclusive: true }),
      group({ id: 2, name: 'Inactive', status: 'inactive', is_exclusive: true })
    ]

    const options = buildApiKeyGroupFilterOptions(groups, labels)

    expect(options).toContainEqual({ value: 1, label: 'Active' })
    expect(options).toContainEqual({ value: -4, label: 'Disabled', kind: 'group', disabled: true })
    expect(options).toContainEqual({ value: 2, label: 'Inactive' })
  })

  it('uses distinct negative values for section headers', () => {
    const groups = [
      group({ id: 1, name: 'E', is_exclusive: true }),
      group({ id: 2, name: 'P' }),
      group({ id: 3, name: 'S', subscription_type: 'subscription' }),
      group({ id: 4, name: 'D', status: 'inactive' })
    ]

    const headerValues = buildApiKeyGroupFilterOptions(groups, labels)
      .filter((option) => option.kind === 'group')
      .map((option) => option.value)

    expect(new Set(headerValues).size).toBe(headerValues.length)
    headerValues.forEach((value) => expect(value).toBeLessThan(0))
  })

  it('returns only the all option when there are no groups', () => {
    expect(buildApiKeyGroupFilterOptions([], labels)).toEqual([{ value: null, label: 'All' }])
  })
})
