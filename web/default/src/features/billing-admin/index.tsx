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
import { formatDiscountPercentage } from '@/lib/group-discount'
import { cn } from '@/lib/utils'
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
import { Empty, EmptyHeader, EmptyTitle } from '@/components/ui/empty'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { DiscountTooltip } from '@/components/discount-tooltip'
import { SectionPageLayout } from '@/components/layout'
import type {
  BillingAdminStatementItem,
  BillingStatementDispute,
  BillingStatementWorkflowDetail,
} from '@/features/billing-center/types'
import {
  adjustAdminBillingStatement,
  generateAdminBillingStatement,
  getAdminBillingStatement,
  getAdminBillingStatements,
  resolveAdminBillingDispute,
  retryAdminBillingAdjustment,
} from './api'

const PAGE_SIZE = 20

function workflowStatus(item: BillingAdminStatementItem) {
  return item.reconciliation_status === 'exception'
    ? 'exception'
    : item.confirmation_status || item.status
}

function statusText(status: string, t: (key: string) => string) {
  const labels: Record<string, string> = {
    confirmed: 'Confirmed',
    pending: 'Pending',
    disputed: 'Disputed',
    exception: 'Exception',
    accepted: 'Accepted',
    rejected: 'Rejected',
    synced: 'Synced',
    failed: 'Failed',
  }
  return t(labels[status] ?? status)
}

function StatusBadge({ status }: { status: string }) {
  const { t } = useTranslation()
  return (
    <Badge
      variant={
        status === 'exception' || status === 'failed'
          ? 'destructive'
          : status === 'confirmed' || status === 'synced'
            ? 'default'
            : 'secondary'
      }
    >
      {statusText(status, t)}
    </Badge>
  )
}

