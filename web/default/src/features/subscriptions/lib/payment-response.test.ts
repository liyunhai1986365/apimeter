import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  getSubscriptionCheckoutUrl,
  isSubscriptionPaymentSuccess,
} from './payment-response'

describe('subscription payment response helpers', () => {
  test('accepts Stripe responses that use message success with pay_link', () => {
    const response = {
      message: 'success',
      data: { pay_link: 'https://checkout.stripe.com/c/pay/session' },
    }

    assert.equal(isSubscriptionPaymentSuccess(response), true)
    assert.equal(
      getSubscriptionCheckoutUrl(response),
      'https://checkout.stripe.com/c/pay/session'
    )
  })

  test('extracts hosted checkout URLs from supported payment response shapes', () => {
    assert.equal(
      getSubscriptionCheckoutUrl({
        success: true,
        data: { checkout_url: 'https://pay.example.com/checkout' },
      }),
      'https://pay.example.com/checkout'
    )
    assert.equal(
      getSubscriptionCheckoutUrl({
        success: true,
        url: 'https://pay.example.com/direct',
      }),
      'https://pay.example.com/direct'
    )
  })
})
