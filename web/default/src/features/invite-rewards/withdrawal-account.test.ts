import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { formatWithdrawalAccount } from '@/features/withdrawals/withdrawal-account'

describe('affiliate withdrawal account formatting', () => {
  test('labels Alipay accounts for administrator review', () => {
    assert.equal(
      formatWithdrawalAccount('alipay', '  user@example.com  '),
      'Alipay: user@example.com'
    )
  })

  test('includes the USDT transfer network and address', () => {
    assert.equal(
      formatWithdrawalAccount('usdt', '  TXyzAddress  ', '  TRC20  '),
      'USDT (TRC20): TXyzAddress'
    )
  })
})
