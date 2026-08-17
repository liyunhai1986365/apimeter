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
import { useState, useEffect, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getSelf, getSelfUsageStat } from '@/lib/api'
import { trackPurchase } from '@/lib/google-analytics'
import { useStatus } from '@/hooks/use-status'
import { SectionPageLayout } from '@/components/layout'
import { getStripePurchaseConversion } from './api'
import { BillingHistoryDialog } from './components/dialogs/billing-history-dialog'
import { CreemConfirmDialog } from './components/dialogs/creem-confirm-dialog'
import { CryptoNetworkDialog } from './components/dialogs/crypto-network-dialog'
import { CryptoPaymentDialog } from './components/dialogs/crypto-payment-dialog'
import { PaymentConfirmDialog } from './components/dialogs/payment-confirm-dialog'
import { StripeUnavailableDialog } from './components/dialogs/stripe-unavailable-dialog'
import { RechargeFormCard } from './components/recharge-form-card'
import { RedemptionCard } from './components/redemption-card'
import { StripeAutoRechargeCard } from './components/stripe-auto-recharge-card'
import { WalletStatsCard } from './components/wallet-stats-card'
import { DEFAULT_DISCOUNT_RATE } from './constants'
import {
  useTopupInfo,
  usePayment,
  useRedemption,
  useCreemPayment,
  useWaffoPayment,
  useWaffoPancakePayment,
} from './hooks'
import {
  getDefaultPaymentType,
  getMinTopupAmount,
  isCryptoPayment,
  isStripePayment,
  isWaffoPancakePayment,
} from './lib'
import type {
  UserWalletData,
  PaymentMethod,
  PresetAmount,
  CreemProduct,
  CryptoPaymentOrder,
} from './types'

interface WalletProps {
  initialShowHistory?: boolean
  stripeSessionId?: string
  stripeSetupSessionId?: string
}

const STRIPE_REDEMPTION_PURCHASE_URL = 'https://pay.ldxp.cn/shop/ESXRZFTQ'

