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
import { useState, useEffect } from 'react'
import { CreditCard, Loader2, Receipt, WalletCards } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { SiStripe } from 'react-icons/si'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { cn } from '@/lib/utils'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { TitledCard } from '@/components/ui/titled-card'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { DiscountTooltip } from '@/components/discount-tooltip'
import { PAYMENT_TYPES } from '../constants'
import {
  formatCurrency,
  formatPaymentAmountFromUSD,
  getDiscountLabel,
  getDefaultPaymentType,
  getPaymentIcon,
  getMinTopupAmount,
  calculatePresetPricing,
  isCryptoPayment,
  isWaffoPancakePayment,
} from '../lib'
import type {
  PaymentMethod,
  PresetAmount,
  TopupInfo,
  CreemProduct,
  WaffoPayMethod,
} from '../types'
import { CreemProductsSection } from './creem-products-section'

interface RechargeFormCardProps {
  topupInfo: TopupInfo | null
  presetAmounts: PresetAmount[]
  selectedPreset: number | null
  onSelectPreset: (preset: PresetAmount) => void
  topupAmount: number
  onTopupAmountChange: (amount: number) => void
  paymentAmount: number
  paymentMethod?: PaymentMethod
  calculating: boolean
  onPaymentMethodSelect: (method: PaymentMethod) => void
  onCryptoPaymentOpen: () => void
  paymentLoading: string | null
  loading?: boolean
  priceRatio?: number
  onOpenBilling?: () => void
  creemProducts?: CreemProduct[]
  enableCreemTopup?: boolean
  onCreemProductSelect?: (product: CreemProduct) => void
  enableWaffoTopup?: boolean
  waffoPayMethods?: WaffoPayMethod[]
  waffoMinTopup?: number
  onWaffoMethodSelect?: (method: WaffoPayMethod, index: number) => void
  enableWaffoPancakeTopup?: boolean
}

