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
import { useEffect, useMemo, useState } from 'react'
import { VChart } from '@visactor/react-vchart'
import {
  ArrowUpRight,
  CalendarClock,
  ChartNoAxesCombined,
  Check,
  CreditCard,
  Copy,
  Loader2,
  Monitor,
  KeyRound,
  RefreshCw,
  ShieldCheck,
  SquareTerminal,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { getCurrencyDisplay } from '@/lib/currency'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VCHART_OPTION } from '@/lib/vchart'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { CopyButton } from '@/components/copy-button'
import { SectionPageLayout } from '@/components/layout'
import {
  fetchSubscriptionTokenKey,
  getSubscriptionKeyUsage,
  getPublicPlans,
  getSelfSubscriptionFull,
  paySubscriptionCreem,
  paySubscriptionEpay,
  paySubscriptionStripe,
  paySubscriptionWaffoPancake,
} from '@/features/subscriptions/api'
import {
  formatDuration,
  getResetQuota,
  formatResetPeriod,
} from '@/features/subscriptions/lib'
import type {
  PlanRecord,
  SelfSubscriptionData,
  SubscriptionPayResponse,
  SubscriptionUsageStats,
  UserSubscriptionRecord,
} from '@/features/subscriptions/types'

type PaymentProvider = 'stripe' | 'creem' | 'waffo-pancake' | 'epay'
type UsageRange = 7 | 30
type InstallTarget = 'unix' | 'windows'

const MODELSELL_CLI_INSTALL_COMMANDS: Record<InstallTarget, string> = {
  unix: 'curl -fsSL https://static.modelsell.com/modelsell-cli/install.sh | sh',
  windows:
    'powershell -ExecutionPolicy Bypass -c "irm https://static.modelsell.com/modelsell-cli/install.ps1 | iex"',
}

const PAYMENT_PROVIDER_LABEL_KEYS: Record<PaymentProvider, string> = {
  stripe: 'Pay with Stripe',
  creem: 'Pay with Creem',
  'waffo-pancake': 'Pay with Waffo Pancake',
  epay: 'Pay with EPay',
}

function quotaToUSD(quota: number) {
  const { config } = getCurrencyDisplay()
  return quota / config.quotaPerUnit
}

function formatUSDAmount(usd: number) {
  return new Intl.NumberFormat(undefined, {
    style: 'currency',
    currency: 'USD',
    currencyDisplay: 'narrowSymbol',
    minimumFractionDigits: 0,
    maximumFractionDigits: Math.abs(usd) >= 1 ? 2 : 4,
  }).format(usd)
}

function formatQuotaUSD(quota: number) {
  return formatUSDAmount(quotaToUSD(quota))
}

function getAvailableProviders(plan: PlanRecord): PaymentProvider[] {
  const providers: PaymentProvider[] = []
  if (plan.plan.stripe_price_id) providers.push('stripe')
  if (plan.plan.creem_product_id) providers.push('creem')
  if (plan.plan.waffo_pancake_product_id) providers.push('waffo-pancake')
  providers.push('epay')
  return providers
}

function openCheckout(response: SubscriptionPayResponse) {
  const url =
    response.url || response.data?.pay_link || response.data?.checkout_url || ''
  if (url) {
    window.location.href = url
    return true
  }
  return false
}

function planModelsText(plan?: PlanRecord['plan']) {
  if (!plan?.model_limits_enabled || !plan.model_limits) return ''
  return plan.model_limits
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
    .join(', ')
}

function formatSubscriptionKey(key?: string) {
  const trimmed = key?.trim()
  if (!trimmed) return ''
  return trimmed.startsWith('sp-') ? trimmed : `sp-${trimmed}`
}

function UsageMetric({
  label,
  value,
  helper,
}: {
  label: string
  value: string
  helper: string
}) {
  return (
    <div className='bg-muted/20 rounded-md border p-3'>
      <p className='text-muted-foreground text-xs'>{label}</p>
      <p className='mt-1 text-lg font-semibold'>{value}</p>
      <p className='text-muted-foreground mt-1 text-xs'>{helper}</p>
    </div>
  )
}

