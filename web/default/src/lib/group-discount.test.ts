import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { formatGroupDiscount } from './group-discount'

describe('formatGroupDiscount', () => {
  test('formats simple discount ratios as Chinese fold discounts', () => {
    assert.equal(formatGroupDiscount(0.5), '5折')
    assert.equal(formatGroupDiscount(0.8), '8折')
    assert.equal(formatGroupDiscount(1), '原价')
  })

  test('formats precision discounts and surcharges without x multipliers', () => {
    assert.equal(formatGroupDiscount(0.95), '95%折扣')
    assert.equal(formatGroupDiscount(0.125), '12.5%折扣')
    assert.equal(formatGroupDiscount(1.2), '120%价格')
  })

  test('ignores missing or invalid ratios', () => {
    assert.equal(formatGroupDiscount(undefined), undefined)
    assert.equal(formatGroupDiscount(''), undefined)
    assert.equal(formatGroupDiscount('abc'), undefined)
  })
})
