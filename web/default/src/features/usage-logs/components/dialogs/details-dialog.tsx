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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Copy,
  Check,
  Route,
  Settings2,
  AlertTriangle,
  Headphones,
  Globe,
  ShieldCheck,
  UserCog,
  Info,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  formatBillingCurrencyFromUSD,
  getCurrencyDisplay,
} from '@/lib/currency'
import { formatLogQuota, formatTokens, formatUseTime } from '@/lib/format'
import { getDiscountSavingsLabel } from '@/lib/group-discount'
import { cn } from '@/lib/utils'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import { DiscountTooltip } from '@/components/discount-tooltip'
import { StatusBadge, type StatusBadgeProps } from '@/components/status-badge'
import { appendChannelRetryPolicyRule } from '@/features/channels/api'
import {
  buildRetryPolicyRuleFromLog,
  channelsQueryKeys,
} from '@/features/channels/lib'
import type { UsageLog } from '../../data/schema'
import {
  buildBillingDetail,
  type BillingDetailLine,
} from '../../lib/billing-detail'
import {
  parseLogOther,
  getParamOverrideActionLabel,
  parseAuditLine,
  isViolationFeeLog,
  getFirstResponseTimeColor,
  getResponseTimeColor,
  isTaskPreConsumeLog,
} from '../../lib/format'
import { getLocalizedLogContent } from '../../lib/log-content'
import { getLogTypeConfig, isTimingLogType } from '../../lib/utils'
import type { LogOtherData } from '../../types'

function timingTextColorClass(
  variant: 'success' | 'warning' | 'danger'
): string {
  if (variant === 'success') return 'text-emerald-600'
  if (variant === 'warning') return 'text-amber-600'
  return 'text-rose-600'
}

function DetailRow(props: {
  label: React.ReactNode
  value: React.ReactNode
  mono?: boolean
  muted?: boolean
}) {
  const isDiscountValue =
    typeof props.value === 'string' &&
    Boolean(getDiscountSavingsLabel(props.value))

  return (
    <div className='grid min-w-0 grid-cols-[5.25rem_minmax(0,1fr)] gap-2 text-sm sm:grid-cols-[7rem_minmax(0,1fr)] sm:gap-3'>
      <span className='text-muted-foreground min-w-0 text-xs'>
        {props.label}
      </span>
      <span
        className={cn(
          'max-w-full min-w-0 text-xs break-all sm:break-words',
          props.mono && 'font-mono',
          props.muted && 'text-muted-foreground'
        )}
      >
        {isDiscountValue ? (
          <DiscountTooltip label={props.value as string}>
            <span>{props.value}</span>
          </DiscountTooltip>
        ) : (
          props.value
        )}
      </span>
    </div>
  )
}

function DetailSection(props: {
  icon?: React.ReactNode
  label: string
  variant?: 'default' | 'danger'
  children: React.ReactNode
}) {
  const isDanger = props.variant === 'danger'
  return (
    <div className='min-w-0 space-y-1.5'>
      <Label
        className={cn(
          'flex items-center gap-1.5 text-xs font-semibold',
          isDanger && 'text-red-500'
        )}
      >
        {props.icon}
        {props.label}
      </Label>
      <div
        className={cn(
          'min-w-0 space-y-1 overflow-hidden rounded-md border p-2.5 max-sm:p-2',
          isDanger
            ? 'border-red-200 bg-red-50 dark:border-red-900 dark:bg-red-950/20'
            : 'bg-muted/30'
        )}
      >
        {props.children}
      </div>
    </div>
  )
}

function PreConsumedCostValue(props: { value: string }) {
  const { t } = useTranslation()

  return (
    <span className='inline-flex flex-wrap items-center gap-1'>
      <span className='rounded-sm bg-amber-100 px-1 py-px font-sans text-[10px] font-medium text-amber-700 dark:bg-amber-950/50 dark:text-amber-300'>
        {t('Pre-consumed')}
      </span>
      <span>{props.value}</span>
    </span>
  )
}

