import { AxiosError, AxiosHeaders } from 'axios'
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { api } from '@/lib/api'
import { buildBillingHistoryParams, getCryptoPaymentOrder } from './api'

describe('wallet billing history params', () => {
  test('includes status and date range filters', () => {
    const params = buildBillingHistoryParams(2, 20, ' order-123 ', 'success', {
      startDate: '2026-07-01',
      endDate: '2026-07-08',
    })

    assert.equal(params.get('p'), '2')
    assert.equal(params.get('page_size'), '20')
    assert.equal(params.get('keyword'), ' order-123 ')
    assert.equal(params.get('status'), 'success')
    assert.equal(
      params.get('start_time'),
      Math.floor(new Date('2026-07-01T00:00:00').getTime() / 1000).toString()
    )
    assert.equal(
      params.get('end_time'),
      Math.floor(new Date('2026-07-08T23:59:59').getTime() / 1000).toString()
    )
  })

  test('omits all-status and empty date filters', () => {
    const params = buildBillingHistoryParams(1, 10, '', 'all', {
      startDate: '',
      endDate: '',
    })

    assert.equal(params.get('status'), null)
    assert.equal(params.get('start_time'), null)
    assert.equal(params.get('end_time'), null)
  })
})

describe('crypto payment polling failures', () => {
  for (const status of [401, 500]) {
    test(`handles HTTP ${status} without losing the pending order`, async () => {
      const originalGet = api.get
      const error = new AxiosError(
        'HTTP error',
        'ERR_BAD_RESPONSE',
        undefined,
        undefined,
        {
          status,
          statusText: 'error',
          headers: {},
          config: { headers: new AxiosHeaders() },
          data: {},
        }
      )
      api.get = async (_url, config) => {
        const options = config as Record<string, unknown>
        assert.equal(options.skipErrorHandler, true)
        assert.equal(options.timeout, 10000)
        throw error
      }
      try {
        await assert.rejects(
          getCryptoPaymentOrder('pending-order'),
          (caught) => {
            if (status === 401) assert.equal(caught, error)
            else {
              assert.ok(caught instanceof Error)
              assert.equal(caught instanceof AxiosError, false)
              assert.equal(caught.cause, error)
            }
            return true
          }
        )
      } finally {
        api.get = originalGet
      }
    })
  }
})
