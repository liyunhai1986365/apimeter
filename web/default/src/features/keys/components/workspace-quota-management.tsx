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
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CalendarClock, Gauge, Loader2, RotateCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import {
  formatQuota,
  formatTimestampToDate,
  parseQuotaFromDollars,
  quotaUnitsToDollars,
} from '@/lib/format'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  getWorkspaceQuotaResetConfig,
  resetWorkspaceQuota,
  updateWorkspaceQuotaResetConfig,
} from '../api'
import {
  type WorkspaceQuotaResetConfig,
  type WorkspaceQuotaResetPeriod,
} from '../types'
import { useApiKeys } from './api-keys-provider'

type WorkspaceQuotaManagementProps = {
  workspaceId: number
  embedded?: boolean
  summaryMode?: 'always' | 'configured'
}

const PERIODS: WorkspaceQuotaResetPeriod[] = ['daily', 'weekly', 'monthly']

function quotaInputValue(quota: number) {
  const { meta } = getCurrencyDisplay()
  const value = quotaUnitsToDollars(quota)
  if (meta.kind === 'tokens') {
    return String(Math.round(value))
  }
  return value ? String(Number(value.toFixed(2))) : ''
}

export function WorkspaceQuotaManagement({
  workspaceId,
  embedded = false,
  summaryMode = 'always',
}: WorkspaceQuotaManagementProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { triggerRefresh } = useApiKeys()
  const [open, setOpen] = useState(false)
  const [enabled, setEnabled] = useState(false)
  const [period, setPeriod] = useState<WorkspaceQuotaResetPeriod>('daily')
  const [amountInput, setAmountInput] = useState('')

  const currencyLabel = getCurrencyLabel()
  const tokensOnly = getCurrencyDisplay().meta.kind === 'tokens'

  const periodLabels = useMemo<Record<WorkspaceQuotaResetPeriod, string>>(
    () => ({
      daily: t('Daily'),
      weekly: t('Weekly'),
      monthly: t('Monthly'),
    }),
    [t]
  )

  const configQuery = useQuery({
    queryKey: ['workspace-quota-reset-config', workspaceId],
    queryFn: async () => {
      const res = await getWorkspaceQuotaResetConfig(workspaceId)
      if (!res.success || !res.data) {
        throw new Error(res.message || 'Failed to load quota rule')
      }
      return res.data
    },
    enabled: workspaceId > 0,
    staleTime: 60 * 1000,
  })

  const hydrateForm = useCallback(
    (config: WorkspaceQuotaResetConfig | undefined) => {
      if (!config) {
        setEnabled(false)
        setPeriod('daily')
        setAmountInput('')
        return
      }
      setEnabled(config.enabled)
      setPeriod(config.period || 'daily')
      setAmountInput(quotaInputValue(config.amount))
    },
    []
  )

  useEffect(() => {
    if (embedded) {
      hydrateForm(configQuery.data)
    }
  }, [configQuery.data, embedded, hydrateForm])

  const handleOpenDialog = async () => {
    let config = configQuery.data
    if (!config) {
      const result = await configQuery.refetch()
      config = result.data
    }
    hydrateForm(config)
    setOpen(true)
  }

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      setOpen(false)
    }
  }

  const mutation = useMutation({
    mutationFn: async () => {
      const displayAmount = Number(amountInput)
      const quotaAmount = parseQuotaFromDollars(displayAmount)
      if (enabled && (!Number.isFinite(displayAmount) || quotaAmount <= 0)) {
        throw new Error(t('The quota amount must be greater than 0.'))
      }
      const res = await updateWorkspaceQuotaResetConfig(workspaceId, {
        enabled,
        period,
        amount: quotaAmount,
      })
      if (!res.success || !res.data) {
        throw new Error(res.message || t('Failed to save quota rule'))
      }
      return res.data
    },
    onSuccess: (config) => {
      toast.success(t('Workspace quota rule saved'))
      queryClient.setQueryData(
        ['workspace-quota-reset-config', workspaceId],
        config
      )
      void queryClient.invalidateQueries({
        queryKey: ['workspace-quota-reset-config', workspaceId],
      })
      triggerRefresh()
      setOpen(false)
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Failed to save quota rule')
      )
    },
  })

  const resetMutation = useMutation({
    mutationFn: async () => {
      const res = await resetWorkspaceQuota(workspaceId)
      if (!res.success || !res.data) {
        throw new Error(res.message || t('Failed to reset quota'))
      }
      return res.data
    },
    onSuccess: (config) => {
      toast.success(t('Current period quota reset'))
      queryClient.setQueryData(
        ['workspace-quota-reset-config', workspaceId],
        config
      )
      void queryClient.invalidateQueries({
        queryKey: ['workspace-quota-reset-config', workspaceId],
      })
      triggerRefresh()
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Failed to reset quota')
      )
    },
  })

  const config = configQuery.data
  const quotaSummaryItems = getQuotaSummaryItems(config, t)
  const showSummary =
    summaryMode === 'always'
      ? true
      : configQuery.isError || quotaSummaryItems.length > 0

  const dialog = (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className='sm:max-w-[440px]'>
        <DialogHeader>
          <DialogTitle>{t('Workspace quota rule')}</DialogTitle>
          <DialogDescription>
            {t(
              'Set a periodic quota reset for all API keys in this workspace.'
            )}
          </DialogDescription>
        </DialogHeader>

        <WorkspaceQuotaRuleForm
          enabled={enabled}
          setEnabled={setEnabled}
          period={period}
          setPeriod={setPeriod}
          amountInput={amountInput}
          setAmountInput={setAmountInput}
          config={config}
          periodLabels={periodLabels}
          currencyLabel={currencyLabel}
          tokensOnly={tokensOnly}
          t={t}
        />

        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            disabled={
              !config?.enabled || !config.amount || resetMutation.isPending
            }
            onClick={() => resetMutation.mutate()}
            className='sm:mr-auto'
          >
            {resetMutation.isPending ? (
              <Loader2 className='animate-spin' />
            ) : (
              <RotateCcw />
            )}
            {t('Reset current period')}
          </Button>
          <Button
            type='button'
            variant='outline'
            onClick={() => setOpen(false)}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='button'
            disabled={mutation.isPending}
            onClick={() => mutation.mutate()}
          >
            {mutation.isPending ? t('Saving') : t('Save quota rule')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )

  if (embedded) {
    return (
      <div className='space-y-4'>
        <WorkspaceQuotaRuleForm
          enabled={enabled}
          setEnabled={setEnabled}
          period={period}
          setPeriod={setPeriod}
          amountInput={amountInput}
          setAmountInput={setAmountInput}
          config={config}
          periodLabels={periodLabels}
          currencyLabel={currencyLabel}
          tokensOnly={tokensOnly}
          t={t}
        />
        <div className='flex flex-col-reverse gap-2 sm:flex-row sm:items-center sm:justify-between'>
          <Button
            type='button'
            variant='outline'
            disabled={
              !config?.enabled || !config.amount || resetMutation.isPending
            }
            onClick={() => resetMutation.mutate()}
            className='sm:w-auto'
          >
            {resetMutation.isPending ? (
              <Loader2 className='animate-spin' />
            ) : (
              <RotateCcw />
            )}
            {t('Reset current period')}
          </Button>
          <Button
            type='button'
            disabled={mutation.isPending}
            onClick={() => mutation.mutate()}
            className='sm:w-auto'
          >
            {mutation.isPending ? t('Saving') : t('Save quota rule')}
          </Button>
        </div>
      </div>
    )
  }

  return (
    <div className='flex min-w-0 items-start justify-end gap-2 text-right'>
      {showSummary && (
        <div className='flex max-w-64 flex-col items-end gap-1 pt-0.5 text-right text-[11px] leading-4 text-muted-foreground'>
          {configQuery.isLoading ? (
            <span className='truncate'>{t('Loading quota rule')}</span>
          ) : configQuery.isError ? (
            <span className='truncate'>{t('Failed to load quota rule')}</span>
          ) : quotaSummaryItems.length > 0 ? (
            quotaSummaryItems.map((item) => (
              <span
                key={item.label}
                className='flex items-center justify-end gap-1.5'
              >
                <item.icon className='size-3 shrink-0' />
                <span className='truncate'>
                  {item.label}: {item.value}
                </span>
              </span>
            ))
          ) : (
            <span className='truncate'>{t('Quota rule not configured')}</span>
          )}
        </div>
      )}
      <Button
        type='button'
        variant='outline'
        size='sm'
        className='h-8 shrink-0'
        onClick={() => void handleOpenDialog()}
      >
        <Gauge className='size-3.5' />
        {t('Quota management')}
      </Button>
      {dialog}
    </div>
  )
}

