import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({
  post: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    post
  }
}))

import { previewFromCrs, syncFromCrs } from '@/api/admin/accounts'

const credentials = {
  base_url: 'https://crs.example.com',
  username: 'admin',
  password: 'secret'
}

describe('admin accounts CRS API', () => {
  beforeEach(() => {
    post.mockReset()
    post.mockResolvedValue({ data: {} })
  })

  it('allows the serial CRS sync to run for up to 180 seconds', async () => {
    await syncFromCrs(credentials)

    expect(post).toHaveBeenCalledWith('/admin/accounts/sync/crs', credentials, {
      timeout: 180000
    })
  })

  it('keeps the preview request on the shared client timeout', async () => {
    await previewFromCrs(credentials)

    expect(post).toHaveBeenCalledWith('/admin/accounts/sync/crs/preview', credentials)
  })
})