export function Wallet(props: WalletProps) {
  const { t } = useTranslation()
  const [user, setUser] = useState<UserWalletData | null>(null)
  const [userLoading, setUserLoading] = useState(true)
  const [topupAmount, setTopupAmount] = useState(0)
  const [selectedPreset, setSelectedPreset] = useState<number | null>(null)
  const [selectedPaymentMethod, setSelectedPaymentMethod] =
    useState<PaymentMethod>()
  const [paymentLoading, setPaymentLoading] = useState<string | null>(null)
  const [confirmDialogOpen, setConfirmDialogOpen] = useState(false)
  const [stripeUnavailableOpen, setStripeUnavailableOpen] = useState(false)
  const [billingDialogOpen, setBillingDialogOpen] = useState(false)
  const [redemptionCode, setRedemptionCode] = useState('')
  const [creemDialogOpen, setCreemDialogOpen] = useState(false)
  const [cryptoNetworkDialogOpen, setCryptoNetworkDialogOpen] = useState(false)
  const [cryptoDialogOpen, setCryptoDialogOpen] = useState(false)
  const [cryptoOrder, setCryptoOrder] = useState<CryptoPaymentOrder | null>(
    null
  )
  const [selectedCreemProduct, setSelectedCreemProduct] =
    useState<CreemProduct | null>(null)

  const { status } = useStatus()
  const totalUsageQuery = useQuery({
    queryKey: ['user', 'self', 'usage-stat'],
    queryFn: getSelfUsageStat,
    staleTime: 60 * 1000,
  })
  const { topupInfo, presetAmounts, loading: topupLoading } = useTopupInfo()
  const {
    amount: paymentAmount,
    calculating,
    processing,
    calculatePaymentAmount,
    processPayment,
  } = usePayment()
  const { redeeming, redeemCode } = useRedemption()
  const { processing: creemProcessing, processCreemPayment } = useCreemPayment()
  const { processWaffoPayment } = useWaffoPayment()
  const { processing: pancakeProcessing, processWaffoPancakePayment } =
    useWaffoPancakePayment()

  // Fetch and refresh user data
  const fetchUser = useCallback(async () => {
    try {
      setUserLoading(true)
      const response = await getSelf()
      if (response.success && response.data) {
        setUser(response.data as UserWalletData)
      }
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to fetch user data:', error)
    } finally {
      setUserLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchUser()
  }, [fetchUser])

  useEffect(() => {
    if (props.initialShowHistory) {
      setBillingDialogOpen(true)
      const returnURL = new URL(window.location.href)
      returnURL.searchParams.delete('show_history')
      window.history.replaceState({}, '', returnURL)
    }
  }, [props.initialShowHistory])

  useEffect(() => {
    if (!props.stripeSessionId) return

    let cancelled = false
    getStripePurchaseConversion(props.stripeSessionId)
      .then((response) => {
        const conversion = response.data
        if (
          cancelled ||
          !response.success ||
          conversion?.status !== 'paid' ||
          !conversion.transaction_id ||
          conversion.value == null ||
          !conversion.currency
        ) {
          return
        }
        trackPurchase({
          transactionId: conversion.transaction_id,
          value: conversion.value,
          currency: conversion.currency,
        })
        const returnURL = new URL(window.location.href)
        returnURL.searchParams.delete('stripe_session_id')
        window.history.replaceState({}, '', returnURL)
      })
      .catch(() => {
        // The payment history remains available if confirmation is delayed.
      })

    return () => {
      cancelled = true
    }
  }, [props.stripeSessionId])

  // Initialize topup amount when topup info is loaded
  useEffect(() => {
    if (topupInfo && topupAmount === 0) {
      const minTopup = getMinTopupAmount(topupInfo)
      setTopupAmount(minTopup)

      // Calculate initial payment amount with default payment type
      const defaultPaymentType = getDefaultPaymentType(topupInfo)
      calculatePaymentAmount(minTopup, defaultPaymentType)
    }
  }, [topupInfo, topupAmount, calculatePaymentAmount])

  // Get current payment type (selected or default)
  const getCurrentPaymentType = useCallback(() => {
    return selectedPaymentMethod?.type || getDefaultPaymentType(topupInfo)
  }, [selectedPaymentMethod, topupInfo])

  // Handle preset selection
  const handleSelectPreset = (preset: PresetAmount) => {
    setTopupAmount(preset.value)
    setSelectedPreset(preset.value)
    calculatePaymentAmount(preset.value, getCurrentPaymentType())
  }

  // Handle topup amount change
  const handleTopupAmountChange = (amount: number) => {
    setTopupAmount(amount)
    setSelectedPreset(null)
    calculatePaymentAmount(amount, getCurrentPaymentType())
  }

  // Handle payment method selection
  const handlePaymentMethodSelect = async (method: PaymentMethod) => {
    setSelectedPaymentMethod(method)

    if (isStripePayment(method.type)) {
      setStripeUnavailableOpen(true)
      return
    }

    setPaymentLoading(method.type)

    try {
      // Validate minimum topup
      const minTopup = getMinTopupAmount(topupInfo)
      if (topupAmount < minTopup) {
        return
      }

      // Calculate payment amount and show confirmation dialog
      await calculatePaymentAmount(topupAmount, method.type)
      setConfirmDialogOpen(true)
    } finally {
      setPaymentLoading(null)
    }
  }

  const cryptoPaymentMethods =
    topupInfo?.pay_methods?.filter((method) => isCryptoPayment(method.type)) ??
    []

  const handleCryptoPaymentOpen = async () => {
    const availableMethods = cryptoPaymentMethods.filter(
      (method) => (method.min_topup || 0) <= topupAmount
    )
    const method =
      availableMethods.find(
        (item) => item.type === selectedPaymentMethod?.type
      ) ?? availableMethods[0]

    if (!method) return

    setSelectedPaymentMethod(method)
    setCryptoNetworkDialogOpen(true)
    await calculatePaymentAmount(topupAmount, method.type)
  }

  const handleCryptoNetworkChange = (method: PaymentMethod) => {
    setSelectedPaymentMethod(method)
    void calculatePaymentAmount(topupAmount, method.type)
  }

  const handleCryptoNetworkConfirm = async () => {
    if (
      !selectedPaymentMethod ||
      !isCryptoPayment(selectedPaymentMethod.type)
    ) {
      return
    }

    const result = await processPayment(topupAmount, selectedPaymentMethod.type)
    if (result.success && result.cryptoOrder) {
      setCryptoNetworkDialogOpen(false)
      setCryptoOrder(result.cryptoOrder)
      setCryptoDialogOpen(true)
    }
  }

  // Handle payment confirmation
  const handlePaymentConfirm = async () => {
    if (!selectedPaymentMethod) return

    const isPancake = isWaffoPancakePayment(selectedPaymentMethod.type)
    const result = isPancake
      ? {
          success: await processWaffoPancakePayment(topupAmount),
          cryptoOrder: undefined,
        }
      : await processPayment(topupAmount, selectedPaymentMethod.type)

    if (result.success) {
      setConfirmDialogOpen(false)
      if (result.cryptoOrder) {
        setCryptoOrder(result.cryptoOrder)
        setCryptoDialogOpen(true)
      } else {
        await fetchUser()
      }
    }
  }

  const handleCryptoPaid = useCallback(async () => {
    await fetchUser()
  }, [fetchUser])

  // Handle redemption
  const handleRedeem = async () => {
    if (!redemptionCode) return

    const success = await redeemCode(redemptionCode)
    if (success) {
      setRedemptionCode('')
      await fetchUser()
    }
  }

  // Handle Creem product selection
  const handleCreemProductSelect = (product: CreemProduct) => {
    setSelectedCreemProduct(product)
    setCreemDialogOpen(true)
  }

  // Handle Creem payment confirmation
  const handleCreemConfirm = async () => {
    if (!selectedCreemProduct) return

    const success = await processCreemPayment(selectedCreemProduct.productId)
    if (success) {
      setCreemDialogOpen(false)
      setSelectedCreemProduct(null)
      await fetchUser()
    }
  }

  const handleWaffoMethodSelect = async (_method: unknown, index: number) => {
    const loadingKey = `waffo-${index}`
    setPaymentLoading(loadingKey)

    try {
      await processWaffoPayment(topupAmount, index)
    } finally {
      setPaymentLoading(null)
    }
  }

  // Get discount rate for current topup amount
  const getDiscountRate = useCallback(() => {
    return topupInfo?.discount?.[topupAmount] || DEFAULT_DISCOUNT_RATE
  }, [topupInfo, topupAmount])

  const activePaymentMethod =
    selectedPaymentMethod ??
    topupInfo?.pay_methods?.find(
      (method) => method.type === getDefaultPaymentType(topupInfo)
    )

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Balance Top-up')}</SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t('Manage your balance and payment methods')}
        </SectionPageLayout.Description>
        <SectionPageLayout.Content>
          <div className='mx-auto flex w-full max-w-7xl flex-col gap-4 sm:gap-6'>
            <WalletStatsCard
              user={user}
              totalUsedQuota={totalUsageQuery.data?.data?.quota}
              forecast={topupInfo?.balance_forecast ?? null}
              forecastLoading={topupLoading}
              loading={userLoading}
            />

            <div className='grid gap-4 sm:gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(360px,0.46fr)] xl:items-start'>
              <div id='wallet-add-funds' className='scroll-mt-4'>
                <RechargeFormCard
                  topupInfo={topupInfo}
                  presetAmounts={presetAmounts}
                  selectedPreset={selectedPreset}
                  onSelectPreset={handleSelectPreset}
                  topupAmount={topupAmount}
                  onTopupAmountChange={handleTopupAmountChange}
                  paymentAmount={paymentAmount}
                  paymentMethod={activePaymentMethod}
                  calculating={calculating}
                  onPaymentMethodSelect={handlePaymentMethodSelect}
                  onCryptoPaymentOpen={handleCryptoPaymentOpen}
                  paymentLoading={paymentLoading}
                  loading={topupLoading}
                  priceRatio={(status?.price as number) || 1}
                  onOpenBilling={() => setBillingDialogOpen(true)}
                  creemProducts={topupInfo?.creem_products}
                  enableCreemTopup={topupInfo?.enable_creem_topup}
                  onCreemProductSelect={handleCreemProductSelect}
                  enableWaffoTopup={topupInfo?.enable_waffo_topup}
                  waffoPayMethods={topupInfo?.waffo_pay_methods}
                  waffoMinTopup={topupInfo?.waffo_min_topup}
                  onWaffoMethodSelect={handleWaffoMethodSelect}
                  enableWaffoPancakeTopup={
                    topupInfo?.enable_waffo_pancake_topup
                  }
                />
              </div>

              <div className='flex flex-col gap-4 sm:gap-5'>
                {topupInfo?.enable_stripe_topup && (
                  <StripeAutoRechargeCard
                    setupSessionId={props.stripeSetupSessionId}
                  />
                )}
                <RedemptionCard
                  code={redemptionCode}
                  onCodeChange={setRedemptionCode}
                  onRedeem={handleRedeem}
                  redeeming={redeeming}
                  enabled={topupInfo?.enable_redemption !== false}
                  topupLink={topupInfo?.topup_link}
                  loading={topupLoading}
                />
              </div>
            </div>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <PaymentConfirmDialog
        open={confirmDialogOpen}
        onOpenChange={setConfirmDialogOpen}
        onConfirm={handlePaymentConfirm}
        topupAmount={topupAmount}
        paymentAmount={paymentAmount}
        paymentMethod={selectedPaymentMethod}
        calculating={calculating}
        processing={processing || pancakeProcessing}
        discountRate={getDiscountRate()}
      />

      <StripeUnavailableDialog
        open={stripeUnavailableOpen}
        onOpenChange={setStripeUnavailableOpen}
        purchaseUrl={STRIPE_REDEMPTION_PURCHASE_URL}
      />

      <CryptoNetworkDialog
        open={cryptoNetworkDialogOpen}
        onOpenChange={setCryptoNetworkDialogOpen}
        methods={cryptoPaymentMethods}
        selectedMethod={
          selectedPaymentMethod && isCryptoPayment(selectedPaymentMethod.type)
            ? selectedPaymentMethod
            : undefined
        }
        onNetworkChange={handleCryptoNetworkChange}
        onConfirm={handleCryptoNetworkConfirm}
        topupAmount={topupAmount}
        paymentAmount={paymentAmount}
        calculating={calculating}
        processing={processing}
      />

      <CryptoPaymentDialog
        key={cryptoOrder?.trade_no ?? 'no-crypto-order'}
        open={cryptoDialogOpen}
        onOpenChange={setCryptoDialogOpen}
        order={cryptoOrder}
        onPaid={handleCryptoPaid}
      />

      <BillingHistoryDialog
        open={billingDialogOpen}
        onOpenChange={setBillingDialogOpen}
      />

      <CreemConfirmDialog
        open={creemDialogOpen}
        onOpenChange={setCreemDialogOpen}
        onConfirm={handleCreemConfirm}
        product={selectedCreemProduct}
        processing={creemProcessing}
      />
    </>
  )
}