function WorkspaceQuotaRuleForm({
  enabled,
  setEnabled,
  period,
  setPeriod,
  amountInput,
  setAmountInput,
  config,
  periodLabels,
  currencyLabel,
  tokensOnly,
  t,
}: {
  enabled: boolean
  setEnabled: (enabled: boolean) => void
  period: WorkspaceQuotaResetPeriod
  setPeriod: (period: WorkspaceQuotaResetPeriod) => void
  amountInput: string
  setAmountInput: (amount: string) => void
  config: WorkspaceQuotaResetConfig | undefined
  periodLabels: Record<WorkspaceQuotaResetPeriod, string>
  currencyLabel: string
  tokensOnly: boolean
  t: (key: string, options?: Record<string, unknown>) => string
}) {
  return (
    <div className='space-y-4'>
      <div className='flex min-h-16 items-center justify-between gap-4 rounded-lg border px-3 py-2.5'>
        <div className='space-y-1'>
          <Label>{t('Enable quota reset')}</Label>
          <p className='text-muted-foreground text-xs leading-5'>
            {t('Normal API keys only. Subscription keys are not changed.')}
          </p>
        </div>
        <Switch checked={enabled} onCheckedChange={setEnabled} />
      </div>

      <div className='grid gap-3 sm:grid-cols-2'>
        <div className='space-y-2'>
          <Label>{t('Reset period')}</Label>
          <Select
            value={period}
            onValueChange={(value) => {
              if (
                value &&
                PERIODS.includes(value as WorkspaceQuotaResetPeriod)
              ) {
                setPeriod(value as WorkspaceQuotaResetPeriod)
              }
            }}
            disabled={!enabled}
          >
            <SelectTrigger className='w-full'>
              <SelectValue>{periodLabels[period]}</SelectValue>
            </SelectTrigger>
            <SelectContent>
              {PERIODS.map((item) => (
                <SelectItem key={item} value={item}>
                  {periodLabels[item]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className='space-y-2'>
          <Label>
            {t('Quota ({{currency}})', { currency: currencyLabel })}
          </Label>
          <Input
            type='number'
            min='0'
            step={tokensOnly ? 1 : 0.01}
            value={amountInput}
            disabled={!enabled}
            placeholder={
              tokensOnly
                ? t('Enter quota in tokens')
                : t('Enter quota in {{currency}}', {
                    currency: currencyLabel,
                  })
            }
            onChange={(event) => setAmountInput(event.target.value)}
          />
        </div>
      </div>

      <div className='bg-muted/40 grid gap-2 rounded-lg border px-3 py-2.5 text-xs sm:grid-cols-2'>
        <div>
          <div className='text-muted-foreground'>{t('Configured quota')}</div>
          <div className='mt-1 font-mono text-sm font-semibold tabular-nums'>
            {config?.amount ? formatQuota(config.amount) : '-'}
          </div>
        </div>
        <div>
          <div className='text-muted-foreground'>{t('Next reset')}</div>
          <div className='mt-1 font-mono text-sm font-semibold tabular-nums'>
            {config?.enabled && config.next_at
              ? formatTimestampToDate(config.next_at)
              : '-'}
          </div>
        </div>
      </div>
    </div>
  )
}

function getQuotaSummaryItems(
  config: WorkspaceQuotaResetConfig | undefined,
  t: (key: string, options?: Record<string, unknown>) => string
) {
  if (!config?.enabled) {
    return []
  }
  const nextAt = config.next_at
    ? formatTimestampToDate(config.next_at)
    : t('Not scheduled')
  return [
    {
      label: t('Configured quota'),
      value: formatQuota(config.amount),
      icon: Gauge,
    },
    {
      label: t('Next reset'),
      value: nextAt,
      icon: CalendarClock,
    },
  ]
}
