/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useState, useCallback } from 'react'
import i18next from 'i18next'
import { toast } from 'sonner'
import {
  closePendingPaymentWindow,
  isSafePaymentUrl,
  navigatePendingPaymentWindow,
  openPendingPaymentWindow,
} from '@/lib/payment-window'
import { requestWaffoPancakePayment, isApiSuccess } from '../api'

function getCheckoutUrl(data: unknown): string | null {
  if (!data || typeof data !== 'object') {
    return null
  }

  if ('checkout_url' in data && typeof data.checkout_url === 'string') {
    return data.checkout_url
  }

  return null
}

function getErrorMessage(message: string | undefined, data: unknown): string {
  if (typeof data === 'string' && data.trim()) {
    return data
  }

  return message || i18next.t('Payment request failed')
}

/**
 * Hook for the Waffo Pancake hosted-checkout flow.
 *
 * A blank page is opened while the click still has user-gesture context, then
 * navigated after the API returns to avoid asynchronous popup blocking.
 */
export function useWaffoPancakePayment() {
  const [processing, setProcessing] = useState(false)

  const processWaffoPancakePayment = useCallback(
    async (topupAmount: number) => {
      const paymentWindow = openPendingPaymentWindow()
      let paymentPageOpened = false
      setProcessing(true)

      try {
        const response = await requestWaffoPancakePayment({
          amount: Math.floor(topupAmount),
        })

        if (isApiSuccess(response)) {
          const checkoutUrl = getCheckoutUrl(response.data)

          if (checkoutUrl) {
            if (!isSafePaymentUrl(checkoutUrl)) {
              toast.error(i18next.t('Invalid payment redirect URL'))
              return false
            }
            navigatePendingPaymentWindow(paymentWindow, checkoutUrl)
            paymentPageOpened = true
            toast.success(i18next.t('Payment page opened'))
            return true
          }
        }

        toast.error(getErrorMessage(response.message, response.data))
        return false
      } catch (_error) {
        toast.error(i18next.t('Payment request failed'))
        return false
      } finally {
        if (!paymentPageOpened) {
          closePendingPaymentWindow(paymentWindow)
        }
        setProcessing(false)
      }
    },
    []
  )

  return { processing, processWaffoPancakePayment }
}
