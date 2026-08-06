/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useEffect, useMemo, useRef, useState } from 'react'
import {
  CheckmarkCircle02Icon,
  Copy01Icon,
  Loading03Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { QRCodeSVG } from 'qrcode.react'
import { useTranslation } from 'react-i18next'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
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
import { getCryptoPaymentOrder, isApiSuccess } from '../../api'
import type { CryptoPaymentOrder } from '../../types'

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
  const [currentOrder, setCurrentOrder] = useState(order)
  const [now, setNow] = useState(() => Math.floor(Date.now() / 1000))
  const paidNotified = useRef(false)

  const tradeNo = currentOrder?.trade_no
  const orderStatus = currentOrder?.status

  useEffect(() => {
    if (!open || !tradeNo || orderStatus !== 'pending') return

    const timer = window.setInterval(() => {
      setNow(Math.floor(Date.now() / 1000))
    }, 1000)
    return () => window.clearInterval(timer)
  }, [open, tradeNo, orderStatus])

  useEffect(() => {
    if (!open || !tradeNo || orderStatus !== 'pending') return

    let cancelled = false
    const poll = async () => {
      try {
        const response = await getCryptoPaymentOrder(tradeNo)
        if (!cancelled && isApiSuccess(response) && response.data) {
          setCurrentOrder(response.data)
        }
      } catch {
        // Keep the order visible and retry. A transient node/API failure must
        // not make the user create a second payment.
      }
    }
    const timer = window.setInterval(poll, 5000)
    void poll()
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [open, tradeNo, orderStatus])

  useEffect(() => {
    if (
      (currentOrder?.status !== 'success' &&
        currentOrder?.status !== 'manual') ||
      paidNotified.current
    )
      return
    paidNotified.current = true
    void onPaid()
  }, [currentOrder?.status, onPaid])

  const remaining = Math.max(0, (currentOrder?.expires_at ?? 0) - now)
  const status = currentOrder?.status ?? 'pending'
  const isPaid = status === 'success' || status === 'manual'
  const isVerifying = status === 'pending' && remaining === 0
  const statusLabel = useMemo(() => {
    if (isPaid) return t('Payment received')
    if (status === 'expired') return t('Order expired')
    if (isVerifying) return t('Payment verification in progress')
    return t('Waiting for payment')
  }, [isPaid, isVerifying, status, t])

  if (!currentOrder) return null

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{t('Crypto payment')}</DialogTitle>
          <DialogDescription>
            {t(
              'Transfer the exact token amount shown below. Your balance will be credited automatically after confirmation.'
            )}
          </DialogDescription>
        </DialogHeader>

        <div className='space-y-4'>
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
              {isVerifying
                ? t('Payment window closed')
                : status === 'pending'
                  ? t('Expires in {{time}}', {
                      time: formatRemaining(remaining),
                    })
                  : currentOrder.network_name}
            </span>
          </div>

          {status === 'pending' && !isVerifying && (
            <div className='flex justify-center rounded-xl border bg-white p-4'>
              <QRCodeSVG
                value={currentOrder.qr_content}
                size={208}
                level='M'
                includeMargin
              />
            </div>
          )}

          <div className='space-y-3 rounded-xl border p-4'>
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

          {status === 'pending' && !isVerifying && (
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

          {isVerifying && (
            <Alert>
              <HugeiconsIcon icon={Loading03Icon} className='animate-spin' />
              <AlertTitle>{t('Payment verification in progress')}</AlertTitle>
              <AlertDescription>
                {t(
                  'Do not send a new transfer. The payment window is closed while the server finishes checking confirmed chain data.'
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
                  'Do not transfer to this expired order. Close it and create a new payment order.'
                )}
              </AlertDescription>
            </Alert>
          )}
        </div>

        <DialogFooter>
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