function BillingBreakdown(props: {
  log: UsageLog
  other: LogOtherData
  isRoot: boolean
}) {
  const { t } = useTranslation()
  const { log, other, isRoot } = props
  const isTaskPreConsume = isTaskPreConsumeLog(log)
  const { config } = getCurrencyDisplay()
  const actualAmountUSD = log.quota / config.quotaPerUnit
  const detail = buildBillingDetail(
    log,
    other,
    actualAmountUSD,
    1 / config.quotaPerUnit
  )
  const amountOptions = {
    digitsLarge: 6,
    digitsSmall: 8,
    abbreviate: false,
  }
  const formatAmount = (amount: number) =>
    formatBillingCurrencyFromUSD(amount, amountOptions)
  const formatFactor = (value: number) =>
    new Intl.NumberFormat(undefined, {
      maximumFractionDigits: 6,
      useGrouping: false,
    }).format(value)
  const formatUnitPrice = (price: number) => {
    const absolute = Math.abs(price)
    const digits =
      absolute > 0 && absolute < 0.0001 ? 8 : absolute < 0.01 ? 6 : 4
    return `$${price.toFixed(digits)}`
  }
  const modeLabel =
    detail.mode === 'dynamic'
      ? t('Dynamic Pricing')
      : detail.mode === 'per-call'
        ? t('Per-call')
        : t('Per-token')
  const formulaForLine = (line: BillingDetailLine) => {
    const parts: string[] = []
    if (line.amountOnly) {
      parts.push(formatAmount(line.originalAmountUSD))
    } else if (
      line.quantity != null &&
      line.divisor != null &&
      line.unitPriceUSD != null
    ) {
      let quantity = line.quantity.toLocaleString()
      if (line.divisor === 1_000_000) quantity += ' / 1M'
      if (line.divisor === 1_000) quantity += ' / 1K'
      if (line.divisor === 1 && line.quantityUnitKey) {
        quantity += ` ${t(line.quantityUnitKey)}`
      }
      parts.push(quantity, formatUnitPrice(line.unitPriceUSD))
    }
    for (const factor of line.factors) {
      parts.push(`${formatFactor(factor.value)} (${t(factor.labelKey)})`)
    }
    parts.push(
      `${formatFactor(detail.discount)} (${t(detail.discountLabelKey)})`
    )
    return parts.join(' × ')
  }

  const costRows: Array<{ label: string; value: React.ReactNode }> = []
  if (isRoot && other.cost_quota != null) {
    if (other.channel_ratio != null) {
      costRows.push({
        label: t('Cost Discount'),
        value: `${other.channel_ratio}`,
      })
    }
    costRows.push({
      label: t('Supplier Cost'),
      value: formatLogQuota(other.cost_quota),
    })
    if (other.profit_quota != null) {
      costRows.push({
        label: t('Profit'),
        value: formatLogQuota(other.profit_quota),
      })
    }
    if (other.profit_rate != null) {
      costRows.push({
        label: t('Profit Rate'),
        value: `${(other.profit_rate * 100).toFixed(2)}%`,
      })
    }
  }

  return (
    <>
      <DetailSection label={t('Billing Details')}>
        <div className='flex flex-wrap items-center gap-1.5'>
          <Badge variant='secondary'>{modeLabel}</Badge>
          {detail.matchedTier && (
            <Badge variant='outline'>
              {t('Matched Tier')}: {detail.matchedTier}
            </Badge>
          )}
        </div>

        <Accordion className='bg-background/70 rounded-md border px-3'>
          <AccordionItem value='billing-process' className='border-0'>
            <AccordionTrigger className='cursor-pointer py-2.5 hover:no-underline'>
              <span className='flex min-w-0 flex-1 flex-wrap items-baseline gap-x-1.5 gap-y-1 pr-2 font-mono text-xs leading-relaxed'>
                <span className='text-muted-foreground font-sans'>
                  {t('Original price')}
                </span>
                <span>{formatAmount(detail.originalAmountUSD)}</span>
                <span className='text-muted-foreground'>×</span>
                <span>
                  {formatFactor(detail.discount)} ({t(detail.discountLabelKey)})
                </span>
                <span className='text-muted-foreground'>=</span>
                <span className='text-muted-foreground font-sans'>
                  {t('Final amount')}
                </span>
                <strong className='text-primary text-sm'>
                  {isTaskPreConsume ? (
                    <PreConsumedCostValue
                      value={formatAmount(detail.finalAmountUSD)}
                    />
                  ) : (
                    formatAmount(detail.finalAmountUSD)
                  )}
                </strong>
                <span className='text-muted-foreground ml-auto shrink-0 font-sans font-medium'>
                  {t('Billing Process')}
                </span>
              </span>
            </AccordionTrigger>
            <AccordionContent className='flex flex-col gap-2 pt-1 pb-3'>
              {detail.lines.map((line, index) => (
                <div key={line.key} className='flex min-w-0 gap-1.5 text-xs'>
                  <span className='text-muted-foreground w-2.5 shrink-0 font-mono'>
                    {index === 0 ? '' : '+'}
                  </span>
                  <div className='min-w-0 flex-1'>
                    <div className='flex min-w-0 flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5'>
                      <span className='font-medium'>{t(line.labelKey)}</span>
                      <span className='shrink-0 font-mono font-medium'>
                        = {formatAmount(line.finalAmountUSD)}
                      </span>
                    </div>
                    <div className='text-muted-foreground min-w-0 font-mono text-[11px] leading-relaxed break-all sm:break-words'>
                      {formulaForLine(line)}
                    </div>
                  </div>
                </div>
              ))}
            </AccordionContent>
          </AccordionItem>
        </Accordion>
      </DetailSection>

      {costRows.length > 0 && (
        <DetailSection label={`${t('Cost')} / ${t('Profit')}`}>
          {costRows.map((row, index) => (
            <DetailRow key={index} label={row.label} value={row.value} mono />
          ))}
        </DetailSection>
      )}
    </>
  )
}

