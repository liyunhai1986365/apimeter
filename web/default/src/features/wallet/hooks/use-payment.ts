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
  calculateAmount,
  calculateStripeAmount,
  calculateWaffoPancakeAmount,
  calculateCryptoAmount,
  requestPayment,
  requestStripePayment,
  isApiSuccess,
  requestCryptoPayment,
} from '../api'
import {
  getCryptoNetwork,
  isStripePayment,
  isWaffoPancakePayment,
  submitPaymentForm,
} from '../lib'
import type { CryptoPaymentOrder } from '../types'

export type ProcessPaymentResult = {
  success: boolean
  cryptoOrder?: CryptoPaymentOrder
}

// ============================================================================
// Payment Hook
// ============================================================================

export function usePayment() {
  const [amount, setAmount] = useState<number>(0)
  const [calculating, setCalculating] = useState(false)
  const [processing, setProcessing] = useState(false)

  // Calculate payment amount
  const calculatePaymentAmount = useCallback(
    async (topupAmount: number, paymentType: string) => {
      try {
        setCalculating(true)

        const isStripe = isStripePayment(paymentType)
        const isPancake = isWaffoPancakePayment(paymentType)
        const cryptoNetwork = getCryptoNetwork(paymentType)
        const response = isStripe
          ? await calculateStripeAmount({ amount: topupAmount })
          : cryptoNetwork
            ? await calculateCryptoAmount({
                amount: topupAmount,
                network: cryptoNetwork,
              })
            : isPancake
              ? await calculateWaffoPancakeAmount({ amount: topupAmount })
              : await calculateAmount({ amount: topupAmount })

        if (isApiSuccess(response) && response.data) {
          const calculatedAmount = parseFloat(response.data)
          setAmount(calculatedAmount)
          return calculatedAmount
        }

        // Don't show error for calculation, just set to 0
        setAmount(0)
        return 0
      } catch (_error) {
        setAmount(0)
        return 0
      } finally {
        setCalculating(false)
      }
    },
    []
  )

  // Process payment
  const processPayment = useCallback(
    async (
      topupAmount: number,
      paymentType: string
    ): Promise<ProcessPaymentResult> => {
      try {
        setProcessing(true)

        const isStripe = isStripePayment(paymentType)
        const cryptoNetwork = getCryptoNetwork(paymentType)
        const amount = Math.floor(topupAmount)

        if (cryptoNetwork) {
          const response = await requestCryptoPayment({
            amount,
            network: cryptoNetwork,
          })
          if (!isApiSuccess(response) || !response.data) {
            toast.error(response.message || i18next.t('Payment request failed'))
            return { success: false }
          }
          return { success: true, cryptoOrder: response.data }
        }

        if (isStripe) {
          const response = await requestStripePayment({
            amount,
            payment_method: 'stripe',
          })
          if (!isApiSuccess(response) || !response.data?.pay_link) {
            toast.error(response.message || i18next.t('Payment request failed'))
            return { success: false }
          }
          window.open(response.data.pay_link, '_blank')
          toast.success(i18next.t('Redirecting to payment page...'))
          return { success: true }
        }

        const response = await requestPayment({
          amount,
          payment_method: paymentType,
        })

        if (!isApiSuccess(response)) {
          toast.error(response.message || i18next.t('Payment request failed'))
          return { success: false } satisfies ProcessPaymentResult
        }

        if (response.data) {
          const url = (response as unknown as { url?: string }).url
          if (url) {
            submitPaymentForm(url, response.data)
            toast.success(i18next.t('Redirecting to payment page...'))
            return { success: true } satisfies ProcessPaymentResult
          }
        }

        return { success: false } satisfies ProcessPaymentResult
      } catch (_error) {
        toast.error(i18next.t('Payment request failed'))
        return { success: false } satisfies ProcessPaymentResult
      } finally {
        setProcessing(false)
      }
    },
    []
  )

  return {
    amount,
    calculating,
    processing,
    calculatePaymentAmount,
    processPayment,
    setAmount,
  }
}
