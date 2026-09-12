import {
  CheckmarkCircle02Icon,
  Loading03Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Progress, ProgressLabel } from '@/components/ui/progress'
import { getCryptoPaymentView } from '../../lib/crypto-payment-progress'
import type { CryptoPaymentOrder } from '../../types'

interface CryptoPaymentProgressProps {
  order: CryptoPaymentOrder
  now: number
  pollingFailed: boolean
}

export function CryptoPaymentProgress(props: CryptoPaymentProgressProps) {
  const { t } = useTranslation()
  const view = getCryptoPaymentView(props.order, props.now, props.pollingFailed)
  const progress = props.order.progress
  const isEVM = props.order.network_type === 'evm'
  const required = isEVM
    ? progress?.required_confirmations
    : progress?.required_seconds
  const current = isEVM ? progress?.confirmations : progress?.confirmed_seconds
  const showCount = view.hasTransfer && Boolean(required && required > 0)
  const count = Math.max(0, current ?? 0)
  const steps = [
    {
      label: t('Transfer detected'),
      complete: view.hasTransfer || view.isPaid,
    },
    {
      label: t('Chain confirmation'),
      complete: progress?.stage === 'crediting' || view.isPaid,
    },
    { label: t('Balance credited'), complete: view.isPaid },
  ]
  let description = t('Checking the network for your transfer.')
  if (view.hasTransfer) {
    description = t('Transfer detected. Waiting for chain confirmation.')
  }
  if (progress?.stage === 'crediting') {
    description = t('Chain verification completed. Crediting your balance.')
  }
  if (view.isPaid)
    description = t('The top-up has been credited to your balance.')
  if (view.isExpired) description = t('The payment window has closed.')

  if (props.order.status === 'manual') {
    return (
      <Alert>
        <HugeiconsIcon icon={CheckmarkCircle02Icon} />
        <AlertTitle>{t('Payment manually credited')}</AlertTitle>
        <AlertDescription>
          {t('The top-up has been credited to your balance.')}
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <div className='flex flex-col gap-3 rounded-xl border p-4'>
      <ol className='grid grid-cols-3 gap-3' aria-label={t('Payment progress')}>
        {steps.map((step, index) => (
          <li
            key={step.label}
            className='flex flex-col items-center gap-2 text-center text-xs'
          >
            <span
              className={cn(
                'flex size-7 items-center justify-center rounded-full border',
                step.complete && 'bg-primary text-primary-foreground'
              )}
            >
              {step.complete ? (
                <HugeiconsIcon
                  icon={CheckmarkCircle02Icon}
                  size={16}
                  aria-hidden='true'
                />
              ) : (
                index + 1
              )}
            </span>
            {step.label}
          </li>
        ))}
      </ol>

      <div role='status' aria-live='polite' className='flex flex-col gap-2'>
        <p className='text-sm'>{description}</p>
        {showCount && !view.isPaid && (
          <Progress value={Math.min(count, required!)} max={required}>
            <ProgressLabel>
              {isEVM
                ? t('Confirmation blocks: {{current}} / {{required}}', {
                    current: count,
                    required,
                  })
                : t('Confirmation wait: {{current}} / {{required}} seconds', {
                    current: count,
                    required,
                  })}
            </ProgressLabel>
          </Progress>
        )}
      </div>

      {showCount && isEVM && !view.isPaid && (
        <p className='text-muted-foreground text-xs'>
          {t('Counts blocks added after the block containing your transfer.')}
        </p>
      )}
      {!view.isPaid && (
        <p className='text-muted-foreground text-xs'>
          {t(
            'Status refreshes automatically every 5 seconds while this window is active.'
          )}
          {Boolean(progress?.checked_at) && (
            <>
              {' '}
              {t('Last chain check: {{time}}', {
                time: new Date(
                  progress!.checked_at * 1000
                ).toLocaleTimeString(),
              })}
            </>
          )}
        </p>
      )}

      {view.isDelayed && (
        <Alert>
          <HugeiconsIcon icon={Loading03Icon} className='animate-spin' />
          <AlertTitle>{t('Status updates are temporarily delayed')}</AlertTitle>
          <AlertDescription>
            {t(
              'We are retrying automatically. The last known progress is shown. Do not pay again.'
            )}
          </AlertDescription>
        </Alert>
      )}
    </div>
  )
}
