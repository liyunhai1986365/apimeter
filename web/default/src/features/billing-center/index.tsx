import { useCallback, useEffect, useMemo, useState } from 'react'
import { getRouteApi, useNavigate } from '@tanstack/react-router'
import {
  Activity,
  Download,
  Gauge,
  ReceiptText,
  RefreshCw,
  Scale,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import dayjs from '@/lib/dayjs'
import {
  formatNumber,
  formatQuota,
  formatTimestampToDate,
  formatTokens,
  parseQuotaFromDollars,
} from '@/lib/format'
import { cn } from '@/lib/utils'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
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
import { Textarea } from '@/components/ui/textarea'
import { SectionPageLayout } from '@/components/layout'
import {
  exportBillingMonthlyStatement,
  confirmBillingStatement,
  createBillingStatementDispute,
  getBillingStatementWorkflow,
  getBillingMonthlyStatements,
} from './api'
import type {
  BillingBreakdownRow,
  BillingCenterSectionId,
  BillingStatement,
  BillingStatementSummary,
  BillingStatementWorkflowDetail,
} from './types'

const route = getRouteApi('/_authenticated/billing/$section')

function statusLabel(value: string, t: (key: string) => string) {
  if (value === 'confirmed') return t('Confirmed')
  if (value === 'exception') return t('Exception')
  if (value === 'open') return t('Open')
  if (value === 'estimated') return t('Estimated')
  if (value === 'pending') return t('Pending')
  if (value === 'disputed') return t('Disputed')
  if (value === 'accepted') return t('Accepted')
  if (value === 'rejected') return t('Rejected')
  if (value === 'synced') return t('Synced')
  if (value === 'failed') return t('Failed')
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
    <div className='bg-muted/20 flex flex-col gap-3 rounded-lg border p-3 md:flex-row md:flex-wrap md:items-end'>
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

function StatementBadge({ statement }: { statement: BillingStatement }) {
  const { t } = useTranslation()
  const status =
    statement.reconciliation_status === 'exception'
      ? 'exception'
      : statement.confirmation_status || statement.status
  return (
    <Badge
      variant={
        status === 'exception' || status === 'disputed'
          ? 'destructive'
          : status === 'confirmed'
            ? 'default'
            : 'secondary'
      }
    >
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
  const [workflow, setWorkflow] =
    useState<BillingStatementWorkflowDetail | null>(null)
  const [loading, setLoading] = useState(false)
  const [exporting, setExporting] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [disputeOpen, setDisputeOpen] = useState(false)
  const [disputing, setDisputing] = useState(false)
  const [disputeReason, setDisputeReason] = useState('amount')
  const [disputeDescription, setDisputeDescription] = useState('')
  const [expectedAmount, setExpectedAmount] = useState('')

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
        setSelected((current) =>
          current
            ? (rows.find((row) => row.statement_no === current.statement_no) ??
              rows[0] ??
              null)
            : (rows[0] ?? null)
        )
      }
    } finally {
      setLoading(false)
    }
  }, [month])

  const refreshWorkflow = useCallback(async (statementNo: string) => {
    const response = await getBillingStatementWorkflow(statementNo)
    if (!response.success) return
    setWorkflow(response.data)
    setSelected(response.data.statement)
    setStatements((rows) =>
      rows.map((row) =>
        row.statement_no === response.data.statement.statement_no
          ? response.data.statement
          : row
      )
    )
  }, [])
  const selectedStatementNo = selected?.statement_no ?? ''

  useEffect(() => {
    void fetchStatements()
  }, [fetchStatements])

  useEffect(() => {
    if (!selectedStatementNo) {
      setWorkflow(null)
      return
    }
    void refreshWorkflow(selectedStatementNo)
  }, [refreshWorkflow, selectedStatementNo])

  const handleConfirm = useCallback(async () => {
    if (!selected) return
    setConfirming(true)
    try {
      const response = await confirmBillingStatement(
        selected.statement_no,
        selected.revision
      )
      if (response.success) {
        toast.success(t('Statement confirmed'))
        setConfirmOpen(false)
        await refreshWorkflow(selected.statement_no)
      }
    } finally {
      setConfirming(false)
    }
  }, [refreshWorkflow, selected, t])

  const handleDispute = useCallback(async () => {
    if (!selected || disputeDescription.trim().length < 5) {
      toast.error(t('Please provide at least 5 characters'))
      return
    }
    setDisputing(true)
    try {
      const hasExpectedAmount = expectedAmount.trim() !== ''
      const response = await createBillingStatementDispute(
        selected.statement_no,
        {
          revision: selected.revision,
          reason_type: disputeReason,
          description: disputeDescription.trim(),
          expected_amount: hasExpectedAmount
            ? parseQuotaFromDollars(Number(expectedAmount))
            : 0,
          has_expected_amount: hasExpectedAmount,
        }
      )
      if (response.success) {
        toast.success(t('Dispute submitted'))
        setDisputeOpen(false)
        setDisputeDescription('')
        setExpectedAmount('')
        await refreshWorkflow(selected.statement_no)
      }
    } finally {
      setDisputing(false)
    }
  }, [
    disputeDescription,
    disputeReason,
    expectedAmount,
    refreshWorkflow,
    selected,
    t,
  ])

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
        (workflow?.summaries ?? []).filter(
          (row) => row.dimension === 'month_model_group'
        )
      ),
    [workflow?.summaries]
  )

  return (
    <div className='flex flex-col gap-4'>
      <FilterShell>
        <label className='flex min-w-44 flex-col gap-1'>
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
            data-icon='inline-start'
            className={cn(loading && 'animate-spin')}
            aria-hidden='true'
          />
          {t('Refresh')}
        </Button>
        <Button
          type='button'
          variant='outline'
          onClick={() => void handleExport()}
          disabled={exporting || !selected}
        >
          <Download
            data-icon='inline-start'
            className={cn(exporting && 'animate-pulse')}
            aria-hidden='true'
          />
          {t('Export monthly bill')}
        </Button>
      </FilterShell>

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
                  <StatementBadge statement={statement} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        {!loading && statements.length === 0 && (
          <EmptyBillingState
            title={t('No monthly statements found')}
            description={t('Monthly statements will appear after settlement')}
          />
        )}
      </div>

      <div className='flex flex-col gap-3'>
        {selected && (
          <>
            <div className='flex flex-wrap items-center justify-between gap-3 rounded-lg border p-3'>
              <div className='flex flex-col gap-1'>
                <div className='flex items-center gap-2'>
                  <StatementBadge statement={selected} />
                  <span className='text-muted-foreground text-xs'>
                    {t('Revision')} {selected.revision}
                  </span>
                </div>
                <p className='text-muted-foreground text-sm'>
                  {selected.confirmation_status === 'confirmed'
                    ? `${t('Confirmed at')} ${formatTimestampToDate(selected.confirmed_at)}`
                    : selected.confirmation_status === 'disputed'
                      ? t('This statement is under review')
                      : t('Please review and confirm this statement')}
                </p>
              </div>
              {selected.reconciliation_status === 'matched' &&
                selected.confirmation_status === 'pending' && (
                  <div className='flex flex-wrap gap-2'>
                    <Button
                      type='button'
                      variant='outline'
                      onClick={() => setDisputeOpen(true)}
                    >
                      {t('Dispute statement')}
                    </Button>
                    <Button type='button' onClick={() => setConfirmOpen(true)}>
                      {t('Confirm statement')}
                    </Button>
                  </div>
                )}
            </div>
            <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-5'>
              <MetricCard
                title={t('Base settlement amount')}
                value={formatQuota(selected.base_settlement_amount)}
                icon={Scale}
              />
              <MetricCard
                title={t('Adjustment amount')}
                value={formatQuota(selected.adjustment_amount)}
                icon={Scale}
                tone={
                  selected.adjustment_amount === 0
                    ? undefined
                    : selected.adjustment_amount > 0
                      ? 'negative'
                      : 'positive'
                }
              />
              <MetricCard
                title={t('Final settlement amount')}
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
            </div>
          </>
        )}

        <BillingBreakdownTable
          rows={selectedBreakdowns}
          title={t('Monthly model group bill')}
          loading={loading}
        />
        {workflow && workflow.adjustments.length > 0 && (
          <StatementAdjustmentsTable workflow={workflow} />
        )}
        {workflow && workflow.disputes.length > 0 && (
          <StatementDisputesTable workflow={workflow} />
        )}
      </div>

      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent size='sm'>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Confirm statement')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('Confirm that you recognize the final settlement amount')}{' '}
              {selected ? formatQuota(selected.settlement_amount) : '-'}.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={confirming}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={confirming}
              onClick={(event) => {
                event.preventDefault()
                void handleConfirm()
              }}
            >
              {confirming ? t('Confirming...') : t('Confirm')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Dialog open={disputeOpen} onOpenChange={setDisputeOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('Dispute statement')}</DialogTitle>
            <DialogDescription>
              {t('Describe the discrepancy for administrator review')}
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor='billing-dispute-reason'>
                {t('Dispute reason')}
              </FieldLabel>
              <NativeSelect
                id='billing-dispute-reason'
                className='w-full'
                value={disputeReason}
                onChange={(event) => setDisputeReason(event.target.value)}
              >
                <NativeSelectOption value='amount'>
                  {t('Amount mismatch')}
                </NativeSelectOption>
                <NativeSelectOption value='usage'>
                  {t('Usage record mismatch')}
                </NativeSelectOption>
                <NativeSelectOption value='other'>
                  {t('Other')}
                </NativeSelectOption>
              </NativeSelect>
            </Field>
            <Field>
              <FieldLabel htmlFor='billing-dispute-description'>
                {t('Description')}
              </FieldLabel>
              <Textarea
                id='billing-dispute-description'
                value={disputeDescription}
                onChange={(event) => setDisputeDescription(event.target.value)}
                maxLength={2000}
                placeholder={t(
                  'Explain which records or amounts are incorrect'
                )}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor='billing-expected-amount'>
                {t('Expected final amount (optional)')}
              </FieldLabel>
              <Input
                id='billing-expected-amount'
                type='number'
                min='0'
                step='0.0001'
                value={expectedAmount}
                onChange={(event) => setExpectedAmount(event.target.value)}
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              onClick={() => setDisputeOpen(false)}
              disabled={disputing}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='button'
              onClick={() => void handleDispute()}
              disabled={disputing}
            >
              {disputing ? t('Submitting...') : t('Submit dispute')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function StatementAdjustmentsTable({
  workflow,
}: {
  workflow: BillingStatementWorkflowDetail
}) {
  const { t } = useTranslation()
  return (
    <div className='overflow-hidden rounded-lg border'>
      <div className='border-b px-3 py-2 text-sm font-medium'>
        {t('Adjustment history')}
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Time')}</TableHead>
            <TableHead>{t('Reason')}</TableHead>
            <TableHead className='text-right'>{t('Amount')}</TableHead>
            <TableHead>{t('Balance sync')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {workflow.adjustments.map((item) => (
            <TableRow key={item.adjustment_no}>
              <TableCell>{formatTimestampToDate(item.created_at)}</TableCell>
              <TableCell>{item.reason}</TableCell>
              <TableCell
                className={cn(
                  'text-right tabular-nums',
                  signedQuotaClass(item.amount)
                )}
              >
                {formatQuota(item.amount)}
              </TableCell>
              <TableCell>{statusLabel(item.balance_sync_status, t)}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

function StatementDisputesTable({
  workflow,
}: {
  workflow: BillingStatementWorkflowDetail
}) {
  const { t } = useTranslation()
  return (
    <div className='overflow-hidden rounded-lg border'>
      <div className='border-b px-3 py-2 text-sm font-medium'>
        {t('Dispute history')}
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Time')}</TableHead>
            <TableHead>{t('Description')}</TableHead>
            <TableHead>{t('Resolution')}</TableHead>
            <TableHead>{t('Status')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {workflow.disputes.map((item) => (
            <TableRow key={item.dispute_no}>
              <TableCell>{formatTimestampToDate(item.created_at)}</TableCell>
              <TableCell className='max-w-md whitespace-normal'>
                {item.description}
              </TableCell>
              <TableCell>{item.resolution || '-'}</TableCell>
              <TableCell>{statusLabel(item.status, t)}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
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
              <TableHead className='text-right'>
                {t('Original amount')}
              </TableHead>
              <TableHead className='text-right'>
                {t('Group discount')}
              </TableHead>
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
      'Review monthly statements, confirm charges, or submit a dispute',
    icon: ReceiptText,
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
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
