import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  formatDiscountPercentage,
  formatGroupDiscount,
  getDiscountSavingsLabel,
  getLowestGroupDiscountSummary,
  normalizeDiscountLabel,
} from './group-discount'

const zhDiscountLabels = {
  originalPrice: '原价',
  percentPrice: '{{value}}%价格',
}

describe('formatGroupDiscount', () => {
  test('formats simple discount ratios as signed savings by default', () => {
    assert.equal(formatGroupDiscount(0.35), '-65%')
    assert.equal(formatGroupDiscount(0.5), '-50%')
    assert.equal(formatGroupDiscount(0.8), '-20%')
    assert.equal(formatGroupDiscount(1), 'Original price')
  })

  test('uses the same signed discount expression with translated labels', () => {
    assert.equal(formatGroupDiscount(0.5, zhDiscountLabels), '-50%')
    assert.equal(formatGroupDiscount(0.4, zhDiscountLabels), '-60%')
    assert.equal(formatGroupDiscount(0.05, zhDiscountLabels), '-95%')
    assert.equal(formatGroupDiscount(0.8, zhDiscountLabels), '-20%')
    assert.equal(formatGroupDiscount(0.85, zhDiscountLabels), '-15%')
    assert.equal(formatGroupDiscount(1, zhDiscountLabels), '原价')
  })

  test('formats precision discounts and surcharges without x multipliers', () => {
    assert.equal(formatGroupDiscount(0.95), '-5%')
    assert.equal(formatGroupDiscount(0.125), '-87.5%')
    assert.equal(formatGroupDiscount(1.2), '120% price')
  })

  test('formats standalone ratios as universal discount labels', () => {
    assert.equal(formatDiscountPercentage(0.8), '-20%')
    assert.equal(formatDiscountPercentage('0.355'), '-64.5%')
    assert.equal(formatDiscountPercentage(1), undefined)
    assert.equal(formatDiscountPercentage('invalid'), undefined)
  })

  test('normalizes stored discount descriptions to universal labels', () => {
    assert.equal(normalizeDiscountLabel('限时 4 折'), '-60%')
    assert.equal(normalizeDiscountLabel('40% OFF'), '-40%')
    assert.equal(normalizeDiscountLabel('-20%'), '-20%')
    assert.equal(normalizeDiscountLabel('special offer'), undefined)
  })

  test('extracts a positive savings value for tooltip descriptions', () => {
    assert.equal(getDiscountSavingsLabel('-20%'), '20%')
    assert.equal(getDiscountSavingsLabel('-64.5%'), '64.5%')
    assert.equal(getDiscountSavingsLabel('20% off'), undefined)
    assert.equal(getDiscountSavingsLabel('Original price'), undefined)
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
      '-70%'
    )
    assert.equal(getLowestGroupDiscountSummary(['vip'], { vip: 0.3 }), '-70%')
    assert.equal(
      getLowestGroupDiscountSummary(
        ['default', 'vip'],
        {
          default: 1,
          vip: 0.3,
        },
        zhDiscountLabels
      ),
      '-70%'
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
