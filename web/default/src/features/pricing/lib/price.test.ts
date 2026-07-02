import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  CACHE_CREATE_1H_PRICE_MULTIPLIER,
  deriveCacheCreate1hValue,
  formatDerivedCacheCreate1hValue,
} from './price'

describe('cache create 1h pricing helpers', () => {
  test('derives 1h cache write value from the configured 5m value', () => {
    assert.equal(CACHE_CREATE_1H_PRICE_MULTIPLIER, 1.6)
    assert.equal(deriveCacheCreate1hValue(3.75), 6)
    assert.equal(deriveCacheCreate1hValue(1.25), 2)
  })

  test('formats derived 1h values without floating point noise', () => {
    assert.equal(formatDerivedCacheCreate1hValue('3.75'), '6')
    assert.equal(formatDerivedCacheCreate1hValue('0.1'), '0.16')
    assert.equal(formatDerivedCacheCreate1hValue(''), '')
    assert.equal(formatDerivedCacheCreate1hValue('invalid'), '')
  })
})
