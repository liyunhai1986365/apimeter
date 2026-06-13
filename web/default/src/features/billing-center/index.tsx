import { useCallback, useEffect, useMemo, useState } from 'react'
import { getRouteApi, useNavigate } from '@tanstack/react-router'
import {
  Activity,
  AlertTriangle,
  ArrowDownCircle,
  ArrowUpCircle,
  BadgePercent,
  DatabaseZap,
  Gauge,
  Layers3,
  ReceiptText,
  RefreshCw,
  Rows3,
  Scale,
  WalletCards,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import dayjs from '@/lib/dayjs'
import {
  formatNumber,
  formatQuota,
  formatTimestampToDate,
  formatTokens,
} from '@/lib/format'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
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
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { SectionPageLayout } from '@/components/layout'
import {
  generateBillingMonthlyStatement,
  getAccountLedgerEntries,
  getBillingCurrentPeriod,
  getBillingMonthlyStatementSummaries,
  getBillingMonthlyStatements,
  getDailyBillingReconciliations,
} from './api'
import type {
  AccountLedgerEntry,
  BillingCenterSectionId,
  BillingCurrentPeriod,
  BillingStatement,
  BillingStatementSummary,
  DailyBillingReconciliation,
} from './types'

const route = getRouteApi('/_authenticated/billing/$section')
const ALL_ENTRY_TYPES = 'all'

function startOfDate(date: string) {
  return date ? dayjs(date).startOf('day').unix() : undefined
}

function endOfDate(date: string) {
  return date ? dayjs(date).endOf('day').unix() : undefined
}

function sourceLabel(value: string, t: (key: string) => string) {
  if (value === 'wallet') return t('Wallet')
  if (value === 'subscription') return t('Subscription')
  return value || '-'
}

function statusLabel(value: string, t: (key: string) => string) {
  if (value === 'confirmed') return t('Confirmed')
  if (value === 'exception') return t('Exception')
  if (value === 'open') return t('Open')
  if (value === 'estimated') return t('Estimated')
  return value || '-'
}

function entryTypeLabel(value: string, t: (key: string) => string) {
  if (value === 'topup') return t('Top-up')
  if (value === 'consume') return t('Usage')
  if (value === 'refund') return t('Refund')
  if (value === 'adjustment') return t('Adjustment')
  return value || '-'
}

function dimensionLabel(value: string, t: (key: string) => string) {
  if (value === 'model') return t('Model')
  if (value === 'group') return t('Group')
  if (value === 'key') return t('Key')
  if (value === 'source') return t('Billing source')
  return value || '-'
}

function signedQuotaClass(value: number) {
  if (value > 0) return 'text-emerald-600 dark:text-emerald-400'
  if (value < 0) return 'text-destructive'
  return 'text-muted-foreground'
}

function discountRate(original: number, discount: number) {
  if (original <= 0) return '-'
  return `${((discount / original) * 100).toFixed(2)}%`
}

function MetricCard({
  title,
  value,
  icon: Icon,
  tone,
}: {
  title: string
  value: string
  icon: React.ComponentType<{ className?: string; 'aria-hidden'?: boolean }>
  tone?: 'positive' | 'negative' | 'accent'
}) {
  return (
    <Card size='sm' className='rounded-lg'>
      <CardHeader className='flex flex-row items-center justify-between gap-3 pb-2'>
        <CardTitle className='text-muted-foreground text-xs font-normal'>
          {title}
        </CardTitle>
        <Icon
          className={cn(
            'size-4',
            tone === 'positive' && 'text-emerald-600 dark:text-emerald-400',
            tone === 'negative' && 'text-destructive',
            tone === 'accent' && 'text-primary'
          )}
          aria-hidden
        />
      </CardHeader>
      <CardContent className='text-xl font-semibold tabular-nums'>
        {value}
      </CardContent>
    </Card>
  )
}

function FilterShell({ children }: { children: React.ReactNode }) {
  return (
    <div className='flex flex-col gap-3 rounded-lg border bg-muted/20 p-3 md:flex-row md:flex-wrap md:items-end'>
      {children}
    </div>
  )
}

function EmptyBillingState({
  title,
  description,
}: {
  title: string
  description?: string
}) {
  return (
    <Empty className='border-0 py-12'>
      <EmptyHeader>
        <EmptyMedia variant='icon'>
          <ReceiptText className='size-4' aria-hidden='true' />
        </EmptyMedia>
        <EmptyTitle>{title}</EmptyTitle>
        {description && <EmptyDescription>{description}</EmptyDescription>}
      </EmptyHeader>
    </Empty>
  )
}

function StatementBadge({ status }: { status: string }) {
  const { t } = useTranslation()
  return (
    <Badge variant={status === 'exception' ? 'destructive' : 'secondary'}>
      {statusLabel(status, t)}
    </Badge>
  )
}

function CurrentPeriodTab() {
  const { t } = useTranslation()
  const [current, setCurrent] = useState<BillingCurrentPeriod | null>(null)
  const [loading, setLoading] = useState(false)

  const fetchCurrent = useCallback(async () => {
    setLoading(true)
    try {
      const summaryResponse = await getBillingCurrentPeriod()
      if (summaryResponse.success) setCurrent(summaryResponse.data)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void fetchCurrent()
  }, [fetchCurrent])

  const summary = current?.summary

  return (
    <div className='space-y-4'>
      <FilterShell>
        <div className='min-w-52 flex-1'>
          <div className='text-sm font-medium'>{t('Current period')}</div>
          <div className='text-muted-foreground text-xs'>
            {current?.month ?? dayjs().format('YYYY-MM')} ·{' '}
            {statusLabel(current?.status ?? 'estimated', t)}
          </div>
        </div>
        <Button
          type='button'
          variant='outline'
          onClick={() => void fetchCurrent()}
          disabled={loading}
          className='md:ml-auto'
        >
          <RefreshCw
            className={cn('size-4', loading && 'animate-spin')}
            aria-hidden='true'
          />
          {t('Refresh')}
        </Button>
      </FilterShell>

      <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        <MetricCard
          title={t('Settlement amount')}
          value={formatQuota(summary?.settlement_amount ?? 0)}
          icon={Scale}
          tone='accent'
        />
        <MetricCard
          title={t('Original amount')}
          value={formatQuota(summary?.original_amount ?? 0)}
          icon={ReceiptText}
        />
        <MetricCard
          title={t('Group discount')}
          value={formatQuota(summary?.discount_amount ?? 0)}
          icon={BadgePercent}
          tone={(summary?.discount_amount ?? 0) >= 0 ? 'positive' : 'negative'}
        />
        <MetricCard
          title={t('Requests')}
          value={formatNumber(summary?.request_count ?? 0)}
          icon={Activity}
        />
      </div>

      <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        <MetricCard
          title={t('Input tokens')}
          value={formatTokens(summary?.input_tokens ?? 0)}
          icon={ArrowDownCircle}
        />
        <MetricCard
          title={t('Output tokens')}
          value={formatTokens(summary?.output_tokens ?? 0)}
          icon={ArrowUpCircle}
        />
        <MetricCard
          title={t('Cache read')}
          value={formatTokens(summary?.cache_read_tokens ?? 0)}
          icon={Rows3}
        />
        <MetricCard
          title={t('Cache write')}
          value={formatTokens(summary?.cache_write_tokens ?? 0)}
          icon={Layers3}
        />
      </div>
    </div>
  )
}

function MonthlyStatementsTab() {
  const { t } = useTranslation()
  const [month, setMonth] = useState(() => dayjs().format('YYYY-MM'))
  const [statements, setStatements] = useState<BillingStatement[]>([])
  const [selected, setSelected] = useState<BillingStatement | null>(null)
  const [summaries, setSummaries] = useState<BillingStatementSummary[]>([])
  const [loading, setLoading] = useState(false)
  const [generating, setGenerating] = useState(false)

  const fetchStatements = useCallback(async () => {
    setLoading(true)
    try {
      const response = await getBillingMonthlyStatements({
        start_month: month,
        end_month: month,
        limit: 12,
      })
      if (response.success) {
        const rows = response.data ?? []
        setStatements(rows)
        setSelected((current) => {
          if (current) {
            return (
              rows.find((row) => row.statement_no === current.statement_no) ??
              rows[0] ??
              null
            )
          }
          return rows[0] ?? null
        })
      }
    } finally {
      setLoading(false)
    }
  }, [month])

  useEffect(() => {
    void fetchStatements()
  }, [fetchStatements])

  useEffect(() => {
    if (!selected) {
      setSummaries([])
      return
    }
    getBillingMonthlyStatementSummaries(selected.statement_no).then(
      (response) => {
        if (response.success) setSummaries(response.data ?? [])
      }
    )
  }, [selected])

  const handleGenerate = useCallback(async () => {
    setGenerating(true)
    try {
      const response = await generateBillingMonthlyStatement(month)
      if (response.success) {
        toast.success(t('Monthly statement generated'))
        setSelected(response.data.statement)
        setSummaries(response.data.summaries ?? [])
        await fetchStatements()
      }
    } finally {
      setGenerating(false)
    }
  }, [fetchStatements, month, t])

  const selectedSummaries = useMemo(
    () =>
      summaries.reduce(
        (acc, row) => {
          if (!acc[row.dimension]) acc[row.dimension] = []
          acc[row.dimension].push(row)
          return acc
        },
        {} as Record<string, BillingStatementSummary[]>
      ),
    [summaries]
  )

  return (
    <div className='space-y-4'>
      <FilterShell>
        <label className='min-w-44 space-y-1'>
          <span className='text-muted-foreground text-xs'>{t('Month')}</span>
          <Input
            type='month'
            value={month}
            onChange={(event) => setMonth(event.target.value)}
          />
        </label>
        <Button
          type='button'
          variant='outline'
          onClick={() => void fetchStatements()}
          disabled={loading}
          className='md:ml-auto'
        >
          <RefreshCw
            className={cn('size-4', loading && 'animate-spin')}
            aria-hidden='true'
          />
          {t('Refresh')}
        </Button>
        <Button
          type='button'
          variant='outline'
          onClick={() => void handleGenerate()}
          disabled={generating}
        >
          <DatabaseZap
            className={cn('size-4', generating && 'animate-pulse')}
            aria-hidden='true'
          />
          {t('Generate monthly statement')}
        </Button>
      </FilterShell>

      <div className='grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1.5fr)]'>
        <div className='overflow-hidden rounded-lg border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Statement')}</TableHead>
                <TableHead className='text-right'>
                  {t('Settlement amount')}
                </TableHead>
                <TableHead className='text-right'>{t('Difference')}</TableHead>
                <TableHead>{t('Status')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {statements.map((statement) => (
                <TableRow
                  key={statement.statement_no}
                  className='cursor-pointer'
                  data-state={
                    selected?.statement_no === statement.statement_no
                      ? 'selected'
                      : undefined
                  }
                  onClick={() => setSelected(statement)}
                >
                  <TableCell>
                    <div className='font-medium'>{statement.period_value}</div>
                    <div className='text-muted-foreground text-xs'>
                      {statement.statement_no}
                    </div>
                  </TableCell>
                  <TableCell className='text-right tabular-nums'>
                    {formatQuota(statement.settlement_amount)}
                  </TableCell>
                  <TableCell
                    className={cn(
                      'text-right tabular-nums',
                      signedQuotaClass(statement.difference_amount)
                    )}
                  >
                    {formatQuota(statement.difference_amount)}
                  </TableCell>
                  <TableCell>
                    <StatementBadge status={statement.status} />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          {!loading && statements.length === 0 && (
            <EmptyBillingState
              title={t('No monthly statements found')}
              description={t('Generate a monthly statement to freeze totals')}
            />
          )}
        </div>

        <div className='space-y-3'>
          {selected && (
            <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
              <MetricCard
                title={t('Settlement amount')}
                value={formatQuota(selected.settlement_amount)}
                icon={Scale}
                tone='accent'
              />
              <MetricCard
                title={t('Usage')}
                value={formatQuota(selected.consume_amount)}
                icon={Gauge}
              />
              <MetricCard
                title={t('Requests')}
                value={formatNumber(selected.request_count)}
                icon={Activity}
              />
              <MetricCard
                title={t('Difference')}
                value={formatQuota(selected.difference_amount)}
                icon={AlertTriangle}
                tone={selected.difference_amount === 0 ? 'positive' : 'negative'}
              />
            </div>
          )}

          <StatementSummaryTable
            rows={selectedSummaries.model ?? []}
            title={t('Model statement summary')}
            loading={loading}
          />
          <StatementSummaryTable
            rows={selectedSummaries.group ?? []}
            title={t('Group pricing summary')}
            loading={loading}
          />
          <StatementSummaryTable
            rows={selectedSummaries.source ?? []}
            title={t('Billing source summary')}
            loading={loading}
          />
        </div>
      </div>
    </div>
  )
}

function StatementSummaryTable({
  rows,
  title,
  loading,
}: {
  rows: BillingStatementSummary[]
  title: string
  loading?: boolean
}) {
  const { t } = useTranslation()

  return (
    <div className='overflow-hidden rounded-lg border'>
      <div className='flex items-center justify-between gap-3 border-b px-3 py-2'>
        <h3 className='text-sm font-medium'>{title}</h3>
        {rows[0] && (
          <Badge variant='outline'>{dimensionLabel(rows[0].dimension, t)}</Badge>
        )}
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Dimension')}</TableHead>
            <TableHead className='text-right'>{t('Requests')}</TableHead>
            <TableHead className='text-right'>{t('Input tokens')}</TableHead>
            <TableHead className='text-right'>{t('Output tokens')}</TableHead>
            <TableHead className='text-right'>{t('Discount rate')}</TableHead>
            <TableHead className='text-right'>
              {t('Settlement amount')}
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((row) => (
            <TableRow key={`${row.dimension}-${row.dimension_value}`}>
              <TableCell>
                <Badge variant='outline'>{row.dimension_value || '-'}</Badge>
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
                {discountRate(row.original_amount, row.discount_amount)}
              </TableCell>
              <TableCell className='text-right font-medium tabular-nums'>
                {formatQuota(row.settlement_amount)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {!loading && rows.length === 0 && (
        <EmptyBillingState title={t('No statement summary found')} />
      )}
    </div>
  )
}

function DailyReconciliationTab() {
  const { t } = useTranslation()
  const [startMonth, setStartMonth] = useState(() =>
    dayjs().subtract(1, 'month').format('YYYY-MM')
  )
  const [endMonth, setEndMonth] = useState(() => dayjs().format('YYYY-MM'))
  const [rows, setRows] = useState<DailyBillingReconciliation[]>([])
  const [loading, setLoading] = useState(false)

  const fetchRows = useCallback(async () => {
    setLoading(true)
    try {
      const response = await getDailyBillingReconciliations({
        start_month: startMonth,
        end_month: endMonth,
        limit: 90,
      })
      if (response.success) setRows(response.data ?? [])
    } finally {
      setLoading(false)
    }
  }, [endMonth, startMonth])

  useEffect(() => {
    void fetchRows()
  }, [fetchRows])

  const totals = useMemo(
    () =>
      rows.reduce(
        (acc, row) => {
          acc.usage += row.usage_settlement_amount
          acc.ledger += row.account_consume_amount
          acc.diff += Math.abs(row.difference_amount)
          acc.exceptions += row.status === 'exception' ? 1 : 0
          return acc
        },
        { usage: 0, ledger: 0, diff: 0, exceptions: 0 }
      ),
    [rows]
  )

  return (
    <div className='space-y-4'>
      <FilterShell>
        <label className='min-w-44 space-y-1'>
          <span className='text-muted-foreground text-xs'>
            {t('Start month')}
          </span>
          <Input
            type='month'
            value={startMonth}
            onChange={(event) => setStartMonth(event.target.value)}
          />
        </label>
        <label className='min-w-44 space-y-1'>
          <span className='text-muted-foreground text-xs'>{t('End month')}</span>
          <Input
            type='month'
            value={endMonth}
            onChange={(event) => setEndMonth(event.target.value)}
          />
        </label>
        <Button
          type='button'
          variant='outline'
          onClick={() => void fetchRows()}
          disabled={loading}
          className='md:ml-auto'
        >
          <RefreshCw
            className={cn('size-4', loading && 'animate-spin')}
            aria-hidden='true'
          />
          {t('Refresh')}
        </Button>
      </FilterShell>

      <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        <MetricCard
          title={t('Usage facts total')}
          value={formatQuota(totals.usage)}
          icon={ReceiptText}
        />
        <MetricCard
          title={t('Ledger consume total')}
          value={formatQuota(totals.ledger)}
          icon={WalletCards}
        />
        <MetricCard
          title={t('Differences')}
          value={formatQuota(totals.diff)}
          icon={AlertTriangle}
          tone={totals.diff === 0 ? 'positive' : 'negative'}
        />
        <MetricCard
          title={t('Exception days')}
          value={formatNumber(totals.exceptions)}
          icon={Gauge}
          tone={totals.exceptions === 0 ? 'positive' : 'negative'}
        />
      </div>

      <div className='overflow-hidden rounded-lg border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Date')}</TableHead>
              <TableHead className='text-right'>
                {t('Usage facts total')}
              </TableHead>
              <TableHead className='text-right'>
                {t('Ledger consume total')}
              </TableHead>
              <TableHead className='text-right'>{t('Difference')}</TableHead>
              <TableHead className='text-right'>{t('Requests')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row) => (
              <TableRow key={row.date}>
                <TableCell className='font-medium'>{row.date}</TableCell>
                <TableCell className='text-right tabular-nums'>
                  {formatQuota(row.usage_settlement_amount)}
                </TableCell>
                <TableCell className='text-right tabular-nums'>
                  {formatQuota(row.account_consume_amount)}
                </TableCell>
                <TableCell
                  className={cn(
                    'text-right tabular-nums',
                    signedQuotaClass(row.difference_amount)
                  )}
                >
                  {formatQuota(row.difference_amount)}
                </TableCell>
                <TableCell className='text-right tabular-nums'>
                  {formatNumber(row.request_count)}
                </TableCell>
                <TableCell>
                  <StatementBadge status={row.status} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        {!loading && rows.length === 0 && (
          <EmptyBillingState
            title={t('No reconciliation data found')}
            description={t('Try changing the date range or filters')}
          />
        )}
      </div>
    </div>
  )
}

function AccountLedgerTab() {
  const { t } = useTranslation()
  const [startDate, setStartDate] = useState(() =>
    dayjs().subtract(29, 'day').format('YYYY-MM-DD')
  )
  const [endDate, setEndDate] = useState(() => dayjs().format('YYYY-MM-DD'))
  const [entryType, setEntryType] = useState(ALL_ENTRY_TYPES)
  const [rows, setRows] = useState<AccountLedgerEntry[]>([])
  const [loading, setLoading] = useState(false)

  const fetchRows = useCallback(async () => {
    setLoading(true)
    try {
      const response = await getAccountLedgerEntries({
        start_time: startOfDate(startDate),
        end_time: endOfDate(endDate),
        entry_type: entryType === ALL_ENTRY_TYPES ? undefined : entryType,
        limit: 300,
      })
      if (response.success) setRows(response.data ?? [])
    } finally {
      setLoading(false)
    }
  }, [endDate, entryType, startDate])

  useEffect(() => {
    void fetchRows()
  }, [fetchRows])

  const totals = useMemo(
    () =>
      rows.reduce(
        (acc, row) => {
          if (row.entry_type === 'consume') acc.consume += Math.abs(row.amount)
          if (row.entry_type === 'topup') acc.topup += row.amount
          if (row.entry_type === 'refund') acc.refund += row.amount
          acc.lastBalance = row.balance_after
          return acc
        },
        { consume: 0, topup: 0, refund: 0, lastBalance: 0 }
      ),
    [rows]
  )

  return (
    <div className='space-y-4'>
      <FilterShell>
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
          <span className='text-muted-foreground text-xs'>{t('End date')}</span>
          <Input
            type='date'
            value={endDate}
            onChange={(event) => setEndDate(event.target.value)}
          />
        </label>
        <label className='min-w-44 space-y-1'>
          <span className='text-muted-foreground text-xs'>{t('Type')}</span>
          <Select
            value={entryType}
            onValueChange={(value) => setEntryType(value ?? ALL_ENTRY_TYPES)}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value={ALL_ENTRY_TYPES}>{t('All types')}</SelectItem>
                <SelectItem value='topup'>{t('Top-up')}</SelectItem>
                <SelectItem value='consume'>{t('Usage')}</SelectItem>
                <SelectItem value='refund'>{t('Refund')}</SelectItem>
                <SelectItem value='adjustment'>{t('Adjustment')}</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </label>
        <Button
          type='button'
          variant='outline'
          onClick={() => void fetchRows()}
          disabled={loading}
          className='md:ml-auto'
        >
          <RefreshCw
            className={cn('size-4', loading && 'animate-spin')}
            aria-hidden='true'
          />
          {t('Refresh')}
        </Button>
      </FilterShell>

      <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        <MetricCard
          title={t('Ledger consume total')}
          value={formatQuota(totals.consume)}
          icon={WalletCards}
        />
        <MetricCard
          title={t('Top-up')}
          value={formatQuota(totals.topup)}
          icon={ArrowUpCircle}
          tone='positive'
        />
        <MetricCard
          title={t('Refund')}
          value={formatQuota(totals.refund)}
          icon={ArrowDownCircle}
        />
        <MetricCard
          title={t('Latest balance')}
          value={formatQuota(totals.lastBalance)}
          icon={Scale}
          tone='accent'
        />
      </div>

      <div className='overflow-hidden rounded-lg border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Time')}</TableHead>
              <TableHead>{t('Type')}</TableHead>
              <TableHead>{t('Account')}</TableHead>
              <TableHead>{t('Model')}</TableHead>
              <TableHead>{t('Key')}</TableHead>
              <TableHead>{t('Request ID')}</TableHead>
              <TableHead className='text-right'>{t('Amount')}</TableHead>
              <TableHead className='text-right'>
                {t('Balance before')}
              </TableHead>
              <TableHead className='text-right'>{t('Balance after')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row) => (
              <TableRow key={row.id}>
                <TableCell className='text-xs whitespace-nowrap'>
                  {formatTimestampToDate(row.occurred_at)}
                </TableCell>
                <TableCell>{entryTypeLabel(row.entry_type, t)}</TableCell>
                <TableCell>{sourceLabel(row.account_type, t)}</TableCell>
                <TableCell>
                  {row.model_name ? (
                    <Badge variant='outline'>{row.model_name}</Badge>
                  ) : (
                    '-'
                  )}
                </TableCell>
                <TableCell>{row.token_name || '-'}</TableCell>
                <TableCell className='max-w-52 truncate'>
                  {row.request_id || '-'}
                </TableCell>
                <TableCell
                  className={cn(
                    'text-right tabular-nums',
                    signedQuotaClass(row.amount)
                  )}
                >
                  {formatQuota(row.amount)}
                </TableCell>
                <TableCell className='text-right tabular-nums'>
                  {formatQuota(row.balance_before)}
                </TableCell>
                <TableCell className='text-right font-medium tabular-nums'>
                  {formatQuota(row.balance_after)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        {!loading && rows.length === 0 && (
          <EmptyBillingState
            title={t('No account ledger records found')}
            description={t('Try changing the date range or filters')}
          />
        )}
      </div>
    </div>
  )
}

const BILLING_SECTION_META: Record<
  BillingCenterSectionId,
  {
    titleKey: string
    descriptionKey: string
    icon: React.ComponentType<{ className?: string; 'aria-hidden'?: boolean }>
  }
> = {
  current: {
    titleKey: 'Current period',
    descriptionKey:
      'Review this period usage, tokens, discounts, and settlement amount',
    icon: Gauge,
  },
  monthly: {
    titleKey: 'Monthly statements',
    descriptionKey:
      'Generate and review frozen monthly statements by model, group, key, and source',
    icon: ReceiptText,
  },
  reconciliation: {
    titleKey: 'Daily reconciliation',
    descriptionKey:
      'Match usage facts with account ledger consumption and inspect differences',
    icon: Scale,
  },
  ledger: {
    titleKey: 'Account ledger',
    descriptionKey:
      'Review balance-changing entries from top-ups, usage, refunds, and adjustments',
    icon: WalletCards,
  },
}

export function BillingCenter() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const params = route.useParams()
  const activeSection = params.section as BillingCenterSectionId
  const meta = BILLING_SECTION_META[activeSection]

  const handleSectionChange = useCallback(
    (section: string) => {
      void navigate({
        to: '/billing/$section',
        params: { section: section as BillingCenterSectionId },
      })
    },
    [navigate]
  )

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Billing')}</SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t(meta.descriptionKey)}
      </SectionPageLayout.Description>
      <SectionPageLayout.Content>
        <div className='flex flex-col gap-4'>
          <Tabs value={activeSection} onValueChange={handleSectionChange}>
            <TabsList className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'>
              {Object.entries(BILLING_SECTION_META).map(([section, item]) => {
                const Icon = item.icon
                return (
                  <TabsTrigger key={section} value={section}>
                    <Icon className='size-4' aria-hidden />
                    {t(item.titleKey)}
                  </TabsTrigger>
                )
              })}
            </TabsList>
          </Tabs>
          <Separator />
          {activeSection === 'current' && <CurrentPeriodTab />}
          {activeSection === 'monthly' && <MonthlyStatementsTab />}
          {activeSection === 'reconciliation' && <DailyReconciliationTab />}
          {activeSection === 'ledger' && <AccountLedgerTab />}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
