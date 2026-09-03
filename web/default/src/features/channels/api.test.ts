import assert from 'node:assert/strict'
import { test } from 'node:test'
import { api } from '@/lib/api'
import { batchUpdateChannelStatus, updateChannelStatus } from './api'

test('channel status APIs use the dedicated operational endpoints', async () => {
  const originalPost = api.post
  const calls: Array<{
    url: string
    data: unknown
    config: unknown
  }> = []

  api.post = (async (url, data, config) => {
    calls.push({ url, data, config })
    return {
      data: {
        success: true,
        data: url === '/api/channel/status/batch' ? 2 : true,
      },
    }
  }) as typeof api.post

  try {
    await updateChannelStatus(3, 1)
    await batchUpdateChannelStatus([3, 4], 2)
  } finally {
    api.post = originalPost
  }

  assert.deepEqual(calls, [
    {
      url: '/api/channel/3/status',
      data: { status: 1 },
      config: { skipBusinessError: true, skipErrorHandler: true },
    },
    {
      url: '/api/channel/status/batch',
      data: { ids: [3, 4], status: 2 },
      config: { skipBusinessError: true, skipErrorHandler: true },
    },
  ])
})
