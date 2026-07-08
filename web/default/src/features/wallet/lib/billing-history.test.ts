import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { TopupRecord } from '../types'
import { summarizeBillingHistory } from './billing-history'

describe('wallet billing history summary', () => {
  test('summarizes the currently loaded query results', () => {
    const records: TopupRecord[] = [
      {
        id: 1,
        user_id: 10,
        amount: 100,
        money: 2.5,
        trade_no: 'paid-order',
        payment_method: 'stripe',
        payment_provider: 'stripe',
        create_time: 1700000000,
        complete_time: 1700000300,
        status: 'success',
      },
      {
        id: 2,
        user_id: 10,
        amount: 50,
        money: 1.2,
        trade_no: 'pending-order',
        payment_method: 'alipay',
        payment_provider: 'epay',
        create_time: 1700000400,
        status: 'pending',
      },
      {
        id: 3,
        user_id: 10,
        amount: 30,
        money: 0.8,
        trade_no: 'expired-order',
        payment_method: 'wxpay',
        payment_provider: 'epay',
        create_time: 1700000800,
        status: 'expired',
      },
    ]

    assert.deepEqual(summarizeBillingHistory(records), {
      currentPageCount: 3,
      successAmount: 100,
      paymentTotal: 4.5,
      pendingCount: 1,
    })
  })
})