export function BillingAdmin() {
  const { t } = useTranslation()
  const [month, setMonth] = useState(() =>
    dayjs().subtract(1, 'month').format('YYYY-MM')
  )
  const [username, setUsername] = useState('')
  const [userId, setUserId] = useState('')
  const [confirmationStatus, setConfirmationStatus] = useState('')
  const [items, setItems] = useState<BillingAdminStatementItem[]>([])
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(false)
  const [selectedNo, setSelectedNo] = useState('')
  const [detail, setDetail] = useState<BillingStatementWorkflowDetail | null>(
    null
  )
  const [adjustOpen, setAdjustOpen] = useState(false)
  const [adjustDirection, setAdjustDirection] = useState('increase')
  const [adjustAmount, setAdjustAmount] = useState('')
  const [adjustReason, setAdjustReason] = useState('')
  const [linkedDisputeId, setLinkedDisputeId] = useState(0)
  const [adjusting, setAdjusting] = useState(false)
  const [resolveDispute, setResolveDispute] =
    useState<BillingStatementDispute | null>(null)
  const [resolution, setResolution] = useState('')
  const [resolving, setResolving] = useState(false)
  const [generating, setGenerating] = useState(false)

  const query = useMemo(
    () => ({
      user_id: Number(userId) || undefined,
      username: username.trim() || undefined,
      month: month || undefined,
      confirmation_status: confirmationStatus || undefined,
      limit: PAGE_SIZE,
      offset,
    }),
    [confirmationStatus, month, offset, userId, username]
  )

  const fetchList = useCallback(async () => {
    setLoading(true)
    try {
      const response = await getAdminBillingStatements(query)
      if (!response.success) return
      setItems(response.data.items ?? [])
      setTotal(response.data.total ?? 0)
      setSelectedNo((current) =>
        response.data.items.some((item) => item.statement_no === current)
          ? current
          : (response.data.items[0]?.statement_no ?? '')
      )
    } finally {
      setLoading(false)
    }
  }, [query])

  const fetchDetail = useCallback(async (statementNo: string) => {
    if (!statementNo) {
      setDetail(null)
      return
    }
    const response = await getAdminBillingStatement(statementNo)
    if (response.success) setDetail(response.data)
  }, [])

  useEffect(() => {
    void fetchList()
  }, [fetchList])

  useEffect(() => {
    void fetchDetail(selectedNo)
  }, [fetchDetail, selectedNo])

  const refreshAll = useCallback(async () => {
    await fetchList()
    if (selectedNo) await fetchDetail(selectedNo)
  }, [fetchDetail, fetchList, selectedNo])

  const handleGenerate = useCallback(async () => {
    const numericUserId = Number(userId)
    if (!numericUserId || !month) {
      toast.error(t('Enter a user ID and month first'))
      return
    }
    setGenerating(true)
    try {
      const response = await generateAdminBillingStatement(numericUserId, month)
      if (response.success) {
        toast.success(t('Monthly statement generated'))
        setSelectedNo(response.data.statement.statement_no)
        await fetchList()
        await fetchDetail(response.data.statement.statement_no)
      }
    } finally {
      setGenerating(false)
    }
  }, [fetchDetail, fetchList, month, t, userId])

  const openAdjustment = useCallback((disputeId = 0) => {
    setLinkedDisputeId(disputeId)
    setAdjustDirection('increase')
    setAdjustAmount('')
    setAdjustReason('')
    setAdjustOpen(true)
  }, [])

  const handleAdjustment = useCallback(async () => {
    if (!detail) return
    const quotaAmount = parseQuotaFromDollars(Number(adjustAmount))
    if (quotaAmount <= 0 || adjustReason.trim().length < 3) {
      toast.error(t('Enter a valid amount and reason'))
      return
    }
    const amount = adjustDirection === 'increase' ? quotaAmount : -quotaAmount
    setAdjusting(true)
    try {
      const response = await adjustAdminBillingStatement(
        detail.statement.statement_no,
        {
          amount,
          reason: adjustReason.trim(),
          dispute_id: linkedDisputeId,
          idempotency_key: crypto.randomUUID(),
        }
      )
      if (response.success) {
        toast.success(t('Statement adjusted and balance synchronized'))
        setAdjustOpen(false)
        await refreshAll()
      }
    } finally {
      setAdjusting(false)
    }
  }, [
    adjustAmount,
    adjustDirection,
    adjustReason,
    detail,
    linkedDisputeId,
    refreshAll,
    t,
  ])

  const handleRejectDispute = useCallback(async () => {
    if (!resolveDispute || resolution.trim().length < 3) {
      toast.error(t('Enter a resolution note'))
      return
    }
    setResolving(true)
    try {
      const response = await resolveAdminBillingDispute(
        resolveDispute.id,
        'reject',
        resolution.trim()
      )
      if (response.success) {
        toast.success(t('Dispute resolved'))
        setResolveDispute(null)
        setResolution('')
        await refreshAll()
      }
    } finally {
      setResolving(false)
    }
  }, [refreshAll, resolution, resolveDispute, t])

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Billing management')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t(
          'Review all user statements, disputes, adjustments, and audit events'
        )}
      </SectionPageLayout.Description>
      <SectionPageLayout.Content>
        <div className='flex flex-col gap-4'>
          <div className='bg-muted/20 grid gap-3 rounded-lg border p-3 md:grid-cols-5'>
            <Field>
              <FieldLabel htmlFor='admin-billing-user-id'>
                {t('User ID')}
              </FieldLabel>
              <Input
                id='admin-billing-user-id'
                inputMode='numeric'
                value={userId}
                onChange={(event) => setUserId(event.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor='admin-billing-username'>
                {t('Username')}
              </FieldLabel>
              <Input
                id='admin-billing-username'
                value={username}
                onChange={(event) => setUsername(event.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor='admin-billing-month'>
                {t('Month')}
              </FieldLabel>
              <Input
                id='admin-billing-month'
                type='month'
                value={month}
                onChange={(event) => setMonth(event.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor='admin-billing-status'>
                {t('Confirmation status')}
              </FieldLabel>
              <NativeSelect
                id='admin-billing-status'
                className='w-full'
                value={confirmationStatus}
                onChange={(event) => setConfirmationStatus(event.target.value)}
              >
                <NativeSelectOption value=''>{t('All')}</NativeSelectOption>
                <NativeSelectOption value='pending'>
                  {t('Pending')}
                </NativeSelectOption>
                <NativeSelectOption value='confirmed'>
                  {t('Confirmed')}
                </NativeSelectOption>
                <NativeSelectOption value='disputed'>
                  {t('Disputed')}
                </NativeSelectOption>
              </NativeSelect>
            </Field>
            <div className='flex items-end gap-2'>
              <Button
                type='button'
                variant='outline'
                onClick={() => {
                  setOffset(0)
                  void fetchList()
                }}
                disabled={loading}
              >
                {t('Search')}
              </Button>
              <Button
                type='button'
                onClick={() => void handleGenerate()}
                disabled={generating}
              >
                {generating ? t('Generating...') : t('Regenerate')}
              </Button>
            </div>
          </div>

          <div className='overflow-hidden rounded-lg border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('User')}</TableHead>
                  <TableHead>{t('Statement')}</TableHead>
                  <TableHead className='text-right'>
                    {t('Base amount')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('Adjustments')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('Final amount')}
                  </TableHead>
                  <TableHead>{t('Status')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((item) => (
                  <TableRow
                    key={item.statement_no}
                    className='cursor-pointer'
                    data-state={
                      selectedNo === item.statement_no ? 'selected' : undefined
                    }
                    onClick={() => setSelectedNo(item.statement_no)}
                  >
                    <TableCell>
                      <div className='font-medium'>{item.username}</div>
                      <div className='text-muted-foreground text-xs'>
                        ID {item.user_id} · {item.email || '-'}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div>{item.period_value}</div>
                      <div className='text-muted-foreground text-xs'>
                        {item.statement_no}
                      </div>
                    </TableCell>
                    <TableCell className='text-right tabular-nums'>
                      {formatQuota(item.base_settlement_amount)}
                    </TableCell>
                    <TableCell
                      className={cn(
                        'text-right tabular-nums',
                        item.adjustment_amount > 0
                          ? 'text-destructive'
                          : item.adjustment_amount < 0
                            ? 'text-emerald-600 dark:text-emerald-400'
                            : 'text-muted-foreground'
                      )}
                    >
                      {formatQuota(item.adjustment_amount)}
                    </TableCell>
                    <TableCell className='text-right font-medium tabular-nums'>
                      {formatQuota(item.settlement_amount)}
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={workflowStatus(item)} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            {!loading && items.length === 0 && (
              <Empty className='py-10'>
                <EmptyHeader>
                  <EmptyTitle>{t('No statements found')}</EmptyTitle>
                </EmptyHeader>
              </Empty>
            )}
          </div>

          <div className='flex items-center justify-between gap-3'>
            <span className='text-muted-foreground text-sm'>
              {t('Total')}: {total}
            </span>
            <div className='flex gap-2'>
              <Button
                type='button'
                variant='outline'
                disabled={offset === 0}
                onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
              >
                {t('Previous')}
              </Button>
              <Button
                type='button'
                variant='outline'
                disabled={offset + PAGE_SIZE >= total}
                onClick={() => setOffset(offset + PAGE_SIZE)}
              >
                {t('Next')}
              </Button>
            </div>
          </div>

          {detail && (
            <BillingAdminDetail
              detail={detail}
              onAdjust={() => openAdjustment()}
              onAdjustDispute={(id) => openAdjustment(id)}
              onRejectDispute={(dispute) => {
                setResolveDispute(dispute)
                setResolution('')
              }}
              onRetry={async (adjustmentNo) => {
                const response = await retryAdminBillingAdjustment(adjustmentNo)
                if (response.success) {
                  toast.success(t('Balance synchronization retried'))
                  await refreshAll()
                }
              }}
            />
          )}
        </div>

        <Dialog open={adjustOpen} onOpenChange={setAdjustOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t('Adjust statement')}</DialogTitle>
              <DialogDescription>
                {t(
                  'The adjustment updates both the statement and user balance'
                )}
              </DialogDescription>
            </DialogHeader>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor='billing-adjust-direction'>
                  {t('Adjustment direction')}
                </FieldLabel>
                <NativeSelect
                  id='billing-adjust-direction'
                  className='w-full'
                  value={adjustDirection}
                  onChange={(event) => setAdjustDirection(event.target.value)}
                >
                  <NativeSelectOption value='increase'>
                    {t('Increase bill (deduct balance)')}
                  </NativeSelectOption>
                  <NativeSelectOption value='decrease'>
                    {t('Decrease bill (refund balance)')}
                  </NativeSelectOption>
                </NativeSelect>
              </Field>
              <Field>
                <FieldLabel htmlFor='billing-adjust-amount'>
                  {t('Amount')}
                </FieldLabel>
                <Input
                  id='billing-adjust-amount'
                  type='number'
                  min='0'
                  step='0.0001'
                  value={adjustAmount}
                  onChange={(event) => setAdjustAmount(event.target.value)}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor='billing-adjust-reason'>
                  {t('Reason')}
                </FieldLabel>
                <Textarea
                  id='billing-adjust-reason'
                  maxLength={2000}
                  value={adjustReason}
                  onChange={(event) => setAdjustReason(event.target.value)}
                />
              </Field>
            </FieldGroup>
            <DialogFooter>
              <Button
                type='button'
                variant='outline'
                onClick={() => setAdjustOpen(false)}
                disabled={adjusting}
              >
                {t('Cancel')}
              </Button>
              <Button
                type='button'
                onClick={() => void handleAdjustment()}
                disabled={adjusting}
              >
                {adjusting ? t('Saving...') : t('Apply adjustment')}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog
          open={Boolean(resolveDispute)}
          onOpenChange={(open) => !open && setResolveDispute(null)}
        >
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t('Reject dispute')}</DialogTitle>
              <DialogDescription>
                {t(
                  'Provide a clear resolution that will be visible to the user'
                )}
              </DialogDescription>
            </DialogHeader>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor='billing-dispute-resolution'>
                  {t('Resolution')}
                </FieldLabel>
                <Textarea
                  id='billing-dispute-resolution'
                  value={resolution}
                  onChange={(event) => setResolution(event.target.value)}
                />
              </Field>
            </FieldGroup>
            <DialogFooter>
              <Button
                type='button'
                variant='outline'
                onClick={() => setResolveDispute(null)}
                disabled={resolving}
              >
                {t('Cancel')}
              </Button>
              <Button
                type='button'
                variant='destructive'
                onClick={() => void handleRejectDispute()}
                disabled={resolving}
              >
                {resolving ? t('Saving...') : t('Reject dispute')}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function BillingAdminDetail({
  detail,
  onAdjust,
  onAdjustDispute,
  onRejectDispute,
  onRetry,
}: {
  detail: BillingStatementWorkflowDetail
  onAdjust: () => void
  onAdjustDispute: (id: number) => void
  onRejectDispute: (dispute: BillingStatementDispute) => void
  onRetry: (adjustmentNo: string) => void
}) {
  const { t } = useTranslation()
  const statement = detail.statement
  return (
    <Card>
      <CardHeader className='flex flex-row items-center justify-between gap-3'>
        <div className='flex flex-col gap-1'>
          <CardTitle>{statement.statement_no}</CardTitle>
          <span className='text-muted-foreground text-sm'>
            {statement.period_value} · {t('Revision')} {statement.revision}
          </span>
        </div>
        <Button type='button' onClick={onAdjust}>
          {t('Adjust statement')}
        </Button>
      </CardHeader>
      <CardContent className='flex flex-col gap-5'>
        <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-6'>
          <div className='rounded-lg border p-3'>
            <div className='text-muted-foreground text-xs'>
              {t('Base amount')}
            </div>
            <div className='mt-1 font-semibold tabular-nums'>
              {formatQuota(statement.base_settlement_amount)}
            </div>
          </div>
          <div className='rounded-lg border p-3'>
            <div className='text-muted-foreground text-xs'>
              {t('Adjustments')}
            </div>
            <div className='mt-1 font-semibold tabular-nums'>
              {formatQuota(statement.adjustment_amount)}
            </div>
          </div>
          <div className='rounded-lg border p-3'>
            <div className='text-muted-foreground text-xs'>
              {t('Final amount')}
            </div>
            <div className='mt-1 font-semibold tabular-nums'>
              {formatQuota(statement.settlement_amount)}
            </div>
          </div>
          <div className='rounded-lg border p-3'>
            <div className='text-muted-foreground text-xs'>{t('Usage')}</div>
            <div className='mt-1 font-semibold tabular-nums'>
              {formatQuota(statement.consume_amount)}
            </div>
          </div>
          <div className='rounded-lg border p-3'>
            <div className='text-muted-foreground text-xs'>{t('Refund')}</div>
            <div className='mt-1 font-semibold tabular-nums'>
              {formatQuota(statement.refund_amount)}
            </div>
          </div>
          <div className='rounded-lg border p-3'>
            <div className='text-muted-foreground text-xs'>{t('Requests')}</div>
            <div className='mt-1 font-semibold tabular-nums'>
              {formatNumber(statement.request_count)}
            </div>
          </div>
        </div>

        <DetailSection title={t('Monthly model supplier bill')}>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Model')}</TableHead>
                <TableHead>{t('Supplier')}</TableHead>
                <TableHead className='text-right'>{t('Requests')}</TableHead>
                <TableHead className='text-right'>
                  {t('Input tokens')}
                </TableHead>
                <TableHead className='text-right'>
                  {t('Output tokens')}
                </TableHead>
                <TableHead className='text-right'>
                  {t('Original amount')}
                </TableHead>
                <TableHead className='text-right'>
                  {t('Billing discount')}
                </TableHead>
                <TableHead className='text-right'>
                  {t('Settlement amount')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {detail.summaries.map((item) => (
                <TableRow key={item.id || item.dimension_value}>
                  <TableCell>{item.model_name || '-'}</TableCell>
                  <TableCell className='whitespace-nowrap'>
                    {item.group || '-'}
                  </TableCell>
                  <TableCell className='text-right tabular-nums'>
                    {formatNumber(item.request_count)}
                  </TableCell>
                  <TableCell className='text-right tabular-nums'>
                    {formatTokens(item.input_tokens)}
                  </TableCell>
                  <TableCell className='text-right tabular-nums'>
                    {formatTokens(item.output_tokens)}
                  </TableCell>
                  <TableCell className='text-right tabular-nums'>
                    {formatQuota(item.original_amount)}
                  </TableCell>
                  <TableCell className='text-right tabular-nums'>
                    <DiscountTooltip
                      label={formatDiscountPercentage(item.group_ratio)}
                    >
                      <span>
                        {formatDiscountPercentage(item.group_ratio) || '-'}
                      </span>
                    </DiscountTooltip>
                  </TableCell>
                  <TableCell className='text-right font-medium tabular-nums'>
                    {formatQuota(item.settlement_amount)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </DetailSection>

        <DetailSection title={t('Disputes')}>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Time')}</TableHead>
                <TableHead>{t('Description')}</TableHead>
                <TableHead>{t('Expected amount')}</TableHead>
                <TableHead>{t('Status')}</TableHead>
                <TableHead className='text-right'>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {detail.disputes.map((item) => (
                <TableRow key={item.dispute_no}>
                  <TableCell>
                    {formatTimestampToDate(item.created_at)}
                  </TableCell>
                  <TableCell className='max-w-md whitespace-normal'>
                    {item.description}
                    {item.resolution && (
                      <div className='text-muted-foreground mt-1 text-xs'>
                        {item.resolution}
                      </div>
                    )}
                  </TableCell>
                  <TableCell>
                    {item.has_expected_amount
                      ? formatQuota(item.expected_amount)
                      : '-'}
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={item.status} />
                  </TableCell>
                  <TableCell className='text-right'>
                    {item.status === 'pending' && (
                      <div className='flex justify-end gap-2'>
                        <Button
                          type='button'
                          size='sm'
                          variant='outline'
                          onClick={() => onRejectDispute(item)}
                        >
                          {t('Reject')}
                        </Button>
                        <Button
                          type='button'
                          size='sm'
                          onClick={() => onAdjustDispute(item.id)}
                        >
                          {t('Adjust and accept')}
                        </Button>
                      </div>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </DetailSection>

        <DetailSection title={t('Adjustment history')}>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Time')}</TableHead>
                <TableHead>{t('Operator')}</TableHead>
                <TableHead>{t('Reason')}</TableHead>
                <TableHead className='text-right'>{t('Amount')}</TableHead>
                <TableHead>{t('Balance sync')}</TableHead>
                <TableHead className='text-right'>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {detail.adjustments.map((item) => (
                <TableRow key={item.adjustment_no}>
                  <TableCell>
                    {formatTimestampToDate(item.created_at)}
                  </TableCell>
                  <TableCell>{item.operator_username}</TableCell>
                  <TableCell>{item.reason}</TableCell>
                  <TableCell className='text-right tabular-nums'>
                    {formatQuota(item.amount)}
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={item.balance_sync_status} />
                  </TableCell>
                  <TableCell className='text-right'>
                    {item.balance_sync_status === 'failed' && (
                      <Button
                        type='button'
                        size='sm'
                        variant='outline'
                        onClick={() => onRetry(item.adjustment_no)}
                      >
                        {t('Retry')}
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </DetailSection>

        <DetailSection title={t('Audit events')}>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Time')}</TableHead>
                <TableHead>{t('Event')}</TableHead>
                <TableHead>{t('Actor')}</TableHead>
                <TableHead>{t('Detail')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {detail.events.map((item) => (
                <TableRow key={item.id}>
                  <TableCell>
                    {formatTimestampToDate(item.created_at)}
                  </TableCell>
                  <TableCell>{item.event_type}</TableCell>
                  <TableCell>
                    {item.actor_username || item.actor_type}
                  </TableCell>
                  <TableCell>{item.detail || '-'}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </DetailSection>
      </CardContent>
    </Card>
  )
}

function DetailSection({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <section className='overflow-hidden rounded-lg border'>
      <h3 className='border-b px-3 py-2 text-sm font-medium'>{title}</h3>
      <div className='overflow-x-auto'>{children}</div>
    </section>
  )
}