function SubscriptionUsagePanel({
  stats,
  range,
  onRangeChange,
  loading,
}: {
  stats?: SubscriptionUsageStats
  range: UsageRange
  onRangeChange: (range: UsageRange) => void
  loading: boolean
}) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()

  const chartValues = useMemo(() => {
    return (stats?.points || []).flatMap((point) => [
      {
        label: point.label,
        date: point.date,
        metric: t('Requests'),
        value: point.requests,
      },
      {
        label: point.label,
        date: point.date,
        metric: t('Quota Used'),
        value: quotaToUSD(point.quota),
      },
    ])
  }, [stats?.points, t])

  const spec = useMemo(() => {
    if (chartValues.length === 0) return null
    return {
      type: 'line' as const,
      data: [{ id: 'subscription-key-usage', values: chartValues }],
      xField: 'label',
      yField: 'value',
      seriesField: 'metric',
      point: { visible: true, style: { size: 4 } },
      line: { style: { lineWidth: 2 } },
      legends: {
        visible: true,
        orient: 'top' as const,
        item: { label: { style: { fill: 'currentColor' } } },
      },
      axes: [
        {
          orient: 'bottom' as const,
          label: {
            style: { fill: 'currentColor', fontSize: 10 },
            autoHide: true,
          },
          tick: { visible: false },
        },
        {
          orient: 'left' as const,
          label: { style: { fill: 'currentColor', fontSize: 10 } },
          grid: { visible: true, style: { lineDash: [3, 3] } },
        },
      ],
      tooltip: {
        mark: {
          title: (datum: Record<string, unknown>) => String(datum?.date ?? ''),
          content: [
            {
              key: (datum: Record<string, unknown>) =>
                String(datum?.metric ?? ''),
              value: (datum: Record<string, unknown>) =>
                datum?.metric === t('Quota Used')
                  ? formatUSDAmount(Number(datum?.value || 0))
                  : String(datum?.value ?? 0),
            },
          ],
        },
      },
    }
  }, [chartValues])

  return (
    <Card>
      <CardHeader className='pb-3'>
        <div className='flex flex-wrap items-center justify-between gap-3'>
          <CardTitle className='flex items-center gap-2 text-base'>
            <ChartNoAxesCombined className='h-4 w-4' />
            {t('Key usage')}
          </CardTitle>
          <div className='bg-muted/40 inline-flex h-8 overflow-hidden rounded-md border p-0.5'>
            {[7, 30].map((days) => (
              <button
                key={days}
                type='button'
                onClick={() => onRangeChange(days as UsageRange)}
                className={`rounded px-3 text-xs font-medium transition-colors ${
                  range === days
                    ? 'bg-background text-foreground shadow-sm'
                    : 'text-muted-foreground hover:text-foreground'
                }`}
              >
                {days === 7 ? t('7 days') : t('30 days')}
              </button>
            ))}
          </div>
        </div>
      </CardHeader>
      <CardContent className='space-y-4'>
        {loading ? (
          <>
            <div className='grid gap-3 sm:grid-cols-3'>
              <Skeleton className='h-24 rounded-md' />
              <Skeleton className='h-24 rounded-md' />
              <Skeleton className='h-24 rounded-md' />
            </div>
            <Skeleton className='h-72 rounded-md' />
          </>
        ) : (
          <>
            <div className='grid gap-3 sm:grid-cols-3'>
              <UsageMetric
                label={t('Total requests')}
                value={String(stats?.total_requests || 0)}
                helper={t('Successful calls in range')}
              />
              <UsageMetric
                label={t('Today requests')}
                value={String(stats?.today_requests || 0)}
                helper={t('Since local midnight')}
              />
              <UsageMetric
                label={t('Quota used')}
                value={formatQuotaUSD(stats?.total_quota || 0)}
                helper={t('Subscription key consumption')}
              />
            </div>
            <div className='bg-muted/10 h-72 rounded-md border p-2'>
              {themeReady && spec ? (
                <VChart
                  spec={{
                    ...spec,
                    theme: resolvedTheme === 'dark' ? 'dark' : 'light',
                    background: 'transparent',
                  }}
                  option={VCHART_OPTION}
                />
              ) : (
                <div className='text-muted-foreground flex h-full items-center justify-center text-sm'>
                  {t('No usage data yet')}
                </div>
              )}
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}

function ModelSellCliCard() {
  const { t } = useTranslation()
  const [target, setTarget] = useState<InstallTarget>('unix')
  const command = MODELSELL_CLI_INSTALL_COMMANDS[target]

  return (
    <div className='bg-muted/20 space-y-3 rounded-md border p-3'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div className='min-w-0'>
          <p className='text-foreground flex items-center gap-2 text-sm font-medium'>
            <SquareTerminal className='h-4 w-4' />
            ModelSell CLI
          </p>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('Configure mainstream Agent platforms in one click.')}
          </p>
        </div>
        <Button
          type='button'
          variant='outline'
          size='sm'
          render={<a href='https://docs.modelsell.com/' target='_blank' />}
        >
          {t('Learn more')}
          <ArrowUpRight className='h-4 w-4' />
        </Button>
      </div>

      <div className='bg-background/60 flex flex-wrap items-center justify-between gap-2 rounded-md border p-2'>
        <div
          className='bg-muted/40 inline-flex overflow-hidden rounded-md border p-0.5'
          role='tablist'
          aria-label={t('Installation system')}
        >
          <button
            type='button'
            role='tab'
            aria-selected={target === 'unix'}
            onClick={() => setTarget('unix')}
            className={`inline-flex items-center gap-2 rounded px-3 py-1.5 text-xs font-medium transition-colors ${
              target === 'unix'
                ? 'bg-background text-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground'
            }`}
          >
            <SquareTerminal className='h-3.5 w-3.5' />
            macOS / Linux
          </button>
          <button
            type='button'
            role='tab'
            aria-selected={target === 'windows'}
            onClick={() => setTarget('windows')}
            className={`inline-flex items-center gap-2 rounded px-3 py-1.5 text-xs font-medium transition-colors ${
              target === 'windows'
                ? 'bg-background text-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground'
            }`}
          >
            <Monitor className='h-3.5 w-3.5' />
            Windows
          </button>
        </div>
        <CopyButton
          value={command}
          tooltip={t('Copy install script')}
          successTooltip={t('Copied!')}
          variant='ghost'
          size='sm'
        >
          {t('Copy')}
        </CopyButton>
      </div>

      <pre className='overflow-x-auto rounded-md border bg-slate-950 p-3 text-xs leading-6 text-slate-100'>
        <code>{command}</code>
      </pre>
    </div>
  )
}

function ActiveSubscriptionCard({
  item,
  resolvedKey,
  onRevealKey,
  onCopyKey,
  copyingKey,
  usageStats,
  usageRange,
  usageLoading,
  onUsageRangeChange,
}: {
  item: UserSubscriptionRecord
  resolvedKey?: string
  onRevealKey: (id: number) => void
  onCopyKey: (id: number) => void
  copyingKey: boolean
  usageStats?: SubscriptionUsageStats
  usageRange: UsageRange
  usageLoading: boolean
  onUsageRangeChange: (range: UsageRange) => void
}) {
  const { t } = useTranslation()
  const sub = item.subscription
  const plan = item.plan
  const token = item.token
  const total = Number(
    sub.amount_total || getResetQuota(plan || {}) || token?.remain_quota || 0
  )
  const used = Number(sub.amount_used || token?.used_quota || 0)
  const remain = total > 0 ? Math.max(total - used, 0) : 0
  const percent =
    total > 0 ? Math.min(100, Math.round((used / total) * 100)) : 0
  const models = planModelsText(plan)
  const displayKey = formatSubscriptionKey(resolvedKey || token?.key)

  return (
    <div className='space-y-4'>
      <div className='grid gap-4 lg:grid-cols-[minmax(0,1.15fr)_minmax(320px,0.85fr)]'>
        <Card>
          <CardHeader className='pb-3'>
            <div className='flex flex-wrap items-start justify-between gap-3'>
              <div>
                <CardTitle className='text-xl'>
                  {plan?.title || t('Active subscription')}
                </CardTitle>
                <p className='text-muted-foreground mt-1 text-sm'>
                  {plan?.subtitle ||
                    t('Your subscription key is ready to use.')}
                </p>
              </div>
              <Badge variant='secondary'>{t('Active')}</Badge>
            </div>
          </CardHeader>
          <CardContent className='space-y-5'>
            <div className='grid gap-3 sm:grid-cols-3'>
              <div className='rounded-md border p-3'>
                <p className='text-muted-foreground text-xs'>
                  {t('Remaining')}
                </p>
                <p className='mt-1 text-lg font-semibold'>
                  {total > 0 ? formatQuota(remain) : t('Unlimited')}
                </p>
              </div>
              <div className='rounded-md border p-3'>
                <p className='text-muted-foreground text-xs'>{t('Used')}</p>
                <p className='mt-1 text-lg font-semibold'>
                  {formatQuota(used)}
                </p>
              </div>
              <div className='rounded-md border p-3'>
                <p className='text-muted-foreground text-xs'>
                  {t('Next reset')}
                </p>
                <p className='mt-1 text-sm font-medium'>
                  {formatTimestampToDate(sub.next_reset_time || 0)}
                </p>
              </div>
            </div>
            {total > 0 ? (
              <div className='space-y-2'>
                <div className='text-muted-foreground flex justify-between text-xs'>
                  <span>{t('Usage')}</span>
                  <span>{percent}%</span>
                </div>
                <Progress value={percent} />
              </div>
            ) : null}
            <div className='grid gap-3 sm:grid-cols-2'>
              <div className='flex gap-3 rounded-md border p-3'>
                <RefreshCw className='text-muted-foreground mt-0.5 h-4 w-4' />
                <div>
                  <p className='text-sm font-medium'>{t('Quota refresh')}</p>
                  <p className='text-muted-foreground text-sm'>
                    {plan
                      ? `${formatResetPeriod(plan, t)} · ${
                          getResetQuota(plan) > 0
                            ? formatQuota(getResetQuota(plan))
                            : t('Unlimited')
                        }`
                      : '-'}
                  </p>
                </div>
              </div>
              <div className='flex gap-3 rounded-md border p-3'>
                <ShieldCheck className='text-muted-foreground mt-0.5 h-4 w-4' />
                <div>
                  <p className='text-sm font-medium'>{t('Allowed Models')}</p>
                  <p className='text-muted-foreground text-sm break-words'>
                    {models || t('All available models')}
                  </p>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className='pb-3'>
            <CardTitle className='flex items-center gap-2 text-base'>
              <KeyRound className='h-4 w-4' />
              {t('Subscription API Key')}
            </CardTitle>
          </CardHeader>
          <CardContent className='space-y-4'>
            <div className='bg-muted/30 rounded-md border p-3'>
              <div className='mb-2 flex items-center justify-between gap-2'>
                <span className='text-muted-foreground text-xs'>
                  {t('Dedicated key')}
                </span>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => onRevealKey(sub.id)}
                >
                  {resolvedKey ? t('Refresh key') : t('Show key')}
                </Button>
              </div>
              <div className='flex items-center gap-2'>
                <code className='bg-background min-w-0 flex-1 truncate rounded px-2 py-2 text-xs'>
                  {displayKey || t('Key is being prepared')}
                </code>
                {displayKey ? (
                  <Button
                    variant='ghost'
                    size='icon'
                    className='shrink-0'
                    onClick={() => onCopyKey(sub.id)}
                    disabled={copyingKey}
                    aria-label={t('Copy subscription key')}
                    title={t('Copy subscription key')}
                  >
                    {copyingKey ? (
                      <Loader2 className='h-4 w-4 animate-spin' />
                    ) : (
                      <Copy className='h-4 w-4' />
                    )}
                  </Button>
                ) : null}
              </div>
            </div>
            <ModelSellCliCard />
          </CardContent>
        </Card>
      </div>

      <SubscriptionUsagePanel
        stats={usageStats}
        range={usageRange}
        onRangeChange={onUsageRangeChange}
        loading={usageLoading}
      />
    </div>
  )
}

