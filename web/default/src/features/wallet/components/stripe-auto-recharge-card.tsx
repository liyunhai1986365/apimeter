import { useCallback, useEffect, useState } from 'react'
import { CreditCard, ExternalLink, Loader2, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { TitledCard } from '@/components/ui/titled-card'
import {
  confirmStripeAutoRechargeSetup,
  createStripeAutoRechargeSetup,
  deleteStripeAutoRecharge,
  getStripeAutoRecharge,
  isApiSuccess,
  updateStripeAutoRecharge,
} from '../api'
import type { StripeAutoRechargeStatus } from '../types'

interface StripeAutoRechargeCardProps {
  setupSessionId?: string
}

export function StripeAutoRechargeCard({
  setupSessionId,
}: StripeAutoRechargeCardProps) {
  const { t } = useTranslation()
  const [status, setStatus] = useState<StripeAutoRechargeStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [binding, setBinding] = useState(false)
  const [removing, setRemoving] = useState(false)
  const [enabled, setEnabled] = useState(false)
  const [threshold, setThreshold] = useState('10')
  const [topupAmount, setTopupAmount] = useState('20')
  const [consent, setConsent] = useState(false)

  const applyStatus = useCallback((next: StripeAutoRechargeStatus) => {
    setStatus(next)
    setEnabled(next.enabled)
    setThreshold(String(next.threshold || 10))
    setTopupAmount(String(next.topup_amount || 20))
    setConsent(next.enabled)
  }, [])

  const loadStatus = useCallback(async () => {
    const response = await getStripeAutoRecharge()
    if (isApiSuccess(response) && response.data) {
      applyStatus(response.data)
      return
    }
    throw new Error(response.message || t('Failed to load automatic recharge'))
  }, [applyStatus, t])

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      try {
        setLoading(true)
        if (setupSessionId) {
          const response = await confirmStripeAutoRechargeSetup(setupSessionId)
          if (!isApiSuccess(response) || !response.data) {
            throw new Error(
              response.message || t('Failed to load automatic recharge')
            )
          }
          if (!cancelled) {
            applyStatus(response.data)
            toast.success(t('Card linked successfully'))
            const returnURL = new URL(window.location.href)
            returnURL.searchParams.delete('stripe_setup_session_id')
            window.history.replaceState({}, '', returnURL)
          }
        } else {
          await loadStatus()
        }
      } catch (error) {
        if (!cancelled) {
          toast.error(
            error instanceof Error
              ? error.message
              : t('Failed to load automatic recharge')
          )
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [applyStatus, loadStatus, setupSessionId, t])

  const handleBind = async () => {
    try {
      setBinding(true)
      const response = await createStripeAutoRechargeSetup()
      if (!isApiSuccess(response) || !response.data?.setup_url) {
        throw new Error(response.message || t('Failed to open Stripe'))
      }
      window.location.href = response.data.setup_url
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to open Stripe')
      )
      setBinding(false)
    }
  }

  const handleSave = async () => {
    const thresholdValue = Number(threshold)
    const topupValue = Number(topupAmount)
    if (!Number.isFinite(thresholdValue) || thresholdValue < 1) {
      toast.error(t('Enter a threshold of at least $1'))
      return
    }
    if (
      !Number.isInteger(topupValue) ||
      topupValue < (status?.min_topup_amount || 1)
    ) {
      toast.error(t('Enter a valid recharge amount'))
      return
    }
    if (enabled && !consent) {
      toast.error(t('Confirm the automatic charge authorization first'))
      return
    }
    try {
      setSaving(true)
      const response = await updateStripeAutoRecharge({
        enabled,
        threshold: thresholdValue,
        topup_amount: topupValue,
        consent,
      })
      if (!isApiSuccess(response) || !response.data) {
        throw new Error(
          response.message || t('Failed to save automatic recharge')
        )
      }
      applyStatus(response.data)
      toast.success(
        response.data.enabled
          ? t('Automatic recharge enabled')
          : t('Automatic recharge disabled')
      )
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to save automatic recharge')
      )
    } finally {
      setSaving(false)
    }
  }

  const handleRemove = async () => {
    if (
      !window.confirm(
        t('Remove this saved card and disable automatic recharge?')
      )
    ) {
      return
    }
    try {
      setRemoving(true)
      const response = await deleteStripeAutoRecharge()
      if (!isApiSuccess(response)) {
        throw new Error(response.message || t('Failed to remove saved card'))
      }
      await loadStatus()
      toast.success(t('Saved card removed'))
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to remove saved card')
      )
    } finally {
      setRemoving(false)
    }
  }

  if (loading) {
    return (
      <TitledCard
        title={t('Automatic recharge')}
        description={t('Keep your wallet funded with a saved card')}
        icon={<RefreshCw className='h-4 w-4' />}
      >
        <div className='space-y-3'>
          <Skeleton className='h-16 w-full' />
          <Skeleton className='h-9 w-full' />
        </div>
      </TitledCard>
    )
  }

  const busy = status?.state === 'processing' || status?.state === 'pending'

  return (
    <TitledCard
      title={t('Automatic recharge')}
      description={t('Keep your wallet funded with a saved card')}
      icon={<RefreshCw className='h-4 w-4' />}
      contentClassName='space-y-4'
    >
      {!status?.available && (
        <Alert>
          <AlertDescription>
            {t(
              'Automatic recharge is not available for the current Stripe configuration.'
            )}
          </AlertDescription>
        </Alert>
      )}

      {status?.bound ? (
        <>
          <div className='bg-muted/30 flex items-center justify-between gap-3 rounded-lg border p-3'>
            <div className='flex min-w-0 items-center gap-3'>
              <div className='bg-background flex size-9 shrink-0 items-center justify-center rounded-md border'>
                <CreditCard className='h-4 w-4' />
              </div>
              <div className='min-w-0'>
                <p className='truncate text-sm font-medium uppercase'>
                  {status.card_brand || t('Card')} •••• {status.card_last4}
                </p>
                <p className='text-muted-foreground text-xs'>
                  {t('Expires {{month}}/{{year}}', {
                    month: String(status.card_exp_month || '').padStart(2, '0'),
                    year: status.card_exp_year,
                  })}
                </p>
              </div>
            </div>
            <Button
              variant='ghost'
              size='sm'
              onClick={handleRemove}
              disabled={busy || removing}
            >
              {removing && <Loader2 className='animate-spin' />}
              {t('Remove')}
            </Button>
          </div>

          {status.last_error && (
            <Alert variant='destructive'>
              <AlertDescription>{status.last_error}</AlertDescription>
            </Alert>
          )}

          <div className='flex items-start justify-between gap-3 rounded-lg border p-3'>
            <div>
              <p className='text-sm font-medium'>
                {t('Enable automatic recharge')}
              </p>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Stripe charges the saved card after your balance falls below the threshold.'
                )}
              </p>
            </div>
            <Switch
              checked={enabled}
              onCheckedChange={setEnabled}
              disabled={!status.available || busy || saving}
            />
          </div>

          <div className='grid grid-cols-2 gap-3'>
            <div className='space-y-2'>
              <Label htmlFor='auto-recharge-threshold'>
                {t('Balance threshold (USD)')}
              </Label>
              <Input
                id='auto-recharge-threshold'
                type='number'
                min={1}
                max={10000}
                step='0.01'
                value={threshold}
                onChange={(event) => setThreshold(event.target.value)}
                disabled={busy || saving}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='auto-recharge-amount'>
                {t('Recharge amount (USD)')}
              </Label>
              <Input
                id='auto-recharge-amount'
                type='number'
                min={status.min_topup_amount || 1}
                max={10000}
                step='1'
                value={topupAmount}
                onChange={(event) => setTopupAmount(event.target.value)}
                disabled={busy || saving}
              />
            </div>
          </div>

          <label className='flex cursor-pointer items-start gap-2.5 text-xs leading-5'>
            <Checkbox
              checked={consent}
              onCheckedChange={(checked) => setConsent(!!checked)}
              disabled={!enabled || busy || saving}
              className='mt-0.5'
            />
            <span className='text-muted-foreground'>
              {t(
                'I authorize automatic charges to this card when my wallet balance falls below the threshold. Charges continue until I disable automatic recharge.'
              )}
            </span>
          </label>

          {busy && (
            <p className='text-muted-foreground flex items-center gap-2 text-xs'>
              <Loader2 className='h-3.5 w-3.5 animate-spin' />
              {t('An automatic recharge is being processed.')}
            </p>
          )}

          <Button
            className='w-full'
            onClick={handleSave}
            disabled={busy || saving}
          >
            {saving && <Loader2 className='animate-spin' />}
            {t('Save automatic recharge')}
          </Button>
        </>
      ) : (
        <div className='space-y-3'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Link a credit or debit card on Stripe, then choose a balance threshold and recharge amount.'
            )}
          </p>
          <Button
            className='w-full'
            onClick={handleBind}
            disabled={!status?.available || binding}
          >
            {binding ? <Loader2 className='animate-spin' /> : <ExternalLink />}
            {t('Link card with Stripe')}
          </Button>
          <p className='text-muted-foreground text-center text-xs'>
            {t(
              'Card details are collected and stored by Stripe. Only the Stripe payment method ID and masked card details are retained.'
            )}
          </p>
        </div>
      )}
    </TitledCard>
  )
}