function TokenBreakdown(props: { log: UsageLog; other: LogOtherData }) {
  const { t } = useTranslation()
  const { log, other } = props

  const promptTokens = log.prompt_tokens || 0
  const completionTokens = log.completion_tokens || 0
  const cacheRead = other.cache_tokens || 0
  const cacheWrite = other.cache_creation_tokens || 0
  const cacheWrite5m = other.cache_creation_tokens_5m || 0
  const cacheWrite1h = other.cache_creation_tokens_1h || 0
  const hasTokens = promptTokens > 0 || completionTokens > 0

  if (!hasTokens) return null

  const rows: Array<{ label: string; value: string }> = []

  rows.push({ label: t('Input Tokens'), value: promptTokens.toLocaleString() })
  rows.push({
    label: t('Output Tokens'),
    value: completionTokens.toLocaleString(),
  })

  if (cacheRead > 0) {
    rows.push({
      label: t('Cache Read'),
      value: cacheRead.toLocaleString(),
    })
  }

  if (cacheWrite > 0 && cacheWrite5m === 0 && cacheWrite1h === 0) {
    rows.push({
      label: t('Cache Write'),
      value: cacheWrite.toLocaleString(),
    })
  }

  if (cacheWrite5m > 0) {
    rows.push({
      label: t('Cache Write (5m)'),
      value: cacheWrite5m.toLocaleString(),
    })
  }

  if (cacheWrite1h > 0) {
    rows.push({
      label: t('Cache Write (1h)'),
      value: cacheWrite1h.toLocaleString(),
    })
  }

  if (other.image && other.image_output) {
    rows.push({
      label: t('Image Tokens'),
      value: other.image_output.toLocaleString(),
    })
  }

  return (
    <DetailSection label={t('Token Breakdown')}>
      {rows.map((row, idx) => (
        <DetailRow key={idx} label={row.label} value={row.value} mono />
      ))}
    </DetailSection>
  )
}