export function RechargeFormCard({
  topupInfo,
  presetAmounts,
  selectedPreset,
  onSelectPreset,
  topupAmount,
  onTopupAmountChange,
  paymentAmount,
  paymentMethod,
  calculating,
  onPaymentMethodSelect,
  onCryptoPaymentOpen,
  paymentLoading,
  loading,
  priceRatio = 1,
  onOpenBilling,
  creemProducts,
  enableCreemTopup,
  onCreemProductSelect,
  enableWaffoTopup,
  waffoPayMethods,
  waffoMinTopup,
  onWaffoMethodSelect,
  enableWaffoPancakeTopup,
}: RechargeFormCardProps) {
  const { t } = useTranslation()
  const [localAmount, setLocalAmount] = useState(topupAmount.toString())

  useEffect(() => {
    setLocalAmount(topupAmount.toString())
  }, [topupAmount])

  const handleAmountChange = (value: string) => {
    setLocalAmount(value)
    const numValue = parseInt(value) || 0
    if (numValue >= 0) {
      onTopupAmountChange(numValue)
    }
  }

  const hasConfigurableTopup =
    topupInfo?.enable_online_topup ||
    topupInfo?.enable_stripe_topup ||
    topupInfo?.enable_crypto_topup ||
    enableWaffoTopup ||
    enableWaffoPancakeTopup
  const hasAnyTopup = hasConfigurableTopup || enableCreemTopup
  const paymentMethods = topupInfo?.pay_methods ?? []
  const standardPaymentMethods = paymentMethods.filter(
    (method) => !isCryptoPayment(method.type)
  )
  const cryptoPaymentMethods = paymentMethods.filter((method) =>
    isCryptoPayment(method.type)
  )
  const hasStandardPaymentMethods = standardPaymentMethods.length > 0
  const hasCryptoPaymentMethods = cryptoPaymentMethods.length > 0
  const defaultPaymentType = getDefaultPaymentType(topupInfo)
  const hasPrimaryPayment = standardPaymentMethods.some(
    (method) => method.type === defaultPaymentType
  )
  const cryptoPaymentAvailable = cryptoPaymentMethods.some(
    (method) => (method.min_topup || 0) <= topupAmount
  )
  const hasWaffoPaymentMethods =
    Array.isArray(waffoPayMethods) && waffoPayMethods.length > 0
  const minTopup = getMinTopupAmount(topupInfo)
  const cryptoPayment = isCryptoPayment(paymentMethod?.type ?? '')
  const waffoPancakePayment = isWaffoPancakePayment(paymentMethod?.type ?? '')
  const stripeSupportItems = [
    {
      label: t('Bank card'),
      icon: <CreditCard className='h-3.5 w-3.5 text-slate-600' />,
    },
    {
      label: t('WeChat Pay'),
      icon: getPaymentIcon(PAYMENT_TYPES.WECHAT, 'h-3.5 w-3.5'),
    },
    {
      label: t('Alipay'),
      icon: getPaymentIcon(PAYMENT_TYPES.ALIPAY, 'h-3.5 w-3.5'),
    },
  ]

  if (loading) {
    return (
      <Card className='gap-0 overflow-hidden py-0'>
        <CardHeader className='border-b p-3 !pb-3 sm:p-5 sm:!pb-5'>
          <Skeleton className='h-6 w-32' />
          <Skeleton className='mt-2 h-4 w-48' />
        </CardHeader>
        <CardContent className='space-y-4 p-3 sm:space-y-6 sm:p-5'>
          <div className='space-y-4 sm:space-y-6'>
            {/* Preset Amounts Skeleton */}
            <div className='space-y-3'>
              <Skeleton className='h-3 w-16' />
              <div className='grid grid-cols-2 gap-3 sm:grid-cols-4'>
                {Array.from({ length: 8 }).map((_, i) => (
                  <Skeleton key={i} className='h-[72px] rounded-lg' />
                ))}
              </div>
            </div>

            {/* Custom Amount Input Skeleton */}
            <div className='space-y-3'>
              <Skeleton className='h-3 w-28' />
              <Skeleton className='h-[42px] w-full' />
            </div>

            {/* Payment Methods Skeleton */}
            <div className='space-y-3'>
              <Skeleton className='h-3 w-32' />
              <div className='flex flex-wrap gap-3'>
                {Array.from({ length: 3 }).map((_, i) => (
                  <Skeleton key={i} className='h-10 w-24 rounded-lg' />
                ))}
              </div>
            </div>
          </div>
        </CardContent>
      </Card>
    )
  }

  return (
    <TitledCard
      title={t('Add Funds')}
      description={t('Choose an amount and payment method')}
      icon={<WalletCards className='h-4 w-4' />}
      action={
        onOpenBilling ? (
          <Button
            variant='outline'
            size='sm'
            onClick={onOpenBilling}
            className='w-full gap-2 sm:w-auto'
          >
            <Receipt className='h-4 w-4' />
            {t('Order History')}
          </Button>
        ) : null
      }
      contentClassName='space-y-4 sm:space-y-6'
    >
      {/* Online Topup Section */}
      {hasAnyTopup ? (
        <div className='space-y-4 sm:space-y-6'>
          {hasConfigurableTopup && (
            <>
              {presetAmounts.length > 0 && (
                <div className='space-y-2.5 sm:space-y-3'>
                  <Label className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
                    {t('Amount')}
                  </Label>
                  <div className='grid grid-cols-2 gap-1.5 sm:gap-3 md:grid-cols-4'>
                    {presetAmounts.map((preset, index) => {
                      const discount =
                        preset.discount ||
                        topupInfo?.discount?.[preset.value] ||
                        1.0
                      const { actualPrice, savedAmount, hasDiscount } =
                        calculatePresetPricing(
                          preset.value,
                          priceRatio,
                          discount
                        )
                      return (
                        <Button
                          key={index}
                          variant='outline'
                          className={cn(
                            'hover:border-foreground flex min-h-16 flex-col items-start rounded-lg px-3 py-2.5 text-left whitespace-normal sm:min-h-[72px] sm:p-4',
                            selectedPreset === preset.value
                              ? 'border-foreground bg-foreground/5 dark:border-foreground dark:bg-foreground/10'
                              : 'border-muted'
                          )}
                          onClick={() => onSelectPreset(preset)}
                        >
                          <div className='flex w-full items-center justify-between'>
                            <div className='text-base font-semibold sm:text-lg'>
                              {formatCurrencyFromUSD(preset.value, {
                                digitsLarge: 2,
                                digitsSmall: 2,
                                abbreviate: false,
                              })}
                            </div>
                            {hasDiscount && (
                              <DiscountTooltip
                                label={getDiscountLabel(discount)}
                              >
                                <div className='text-xs font-medium text-green-600'>
                                  {getDiscountLabel(discount)}
                                </div>
                              </DiscountTooltip>
                            )}
                          </div>
                          <div className='text-muted-foreground mt-1.5 w-full text-xs sm:mt-2'>
                            Pay {formatCurrency(actualPrice)}
                            {hasDiscount && savedAmount > 0 && (
                              <span className='text-green-600'>
                                {' '}
                                • Save {formatCurrency(savedAmount)}
                              </span>
                            )}
                          </div>
                        </Button>
                      )
                    })}
                  </div>
                </div>
              )}

              <div className='space-y-2.5 sm:space-y-3'>
                <Label
                  htmlFor='topup-amount'
                  className='text-muted-foreground text-xs font-medium tracking-wider uppercase'
                >
                  {t('Custom Amount')}
                </Label>
                <div className='grid grid-cols-[minmax(0,1fr)_minmax(110px,0.55fr)] gap-2 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center'>
                  <Input
                    id='topup-amount'
                    type='number'
                    value={localAmount}
                    onChange={(e) => handleAmountChange(e.target.value)}
                    min={minTopup}
                    placeholder={`Minimum ${minTopup}`}
                    className='h-9 text-base sm:h-10 sm:text-lg'
                  />
                  <div className='bg-muted/30 flex min-h-9 items-center justify-between gap-2 rounded-md border px-3 lg:min-w-52'>
                    <span className='text-muted-foreground truncate text-xs'>
                      {t('Amount to pay:')}
                    </span>
                    {calculating ? (
                      <Skeleton className='h-5 w-16' />
                    ) : (
                      <span className='text-sm font-semibold'>
                        {cryptoPayment
                          ? `${paymentAmount} ${paymentMethod?.token_symbol ?? ''}`
                          : waffoPancakePayment
                            ? formatPaymentAmountFromUSD(paymentAmount, {
                                digitsLarge: 2,
                                digitsSmall: 2,
                                abbreviate: false,
                              })
                            : formatCurrency(paymentAmount)}
                      </span>
                    )}
                  </div>
                </div>
              </div>

              <div className='space-y-2.5 sm:space-y-3'>
                <Label className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
                  {t('Payment Method')}
                </Label>
                {hasStandardPaymentMethods || hasCryptoPaymentMethods ? (
                  <div className='grid grid-cols-2 gap-1.5 sm:gap-3 lg:grid-cols-3'>
                    {standardPaymentMethods.map((method) => {
                      const minTopup = method.min_topup || 0
                      const disabled = minTopup > topupAmount
                      const isStripe = method.type === PAYMENT_TYPES.STRIPE
                      const isPrimary = method.type === defaultPaymentType

                      const button = (
                        <Button
                          variant={isPrimary ? 'default' : 'outline'}
                          onClick={() => onPaymentMethodSelect(method)}
                          disabled={disabled || !!paymentLoading}
                          className={cn(
                            'h-9 w-full min-w-0 justify-start gap-2 rounded-lg px-3',
                            isPrimary &&
                              'h-12 justify-center text-base font-semibold shadow-lg disabled:opacity-70'
                          )}
                        >
                          {paymentLoading === method.type ? (
                            <Loader2 className='h-4 w-4 animate-spin' />
                          ) : isStripe ? (
                            <SiStripe
                              className={cn(
                                'size-5',
                                isPrimary && 'text-primary-foreground'
                              )}
                            />
                          ) : (
                            getPaymentIcon(
                              method.type,
                              'h-4 w-4',
                              method.icon,
                              method.name
                            )
                          )}
                          <span className='truncate'>
                            {isPrimary ? t('Pay Now') : method.name}
                          </span>
                        </Button>
                      )

                      return (
                        <div
                          key={method.type}
                          className={cn(
                            'min-w-0',
                            isPrimary &&
                              'col-span-2 flex flex-col gap-2 lg:col-span-3'
                          )}
                        >
                          <div
                            className={cn(
                              isPrimary && hasCryptoPaymentMethods
                                ? 'grid grid-cols-[minmax(0,1fr)_minmax(104px,0.28fr)] gap-2'
                                : 'contents'
                            )}
                          >
                            {disabled ? (
                              <TooltipProvider>
                                <Tooltip>
                                  <TooltipTrigger
                                    render={button}
                                  ></TooltipTrigger>
                                  <TooltipContent>
                                    {t('Minimum topup amount: {{amount}}', {
                                      amount: minTopup,
                                    })}
                                  </TooltipContent>
                                </Tooltip>
                              </TooltipProvider>
                            ) : (
                              button
                            )}
                            {isPrimary && hasCryptoPaymentMethods && (
                              <Button
                                variant='outline'
                                onClick={onCryptoPaymentOpen}
                                disabled={
                                  !cryptoPaymentAvailable || !!paymentLoading
                                }
                                className='h-12 min-w-0 gap-1.5 rounded-lg px-2 text-sm font-semibold'
                              >
                                <WalletCards data-icon='inline-start' />
                                <span className='truncate'>
                                  {t('USDT Transfer')}
                                </span>
                              </Button>
                            )}
                          </div>
                          {isStripe && isPrimary && (
                            <div className='text-muted-foreground flex flex-wrap items-center justify-center gap-x-3 gap-y-1 text-center text-xs'>
                              <span>{t('Supports')}</span>
                              {stripeSupportItems.map((item) => (
                                <span
                                  key={item.label}
                                  className='inline-flex items-center gap-1 whitespace-nowrap'
                                >
                                  {item.icon}
                                  {item.label}
                                </span>
                              ))}
                            </div>
                          )}
                        </div>
                      )
                    })}
                    {!hasPrimaryPayment && hasCryptoPaymentMethods && (
                      <Button
                        variant='outline'
                        onClick={onCryptoPaymentOpen}
                        disabled={!cryptoPaymentAvailable || !!paymentLoading}
                        className='col-span-2 h-12 gap-2 rounded-lg text-sm font-semibold lg:col-span-3'
                      >
                        <WalletCards data-icon='inline-start' />
                        {t('USDT Transfer')}
                      </Button>
                    )}
                  </div>
                ) : hasWaffoPaymentMethods ? null : (
                  <Alert>
                    <AlertDescription>
                      {t(
                        'No payment methods available. Please contact administrator.'
                      )}
                    </AlertDescription>
                  </Alert>
                )}
              </div>

              {enableWaffoTopup &&
                hasWaffoPaymentMethods &&
                onWaffoMethodSelect && (
                  <div className='space-y-2.5 sm:space-y-3'>
                    <Label className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
                      {t('Waffo Payment')}
                    </Label>
                    <div className='grid grid-cols-2 gap-1.5 sm:gap-3 lg:grid-cols-3'>
                      {waffoPayMethods?.map((method, index) => {
                        const loadingKey = `waffo-${index}`
                        const waffoMin = waffoMinTopup || 0
                        const belowMin = waffoMin > topupAmount

                        const button = (
                          <Button
                            key={`${method.name}-${index}`}
                            variant='outline'
                            onClick={() => onWaffoMethodSelect(method, index)}
                            disabled={belowMin || !!paymentLoading}
                            className='h-9 min-w-0 justify-start gap-2 rounded-lg px-3'
                          >
                            {paymentLoading === loadingKey ? (
                              <Loader2 className='h-4 w-4 animate-spin' />
                            ) : method.icon ? (
                              <img
                                src={method.icon}
                                alt={method.name}
                                className='h-4 w-4 object-contain'
                              />
                            ) : (
                              getPaymentIcon('waffo')
                            )}
                            <span className='truncate'>{method.name}</span>
                          </Button>
                        )

                        return belowMin ? (
                          <TooltipProvider key={`${method.name}-${index}`}>
                            <Tooltip>
                              <TooltipTrigger render={button}></TooltipTrigger>
                              <TooltipContent>
                                {t('Minimum topup amount: {{amount}}', {
                                  amount: waffoMin,
                                })}
                              </TooltipContent>
                            </Tooltip>
                          </TooltipProvider>
                        ) : (
                          button
                        )
                      })}
                    </div>
                  </div>
                )}
            </>
          )}
        </div>
      ) : (
        <Alert>
          <AlertDescription>
            {t(
              'Online topup is not enabled. Please use redemption code or contact administrator.'
            )}
          </AlertDescription>
        </Alert>
      )}

      {/* Creem Products Section */}
      {enableCreemTopup &&
        Array.isArray(creemProducts) &&
        creemProducts.length > 0 &&
        onCreemProductSelect && (
          <div className='space-y-2.5 border-t pt-4 sm:space-y-3 sm:pt-6'>
            <Label className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
              {t('Creem Payment')}
            </Label>
            <CreemProductsSection
              products={creemProducts}
              onProductSelect={onCreemProductSelect}
            />
          </div>
        )}
    </TitledCard>
  )
}
