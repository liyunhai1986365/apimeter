import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { buildBillingHistoryParams } from './api'

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