function PlanCard({
  item,
  paying,
  onPay,
}: {
  item: PlanRecord
  paying?: string
  onPay: (plan: PlanRecord, provider: PaymentProvider) => void
}) {
  const { t } = useTranslation()
  const providers = getAvailableProviders(item)
  const models = planModelsText(item.plan)

  return (
    <Card>
      <CardHeader className='pb-3'>
        <div className='flex items-start justify-between gap-3'>
          <div>
            <CardTitle className='text-lg'>{item.plan.title}</CardTitle>
            <p className='text-muted-foreground mt-1 text-sm'>
              {item.plan.subtitle ||
                t('Subscription quota with a dedicated API key')}
            </p>
          </div>
          <div className='text-right'>
            <p className='text-2xl font-semibold'>
              ${Number(item.plan.price_amount || 0).toFixed(2)}
            </p>
            <p className='text-muted-foreground text-xs'>
              {formatDuration(item.plan, t)}
            </p>
          </div>
        </div>
      </CardHeader>
      <CardContent className='space-y-4'>
        <div className='grid gap-2 text-sm'>
          <div className='flex items-center gap-2'>
            <Check className='text-success h-4 w-4' />
            <span>
              {item.plan.total_amount > 0
                ? formatQuota(item.plan.total_amount)
                : t('Unlimited quota')}
            </span>
          </div>
          <div className='flex items-center gap-2'>
            <RefreshCw className='text-muted-foreground h-4 w-4' />
            <span>
              {formatResetPeriod(item.plan, t)} ·{' '}
              {getResetQuota(item.plan) > 0
                ? formatQuota(getResetQuota(item.plan))
                : t('Unlimited')}
            </span>
          </div>
          <div className='flex items-center gap-2'>
            <KeyRound className='text-muted-foreground h-4 w-4' />
            <span>{t('Dedicated API key generated after purchase')}</span>
          </div>
          <div className='flex items-start gap-2'>
            <ShieldCheck className='text-muted-foreground mt-0.5 h-4 w-4' />
            <span>{models || t('All available models')}</span>
          </div>
        </div>
        <div className='flex flex-wrap gap-2'>
          {providers.map((provider) => (
            <Button
              key={provider}
              size='sm'
              onClick={() => onPay(item, provider)}
              disabled={paying === `${item.plan.id}:${provider}`}
            >
              <CreditCard className='h-4 w-4' />
              {paying === `${item.plan.id}:${provider}`
                ? t('Processing...')
                : t(PAYMENT_PROVIDER_LABEL_KEYS[provider])}
              <ArrowUpRight className='h-4 w-4' />
            </Button>
          ))}
        </div>
      </CardContent>
    </Card>
  )
}

