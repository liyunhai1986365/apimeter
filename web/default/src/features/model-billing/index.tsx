import { useCallback, useEffect, useMemo, useState } from 'react'
import { DatabaseZap, RefreshCw, ReceiptText } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import dayjs from '@/lib/dayjs'
import { formatNumber, formatQuota, formatTokens } from '@/lib/format'
import { ROLE } from '@/lib/roles'
import { cn } from '@/lib/utils'
import { backfillModelBilling, getModelBillingSummary } from './api'
import type { ModelBillingPeriod, ModelBillingSummaryRow } from './types'

const ALL_SOURCES = 'all'

function toTimestampStart(date: string) {
  if (!date) return undefined
  return dayjs(date).startOf('day').unix()
}

function toTimestampEnd(date: string) {
  if (!date) return undefined
  return dayjs(date).endOf('day').unix()
}

function compactFilter(value: string) {
  const trimmed = value.trim()
  return trimmed === '' ? undefined : trimmed
}

function billingSourceLabel(source: string, t: (key: string) => string) {
  if (source === 'wallet') return t('Wallet')
  if (source === 'subscription') return t('Subscription')
  return t('All sources')
}

function MetricCard({
  label,
  value,
  tone,
}: {
  label: string
  value: string
  tone?: 'positive' | 'negative'
}) {
  return (
    <Card size='sm' className='rounded-lg'>
      <CardHeader className='pb-1'>
        <CardTitle className='text-muted-foreground text-xs font-normal'>
          {label}
        </CardTitle>
      </CardHeader>
      <CardContent
        className={cn(
          'text-xl font-semibold tabular-nums',
          tone === 'positive' && 'text-emerald-600 dark:text-emerald-400',
          tone === 'negative' && 'text-destructive'
        )}
      >
        {value}
      </CardContent>
    </Card>
  )
}

