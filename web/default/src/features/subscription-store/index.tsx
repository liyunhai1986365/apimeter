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
import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import type { TFunction } from 'i18next'
import {
  ArrowRight,
  CreditCard,
  Gauge,
  KeyRound,
  PackageCheck,
  Sparkles,
  Check,
  Cpu,
  Layers,
  SquareTerminal,
  Copy,
  Monitor,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { formatQuota } from '@/lib/format'
import { normalizeDiscountLabel } from '@/lib/group-discount'
import {
  formatSystemTemplate,
  getAgentToolsURL,
  getCliDisplayName,
  getCliDocsURL,
  getCliInstallCommands,
  getSitePlanName,
} from '@/lib/site-branding'
import { cn } from '@/lib/utils'
import { useSystemConfig } from '@/hooks/use-system-config'
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { DiscountTooltip } from '@/components/discount-tooltip'
import { PublicLayout } from '@/components/layout'
import { PageTransition } from '@/components/page-transition'
import {
  getPublicPlans,
  paySubscriptionStripe,
} from '@/features/subscriptions/api'
import {
  formatDuration,
  formatResetPeriod,
  getResetQuota,
} from '@/features/subscriptions/lib'
import {
  getSubscriptionCheckoutUrl,
  isSubscriptionPaymentSuccess,
} from '@/features/subscriptions/lib/payment-response'
import type {
  PlanRecord,
  SubscriptionPlan,
} from '@/features/subscriptions/types'

type SellingPlan = PlanRecord & {
  resetAmount: number
  totalAmount: number
  priceAmount: number
  resetPeriodLabel: string
  durationLabel: string
  accessLabel: string
  includedLabel: string
  cadenceLabel: string
  isFeatured: boolean
}

// Tokens anchor rate (approx 30 Tokens = $1, floating anchor).

function formatPlanPrice(plan: SubscriptionPlan) {
  return formatBillingCurrencyFromUSD(Number(plan.price_amount || 0), {
    digitsLarge: 2,
    digitsSmall: 2,
    abbreviate: false,
  })
}

function getCadence(plan: SubscriptionPlan, t: TFunction) {
  if (plan.duration_unit === 'month' && plan.duration_value === 1) {
    return t('per seat / month')
  }
  if (plan.duration_unit === 'year' && plan.duration_value === 1) {
    return t('per seat / year')
  }
  if (plan.duration_unit === 'day' && plan.duration_value === 1) {
    return t('per seat / day')
  }
  return `/${formatDuration(plan, t)}`
}

function getAccessLabel(
  plan: SubscriptionPlan,
  t: TFunction,
  mainlandChinaPresentationEnabled: boolean
) {
  if (plan.model_limits_enabled && plan.model_limits?.trim()) {
    const count = plan.model_limits
      .split(',')
      .map((item) => item.trim())
      .filter(Boolean).length
    if (count > 0) {
      return t('{{count}} selected models', { count })
    }
  }
  if (plan.upgrade_group) {
    return t('{{group}} group access', { group: plan.upgrade_group })
  }
  return t(mainlandChinaPresentationEnabled ? 'Domestic models' : 'All models')
}

function useStorePlans() {
  const { t } = useTranslation()
  const { mainlandChinaPresentationEnabled = false } = useSystemConfig()
  const plansQuery = useQuery({
    queryKey: ['public-subscription-plans'],
    queryFn: getPublicPlans,
    staleTime: 5 * 60 * 1000,
  })

  const plans = useMemo<SellingPlan[]>(() => {
    const source = plansQuery.data?.data || []
    return source
      .filter((item) => item.plan?.enabled)
      .map((item, index) => {
        const plan = item.plan
        const totalAmount = Number(plan.total_amount || 0)
        const resetAmount = getResetQuota(plan)

        return {
          ...item,
          resetAmount,
          totalAmount,
          priceAmount: Number(plan.price_amount || 0),
          resetPeriodLabel: formatResetPeriod(plan, t),
          durationLabel: formatDuration(plan, t),
          accessLabel: getAccessLabel(
            plan,
            t,
            mainlandChinaPresentationEnabled
          ),
          includedLabel:
            totalAmount > 0 ? formatQuota(totalAmount) : t('Unlimited'),
          cadenceLabel: getCadence(plan, t),
          isFeatured: index === 0 && source.length > 1,
        }
      })
      .sort((a, b) => {
        const sortDelta =
          Number(b.plan.sort_order || 0) - Number(a.plan.sort_order || 0)
        if (sortDelta !== 0) return sortDelta
        return a.priceAmount - b.priceAmount
      })
  }, [mainlandChinaPresentationEnabled, plansQuery.data?.data, t])

  return {
    plans,
    isLoading: plansQuery.isLoading,
    isError: plansQuery.isError,
    error: plansQuery.error,
    refetch: plansQuery.refetch,
  }
}

function PlanSkeleton() {
  return (
    <div className='grid gap-6 md:grid-cols-2 xl:grid-cols-3'>
      {[0, 1, 2].map((item) => (
        <Card
          key={item}
          className='rounded-[32px] border-black/5 bg-white p-2 dark:border-zinc-800 dark:bg-zinc-950'
        >
          <CardHeader className='space-y-4 p-8'>
            <Skeleton className='h-6 w-32 rounded-full' />
            <Skeleton className='h-10 w-28' />
          </CardHeader>
          <CardContent className='space-y-4 px-8 pb-8'>
            <Skeleton className='h-4 w-full' />
            <Skeleton className='h-4 w-4/5' />
            <Skeleton className='h-12 w-full rounded-full' />
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

function PlanCard({
  item,
  onChoose,
  paying,
}: {
  item: SellingPlan
  onChoose: (planId: number) => void
  paying?: boolean
}) {
  const { t } = useTranslation()
  const plan = item.plan
  const resetText =
    item.resetPeriodLabel === t('No Reset')
      ? t('One-time quota')
      : t('{{quota}} / {{period}}', {
          quota:
            item.resetAmount > 0
              ? formatQuota(item.resetAmount)
              : t('Unlimited'),
          period: item.resetPeriodLabel,
        })

  const discountLabel = normalizeDiscountLabel(plan.discount_description)
  const rows = [
    { label: t('Total Quota'), value: item.includedLabel },
    ...(discountLabel ? [{ value: discountLabel, discount: true }] : []),
    { label: t('Reset Cadence'), value: resetText },
    { label: t('Model Coverage'), value: item.accessLabel },
  ]

  const isPopular = item.isFeatured

  return (
    <Card
      className={cn(
        'group relative flex flex-col justify-between overflow-hidden rounded-[32px] border bg-white transition-all duration-500 hover:shadow-2xl hover:shadow-black/5 dark:bg-zinc-950 dark:hover:shadow-none',
        isPopular
          ? 'border-black ring-1 ring-black/5 dark:border-zinc-100 dark:ring-zinc-100/10'
          : 'border-black/5 dark:border-zinc-800'
      )}
    >
      {isPopular ? (
        <div className='absolute top-6 right-6 rounded-full bg-black px-4 py-1 text-[11px] font-bold tracking-wide text-white shadow-sm dark:bg-white dark:text-black'>
          {t('Popular')}
        </div>
      ) : null}
      <CardHeader className='space-y-4 p-8 pb-4'>
        <span className='text-muted-foreground text-xs font-semibold tracking-widest uppercase'>
          {plan.title}
        </span>
        <div className='space-y-2'>
          <div className='flex items-baseline gap-1.5'>
            <span className='text-foreground text-5xl font-extrabold tracking-tight'>
              {formatPlanPrice(plan)}
            </span>
            <span className='text-muted-foreground text-sm font-medium'>
              {item.cadenceLabel}
            </span>
          </div>
          <CardDescription className='text-muted-foreground line-clamp-2 min-h-12 pt-1 text-sm leading-relaxed'>
            {plan.subtitle || t('Stable AI API subscription package.')}
          </CardDescription>
        </div>
      </CardHeader>

      <CardContent className='flex flex-col gap-8 p-8 pt-0'>
        <div className='space-y-4 border-t border-black/5 pt-6 dark:border-zinc-800'>
          {rows.map((row) => {
            if (row.discount) {
              return (
                <div key={`discount-${row.value}`} className='flex justify-end'>
                  <DiscountTooltip label={row.value}>
                    <Badge variant='secondary' className='font-mono'>
                      {row.value}
                    </Badge>
                  </DiscountTooltip>
                </div>
              )
            }

            const isModelCoverage = row.label === t('Model Coverage')
            const isCustomLimits =
              plan.model_limits_enabled && plan.model_limits?.trim()

            return (
              <div key={row.label} className='space-y-1.5'>
                <div className='flex items-center justify-between gap-4'>
                  <span className='text-muted-foreground text-sm font-medium'>
                    {row.label}
                  </span>
                  {isModelCoverage && isCustomLimits ? (
                    <TooltipProvider>
                      <Tooltip>
                        <TooltipTrigger>
                          <span className='max-w-xs cursor-pointer truncate text-right text-sm font-semibold text-blue-600 underline decoration-dotted outline-none hover:text-blue-500'>
                            {row.value}
                          </span>
                        </TooltipTrigger>
                        <TooltipContent className='dark:border-zinc-850 max-w-xs space-y-1.5 rounded-xl border border-zinc-200 bg-white p-3 text-zinc-900 shadow-xl dark:bg-zinc-950 dark:text-zinc-100'>
                          <p className='dark:border-zinc-850 mb-1 border-b border-zinc-100 pb-1 text-xs font-bold'>
                            {t('Supported Models')}
                          </p>
                          <div className='flex max-h-36 flex-wrap gap-1 overflow-y-auto pr-1 text-[11px] leading-relaxed'>
                            {plan
                              .model_limits!.split(',')
                              .map((item) => item.trim())
                              .filter(Boolean)
                              .map((item) => (
                                <Badge
                                  key={item}
                                  variant='secondary'
                                  className='rounded-md px-1.5 py-0 text-[10px]'
                                >
                                  {item}
                                </Badge>
                              ))}
                          </div>
                        </TooltipContent>
                      </Tooltip>
                    </TooltipProvider>
                  ) : (
                    <span className='text-foreground max-w-[62%] truncate text-right text-sm font-semibold'>
                      {row.value}
                    </span>
                  )}
                </div>
              </div>
            )
          })}
        </div>

        <div className='space-y-3.5 border-t border-black/5 pt-6 dark:border-zinc-800'>
          <div className='text-muted-foreground flex items-center gap-3 text-sm'>
            <Check className='h-4.5 w-4.5 shrink-0 text-black dark:text-white' />
            <span>{t('Guaranteed zero data usage for model training')}</span>
          </div>
          <div className='text-muted-foreground flex items-center gap-3 text-sm'>
            <Check className='h-4.5 w-4.5 shrink-0 text-black dark:text-white' />
            <span>
              {t('Priority technical support, early access to new models')}
            </span>
          </div>
        </div>

        <Button
          className={cn(
            'h-12 w-full cursor-pointer rounded-full text-sm font-semibold shadow-sm transition-all duration-300',
            isPopular
              ? 'hover:bg-zinc-850 bg-black text-white dark:bg-white dark:text-black dark:hover:bg-zinc-100'
              : 'border border-black/10 hover:bg-zinc-50 dark:border-zinc-800 dark:hover:bg-zinc-900'
          )}
          onClick={() => onChoose(plan.id)}
          disabled={paying}
        >
          {paying ? t('Processing...') : t('Subscribe Now')}
          <ArrowRight className='ml-2 h-4 w-4 transition-transform group-hover/button:translate-x-1' />
        </Button>
      </CardContent>
    </Card>
  )
}

function EmptyPlans() {
  const { t } = useTranslation()
  return (
    <div className='mx-auto max-w-lg rounded-[32px] border border-dashed border-black/10 bg-white p-16 text-center shadow-sm dark:border-zinc-800 dark:bg-zinc-950'>
      <div className='mx-auto flex size-12 items-center justify-center rounded-2xl bg-zinc-50 text-zinc-900 dark:bg-zinc-900 dark:text-zinc-100'>
        <PackageCheck className='h-6 w-6' />
      </div>
      <h3 className='text-foreground mt-6 text-lg font-bold'>
        {t('No plans available')}
      </h3>
      <p className='text-muted-foreground mx-auto mt-2 max-w-sm text-sm leading-relaxed'>
        {t(
          'Published subscription packages will appear here. In the meantime, you can explore pay-as-you-go options.'
        )}
      </p>
      <Button
        variant='outline'
        className='mt-8 h-11 cursor-pointer rounded-full px-6 text-sm font-semibold'
        render={<Link to='/pricing' />}
      >
        {t('View pay-as-you-go pricing')}
      </Button>
    </div>
  )
}

function FAQSection() {
  const { t } = useTranslation()

  const faqs = [
    {
      q: t('Can I use the Token Plan in multiple AI tools?'),
      a: t(
        'Yes. Once you subscribe, your dedicated subscription API Key (starting with `sp-`) can be used in all compatible tools (like Cursor, VS Code, Cline, etc.) simultaneously, sharing the quota pool.'
      ),
    },
    {
      q: t('Does the Token Plan support automatic quota resets?'),
      a: t(
        'Depending on the plan details, your quota will reset automatically on a daily, weekly, or monthly cycle. Any unused quota from the previous cycle does not roll over to the next.'
      ),
    },
    {
      q: t('Can I mix subscription keys with wallet-based pay-as-you-go?'),
      a: t(
        'Absolutely. In your dashboard, you can configure your billing preference. For instance, setting it to "Subscription First" ensures your subscription quota is consumed first, falling back to your wallet balance if you exceed the limit or invoke unsupported models.'
      ),
    },
    {
      q: t('Are there any usage restrictions or rate limits?'),
      a: t(
        'Yes, rate limits (RPM and TPM) are active based on the tier you choose to prevent abuse. Refer to the plan details or model specifications in the Pricing page for detailed limits.'
      ),
    },
  ]

  return (
    <section className='border-t border-black/5 py-20 dark:border-zinc-800'>
      <div className='mx-auto mb-14 max-w-3xl text-center'>
        <h2 className='text-foreground text-3xl font-extrabold tracking-tight sm:text-4xl'>
          {t('Frequently Asked Questions')}
        </h2>
        <p className='text-muted-foreground mt-4 text-base'>
          {t(
            'Everything you need to know about pricing, seats, and integration.'
          )}
        </p>
      </div>

      <div className='mx-auto max-w-3xl'>
        <Accordion className='space-y-4'>
          {faqs.map((faq, index) => (
            <AccordionItem
              key={index}
              value={`faq-${index}`}
              className='rounded-2xl border border-black/5 bg-white/50 px-6 transition-all duration-200 hover:border-black/10 dark:border-zinc-800 dark:bg-zinc-950/50 dark:hover:border-zinc-700'
            >
              <AccordionTrigger className='text-foreground cursor-pointer py-5 text-sm font-bold outline-none hover:no-underline'>
                {faq.q}
              </AccordionTrigger>
              <AccordionContent className='text-muted-foreground pt-1 pb-5 text-sm leading-relaxed'>
                {faq.a}
              </AccordionContent>
            </AccordionItem>
          ))}
        </Accordion>
      </div>
    </section>
  )
}

function TokenPlanStrip() {
  const { t } = useTranslation()
  return (
    <section className='dark:border-zinc-850 rounded-[32px] border border-black/5 bg-zinc-50/50 p-8 shadow-sm backdrop-blur-sm sm:p-10 dark:bg-zinc-900/10'>
      <div className='grid gap-8 lg:grid-cols-[1fr_auto] lg:items-center'>
        <div className='space-y-3'>
          <Badge
            variant='outline'
            className='text-foreground rounded-full border-black/5 bg-white px-3 py-0.5 text-xs font-semibold dark:border-zinc-800 dark:bg-zinc-900'
          >
            <Sparkles className='mr-1.5 h-3.5 w-3.5 text-yellow-500' />
            Token Plan
          </Badge>
          <h2 className='text-foreground text-2xl font-bold tracking-tight sm:text-3xl'>
            {t(
              'Steady capacity from Subscription, flexibility with Pay-as-you-go.'
            )}
          </h2>
          <p className='text-muted-foreground max-w-3xl text-sm leading-relaxed'>
            {t(
              'Utilize subscription keys for predictable daily developer activity and code companion queries. Keep wallet balances active to scale for heavy batches or when experimenting with niche models.'
            )}
          </p>
        </div>
        <div className='flex shrink-0 flex-wrap gap-4'>
          <Button
            variant='outline'
            className='h-11 cursor-pointer rounded-full border-black/10 px-5 text-sm font-semibold dark:border-zinc-800'
            render={<Link to='/pricing' />}
          >
            <Gauge className='mr-2 h-4.5 w-4.5' />
            {t('Model pricing')}
          </Button>
          <Button
            variant='ghost'
            className='h-11 cursor-pointer rounded-full px-5 text-sm font-semibold hover:bg-black/5 dark:hover:bg-zinc-800'
            render={<Link to='/wallet' />}
          >
            <CreditCard className='mr-2 h-4.5 w-4.5' />
            {t('Recharge balance')}
          </Button>
        </div>
      </div>
    </section>
  )
}

function AgentAccessSection() {
  const { t } = useTranslation()
  const { systemName, serverAddress, mainlandChinaPresentationEnabled } =
    useSystemConfig()
  const [installTarget, setInstallTarget] = useState<'unix' | 'windows'>('unix')
  const [copied, setCopied] = useState(false)

  const command = getCliInstallCommands(serverAddress)[installTarget]
  const cliName = getCliDisplayName(systemName)

  const handleCopy = async () => {
    await navigator.clipboard?.writeText(command)
    setCopied(true)
    setTimeout(() => setCopied(false), 1600)
  }

  if (mainlandChinaPresentationEnabled) return null

  return (
    <section className='space-y-10 border-t border-black/5 py-16 dark:border-zinc-800'>
      <div className='grid items-center gap-8 lg:grid-cols-2'>
        <div className='space-y-6'>
          <Badge
            variant='outline'
            className='text-foreground rounded-full border-black/5 bg-white px-3 py-0.5 text-xs font-semibold dark:border-zinc-800 dark:bg-zinc-900'
          >
            <SquareTerminal className='mr-1 h-4 w-4 text-black dark:text-white' />
            {cliName}
          </Badge>
          <h2 className='text-foreground text-3xl leading-tight font-extrabold tracking-tight sm:text-4xl'>
            {t('Configure mainstream Agent platforms in one command')}
          </h2>
          <p className='text-muted-foreground max-w-xl text-sm leading-relaxed'>
            {formatSystemTemplate(
              t(
                'ModelSell CLI automatically writes API keys, API URLs, and default models into Claude Code, Codex CLI, Gemini CLI, and OpenClaw, saving repetitive configuration file edits.'
              ),
              systemName
            )}
          </p>
          <div className='flex flex-wrap gap-4 pt-2'>
            <Button
              className='hover:bg-zinc-850 h-11 cursor-pointer rounded-full bg-black px-6 text-xs font-semibold text-white shadow-sm dark:bg-white dark:text-black dark:hover:bg-zinc-100'
              render={<a href={getCliDocsURL(serverAddress)} />}
            >
              {t('View CLI docs')}
              <ArrowRight className='ml-1.5 h-3.5 w-3.5' />
            </Button>
            <Button
              variant='outline'
              className='h-11 cursor-pointer rounded-full border-black/10 px-6 text-xs font-semibold dark:border-zinc-800'
              render={<a href={getAgentToolsURL(serverAddress)} />}
            >
              <Sparkles className='mr-1.5 h-3.5 w-3.5' />
              {t('Browse Agent tools')}
            </Button>
          </div>
        </div>

        {/* Console Box container matching the visual style */}
        <div className='dark:border-zinc-850 space-y-4 overflow-hidden rounded-[32px] border border-black/5 bg-white p-6 shadow-sm dark:bg-zinc-950'>
          <div className='dark:border-zinc-850 flex items-center justify-between gap-4 border-b border-black/5 pb-3'>
            <div className='flex gap-2' role='tablist'>
              <button
                type='button'
                role='tab'
                aria-selected={installTarget === 'unix'}
                onClick={() => setInstallTarget('unix')}
                className={cn(
                  'inline-flex cursor-pointer items-center gap-2 rounded-full px-4 py-1.5 text-xs font-bold transition-all duration-200',
                  installTarget === 'unix'
                    ? 'bg-zinc-100 text-zinc-900 dark:bg-zinc-800 dark:text-zinc-100'
                    : 'text-muted-foreground hover:text-foreground'
                )}
              >
                <SquareTerminal className='h-3.5 w-3.5' />
                macOS / Linux
              </button>
              <button
                type='button'
                role='tab'
                aria-selected={installTarget === 'windows'}
                onClick={() => setInstallTarget('windows')}
                className={cn(
                  'inline-flex cursor-pointer items-center gap-2 rounded-full px-4 py-1.5 text-xs font-bold transition-all duration-200',
                  installTarget === 'windows'
                    ? 'bg-zinc-100 text-zinc-900 dark:bg-zinc-800 dark:text-zinc-100'
                    : 'text-muted-foreground hover:text-foreground'
                )}
              >
                <Monitor className='h-3.5 w-3.5' />
                Windows
              </button>
            </div>
            <Button
              type='button'
              variant='ghost'
              size='sm'
              className='h-8 cursor-pointer rounded-full px-3 text-xs font-semibold hover:bg-black/5 dark:hover:bg-zinc-800'
              onClick={handleCopy}
            >
              {copied ? (
                <Check className='h-3.5 w-3.5' />
              ) : (
                <Copy className='h-3.5 w-3.5' />
              )}
              <span className='ml-1.5'>{copied ? t('Copied') : t('Copy')}</span>
            </Button>
          </div>

          <pre className='overflow-x-auto rounded-2xl border border-zinc-900 bg-zinc-950 p-5 font-mono text-xs leading-7 text-zinc-100 shadow-inner sm:text-sm'>
            <code>{command}</code>
          </pre>
        </div>
      </div>
    </section>
  )
}
function GuidelinesComparisonGrid() {
  const { t } = useTranslation()

  const permitted = [
    t('Personal development and learning'),
    t('Vibe Coding and rapid prototyping'),
    t('Technical exploration and experimentation'),
    t('Personal projects and non-commercial applications'),
  ]

  const prohibited = [
    t('Formal production environments'),
    t('Commercial products or services'),
    t('Customer-facing terminal applications'),
    t('Multi-account sharing or quota abuse'),
    t('Multiple people sharing a single account'),
  ]

  return (
    <section className='space-y-8 border-t border-black/5 py-12 dark:border-zinc-800'>
      <div className='text-center'>
        <h2 className='text-foreground text-3xl font-extrabold tracking-tight'>
          {t('Usage Guidelines')}
        </h2>
      </div>

      {/* Permitted & Prohibited Comparison (1 row, 2 columns with soft bg colors) */}
      <div className='grid gap-6 md:grid-cols-2'>
        {/* Permitted */}
        <div className='space-y-6 rounded-2xl bg-emerald-50/50 p-8 dark:bg-emerald-950/10'>
          <h3 className='text-xl font-bold text-emerald-600 dark:text-emerald-400'>
            {t('Permitted Use Cases')}
          </h3>
          <ul className='space-y-4'>
            {permitted.map((item, index) => (
              <li
                key={index}
                className='flex items-center gap-3 text-[15px] font-medium text-zinc-700 dark:text-zinc-300'
              >
                <div className='flex size-6 shrink-0 items-center justify-center rounded-full border border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'>
                  <Check className='h-3.5 w-3.5' />
                </div>
                <span>{item}</span>
              </li>
            ))}
          </ul>
        </div>

        {/* Prohibited */}
        <div className='space-y-6 rounded-2xl bg-rose-50/30 p-8 dark:bg-rose-950/10'>
          <h3 className='text-xl font-bold text-rose-600 dark:text-rose-400'>
            {t('Prohibited Use Cases')}
          </h3>
          <ul className='space-y-4'>
            {prohibited.map((item, index) => (
              <li
                key={index}
                className='flex items-center gap-3 text-[15px] font-medium text-zinc-700 dark:text-zinc-300'
              >
                <div className='flex size-6 shrink-0 items-center justify-center rounded-full border border-rose-500/30 bg-rose-500/10 text-rose-600 dark:text-rose-400'>
                  <span className='text-xs font-bold'>✕</span>
                </div>
                <span>{item}</span>
              </li>
            ))}
          </ul>
        </div>
      </div>

      {/* Technical Limits (Single line row layout matching the design) */}
      <div className='flex flex-wrap items-center justify-start gap-x-12 gap-y-4 rounded-2xl border border-black/5 bg-white p-6 text-[15px] font-semibold shadow-sm dark:border-zinc-800 dark:bg-zinc-950'>
        <span className='font-bold text-zinc-900 dark:text-zinc-100'>
          {t('Technical limits:')}
        </span>
        <div className='flex items-center gap-2.5 text-zinc-700 dark:text-zinc-300'>
          <KeyRound className='h-4.5 w-4.5 text-zinc-500' />
          <span>{t('Rate limits')}:</span>
          <span className='font-medium text-zinc-500'>
            {t('10-15 RPM (Requests Per Minute)')}
          </span>
        </div>
        <div className='flex items-center gap-2.5 text-zinc-700 dark:text-zinc-300'>
          <span className='text-lg'>🕒</span>
          <span>{t('Quota window')}:</span>
          <span className='font-medium text-zinc-500'>
            {t('Rolling 5-hour refresh')}
          </span>
        </div>
        <div className='flex items-center gap-2.5 text-zinc-700 dark:text-zinc-300'>
          <span className='text-lg'>📅</span>
          <span>{t('Weekly limits')}:</span>
          <span className='font-medium text-zinc-500'>
            {t('Rolling 7-day reset')}
          </span>
        </div>
      </div>
    </section>
  )
}

function ComparisonTableSection() {
  const { t } = useTranslation()
  const { systemName, mainlandChinaPresentationEnabled } = useSystemConfig()
  const planName = getSitePlanName(systemName)

  if (mainlandChinaPresentationEnabled) return null

  const comparisonRows = [
    {
      feature: t('Monthly Fee'),
      builder: '$20 | $100 | $200',
      competitor: '$20 | $100 | $200',
    },
    {
      feature: t('Model Count'),
      builder: t('100+ Models'),
      builderHighlighted: true,
      competitor: t('Claude family only'),
    },
    {
      feature: t('LLM | Coding Models'),
      builder: t('Claude + GPT + Gemini + GLM + Kimi + MiniMax etc.'),
      builderHighlighted: true,
      competitor: t('Claude only'),
    },
    {
      feature: t('IDE Integration'),
      builder: t('Claude Code, CodeX, Open Code, OpenClaw etc.'),
      builderHighlighted: true,
      competitor: t('Claude Code'),
      competitorHighlighted: true,
    },
    {
      feature: t('Video Generation'),
      builder: t('SeedDance, Veo3 etc.'),
      builderHighlighted: true,
      competitor: false,
    },
    {
      feature: t('One Key, Multiple Models'),
      builder: true,
      competitor: false,
    },
    {
      feature: t('API Access'),
      builder: true,
      competitor: false,
    },
  ]

  const summaryPoints = [
    t(
      'Access 100+ models while other platforms only provide single-vendor models.'
    ),
    t('Cover coding, image generation, and chat all in one place.'),
    t(
      'Use a single API Key to seamlessly switch tools like Claude Code, Codex, etc.'
    ),
  ]

  return (
    <section className='space-y-12 border-t border-black/5 py-16 dark:border-zinc-800'>
      <div className='text-center'>
        <h2 className='text-foreground text-3xl font-extrabold tracking-tight'>
          {formatSystemTemplate(
            t('Why Choose Modelsell Tokens Plan?'),
            systemName
          )}
        </h2>
      </div>

      {/* Comparison Grid Table Container */}
      <div className='overflow-x-auto rounded-[24px] border border-black/5 bg-white shadow-sm dark:border-zinc-800 dark:bg-zinc-950'>
        <table className='w-full border-collapse text-left text-sm'>
          <thead>
            <tr className='dark:border-zinc-850 text-muted-foreground border-b border-black/5 bg-zinc-50/50 text-xs font-semibold tracking-wider uppercase dark:bg-zinc-900/10'>
              <th className='w-[25%] p-6'>{t('Feature')}</th>
              <th className='dark:border-zinc-850 w-[45%] border-x border-black/5 p-6'>
                {planName}
              </th>
              <th className='w-[30%] p-6'>{t('Claude Code Subscription')}</th>
            </tr>
          </thead>
          <tbody className='dark:divide-zinc-850 divide-y divide-black/5 font-medium text-zinc-700 dark:text-zinc-300'>
            {comparisonRows.map((row, idx) => (
              <tr
                key={idx}
                className='transition-colors duration-150 hover:bg-zinc-50/30 dark:hover:bg-zinc-900/10'
              >
                <td className='p-5 font-bold text-zinc-900 dark:text-zinc-100'>
                  {row.feature}
                </td>
                <td className='dark:border-zinc-850 border-x border-black/5 bg-zinc-50/10 p-5 dark:bg-zinc-900/5'>
                  <div className='flex items-center gap-2'>
                    {row.builder === true ? (
                      <div className='flex size-6 items-center justify-center rounded-full border border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'>
                        <Check className='h-3.5 w-3.5' />
                      </div>
                    ) : row.builder === false ? (
                      <div className='flex size-6 items-center justify-center rounded-full border border-rose-500/30 bg-rose-500/10 text-rose-600 dark:text-rose-400'>
                        <span className='text-xs font-bold'>✕</span>
                      </div>
                    ) : (
                      <span
                        className={cn(
                          row.builderHighlighted &&
                            'flex items-center gap-2 font-bold text-emerald-600 dark:text-emerald-400'
                        )}
                      >
                        {row.builderHighlighted && (
                          <div className='flex size-5 shrink-0 items-center justify-center rounded-full bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'>
                            <Check className='h-3.5 w-3.5' />
                          </div>
                        )}
                        {row.builder}
                      </span>
                    )}
                  </div>
                </td>
                <td className='p-5'>
                  <div className='flex items-center gap-2'>
                    {row.competitor === true ? (
                      <div className='flex size-6 items-center justify-center rounded-full border border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'>
                        <Check className='h-3.5 w-3.5' />
                      </div>
                    ) : row.competitor === false ? (
                      <div className='flex size-6 items-center justify-center rounded-full border border-rose-500/30 bg-rose-500/10 text-rose-600 dark:text-rose-400'>
                        <span className='text-xs font-bold'>✕</span>
                      </div>
                    ) : (
                      <span
                        className={cn(
                          row.competitorHighlighted &&
                            'flex items-center gap-2 font-bold text-emerald-600 dark:text-emerald-400'
                        )}
                      >
                        {row.competitorHighlighted && (
                          <div className='flex size-5 shrink-0 items-center justify-center rounded-full bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'>
                            <Check className='h-3.5 w-3.5' />
                          </div>
                        )}
                        {row.competitor}
                      </span>
                    )}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Summary Box Section */}
      <div className='dark:border-zinc-850 space-y-6 rounded-[24px] border border-black/5 bg-zinc-50/30 p-8 dark:bg-zinc-950/20'>
        <div className='space-y-1.5'>
          <h3 className='text-foreground text-xl font-extrabold'>
            {t('Summary')}
          </h3>
          <p className='text-sm font-semibold text-zinc-600 dark:text-zinc-400'>
            {formatSystemTemplate(
              t('For the same $20/month, with Modelsell you can:'),
              systemName
            )}
          </p>
        </div>
        <ul className='space-y-4 pl-1'>
          {summaryPoints.map((point, index) => {
            const icons = [
              <Cpu key={1} className='h-4.5 w-4.5 text-zinc-500' />,
              <Sparkles key={2} className='h-4.5 w-4.5 text-zinc-500' />,
              <KeyRound key={3} className='h-4.5 w-4.5 text-zinc-500' />,
            ]
            return (
              <li
                key={index}
                className='flex items-start gap-3.5 text-sm leading-relaxed font-medium text-zinc-700 dark:text-zinc-300'
              >
                <div className='flex size-8 shrink-0 items-center justify-center rounded-xl border border-black/5 bg-zinc-100 text-zinc-600 dark:border-zinc-800 dark:bg-zinc-900 dark:text-zinc-400'>
                  {icons[index]}
                </div>
                <span className='pt-1.5'>{point}</span>
              </li>
            )
          })}
        </ul>
      </div>
    </section>
  )
}

export function SubscriptionStore() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { systemName, mainlandChinaPresentationEnabled } = useSystemConfig()
  const user = useAuthStore((state) => state.auth.user)
  const { plans, isLoading } = useStorePlans()
  const [payingPlanId, setPayingPlanId] = useState<number | null>(null)

  const handleChoosePlan = async (planId: number) => {
    if (!user) {
      navigate({ to: '/sign-in', search: { redirect: '/subscription' } })
      return
    }
    setPayingPlanId(planId)
    try {
      const res = await paySubscriptionStripe({ plan_id: planId })
      const paymentSucceeded = isSubscriptionPaymentSuccess(res)
      const checkoutUrl = getSubscriptionCheckoutUrl(res)
      if (paymentSucceeded && checkoutUrl) {
        window.location.href = checkoutUrl
        return
      }
      toast.error(res.message || t('Payment request failed'))
    } finally {
      setPayingPlanId(null)
    }
  }

  return (
    <PublicLayout showMainContainer={false}>
      <PageTransition className='text-foreground relative min-h-screen overflow-hidden bg-white transition-colors duration-300 dark:bg-[#09090b]'>
        {/* Soft, gorgeous warm orange gradient background block */}
        <div
          aria-hidden
          className='pointer-events-none absolute inset-x-0 top-0 h-[680px] bg-gradient-to-b from-orange-500/20 via-orange-500/5 to-transparent dark:from-orange-950/20 dark:via-orange-950/5'
        />
        {/* Glowing aura effect */}
        <div
          aria-hidden
          className='pointer-events-none absolute top-[100px] left-1/2 size-[480px] -translate-x-1/2 rounded-full bg-orange-400/20 blur-[100px] dark:bg-orange-900/10'
        />

        <main className='relative mx-auto w-full max-w-6xl space-y-20 px-4 pt-24 pb-20 sm:px-6 lg:px-8'>
          {/* Hero Section */}
          <section className='relative z-10 flex flex-col items-center justify-center space-y-8 pt-12 text-center'>
            <div className='max-w-4xl space-y-4'>
              <h1 className='text-foreground from-foreground to-foreground/80 bg-gradient-to-b bg-clip-text text-4xl font-extrabold tracking-tight sm:text-6xl'>
                {getSitePlanName(systemName)}
              </h1>

              <p className='mx-auto max-w-2xl text-sm leading-relaxed font-semibold text-zinc-600 dark:text-zinc-400'>
                {t(
                  "Fixed monthly fee, unleash infinite creativity — the 'All-Star' era of Vibe Coding."
                )}
              </p>
            </div>

            {/* Quick First-Fold Features Showcase (Premium, styled modern border cards) */}
            <div className='grid w-full max-w-5xl gap-6 pt-6 text-left sm:grid-cols-3'>
              <div className='flex min-h-[200px] flex-col justify-between rounded-3xl border border-black/5 bg-white/60 p-8 shadow-sm backdrop-blur-md transition-all duration-300 hover:border-orange-500/30 hover:shadow-md dark:border-zinc-800 dark:bg-zinc-950/60'>
                <div className='space-y-4'>
                  <div className='flex size-10 items-center justify-center rounded-2xl bg-orange-500/10 text-orange-600 dark:bg-orange-950/30 dark:text-orange-400'>
                    <Sparkles className='h-5 w-5' />
                  </div>
                  <div className='space-y-1.5'>
                    <h3 className='text-lg font-bold text-zinc-900 dark:text-zinc-100'>
                      {t(
                        mainlandChinaPresentationEnabled
                          ? 'Domestic Model Matrix'
                          : 'All-Star Model Matrix'
                      )}
                    </h3>
                    <p className='text-xs font-bold tracking-wider text-[#0071e3] uppercase'>
                      {t(
                        mainlandChinaPresentationEnabled
                          ? 'Leading domestic models'
                          : '100+ Top Models'
                      )}
                    </p>
                  </div>
                  <p className='text-muted-foreground text-sm leading-relaxed'>
                    {t(
                      mainlandChinaPresentationEnabled
                        ? 'One key connects DeepSeek, Qwen, GLM, Kimi, MiniMax and other leading domestic models.'
                        : 'One Key, all models. GPT-5.2, Gemini 3 Pro, etc. — latest models available instantly.'
                    )}
                  </p>
                </div>
              </div>

              <div className='flex min-h-[200px] flex-col justify-between rounded-3xl border border-black/5 bg-white/60 p-8 shadow-sm backdrop-blur-md transition-all duration-300 hover:border-orange-500/30 hover:shadow-md dark:border-zinc-800 dark:bg-zinc-950/60'>
                <div className='space-y-4'>
                  <div className='flex size-10 items-center justify-center rounded-2xl bg-orange-500/10 text-orange-600 dark:bg-orange-950/30 dark:text-orange-400'>
                    <Layers className='h-5 w-5' />
                  </div>
                  <div className='space-y-1.5'>
                    <h3 className='text-lg font-bold text-zinc-900 dark:text-zinc-100'>
                      {t('Multi-scenario Support')}
                    </h3>
                    <p className='text-xs font-bold tracking-wider text-[#0071e3] uppercase'>
                      {t('Coding + Image + Video + Chat')}
                    </p>
                  </div>
                  <p className='text-muted-foreground text-sm leading-relaxed'>
                    {t(
                      mainlandChinaPresentationEnabled
                        ? 'One subscription covers domestic coding, image, video, and chat models for multiple workloads.'
                        : 'One subscription for all needs. Claude Code coding, NanoBanana image generation, GPT-5.2 chat, etc.'
                    )}
                  </p>
                </div>
              </div>

              <div className='flex min-h-[200px] flex-col justify-between rounded-3xl border border-black/5 bg-white/60 p-8 shadow-sm backdrop-blur-md transition-all duration-300 hover:border-orange-500/30 hover:shadow-md dark:border-zinc-800 dark:bg-zinc-950/60'>
                <div className='space-y-4'>
                  <div className='flex size-10 items-center justify-center rounded-2xl bg-orange-500/10 text-orange-600 dark:bg-orange-950/30 dark:text-orange-400'>
                    <KeyRound className='h-5 w-5' />
                  </div>
                  <div className='space-y-1.5'>
                    <h3 className='text-lg font-bold text-zinc-900 dark:text-zinc-100'>
                      {t('Agent Seamless Integration')}
                    </h3>
                    <p className='text-xs font-bold tracking-wider text-[#0071e3] uppercase'>
                      {t(
                        mainlandChinaPresentationEnabled
                          ? 'Compatible chat, response, image, and video APIs'
                          : 'Native support for OpenAI, Anthropic, and Google protocols'
                      )}
                    </p>
                  </div>
                  <p className='text-muted-foreground text-sm leading-relaxed'>
                    {t(
                      mainlandChinaPresentationEnabled
                        ? 'Connect supported applications to domestic models with one API key and a unified endpoint.'
                        : 'Supports Claude Code, CodeX, Open Code, OpenClaw, Cline, VS Code Copilot, etc. — one key for all tools.'
                    )}
                  </p>
                </div>
              </div>
            </div>
          </section>

          {/* Pricing Cards Grid */}
          <section id='subscription-plans' className='scroll-mt-24 py-4'>
            <div className='mb-12 flex flex-col gap-3.5 border-b border-black/5 pb-8 text-left dark:border-zinc-800'>
              <div className='space-y-1.5'>
                <p className='text-xs font-bold tracking-widest text-[#0071e3] uppercase'>
                  Token Plan
                </p>
                <h2 className='text-foreground text-3xl font-bold tracking-tight'>
                  {t('Choose Your Plan')}
                </h2>
              </div>
              <p className='text-muted-foreground max-w-2xl text-sm leading-relaxed'>
                {t(
                  'Select a seat package that best fits your development frequency. Checkout triggers after signing in.'
                )}
              </p>
            </div>

            {isLoading ? (
              <PlanSkeleton />
            ) : plans.length > 0 ? (
              <div className='grid gap-8 md:grid-cols-2 lg:grid-cols-3'>
                {plans.map((item) => (
                  <PlanCard
                    key={item.plan.id}
                    item={item}
                    onChoose={handleChoosePlan}
                    paying={payingPlanId === item.plan.id}
                  />
                ))}
              </div>
            ) : (
              <EmptyPlans />
            )}
          </section>

          {/* Guidelines Permitted & Prohibited Comparison */}
          <GuidelinesComparisonGrid />

          {/* One-click mainstream Agent platforms configuration showcase */}
          <AgentAccessSection />

          {/* Tokens plan vs competitors comparison table */}
          <ComparisonTableSection />

          {/* FAQ Accordion */}
          <FAQSection />

          {/* Bottom Strip */}
          <div className='pt-8'>
            <TokenPlanStrip />
          </div>
        </main>
      </PageTransition>
    </PublicLayout>
  )
}
