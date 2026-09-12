/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import {
  CheckmarkCircle02Icon,
  Copy01Icon,
  Loading03Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { QRCodeSVG } from 'qrcode.react'
import { useTranslation } from 'react-i18next'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { useOpenCustomerService } from '@/hooks/use-open-customer-service'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useCryptoPaymentOrder } from '../../hooks/use-crypto-payment-order'
import { getCryptoPaymentView } from '../../lib/crypto-payment-progress'
import type { CryptoPaymentOrder } from '../../types'
import { CryptoPaymentProgress } from './crypto-payment-progress'

interface CryptoPaymentDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  order: CryptoPaymentOrder | null
  onPaid: () => void | Promise<void>
}

function formatRemaining(seconds: number) {
  const minutes = Math.floor(seconds / 60)
  const remainder = seconds % 60
  return `${String(minutes).padStart(2, '0')}:${String(remainder).padStart(2, '0')}`
}

export function CryptoPaymentDialog({
  open,
  onOpenChange,
  order,
  onPaid,
}: CryptoPaymentDialogProps) {
  const { t } = useTranslation()
  const { copyToClipboard } = useCopyToClipboard()
  const openCustomerService = useOpenCustomerService()
  const { currentOrder, now, pollingFailed } = useCryptoPaymentOrder(
    order,
    open,
    onPaid
  )
  if (!currentOrder) return null

  const {
    isPaid,
    isVerifying,
    hasTransfer,
    remaining,
    showPaymentInstructions,
    transactionHash,
  } = getCryptoPaymentView(currentOrder, now, pollingFailed)
  const status = currentOrder.status
  let statusLabel = t('Waiting for payment')
  if (isVerifying) statusLabel = t('Payment verification in progress')
  if (hasTransfer && !isPaid) statusLabel = t('Confirming on chain')
  if (currentOrder.progress?.stage === 'crediting')
    statusLabel = t('Crediting balance')
  if (status === 'expired') statusLabel = t('Order expired')
  if (isPaid) statusLabel = t('Payment received')

  let timeLabel = currentOrder.network_name
  if (status === 'pending') {
    timeLabel = t('Expires in {{time}}', { time: formatRemaining(remaining) })
  }
  if (isVerifying) timeLabel = t('Payment window closed')

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-h-[90dvh] overflow-y-auto sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{t('Crypto payment')}</DialogTitle>
          <DialogDescription>
            {t(
              'Transfer the exact token amount shown below. Your balance will be credited automatically after confirmation.'
            )}
          </DialogDescription>
        </DialogHeader>

        <div className='flex flex-col gap-4'>
          <div className='flex items-center justify-between gap-3'>
            <Badge variant={isPaid ? 'default' : 'secondary'}>
              {status === 'pending' && (
                <HugeiconsIcon
                  icon={Loading03Icon}
                  className='animate-spin'
                  data-icon='inline-start'
                />
              )}
              {isPaid && (
                <HugeiconsIcon
                  icon={CheckmarkCircle02Icon}
                  data-icon='inline-start'
                />
              )}
              {statusLabel}
            </Badge>
            <span className='text-muted-foreground text-sm tabular-nums'>
              {timeLabel}
            </span>
          </div>

          <CryptoPaymentProgress
            order={currentOrder}
            now={now}
            pollingFailed={pollingFailed}
          />

          {!isPaid && (
            <Alert>
              <AlertTitle>{t('Already paid? Do not pay again')}</AlertTitle>
              <AlertDescription>
                {t(
                  'Please wait patiently for confirmation and automatic crediting. If you have already transferred, do not send another payment.'
                )}
              </AlertDescription>
            </Alert>
          )}

          {showPaymentInstructions && (
            <div className='flex justify-center rounded-xl border bg-white p-4'>
              <QRCodeSVG
                value={currentOrder.qr_content}
                size={208}
                level='M'
                includeMargin
              />
            </div>
          )}

          <div className='flex flex-col gap-3 rounded-xl border p-4'>
            <div>
              <p className='text-muted-foreground text-xs'>
                {t('Exact amount')}
              </p>
              <div className='mt-1 flex items-center justify-between gap-3'>
                <p className='text-xl font-semibold tabular-nums'>
                  {currentOrder.display_amount} {currentOrder.token_symbol}
                </p>
                <Button
                  type='button'
                  variant='outline'
                  size='icon-sm'
                  onClick={() => copyToClipboard(currentOrder.display_amount)}
                  aria-label={t('Copy amount')}
                >
                  <HugeiconsIcon icon={Copy01Icon} />
                </Button>
              </div>
            </div>

            <div>
              <p className='text-muted-foreground text-xs'>
                {t('Receiving address')}
              </p>
              <div className='mt-1 flex items-center gap-2'>
                <code className='bg-muted min-w-0 flex-1 rounded-md px-2 py-1.5 text-xs break-all'>
                  {currentOrder.wallet_address}
                </code>
                <Button
                  type='button'
                  variant='outline'
                  size='icon-sm'
                  onClick={() => copyToClipboard(currentOrder.wallet_address)}
                  aria-label={t('Copy address')}
                >
                  <HugeiconsIcon icon={Copy01Icon} />
                </Button>
              </div>
            </div>

            <div className='text-muted-foreground flex justify-between gap-3 text-xs'>
              <span>{t('Network')}</span>
              <span className='text-foreground font-medium'>
                {currentOrder.network_name}
              </span>
            </div>
          </div>

          {showPaymentInstructions && (
            <Alert>
              <AlertTitle>
                {t(
                  'The receiving amount must be exact; gas fees are paid separately'
                )}
              </AlertTitle>
              <AlertDescription>
                {t(
                  'The receiving wallet must receive the full amount shown above. Use the configured token and network; gas or network fees must not be deducted from the payment amount.'
                )}
              </AlertDescription>
            </Alert>
          )}

          {isPaid && (
            <Alert>
              <HugeiconsIcon icon={CheckmarkCircle02Icon} />
              <AlertTitle>{t('Payment received')}</AlertTitle>
              <AlertDescription>
                {t('The top-up has been credited to your balance.')}
              </AlertDescription>
            </Alert>
          )}

          {status === 'expired' && (
            <Alert variant='destructive'>
              <AlertTitle>{t('Order expired')}</AlertTitle>
              <AlertDescription>
                {t(
                  'This order has expired. If you already paid, do not transfer again. Contact support to verify your payment.'
                )}
              </AlertDescription>
            </Alert>
          )}
          <div className='flex flex-col gap-3 rounded-xl border p-4'>
            <PaymentReference
              label={t('Order number')}
              value={currentOrder.trade_no}
              onCopy={copyToClipboard}
            />
            {transactionHash && (
              <PaymentReference
                label={t('Transaction hash')}
                value={transactionHash}
                onCopy={copyToClipboard}
              />
            )}
            {!isPaid && (
              <p className='text-muted-foreground text-xs'>
                {t(
                  'If your balance is still not credited, contact support with your order number and transaction hash. Support can verify the payment and manually credit it.'
                )}
              </p>
            )}
          </div>
        </div>

        <DialogFooter>
          {!isPaid && (
            <Button
              type='button'
              variant='outline'
              onClick={() => {
                onOpenChange(false)
                openCustomerService()
              }}
            >
              {t('Contact support')}
            </Button>
          )}
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
          >
            {isPaid ? t('Done') : t('Close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function PaymentReference(props: {
  label: string
  value: string
  onCopy: (value: string) => unknown
}) {
  const { t } = useTranslation()
  return (
    <div className='flex flex-col gap-1'>
      <p className='text-muted-foreground text-xs'>{props.label}</p>
      <div className='flex items-center gap-2'>
        <code className='min-w-0 flex-1 text-xs break-all'>{props.value}</code>
        <Button
          type='button'
          variant='outline'
          size='icon-sm'
          onClick={() => props.onCopy(props.value)}
          aria-label={t('Copy {{label}}', { label: props.label })}
        >
          <HugeiconsIcon icon={Copy01Icon} />
        </Button>
      </div>
    </div>
  )
}
