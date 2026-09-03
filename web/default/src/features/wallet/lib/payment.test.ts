import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { TopupInfo } from '../types'
import { getDefaultPaymentType, getMinTopupAmount } from './payment'

function createTopupInfo(overrides: Partial<TopupInfo> = {}): TopupInfo {
  return {
    enable_online_topup: false,
    enable_stripe_topup: false,
    pay_methods: [],
    min_topup: 10,
    stripe_min_topup: 1,
    amount_options: [],
    discount: {},
    ...overrides,
  }
}

describe('wallet default payment method', () => {
  test('defaults to ten USD before top-up settings are available', () => {
    assert.equal(getMinTopupAmount(null), 10)
  })

  test('uses the first configured method instead of reserving the primary action for Stripe', () => {
    const info = createTopupInfo({
      enable_waffo_pancake_topup: true,
      min_topup: 10,
      waffo_pancake_min_topup: 5,
      pay_methods: [
        {
          name: 'Waffo Pancake',
          type: 'waffo_pancake',
          min_topup: 5,
        },
        { name: 'Stripe', type: 'stripe', min_topup: 10 },
      ],
    })

    assert.equal(getDefaultPaymentType(info), 'waffo_pancake')
    assert.equal(getMinTopupAmount(info), 10)
  })

  test('uses the shared minimum when Stripe is not available', () => {
    const info = createTopupInfo({
      enable_waffo_pancake_topup: true,
      min_topup: 10,
      waffo_pancake_min_topup: 12,
      pay_methods: [
        {
          name: 'Waffo Pancake',
          type: 'waffo_pancake',
          min_topup: 12,
        },
      ],
    })

    assert.equal(getDefaultPaymentType(info), 'waffo_pancake')
    assert.equal(getMinTopupAmount(info), 10)
  })
})
