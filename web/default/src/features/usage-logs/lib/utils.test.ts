import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  buildApiParams,
  buildBaseParams,
  normalizeNumericFilterValue,
} from './utils'

describe('usage log filter utils', () => {
  test('normalizes quoted numeric filter values', () => {
    assert.equal(normalizeNumericFilterValue('"11"'), '11')
    assert.equal(normalizeNumericFilterValue("'11'"), '11')
    assert.equal(normalizeNumericFilterValue('11'), '11')
    assert.equal(normalizeNumericFilterValue('"abc"'), undefined)
  })

  test('builds admin log channel params from quoted route values', () => {
    const params = buildApiParams({
      page: 1,
      pageSize: 20,
      searchParams: { channel: '"11"' },
      isAdmin: true,
    })

    assert.equal(params.channel, 11)
  })

  test('builds task channel params from quoted route values', () => {
    const params = buildBaseParams({
      page: 1,
      pageSize: 20,
      searchParams: { channel: '"11"' },
    })

    assert.equal(params.channel_id, '11')
  })
})
