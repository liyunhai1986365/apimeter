import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  formatGroupDiscount,
  getLowestGroupDiscountSummary,
} from './group-discount'

const zhDiscountLabels = {
  originalPrice: '原价',
  fold: '{{value}}折',
  percentDiscount: '{{value}}%折扣',
  percentPrice: '{{value}}%价格',
  startingFrom: '{{value}}起',
}

describe('formatGroupDiscount', () => {
  test('formats simple discount ratios in English by default', () => {
    assert.equal(formatGroupDiscount(0.5), '50% off')
    assert.equal(formatGroupDiscount(0.8), '80% off')
    assert.equal(formatGroupDiscount(1), 'Original price')
  })

  test('formats simple discount ratios with translated labels', () => {
    assert.equal(formatGroupDiscount(0.5, zhDiscountLabels), '5折')
    assert.equal(formatGroupDiscount(0.4, zhDiscountLabels), '4折')
    assert.equal(formatGroupDiscount(0.05, zhDiscountLabels), '0.5折')
    assert.equal(formatGroupDiscount(0.8, zhDiscountLabels), '8折')
    assert.equal(formatGroupDiscount(0.85, zhDiscountLabels), '85折')
    assert.equal(formatGroupDiscount(1, zhDiscountLabels), '原价')
  })

  test('formats precision discounts and surcharges without x multipliers', () => {
    assert.equal(formatGroupDiscount(0.95), '95% off')
    assert.equal(formatGroupDiscount(0.125), '12.5% off')
    assert.equal(formatGroupDiscount(1.2), '120% price')
  })

  test('ignores missing or invalid ratios', () => {
    assert.equal(formatGroupDiscount(undefined), undefined)
    assert.equal(formatGroupDiscount(''), undefined)
    assert.equal(formatGroupDiscount('abc'), undefined)
  })

  test('summarizes the lowest enabled group discount for model cards', () => {
    assert.equal(
      getLowestGroupDiscountSummary(['default', 'vip'], {
        default: 1,
        vip: 0.3,
      }),
      'From 30% off'
    )
    assert.equal(
      getLowestGroupDiscountSummary(['vip'], { vip: 0.3 }),
      '30% off'
    )
    assert.equal(
      getLowestGroupDiscountSummary(
        ['default', 'vip'],
        {
          default: 1,
          vip: 0.3,
        },
        zhDiscountLabels
      ),
      '3折起'
    )
  })

  test('ignores groups without valid ratios when building model card summary', () => {
    assert.equal(
      getLowestGroupDiscountSummary(['default', 'vip'], { default: 1 }),
      'Original price'
    )
    assert.equal(getLowestGroupDiscountSummary(['missing'], {}), undefined)
  })
})
