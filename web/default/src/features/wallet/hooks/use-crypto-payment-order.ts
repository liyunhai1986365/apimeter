import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getCryptoPaymentOrder, isApiSuccess } from '../api'
import type { CryptoPaymentOrder } from '../types'

export function useCryptoPaymentOrder(
  order: CryptoPaymentOrder | null,
  open: boolean,
  onPaid: () => void | Promise<void>
) {
  const [now, setNow] = useState(() => Math.floor(Date.now() / 1000))
  const paidNotified = useRef<string | null>(null)
  const tradeNo = order?.trade_no
  const query = useQuery({
    queryKey: ['crypto-payment-order', tradeNo],
    queryFn: async ({ signal }) => {
      if (!tradeNo) throw new Error('Missing crypto payment order')
      const response = await getCryptoPaymentOrder(tradeNo, signal)
      if (!isApiSuccess(response) || !response.data) {
        throw new Error('Crypto payment status unavailable')
      }
      return response.data
    },
    initialData: order ?? undefined,
    enabled: open && Boolean(order),
    staleTime: 0,
    refetchOnWindowFocus: true,
    refetchOnReconnect: true,
    refetchInterval: (current) => {
      const status = current.state.data?.status
      // Keep expired orders observable while support verifies a manual credit.
      return status === 'success' || status === 'manual' ? false : 5000
    },
    refetchIntervalInBackground: false,
    retry: 1,
  })

  useEffect(() => {
    if (!open) return
    const timer = window.setInterval(() => {
      setNow(Math.floor(Date.now() / 1000))
    }, 1000)
    return () => window.clearInterval(timer)
  }, [open])

  const currentOrder = query.data ?? order
  useEffect(() => {
    if (
      !currentOrder ||
      (currentOrder.status !== 'success' && currentOrder.status !== 'manual') ||
      paidNotified.current === currentOrder.trade_no
    )
      return
    paidNotified.current = currentOrder.trade_no
    void onPaid()
  }, [currentOrder, onPaid])

  return { currentOrder, now, pollingFailed: query.isError }
}
