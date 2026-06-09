import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { getApiKeyQuotaDisplay } from './api-key-quota'

describe('api key quota display', () => {
  test('uses remaining and total quota for limited API keys', () => {
    const display = getApiKeyQuotaDisplay({
      unlimited_quota: false,
      used_quota: 25,
      remain_quota: 75,
    })

    assert.equal(display.unlimited, false)
    assert.equal(display.leftQuota, 75)
    assert.equal(display.totalQuota, 100)
    assert.equal(display.percentage, 75)
  })

  test('uses used quota and no total quota for unlimited API keys', () => {
    const display = getApiKeyQuotaDisplay({
      unlimited_quota: true,
      used_quota: 25,
      remain_quota: 0,
    })

    assert.equal(display.unlimited, true)
    assert.equal(display.leftQuota, 25)
    assert.equal(display.totalQuota, null)
    assert.equal(display.percentage, 100)
  })
})