export function ModelBilling() {
  const { t } = useTranslation()
  const [period, setPeriod] = useState<ModelBillingPeriod>('day')
  const [startDate, setStartDate] = useState(() =>
    dayjs().subtract(29, 'day').format('YYYY-MM-DD')
  )
  const [endDate, setEndDate] = useState(() => dayjs().format('YYYY-MM-DD'))
  const [modelName, setModelName] = useState('')
  const [group, setGroup] = useState('')
  const [billingSource, setBillingSource] = useState(ALL_SOURCES)
  const [rows, setRows] = useState<ModelBillingSummaryRow[]>([])
  const [loading, setLoading] = useState(false)
  const [backfilling, setBackfilling] = useState(false)
  const userRole = useAuthStore((state) => state.auth.user?.role)
  const isAdmin = Boolean(userRole && userRole >= ROLE.ADMIN)

  const fetchRows = useCallback(async () => {
    setLoading(true)
    try {
      const response = await getModelBillingSummary({
        period,
        start_timestamp: toTimestampStart(startDate),
        end_timestamp: toTimestampEnd(endDate),
        model_name: compactFilter(modelName),
        group: compactFilter(group),
        billing_source:
          billingSource === ALL_SOURCES ? undefined : billingSource,
      })
      if (response.success) {
        setRows(response.data ?? [])
      }
    } finally {
      setLoading(false)
    }
  }, [billingSource, endDate, group, modelName, period, startDate])

  useEffect(() => {
    void fetchRows()
  }, [fetchRows])

  const handleBackfill = useCallback(async () => {
    setBackfilling(true)
    try {
      const response = await backfillModelBilling({
        start_timestamp: toTimestampStart(startDate),
        end_timestamp: toTimestampEnd(endDate),
      })
      if (response.success) {
        const data = response.data
        toast.success(
          t(
            'Backfill completed: scanned {{scanned}}, created {{created}}, skipped {{skipped}}, failed {{failed}}',
            {
              scanned: data.scanned,
              created: data.created,
              skipped: data.skipped,
              failed: data.failed,
            }
          )
        )
        await fetchRows()
      }
    } finally {
      setBackfilling(false)
    }
  }, [endDate, fetchRows, startDate, t])

  const totals = useMemo(
    () =>
      rows.reduce(
        (acc, row) => {
          acc.requests += row.request_count
          acc.input += row.input_tokens
          acc.output += row.output_tokens
          acc.cacheWrite += row.cache_write_tokens
          acc.cacheRead += row.cache_read_tokens
          acc.original += row.original_quota
          acc.discount += row.discount_quota
          acc.payable += row.payable_quota
          return acc
        },
        {
          requests: 0,
          input: 0,
          output: 0,
          cacheWrite: 0,
          cacheRead: 0,
          original: 0,
          discount: 0,
          payable: 0,
        }
      ),
    [rows]
  )

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Model Billing')}</SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t('Review model billing grouped by day or month')}
      </SectionPageLayout.Description>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <div className='flex flex-col gap-3 rounded-lg border p-3 md:flex-row md:flex-wrap md:items-end'>
            <div className='space-y-1'>
              <div className='text-muted-foreground text-xs'>{t('Period')}</div>
              <Tabs
                value={period}
                onValueChange={(value) =>
                  setPeriod(value as ModelBillingPeriod)
                }
              >
                <TabsList>
                  <TabsTrigger value='day'>{t('Day')}</TabsTrigger>
                  <TabsTrigger value='month'>{t('Month')}</TabsTrigger>
                </TabsList>
              </Tabs>
            </div>
            <label className='min-w-36 flex-1 space-y-1 md:max-w-44'>
              <span className='text-muted-foreground text-xs'>
                {t('Start date')}
              </span>
              <Input
                type='date'
                value={startDate}
                onChange={(event) => setStartDate(event.target.value)}
              />
            </label>
            <label className='min-w-36 flex-1 space-y-1 md:max-w-44'>
              <span className='text-muted-foreground text-xs'>
                {t('End date')}
              </span>
              <Input
                type='date'
                value={endDate}
                onChange={(event) => setEndDate(event.target.value)}
              />
            </label>
            <label className='min-w-40 flex-1 space-y-1'>
              <span className='text-muted-foreground text-xs'>
                {t('Model')}
              </span>
              <Input
                value={modelName}
                onChange={(event) => setModelName(event.target.value)}
                placeholder={t('All models')}
              />
            </label>
            <label className='min-w-32 flex-1 space-y-1 md:max-w-40'>
              <span className='text-muted-foreground text-xs'>
                {t('Group')}
              </span>
              <Input
                value={group}
                onChange={(event) => setGroup(event.target.value)}
                placeholder={t('All groups')}
              />
            </label>
            <label className='space-y-1'>
              <span className='text-muted-foreground text-xs'>
                {t('Billing source')}
              </span>
              <Select
                value={billingSource}
                onValueChange={(value) =>
                  setBillingSource(value ?? ALL_SOURCES)
                }
              >
                <SelectTrigger className='w-36'>
                  <SelectValue>{billingSourceLabel(billingSource, t)}</SelectValue>
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ALL_SOURCES}>{t('All sources')}</SelectItem>
                  <SelectItem value='wallet'>{t('Wallet')}</SelectItem>
                  <SelectItem value='subscription'>{t('Subscription')}</SelectItem>
                </SelectContent>
              </Select>
            </label>
            <Button
              type='button'
              variant='outline'
              onClick={() => void fetchRows()}
              disabled={loading}
              className={cn(!isAdmin && 'md:ml-auto')}
            >
              <RefreshCw
                className={cn('size-4', loading && 'animate-spin')}
                aria-hidden='true'
              />
              {t('Refresh')}
            </Button>
            {isAdmin && (
              <Button
                type='button'
                variant='outline'
                onClick={() => void handleBackfill()}
                disabled={backfilling}
                className='md:ml-auto'
              >
                <DatabaseZap
                  className={cn('size-4', backfilling && 'animate-pulse')}
                  aria-hidden='true'
                />
                {t('Backfill history')}
              </Button>
            )}
          </div>

          <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
            <MetricCard
              label={t('Payable amount')}
              value={formatQuota(totals.payable)}
            />
            <MetricCard
              label={t('Original amount')}
              value={formatQuota(totals.original)}
            />
            <MetricCard
              label={t('Group discount')}
              value={formatQuota(totals.discount)}
              tone={totals.discount >= 0 ? 'positive' : 'negative'}
            />
            <MetricCard
              label={t('Requests')}
              value={formatNumber(totals.requests)}
            />
          </div>

          <div className='rounded-lg border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Period')}</TableHead>
                  <TableHead>{t('Model')}</TableHead>
                  <TableHead className='text-right'>{t('Requests')}</TableHead>
                  <TableHead className='text-right'>
                    {t('Input tokens')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('Output tokens')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('Cache write')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('Cache read')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('Original amount')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('Group discount')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('Payable amount')}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((row) => (
                  <TableRow key={`${row.period}-${row.model_name}`}>
                    <TableCell className='font-medium'>{row.period}</TableCell>
                    <TableCell>
                      <Badge variant='outline'>{row.model_name}</Badge>
                    </TableCell>
                    <TableCell className='text-right tabular-nums'>
                      {formatNumber(row.request_count)}
                    </TableCell>
                    <TableCell className='text-right tabular-nums'>
                      {formatTokens(row.input_tokens)}
                    </TableCell>
                    <TableCell className='text-right tabular-nums'>
                      {formatTokens(row.output_tokens)}
                    </TableCell>
                    <TableCell className='text-right tabular-nums'>
                      {formatTokens(row.cache_write_tokens)}
                    </TableCell>
                    <TableCell className='text-right tabular-nums'>
                      {formatTokens(row.cache_read_tokens)}
                    </TableCell>
                    <TableCell className='text-right tabular-nums'>
                      {formatQuota(row.original_quota)}
                    </TableCell>
                    <TableCell
                      className={cn(
                        'text-right tabular-nums',
                        row.discount_quota >= 0
                          ? 'text-emerald-600 dark:text-emerald-400'
                          : 'text-destructive'
                      )}
                    >
                      {formatQuota(row.discount_quota)}
                    </TableCell>
                    <TableCell className='text-right font-medium tabular-nums'>
                      {formatQuota(row.payable_quota)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            {!loading && rows.length === 0 && (
              <Empty className='border-0 py-12'>
                <EmptyHeader>
                  <EmptyMedia variant='icon'>
                    <ReceiptText className='size-4' aria-hidden='true' />
                  </EmptyMedia>
                  <EmptyTitle>{t('No billing data found')}</EmptyTitle>
                  <EmptyDescription>
                    {t('Try changing the date range or filters')}
                  </EmptyDescription>
                </EmptyHeader>
              </Empty>
            )}
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
