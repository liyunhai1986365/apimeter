import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { CryptoPaymentOrder } from '../types'
import { getCryptoPaymentView } from './crypto-payment-progress'

const order = {
  status: 'pending',
  create_time: 100,
  expires_at: 1900,
} as CryptoPaymentOrder

describe('crypto payment display safety', () => {
  test('stops showing payment instructions after a transfer is detected', () => {
    const received = {
      ...order,
      progress: {
        transaction_hash: '0xabc',
        checked_at: 150,
        stage: 'confirming',
      },
    } as CryptoPaymentOrder
    const view = getCryptoPaymentView(received, 160)
    assert.equal(view.showPaymentInstructions, false)
    assert.equal(view.isPaid, false)
    assert.equal(view.hasTransfer, true)
  })

  test('confirmation completion alone never means the balance was credited', () => {
    const received = {
      ...order,
      progress: { stage: 'completed' },
    } as CryptoPaymentOrder
    assert.equal(getCryptoPaymentView(received, 160).isPaid, false)
  })

  test('expired paid orders never invite another transfer', () => {
    const expired = { ...order, status: 'expired' } as CryptoPaymentOrder
    assert.equal(
      getCryptoPaymentView(expired, 2000).showPaymentInstructions,
      false
    )
    assert.equal(getCryptoPaymentView(order, 2000).isVerifying, true)
  })

  test('query failures and old snapshots visibly delay progress, not payment status', () => {
    assert.equal(getCryptoPaymentView(order, 120, true).isDelayed, true)
    assert.equal(getCryptoPaymentView(order, 170).isDelayed, true)
    const received = {
      ...order,
      progress: { checked_at: 169 },
    } as CryptoPaymentOrder
    assert.equal(getCryptoPaymentView(received, 170).isDelayed, false)
  })

  test('manual completion is terminal even if its last snapshot is stale', () => {
    const manual = { ...order, status: 'manual' } as CryptoPaymentOrder
    const view = getCryptoPaymentView(manual, 3000, true)
    assert.equal(view.isPaid, true)
    assert.equal(view.isDelayed, false)
    assert.equal(view.showPaymentInstructions, false)
  })
})
