import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { formatCurrency, formatPaymentAmountFromUSD } from './format'

function resetCurrency(displayCurrency: 'USD' | 'CNY') {
  useSystemConfigStore.setState((state) => ({
    ...state,
    displayCurrency,
    config: {
      ...state.config,
      currency: {
        ...state.config.currency,
        quotaDisplayType: 'USD',
        quotaPerUnit: 500000,
        usdExchangeRate: 7.3,
      },
    },
  }))
}

describe('wallet currency formatting', () => {
  test('formats local CNY payment amounts as CNY', () => {
    resetCurrency('CNY')

    assert.equal(formatCurrency(73), '¥73')
  })

  test('converts local CNY payment amounts back to USD for USD display', () => {
    resetCurrency('USD')

    assert.equal(formatCurrency(73), '$10')
  })

  test('keeps provider USD payment amounts unchanged for USD display', () => {
    resetCurrency('USD')

    assert.equal(formatPaymentAmountFromUSD(73), '$73')
  })

  test('converts provider USD payment amounts for CNY display', () => {
    resetCurrency('CNY')

    assert.equal(formatPaymentAmountFromUSD(10), '¥73')
  })
})
