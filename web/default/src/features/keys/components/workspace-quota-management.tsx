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
import { useCallback, useMemo, useState } from 'react'
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

  const hydrateForm = useCallback((config: WorkspaceQuotaResetConfig | undefined) => {
    if (!config) {
      setEnabled(false)
      setPeriod('daily')
      setAmountInput('')
      return
    }
    setEnabled(config.enabled)
    setPeriod(config.period || 'daily')
    setAmountInput(quotaInputValue(config.amount))
  }, [])

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
  const summary = getQuotaSummary(config, periodLabels, t)

  return (
    <div className='flex min-w-0 flex-col items-start gap-1.5'>
      <Button
        type='button'
        variant='outline'
        size='sm'
        className='h-8'
        onClick={() => void handleOpenDialog()}
      >
        <Gauge className='size-3.5' />
        {t('Quota management')}
      </Button>
      <div className='text-muted-foreground flex max-w-56 items-center gap-1.5 text-left text-[11px] leading-4'>
        <CalendarClock className='size-3 shrink-0' />
        <span className='truncate'>
          {configQuery.isLoading
            ? t('Loading quota rule')
            : configQuery.isError
              ? t('Failed to load quota rule')
              : summary}
        </span>
      </div>

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

          <div className='space-y-4'>
            <div className='flex min-h-16 items-center justify-between gap-4 rounded-lg border px-3 py-2.5'>
              <div className='space-y-1'>
                <Label>{t('Enable quota reset')}</Label>
                <p className='text-muted-foreground text-xs leading-5'>
                  {t(
                    'Normal API keys only. Subscription keys are not changed.'
                  )}
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
                <div className='text-muted-foreground'>
                  {t('Configured quota')}
                </div>
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
    </div>
  )
}

function getQuotaSummary(
  config: WorkspaceQuotaResetConfig | undefined,
  periodLabels: Record<WorkspaceQuotaResetPeriod, string>,
  t: (key: string, options?: Record<string, unknown>) => string
) {
  if (!config?.enabled) {
    return t('Quota rule not configured')
  }
  const nextAt = config.next_at
    ? formatTimestampToDate(config.next_at)
    : t('Not scheduled')
  return t('{{period}} · {{quota}} · Next {{time}}', {
    period: periodLabels[config.period || 'daily'],
    quota: formatQuota(config.amount),
    time: nextAt,
  })
}