interface DetailsDialogProps {
  log: UsageLog
  isAdmin: boolean
  isRoot: boolean
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function DetailsDialog(props: DetailsDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })
  const details = getLocalizedLogContent(props.log, t)
  const other = parseLogOther(props.log.other)
  const typeConfig = getLogTypeConfig(props.log.type)

  const isViolation = isViolationFeeLog(other)
  const isRefund = props.log.type === 6
  const isConsume = props.log.type === 2
  const isTopup = props.log.type === 1
  const isManage = props.log.type === 3
  const isSubscription = other?.billing_source === 'subscription'
  const hasAudioTokens = other?.ws || other?.audio
  const showTiming = isTimingLogType(props.log.type)
  const showAdminIp =
    !!props.log.ip && (showTiming || (props.isAdmin && isTopup))
  const adminInfo = other?.admin_info
  const topupAuditFields =
    isTopup && props.isAdmin && adminInfo
      ? ([
          adminInfo.payment_method && {
            label: t('Order Payment Method'),
            value: adminInfo.payment_method,
          },
          adminInfo.callback_payment_method && {
            label: t('Callback Payment Method'),
            value: adminInfo.callback_payment_method,
          },
          adminInfo.caller_ip && {
            label: t('Callback Caller IP'),
            value: adminInfo.caller_ip,
          },
          adminInfo.server_ip && {
            label: t('Server IP'),
            value: adminInfo.server_ip,
          },
          adminInfo.node_name && {
            label: t('Node Name'),
            value: adminInfo.node_name,
          },
          adminInfo.version && {
            label: t('System Version'),
            value: adminInfo.version,
          },
        ].filter(Boolean) as Array<{ label: string; value: string }>)
      : []
  const showLegacyTopupWarning = isTopup && props.isAdmin && !adminInfo
  const showTopupAuditSection =
    isTopup &&
    props.isAdmin &&
    (topupAuditFields.length > 0 || showLegacyTopupWarning)
  const manageOperator = (() => {
    if (!isManage || !props.isAdmin || !adminInfo) return null
    const username = adminInfo.admin_username
    const id = adminInfo.admin_id
    const hasUsername = username != null && String(username).trim() !== ''
    const hasId = id != null && String(id).trim() !== ''
    if (!hasUsername && !hasId) return null
    if (hasUsername && hasId) return `${username} (ID: ${id})`
    if (hasUsername) return String(username)
    return `ID: ${id}`
  })()

  const conversionChain =
    other && Array.isArray(other.request_conversion)
      ? other.request_conversion.filter(Boolean)
      : []
  const conversionLabel =
    conversionChain.length <= 1
      ? t('Native format')
      : conversionChain.join(' -> ')
  const showConversion =
    props.isAdmin &&
    props.log.type !== 6 &&
    (other?.request_path || conversionChain.length > 0)

  const useChannel = other?.admin_info?.use_channel
  const channelChain =
    useChannel && useChannel.length > 0 ? useChannel.join(' → ') : undefined
  const retryPolicyRule = buildRetryPolicyRuleFromLog(props.log)
  const canAppendRetryPolicy =
    props.isAdmin &&
    props.log.type === 5 &&
    props.log.channel > 0 &&
    retryPolicyRule
  const appendRetryPolicy = useMutation({
    mutationFn: async () => {
      if (!retryPolicyRule || props.log.channel <= 0) {
        throw new Error(t('No retry policy can be generated from this log'))
      }
      const updateResponse = await appendChannelRetryPolicyRule(
        props.log.channel,
        retryPolicyRule
      )
      if (!updateResponse.success) {
        throw new Error(updateResponse.message || t('Failed to update channel'))
      }
    },
    onSuccess: () => {
      toast.success(t('Retry policy added to channel'))
      queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() })
      queryClient.invalidateQueries({
        queryKey: channelsQueryKeys.detail(props.log.channel),
      })
    },
    onError: (error) => {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to add retry policy to channel')
      )
    },
  })

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent
        className={cn(
          'min-w-0 overflow-hidden',
          'max-sm:max-h-[calc(100dvh-1.5rem)] max-sm:w-[calc(100vw-1.5rem)] max-sm:max-w-[calc(100vw-1.5rem)] max-sm:p-4',
          isConsume ? 'sm:max-w-2xl' : 'sm:max-w-lg'
        )}
      >
        <DialogHeader className='max-sm:gap-1'>
          <DialogTitle className='flex items-center gap-2 text-base'>
            {t('Log Details')}
            <StatusBadge
              label={t(typeConfig.label)}
              variant={typeConfig.color as StatusBadgeProps['variant']}
              size='sm'
              copyable={false}
            />
          </DialogTitle>
          <DialogDescription className='sr-only'>
            {t('View the complete details for this log entry')}
          </DialogDescription>
        </DialogHeader>

        <ScrollArea className='max-h-[70vh] min-w-0 overflow-hidden pr-2 max-sm:max-h-[calc(100dvh-7rem)] sm:pr-4'>
          <div className='w-full max-w-full min-w-0 space-y-2.5 overflow-hidden py-1 sm:space-y-3'>
            {/* Overview section - key identifiers */}
            <div className='min-w-0 space-y-1'>
              {props.log.request_id && (
                <DetailRow
                  label={t('Request ID')}
                  value={props.log.request_id}
                  mono
                />
              )}
              {props.log.upstream_request_id && (
                <DetailRow
                  label={t('Upstream Request ID')}
                  value={props.log.upstream_request_id}
                  mono
                />
              )}

              {props.isAdmin && props.log.channel > 0 && (
                <DetailRow
                  label={t('Channel')}
                  value={
                    <span>
                      {props.log.channel}
                      {props.log.channel_name && (
                        <span className='text-muted-foreground'>
                          {' '}
                          ({props.log.channel_name})
                        </span>
                      )}
                    </span>
                  }
                  mono
                />
              )}

              {channelChain && props.isAdmin && (
                <DetailRow label={t('Retry Chain')} value={channelChain} mono />
              )}

              {canAppendRetryPolicy && (
                <DetailRow
                  label={t('Retry Policy')}
                  value={
                    <Button
                      type='button'
                      size='sm'
                      variant='outline'
                      className='h-7 px-2 text-xs'
                      disabled={appendRetryPolicy.isPending}
                      onClick={() => appendRetryPolicy.mutate()}
                    >
                      {appendRetryPolicy.isPending
                        ? t('Adding...')
                        : t('Add to channel retry policy')}
                    </Button>
                  }
                />
              )}

              {props.log.token_name && (
                <DetailRow
                  label={t('Token')}
                  value={props.log.token_name}
                  mono
                />
              )}

              {(props.log.group || other?.group) && (
                <DetailRow
                  label={t('Group')}
                  value={props.log.group || other?.group || ''}
                  mono
                />
              )}

              {showAdminIp && (
                <DetailRow
                  label={t('IP Address')}
                  value={
                    <span className='flex items-center gap-1'>
                      <Globe
                        className='size-3 text-amber-500'
                        aria-hidden='true'
                      />
                      {props.log.ip}
                    </span>
                  }
                  mono
                />
              )}

              {showTiming && props.log.use_time > 0 && (
                <DetailRow
                  label={t('Response Time')}
                  value={
                    <span
                      className={cn(
                        'font-medium',
                        timingTextColorClass(
                          getResponseTimeColor(
                            props.log.use_time,
                            props.log.completion_tokens
                          )
                        )
                      )}
                    >
                      {formatUseTime(props.log.use_time)}
                      {props.log.is_stream &&
                        other?.frt != null &&
                        other.frt > 0 && (
                          <span
                            className={cn(
                              'font-normal',
                              timingTextColorClass(
                                getFirstResponseTimeColor(other.frt / 1000)
                              )
                            )}
                          >
                            {' '}
                            (FRT: {formatUseTime(other.frt / 1000)})
                          </span>
                        )}
                    </span>
                  }
                />
              )}
            </div>

            {/* Request conversion (admin only, not for refund) */}
            {showConversion && (
              <DetailSection label={t('Request Conversion')}>
                <div className='relative min-w-0'>
                  <Button
                    variant='ghost'
                    size='sm'
                    className='absolute top-0 right-0 h-5 w-5 p-0'
                    onClick={() => copyToClipboard(conversionLabel)}
                    title={t('Copy to clipboard')}
                    aria-label={t('Copy to clipboard')}
                  >
                    {copiedText === conversionLabel ? (
                      <Check className='size-3 text-green-600' />
                    ) : (
                      <Copy className='size-3' />
                    )}
                  </Button>
                  <div className='min-w-0 space-y-1 pr-6'>
                    {other?.request_path && (
                      <DetailRow
                        label={t('Path')}
                        value={other.request_path}
                        mono
                      />
                    )}
                    <div className='flex min-w-0 items-center gap-1.5 text-xs'>
                      <Route
                        className='text-muted-foreground size-3'
                        aria-hidden='true'
                      />
                      <span className='min-w-0 break-all sm:break-words'>
                        {conversionLabel}
                      </span>
                    </div>
                  </div>
                </div>
              </DetailSection>
            )}

            {/* Reject reason (admin only) */}
            {props.isAdmin && other?.reject_reason && (
              <DetailSection
                icon={<AlertTriangle className='size-3.5' aria-hidden='true' />}
                label={t('Reject Reason')}
                variant='danger'
              >
                <p className='text-xs break-words'>{other.reject_reason}</p>
              </DetailSection>
            )}

            {/* Violation fee info */}
            {isViolation && other && (
              <DetailSection
                icon={<AlertTriangle className='size-3.5' aria-hidden='true' />}
                label={t('Violation Fee')}
                variant='danger'
              >
                {other.violation_fee_code && (
                  <DetailRow
                    label={t('Violation Code')}
                    value={other.violation_fee_code}
                    mono
                  />
                )}
                {other.violation_fee_marker && (
                  <DetailRow
                    label={t('Violation Marker')}
                    value={other.violation_fee_marker}
                  />
                )}
                <DetailRow
                  label={t('Fee Amount')}
                  value={formatLogQuota(other.fee_quota ?? props.log.quota)}
                  mono
                />
              </DetailSection>
            )}

            {/* Refund details (type=6) */}
            {isRefund && other && (other.task_id || other.reason) && (
              <DetailSection label={t('Refund Details')}>
                {other.task_id && (
                  <DetailRow label={t('Task ID')} value={other.task_id} mono />
                )}
                {other.pre_consumed_quota != null && (
                  <DetailRow
                    label={t('Pre-Consumed Quota')}
                    value={formatLogQuota(other.pre_consumed_quota)}
                    mono
                  />
                )}
                {other.actual_quota != null && (
                  <DetailRow
                    label={t('Actual Cost')}
                    value={formatLogQuota(other.actual_quota)}
                    mono
                  />
                )}
                <DetailRow
                  label={t('Refund Amount')}
                  value={formatLogQuota(other.refund_quota ?? props.log.quota)}
                  mono
                />
                {other.actual_total_tokens != null && (
                  <DetailRow
                    label={t('Actual Total Tokens')}
                    value={formatTokens(other.actual_total_tokens)}
                    mono
                  />
                )}
                {other.actual_completion_tokens != null && (
                  <DetailRow
                    label={t('Actual Output Tokens')}
                    value={formatTokens(other.actual_completion_tokens)}
                    mono
                  />
                )}
                {other.matched_tier && (
                  <DetailRow
                    label={t('Matched Tier')}
                    value={other.matched_tier}
                    mono
                  />
                )}
                {other.reason && (
                  <DetailRow label={t('Reason')} value={other.reason} />
                )}
              </DetailSection>
            )}

            {/* Top-up audit info (type=1, admin only) */}
            {showTopupAuditSection && (
              <DetailSection
                icon={<ShieldCheck className='size-3.5' aria-hidden='true' />}
                label={t('Top-up Audit Info')}
              >
                {topupAuditFields.map((field, idx) => (
                  <DetailRow
                    key={idx}
                    label={field.label}
                    value={field.value}
                    mono
                  />
                ))}
                {showLegacyTopupWarning && (
                  <div className='flex items-start gap-1.5 text-xs text-amber-600 dark:text-amber-400'>
                    <Info
                      className='mt-0.5 size-3.5 shrink-0'
                      aria-hidden='true'
                    />
                    <span>
                      {t(
                        'This record was written by a pre-upgrade instance and lacks audit info. Upgrade the instance to record server IP, callback IP, payment method and system version.'
                      )}
                    </span>
                  </div>
                )}
              </DetailSection>
            )}

            {/* Manage operator (type=3, admin only) */}
            {manageOperator && (
              <DetailRow
                label={
                  <span className='flex items-center gap-1.5'>
                    <UserCog
                      className='text-muted-foreground size-3.5'
                      aria-hidden='true'
                    />
                    {t('Operator Admin')}
                  </span>
                }
                value={manageOperator}
                mono
              />
            )}

            {/* Audio/WebSocket token breakdown */}
            {hasAudioTokens && other && (
              <DetailSection
                icon={<Headphones className='size-3.5' aria-hidden='true' />}
                label={t('Audio Tokens')}
              >
                {other.audio_input != null && other.audio_input > 0 && (
                  <DetailRow
                    label={t('Audio Input')}
                    value={formatTokens(other.audio_input)}
                    mono
                  />
                )}
                {other.audio_output != null && other.audio_output > 0 && (
                  <DetailRow
                    label={t('Audio Output')}
                    value={formatTokens(other.audio_output)}
                    mono
                  />
                )}
                {other.text_input != null && other.text_input > 0 && (
                  <DetailRow
                    label={t('Text Input')}
                    value={formatTokens(other.text_input)}
                    mono
                  />
                )}
                {other.text_output != null && other.text_output > 0 && (
                  <DetailRow
                    label={t('Text Output')}
                    value={formatTokens(other.text_output)}
                    mono
                  />
                )}
              </DetailSection>
            )}

            {/* Reasoning effort */}
            {other?.reasoning_effort && (
              <DetailRow
                label={t('Reasoning Effort')}
                value={
                  <StatusBadge
                    label={other.reasoning_effort}
                    variant={
                      other.reasoning_effort === 'high'
                        ? 'orange'
                        : other.reasoning_effort === 'medium'
                          ? 'yellow'
                          : 'green'
                    }
                    size='sm'
                    copyable={false}
                  />
                }
              />
            )}

            {/* System prompt override */}
            {other?.is_system_prompt_overwritten && (
              <DetailRow
                label={t('System Prompt')}
                value={
                  <StatusBadge
                    label={t('Overwritten')}
                    variant='orange'
                    size='sm'
                    copyable={false}
                  />
                }
              />
            )}

            {/* Model mapping */}
            {other?.is_model_mapped && other?.upstream_model_name && (
              <DetailSection label={t('Model Mapping')}>
                <DetailRow
                  label={t('Request Model')}
                  value={props.log.model_name}
                  mono
                />
                <DetailRow
                  label={t('Actual Model')}
                  value={other.upstream_model_name}
                  mono
                />
              </DetailSection>
            )}

            {/* Token breakdown (for consume/error types with token data) */}
            {isDisplayableType(props.log.type) && !isConsume && other && (
              <TokenBreakdown log={props.log} other={other} />
            )}

            {/* Billing breakdown (consume type) */}
            {isConsume && other && !isViolation && (
              <BillingBreakdown
                log={props.log}
                other={other}
                isRoot={props.isRoot}
              />
            )}

            {/* Stream status details (admin only) */}
            {props.isAdmin &&
              other?.stream_status &&
              other.stream_status.status !== 'ok' && (
                <DetailSection label={t('Stream Status')}>
                  <DetailRow
                    label={t('Status')}
                    value={
                      <StatusBadge
                        label={other.stream_status.status || t('Error')}
                        variant='red'
                        size='sm'
                        copyable={false}
                      />
                    }
                  />
                  {other.stream_status.end_reason && (
                    <DetailRow
                      label={t('End Reason')}
                      value={other.stream_status.end_reason}
                    />
                  )}
                  {(other.stream_status.error_count ?? 0) > 0 && (
                    <DetailRow
                      label={t('Soft Errors')}
                      value={String(other.stream_status.error_count)}
                    />
                  )}
                  {other.stream_status.end_error && (
                    <DetailRow
                      label={t('End Error')}
                      value={other.stream_status.end_error}
                    />
                  )}
                  {Array.isArray(other.stream_status.errors) &&
                    other.stream_status.errors.length > 0 && (
                      <pre className='bg-background/60 mt-1 max-h-32 overflow-y-auto rounded border p-2 font-mono text-[11px] leading-relaxed break-words whitespace-pre-wrap'>
                        {other.stream_status.errors.join('\n')}
                      </pre>
                    )}
                </DetailSection>
              )}

            {/* Subscription billing details */}
            {isSubscription && other && (
              <DetailSection label={t('Subscription Billing')}>
                {other.subscription_plan_id && (
                  <DetailRow
                    label={t('Plan')}
                    value={`#${other.subscription_plan_id} ${other.subscription_plan_title || ''}`.trim()}
                  />
                )}
                {other.subscription_id && (
                  <DetailRow
                    label={t('Instance')}
                    value={`#${other.subscription_id}`}
                    mono
                  />
                )}
                {other.subscription_pre_consumed != null && (
                  <DetailRow
                    label={t('Pre-consumed')}
                    value={formatLogQuota(other.subscription_pre_consumed)}
                    mono
                  />
                )}
                {other.subscription_post_delta != null &&
                  other.subscription_post_delta !== 0 && (
                    <DetailRow
                      label={t('Post Delta')}
                      value={formatLogQuota(other.subscription_post_delta)}
                      mono
                    />
                  )}
                {other.subscription_consumed != null && (
                  <DetailRow
                    label={t('Final Consumed')}
                    value={formatLogQuota(other.subscription_consumed)}
                    mono
                  />
                )}
                {other.subscription_remain != null && (
                  <DetailRow
                    label={t('Remaining')}
                    value={`${formatLogQuota(other.subscription_remain)}${other.subscription_total != null ? ` / ${formatLogQuota(other.subscription_total)}` : ''}`}
                    mono
                  />
                )}
              </DetailSection>
            )}

            {/* Param override */}
            {other?.po && Array.isArray(other.po) && other.po.length > 0 && (
              <DetailSection
                icon={<Settings2 className='size-3.5' aria-hidden='true' />}
                label={`${t('Param Override')} (${other.po.length})`}
              >
                {other.po.filter(Boolean).map((line, idx) => {
                  const parsed = parseAuditLine(line)
                  if (!parsed) return null
                  return (
                    <div
                      key={idx}
                      className='bg-background/60 flex min-w-0 flex-col gap-1.5 rounded border p-2 sm:flex-row sm:items-start sm:gap-2'
                    >
                      <StatusBadge
                        variant='neutral'
                        label={getParamOverrideActionLabel(parsed.action, t)}
                        className='shrink-0 font-medium'
                        copyable={false}
                      />
                      <span className='min-w-0 font-mono text-[11px] leading-relaxed break-all sm:break-words'>
                        {parsed.content}
                      </span>
                    </div>
                  )
                })}
              </DetailSection>
            )}

            {/* Content */}
            {details && (
              <div className='space-y-1.5'>
                <Label className='text-xs font-semibold'>{t('Content')}</Label>
                <div className='bg-muted/30 relative min-w-0 overflow-hidden rounded-md border p-2.5'>
                  <Button
                    variant='ghost'
                    size='sm'
                    className='absolute top-1.5 right-1.5 h-5 w-5 p-0'
                    onClick={() => copyToClipboard(details)}
                    title={t('Copy to clipboard')}
                    aria-label={t('Copy to clipboard')}
                  >
                    {copiedText === details ? (
                      <Check className='size-3 text-green-600' />
                    ) : (
                      <Copy className='size-3' />
                    )}
                  </Button>
                  <p className='min-w-0 pr-6 text-xs leading-relaxed break-all whitespace-pre-wrap sm:break-words'>
                    {details}
                  </p>
                </div>
              </div>
            )}
          </div>
        </ScrollArea>
      </DialogContent>
    </Dialog>
  )
}

function isDisplayableType(type: number): boolean {
  return [0, 2, 5, 6].includes(type)
}
