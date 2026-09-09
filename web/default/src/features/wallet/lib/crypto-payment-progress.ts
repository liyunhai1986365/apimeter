import type { CryptoPaymentOrder } from '../types'

export function getCryptoPaymentView(
  order: CryptoPaymentOrder,
  now: number,
  pollingFailed = false
) {
  const isPaid = order.status === 'success' || order.status === 'manual'
  const isExpired = order.status === 'expired'
  const remaining = Math.max(0, order.expires_at - now)
  const transactionHash =
    order.transaction_hash || order.progress?.transaction_hash
  const hasTransfer = Boolean(transactionHash)
  const lastChecked = order.progress?.checked_at || order.create_time
  const isDelayed =
    !isPaid &&
    (pollingFailed ||
      (!isExpired &&
        (order.progress?.retrying ||
          order.progress?.stale ||
          now - lastChecked > 60)))

  return {
    isPaid,
    isExpired,
    remaining,
    transactionHash,
    hasTransfer,
    isDelayed: Boolean(isDelayed),
    isVerifying: order.status === 'pending' && remaining === 0,
    showPaymentInstructions:
      order.status === 'pending' && remaining > 0 && !hasTransfer,
  }
}
