import { useCallback, useEffect, useMemo, useState } from 'react'
import { getRouteApi, useNavigate } from '@tanstack/react-router'
import {
  Activity,
  AlertTriangle,
  DatabaseZap,
  Download,
  Gauge,
  ReceiptText,
  RefreshCw,
  Scale,
  WalletCards,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import dayjs from '@/lib/dayjs'
import {
  formatNumber,
  formatQuota,
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
  exportBillingBreakdowns,
  exportBillingMonthlyStatement,
  generateBillingMonthlyStatement,
  getBillingBreakdowns,
  getBillingMonthlyStatementSummaries,
  getBillingMonthlyStatements,
  getDailyBillingReconciliations,
} from './api'
import type {
  BillingBreakdownRow,
  BillingCenterSectionId,
  BillingStatement,
  BillingStatementSummary,
  DailyBillingReconciliation,
} from './types'

const route = getRouteApi('/_authenticated/billing/$section')

function statusLabel(value: string, t: (key: string) => string) {
  if (value === 'confirmed') return t('Confirmed')
  if (value === 'exception') return t('Exception')
  if (value === 'open') return t('Open')
  if (value === 'estimated') return t('Estimated')
  return value || '-'
}

function signedQuotaClass(value: number) {
  if (value > 0) return 'text-emerald-600 dark:text-emerald-400'
  if (value < 0) return 'text-destructive'
  return 'text-muted-foreground'
}

function downloadBlob(blob: Blob, fileName: string) {
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = fileName
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  URL.revokeObjectURL(url)
}

function summaryRowsToBreakdownRows(
  rows: BillingStatementSummary[]
): BillingBreakdownRow[] {
  return rows.map((row) => ({
    period: row.period,
    period_value: row.period_value,
    model_name: row.model_name || '-',
    group: row.group || '-',
    billing_source: row.billing_source || '-',
    billing_mode: row.billing_mode || '-',
    request_count: row.request_count,
    input_tokens: row.input_tokens,
    output_tokens: row.output_tokens,
    cache_read_tokens: row.cache_read_tokens,
    cache_write_tokens: row.cache_write_tokens,
    original_amount: row.original_amount,
    discount_amount: row.discount_amount,
    settlement_amount: row.settlement_amount,
  }))
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

function MonthlyStatementsTab() {
  const { t } = useTranslation()
  const [month, setMonth] = useState(() =>
    dayjs().subtract(1, 'month').format('YYYY-MM')
  )
  const [statements, setStatements] = useState<BillingStatement[]>([])
  const [selected, setSelected] = useState<BillingStatement | null>(null)
  const [summaries, setSummaries] = useState<BillingStatementSummary[]>([])
  const [loading, setLoading] = useState(false)
  const [generating, setGenerating] = useState(false)
  const [exporting, setExporting] = useState(false)

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

  const handleExport = useCallback(async () => {
    if (!selected) return
    setExporting(true)
    try {
      const blob = await exportBillingMonthlyStatement(selected.statement_no)
      downloadBlob(
        blob,
        `monthly-billing-${selected.period_value}-${selected.statement_no}.csv`
      )
      toast.success(t('Billing exported'))
    } finally {
      setExporting(false)
    }
  }, [selected, t])

  const selectedBreakdowns = useMemo(
    () =>
      summaryRowsToBreakdownRows(
        summaries.filter((row) => row.dimension === 'month_model_group')
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
        <Button
          type='button'
          variant='outline'
          onClick={() => void handleExport()}
          disabled={exporting || !selected}
        >
          <Download
            className={cn('size-4', exporting && 'animate-pulse')}
            aria-hidden='true'
          />
          {t('Export monthly bill')}
        </Button>
      </FilterShell>

      <div className='space-y-4'>
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

          <BillingBreakdownTable
            rows={selectedBreakdowns}
            title={t('Monthly model group bill')}
            loading={loading}
          />
        </div>
      </div>
    </div>
  )
}

function BillingBreakdownTable({
  rows,
  title,
  loading,
}: {
  rows: BillingBreakdownRow[]
  title: string
  loading?: boolean
}) {
  const { t } = useTranslation()

  return (
    <div className='overflow-hidden rounded-lg border'>
      <div className='flex items-center justify-between gap-3 border-b px-3 py-2'>
        <h3 className='text-sm font-medium'>{title}</h3>
        {rows[0] && <Badge variant='outline'>{t('Model group bill')}</Badge>}
      </div>
      <div className='overflow-x-auto'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Period')}</TableHead>
              <TableHead>{t('Model')}</TableHead>
              <TableHead>{t('Group')}</TableHead>
              <TableHead className='text-right'>{t('Requests')}</TableHead>
              <TableHead className='text-right'>{t('Input tokens')}</TableHead>
              <TableHead className='text-right'>{t('Output tokens')}</TableHead>
              <TableHead className='text-right'>{t('Cache read')}</TableHead>
              <TableHead className='text-right'>{t('Cache write')}</TableHead>
              <TableHead className='text-right'>{t('Original amount')}</TableHead>
              <TableHead className='text-right'>{t('Group discount')}</TableHead>
              <TableHead className='text-right'>
                {t('Settlement amount')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row) => (
              <TableRow
                key={`${row.period}-${row.period_value}-${row.model_name}-${row.group}-${row.billing_source}-${row.billing_mode}`}
              >
                <TableCell className='font-medium whitespace-nowrap'>
                  {row.period_value}
                </TableCell>
                <TableCell>
                  <Badge variant='outline'>{row.model_name || '-'}</Badge>
                </TableCell>
                <TableCell>{row.group || '-'}</TableCell>
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
                  {formatTokens(row.cache_read_tokens)}
                </TableCell>
                <TableCell className='text-right tabular-nums'>
                  {formatTokens(row.cache_write_tokens)}
                </TableCell>
                <TableCell className='text-right tabular-nums'>
                  {formatQuota(row.original_amount)}
                </TableCell>
                <TableCell className='text-right tabular-nums'>
                  {formatQuota(row.discount_amount)}
                </TableCell>
                <TableCell className='text-right font-medium tabular-nums'>
                  {formatQuota(row.settlement_amount)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      {!loading && rows.length === 0 && (
        <EmptyBillingState title={t('No billing breakdown found')} />
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
  const [breakdowns, setBreakdowns] = useState<BillingBreakdownRow[]>([])
  const [loading, setLoading] = useState(false)
  const [exporting, setExporting] = useState(false)

  const fetchRows = useCallback(async () => {
    setLoading(true)
    try {
      const response = await getDailyBillingReconciliations({
        start_month: startMonth,
        end_month: endMonth,
        limit: 90,
      })
      if (response.success) setRows(response.data ?? [])
      const breakdownResponse = await getBillingBreakdowns({
        period: 'day',
        start_date: `${startMonth}-01`,
        end_date: dayjs(`${endMonth}-01`).endOf('month').format('YYYY-MM-DD'),
        limit: 500,
      })
      if (breakdownResponse.success) {
        setBreakdowns(breakdownResponse.data ?? [])
      }
    } finally {
      setLoading(false)
    }
  }, [endMonth, startMonth])

  useEffect(() => {
    void fetchRows()
  }, [fetchRows])

  const dailyExportEndDate = useMemo(
    () => dayjs(`${endMonth}-01`).endOf('month').format('YYYY-MM-DD'),
    [endMonth]
  )

  const handleExport = useCallback(async () => {
    const startDate = `${startMonth}-01`
    setExporting(true)
    try {
      const blob = await exportBillingBreakdowns({
        period: 'day',
        start_date: startDate,
        end_date: dailyExportEndDate,
      })
      downloadBlob(blob, `daily-billing-${startDate}-${dailyExportEndDate}.csv`)
      toast.success(t('Billing exported'))
    } finally {
      setExporting(false)
    }
  }, [dailyExportEndDate, startMonth, t])

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
        <Button
          type='button'
          variant='outline'
          onClick={() => void handleExport()}
          disabled={exporting}
        >
          <Download
            className={cn('size-4', exporting && 'animate-pulse')}
            aria-hidden='true'
          />
          {t('Export daily bill')}
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

      <BillingBreakdownTable
        rows={breakdowns}
        title={t('Daily model group bill')}
        loading={loading}
      />
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
          {activeSection === 'monthly' && <MonthlyStatementsTab />}
          {activeSection === 'reconciliation' && <DailyReconciliationTab />}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
