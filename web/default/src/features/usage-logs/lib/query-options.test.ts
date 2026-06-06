import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { usageLogsManualRefreshQueryOptions } from './query-options'

describe('usageLogsManualRefreshQueryOptions', () => {
  test('disables background refetching for log pages', () => {
    assert.deepEqual(usageLogsManualRefreshQueryOptions, {
      refetchInterval: false,
      refetchOnMount: false,
      refetchOnReconnect: false,
      refetchOnWindowFocus: false,
      staleTime: Number.POSITIVE_INFINITY,
    })
  })
})
