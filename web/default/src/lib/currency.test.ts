import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { formatCurrencyFromUSD, formatQuotaWithCurrency } from './currency'

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

describe('currency display preference', () => {
  test('defaults to USD formatting for system USD amounts', () => {
    resetCurrency('USD')

    assert.equal(formatCurrencyFromUSD(10), '$10')
    assert.equal(formatQuotaWithCurrency(5000000), '$10')
  })

  test('formats CNY using the configured USD exchange rate', () => {
    resetCurrency('CNY')

    assert.equal(formatCurrencyFromUSD(10), '¥73')
    assert.equal(formatQuotaWithCurrency(5000000), '¥73')
  })
})