export function UserSubscription() {
  const { t } = useTranslation()
  const { copyToClipboard } = useCopyToClipboard()
  const [plans, setPlans] = useState<PlanRecord[]>([])
  const [self, setSelf] = useState<SelfSubscriptionData | null>(null)
  const [loading, setLoading] = useState(true)
  const [paying, setPaying] = useState<string>('')
  const [resolvedKeys, setResolvedKeys] = useState<Record<number, string>>({})
  const [copyingKeyId, setCopyingKeyId] = useState<number | null>(null)
  const [usageRange, setUsageRange] = useState<UsageRange>(7)
  const [usageStats, setUsageStats] = useState<SubscriptionUsageStats>()
  const [usageLoading, setUsageLoading] = useState(false)

  const activeSubscription = useMemo(
    () => self?.subscriptions?.[0],
    [self?.subscriptions]
  )

  async function refresh() {
    setLoading(true)
    try {
      const [plansRes, selfRes] = await Promise.all([
        getPublicPlans(),
        getSelfSubscriptionFull(),
      ])
      if (plansRes.success) setPlans(plansRes.data || [])
      if (selfRes.success && selfRes.data) setSelf(selfRes.data)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refresh()
  }, [])

  useEffect(() => {
    if (!activeSubscription?.subscription?.id) {
      setUsageStats(undefined)
      return
    }
    let cancelled = false
    const loadUsage = async () => {
      setUsageLoading(true)
      try {
        const res = await getSubscriptionKeyUsage(
          activeSubscription.subscription.id,
          usageRange
        )
        if (!cancelled && res.success) {
          setUsageStats(res.data)
        }
      } finally {
        if (!cancelled) {
          setUsageLoading(false)
        }
      }
    }
    loadUsage()
    return () => {
      cancelled = true
    }
  }, [activeSubscription?.subscription?.id, usageRange])

  async function fetchFullSubscriptionKey(subscriptionId: number) {
    const res = await fetchSubscriptionTokenKey(subscriptionId)
    if (res.success && res.data?.key) {
      const fullKey = formatSubscriptionKey(res.data.key)
      setResolvedKeys((current) => ({
        ...current,
        [subscriptionId]: fullKey,
      }))
      return fullKey
    }
    toast.error(res.message || t('An unexpected error occurred'))
    return ''
  }

  async function handleRevealKey(subscriptionId: number) {
    const fullKey = await fetchFullSubscriptionKey(subscriptionId)
    if (fullKey) toast.success(t('Key loaded'))
  }

  async function handleCopyKey(subscriptionId: number) {
    setCopyingKeyId(subscriptionId)
    try {
      const fullKey =
        resolvedKeys[subscriptionId] ||
        (await fetchFullSubscriptionKey(subscriptionId))
      if (fullKey) await copyToClipboard(fullKey)
    } finally {
      setCopyingKeyId(null)
    }
  }

  async function handlePay(plan: PlanRecord, provider: PaymentProvider) {
    const marker = `${plan.plan.id}:${provider}`
    setPaying(marker)
    try {
      let res: SubscriptionPayResponse
      if (provider === 'stripe') {
        res = await paySubscriptionStripe({ plan_id: plan.plan.id })
      } else if (provider === 'creem') {
        res = await paySubscriptionCreem({ plan_id: plan.plan.id })
      } else if (provider === 'waffo-pancake') {
        res = await paySubscriptionWaffoPancake({ plan_id: plan.plan.id })
      } else {
        res = await paySubscriptionEpay({
          plan_id: plan.plan.id,
          payment_method: 'alipay',
        })
      }
      if (res.success && openCheckout(res)) return
      if (res.success) {
        toast.success(t('Payment request created'))
        await refresh()
      }
    } finally {
      setPaying('')
    }
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Subscription')}</SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {activeSubscription
          ? t('Use your subscription API key and monitor quota refreshes.')
          : t('Choose a plan to get recurring quota and a dedicated API key.')}
      </SectionPageLayout.Description>
      <SectionPageLayout.Content>
        <div className='mx-auto flex w-full max-w-7xl flex-col gap-5'>
          {loading ? (
            <div className='grid gap-4 md:grid-cols-2'>
              <Skeleton className='h-80 rounded-md' />
              <Skeleton className='h-80 rounded-md' />
            </div>
          ) : activeSubscription ? (
            <ActiveSubscriptionCard
              item={activeSubscription}
              resolvedKey={resolvedKeys[activeSubscription.subscription.id]}
              onRevealKey={handleRevealKey}
              onCopyKey={handleCopyKey}
              copyingKey={copyingKeyId === activeSubscription.subscription.id}
              usageStats={usageStats}
              usageRange={usageRange}
              usageLoading={usageLoading}
              onUsageRangeChange={setUsageRange}
            />
          ) : (
            <>
              <div className='bg-muted/20 grid gap-3 rounded-md border p-4 md:grid-cols-3'>
                <div className='flex gap-3'>
                  <KeyRound className='text-muted-foreground mt-0.5 h-5 w-5' />
                  <div>
                    <p className='font-medium'>{t('Dedicated key')}</p>
                    <p className='text-muted-foreground text-sm'>
                      {t(
                        'A subscription key is generated automatically after purchase.'
                      )}
                    </p>
                  </div>
                </div>
                <div className='flex gap-3'>
                  <RefreshCw className='text-muted-foreground mt-0.5 h-5 w-5' />
                  <div>
                    <p className='font-medium'>{t('Recurring quota')}</p>
                    <p className='text-muted-foreground text-sm'>
                      {t(
                        'Daily, weekly, or monthly refresh follows the plan settings.'
                      )}
                    </p>
                  </div>
                </div>
                <div className='flex gap-3'>
                  <CalendarClock className='text-muted-foreground mt-0.5 h-5 w-5' />
                  <div>
                    <p className='font-medium'>{t('Plan bound')}</p>
                    <p className='text-muted-foreground text-sm'>
                      {t(
                        'The key expires with the subscription and inherits model access.'
                      )}
                    </p>
                  </div>
                </div>
              </div>
              <div className='grid gap-4 lg:grid-cols-2'>
                {plans.map((plan) => (
                  <PlanCard
                    key={plan.plan.id}
                    item={plan}
                    paying={paying}
                    onPay={handlePay}
                  />
                ))}
              </div>
            </>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
