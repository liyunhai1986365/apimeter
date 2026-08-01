import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowLeft01Icon,
  ArrowRight01Icon,
  CheckmarkCircle02Icon,
  ReloadIcon,
  Wallet03Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
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
import { Field, FieldLabel } from '@/components/ui/field'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
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
import { LongText } from '@/components/long-text'
import { completeAdminWithdrawal, listAdminWithdrawals } from './api'
import type {
  WithdrawalManagementItem,
  WithdrawalSource,
  WithdrawalStatus,
} from './types'

const PAGE_SIZE = 20

function formatSettlementAmount(amount: number, currency: string) {
  const normalized = currency.toUpperCase() === 'RMB' ? 'RMB' : 'USD'
  const symbol = normalized === 'RMB' ? '¥' : '$'
  return `${symbol}${amount.toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 6,
  })} ${normalized}`
}

function formatWithdrawalAmount(item: WithdrawalManagementItem) {
  if (item.source === 'user') return formatQuota(item.amount_quota)
  if (item.amount_money > 0) {
    return formatSettlementAmount(item.amount_money, item.currency)
  }
  return formatQuota(item.amount_quota)
}

function statusVariant(
  status: WithdrawalStatus
): 'default' | 'outline' | 'secondary' | 'destructive' {
  if (status === 'paid') return 'default'
  if (status === 'approved') return 'secondary'
  if (status === 'rejected') return 'destructive'
  return 'outline'
}

export function WithdrawalManagement() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [source, setSource] = useState<WithdrawalSource | undefined>()
  const [status, setStatus] = useState<WithdrawalStatus | undefined>()
  const [review, setReview] = useState<{
    item: WithdrawalManagementItem
    nextStatus: WithdrawalStatus
  } | null>(null)
  const [adminRemark, setAdminRemark] = useState('')

  const withdrawalsQuery = useQuery({
    queryKey: ['admin', 'withdrawals', page, source, status],
    queryFn: () =>
      listAdminWithdrawals({ page, pageSize: PAGE_SIZE, source, status }),
  })
  const updateMutation = useMutation({
    mutationFn: completeAdminWithdrawal,
    onSuccess: async () => {
      toast.success(t('Updated successfully'))
      setReview(null)
      setAdminRemark('')
      await queryClient.invalidateQueries({
        queryKey: ['admin', 'withdrawals'],
      })
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const data = withdrawalsQuery.data
  const items = data?.items ?? []
  const pageCount = Math.max(1, Math.ceil((data?.total ?? 0) / PAGE_SIZE))
  const pendingCount = items.filter((item) => item.status === 'pending').length

  const beginReview = (
    item: WithdrawalManagementItem,
    nextStatus: WithdrawalStatus
  ) => {
    setReview({ item, nextStatus })
    setAdminRemark(item.admin_remark ?? '')
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('Withdrawal Management')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button
            variant='outline'
            disabled={withdrawalsQuery.isFetching}
            onClick={() => withdrawalsQuery.refetch()}
          >
            <HugeiconsIcon icon={ReloadIcon} data-icon='inline-start' />
            {t('Refresh')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='mx-auto flex w-full max-w-7xl flex-col gap-4'>
            <div className='grid gap-3 sm:grid-cols-2'>
              <Card>
                <CardHeader>
                  <CardDescription>{t('Filtered requests')}</CardDescription>
                  <CardTitle className='tabular-nums'>
                    {data?.total ?? 0}
                  </CardTitle>
                </CardHeader>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>{t('Pending on this page')}</CardDescription>
                  <CardTitle className='tabular-nums'>{pendingCount}</CardTitle>
                </CardHeader>
              </Card>
            </div>

            <Card>
              <CardHeader>
                <CardTitle>{t('Withdrawal requests')}</CardTitle>
                <CardDescription>
                  {t(
                    'Process withdrawal requests from users and agents in one place.'
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent className='flex flex-col gap-4 px-0'>
                <div className='flex flex-col gap-3 px-4 sm:flex-row sm:items-end sm:justify-between'>
                  <Tabs
                    value={source ?? 'all'}
                    onValueChange={(value) => {
                      setSource(
                        value === 'all'
                          ? undefined
                          : (value as WithdrawalSource)
                      )
                      setPage(1)
                    }}
                  >
                    <TabsList>
                      <TabsTrigger value='all'>{t('All')}</TabsTrigger>
                      <TabsTrigger value='user'>
                        {t('User Withdrawal')}
                      </TabsTrigger>
                      <TabsTrigger value='agent'>
                        {t('Agent Withdrawal')}
                      </TabsTrigger>
                    </TabsList>
                  </Tabs>
                  <Field className='w-full sm:w-52'>
                    <FieldLabel htmlFor='withdrawal-status-filter'>
                      {t('Status')}
                    </FieldLabel>
                    <NativeSelect
                      id='withdrawal-status-filter'
                      className='w-full'
                      value={status ?? ''}
                      onChange={(event) => {
                        setStatus(
                          event.target.value
                            ? (event.target.value as WithdrawalStatus)
                            : undefined
                        )
                        setPage(1)
                      }}
                    >
                      <NativeSelectOption value=''>
                        {t('All Statuses')}
                      </NativeSelectOption>
                      <NativeSelectOption value='pending'>
                        {t('pending')}
                      </NativeSelectOption>
                      <NativeSelectOption value='approved'>
                        {t('approved')}
                      </NativeSelectOption>
                      <NativeSelectOption value='paid'>
                        {t('paid')}
                      </NativeSelectOption>
                      <NativeSelectOption value='rejected'>
                        {t('rejected')}
                      </NativeSelectOption>
                      <NativeSelectOption value='cancelled'>
                        {t('cancelled')}
                      </NativeSelectOption>
                    </NativeSelect>
                  </Field>
                </div>

                <div className='overflow-x-auto'>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className='pl-4'>{t('Source')}</TableHead>
                        <TableHead>{t('Applicant')}</TableHead>
                        <TableHead>{t('Amount')}</TableHead>
                        <TableHead>{t('Withdrawal Account')}</TableHead>
                        <TableHead>{t('Status')}</TableHead>
                        <TableHead>{t('Created At')}</TableHead>
                        <TableHead className='pr-4 text-right'>
                          {t('Actions')}
                        </TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {withdrawalsQuery.isPending ? (
                        Array.from({ length: 4 }).map((_, index) => (
                          <TableRow key={index}>
                            <TableCell colSpan={7} className='px-4 py-3'>
                              <Skeleton className='h-7 w-full' />
                            </TableCell>
                          </TableRow>
                        ))
                      ) : withdrawalsQuery.isError ? (
                        <TableRow>
                          <TableCell colSpan={7} className='h-40 text-center'>
                            <Button
                              variant='outline'
                              onClick={() => withdrawalsQuery.refetch()}
                            >
                              {t('Retry')}
                            </Button>
                          </TableCell>
                        </TableRow>
                      ) : items.length === 0 ? (
                        <TableRow>
                          <TableCell colSpan={7} className='h-64'>
                            <Empty>
                              <EmptyHeader>
                                <EmptyMedia variant='icon'>
                                  <HugeiconsIcon icon={Wallet03Icon} />
                                </EmptyMedia>
                                <EmptyTitle>{t('No Withdrawals')}</EmptyTitle>
                                <EmptyDescription>
                                  {t(
                                    'No withdrawal requests match the filters.'
                                  )}
                                </EmptyDescription>
                              </EmptyHeader>
                            </Empty>
                          </TableCell>
                        </TableRow>
                      ) : (
                        items.map((item) => {
                          const isPending = item.status === 'pending'
                          const isApproved = item.status === 'approved'
                          return (
                            <TableRow key={`${item.source}-${item.id}`}>
                              <TableCell className='pl-4'>
                                <Badge variant='outline'>
                                  {item.source === 'user'
                                    ? t('User Withdrawal')
                                    : t('Agent Withdrawal')}
                                </Badge>
                              </TableCell>
                              <TableCell>
                                <div className='flex flex-col'>
                                  <span className='font-medium'>
                                    {item.applicant_name || '-'}
                                  </span>
                                  <span className='text-muted-foreground text-xs'>
                                    #{item.applicant_id}
                                  </span>
                                </div>
                              </TableCell>
                              <TableCell className='font-medium tabular-nums'>
                                {formatWithdrawalAmount(item)}
                              </TableCell>
                              <TableCell className='max-w-72'>
                                <LongText className='max-w-72'>
                                  {item.account_info}
                                </LongText>
                              </TableCell>
                              <TableCell>
                                <Badge variant={statusVariant(item.status)}>
                                  {t(item.status)}
                                </Badge>
                              </TableCell>
                              <TableCell className='text-muted-foreground whitespace-nowrap'>
                                {formatTimestampToDate(item.created_at)}
                              </TableCell>
                              <TableCell className='pr-4 text-right'>
                                <div className='flex justify-end gap-2'>
                                  <Button
                                    size='sm'
                                    variant='outline'
                                    disabled={!isPending}
                                    onClick={() =>
                                      beginReview(item, 'approved')
                                    }
                                  >
                                    {t('Approve')}
                                  </Button>
                                  <Button
                                    size='sm'
                                    disabled={!isApproved}
                                    onClick={() => beginReview(item, 'paid')}
                                  >
                                    {t('Mark Paid')}
                                  </Button>
                                  <Button
                                    size='sm'
                                    variant='outline'
                                    disabled={!isPending && !isApproved}
                                    onClick={() =>
                                      beginReview(item, 'rejected')
                                    }
                                  >
                                    {t('Reject')}
                                  </Button>
                                </div>
                              </TableCell>
                            </TableRow>
                          )
                        })
                      )}
                    </TableBody>
                  </Table>
                </div>

                <div className='flex items-center justify-between gap-3 px-4'>
                  <span className='text-muted-foreground text-sm'>
                    {t('Page {{page}} of {{total}}', {
                      page,
                      total: pageCount,
                    })}
                  </span>
                  <div className='flex gap-2'>
                    <Button
                      variant='outline'
                      size='icon-sm'
                      disabled={page <= 1 || withdrawalsQuery.isFetching}
                      onClick={() => setPage((value) => value - 1)}
                      aria-label={t('Previous')}
                    >
                      <HugeiconsIcon icon={ArrowLeft01Icon} />
                    </Button>
                    <Button
                      variant='outline'
                      size='icon-sm'
                      disabled={
                        page >= pageCount || withdrawalsQuery.isFetching
                      }
                      onClick={() => setPage((value) => value + 1)}
                      aria-label={t('Next')}
                    >
                      <HugeiconsIcon icon={ArrowRight01Icon} />
                    </Button>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <Dialog
        open={review !== null}
        onOpenChange={(open) => {
          if (!open && !updateMutation.isPending) {
            setReview(null)
            setAdminRemark('')
          }
        }}
      >
        <DialogContent className='sm:max-w-md'>
          <DialogHeader>
            <DialogTitle className='flex items-center gap-2'>
              <HugeiconsIcon icon={CheckmarkCircle02Icon} className='size-5' />
              {t('Process withdrawal')}
            </DialogTitle>
            <DialogDescription>
              {review
                ? t('Update withdrawal #{{id}} to {{status}}.', {
                    id: review.item.id,
                    status: t(review.nextStatus),
                  })
                : null}
            </DialogDescription>
          </DialogHeader>
          <Field data-disabled={updateMutation.isPending || undefined}>
            <FieldLabel htmlFor='withdrawal-admin-remark'>
              {t('Admin Remark')}
            </FieldLabel>
            <Textarea
              id='withdrawal-admin-remark'
              value={adminRemark}
              onChange={(event) => setAdminRemark(event.target.value)}
              maxLength={255}
              disabled={updateMutation.isPending}
              placeholder={t('Optional')}
            />
          </Field>
          <DialogFooter>
            <Button
              variant='outline'
              disabled={updateMutation.isPending}
              onClick={() => setReview(null)}
            >
              {t('Cancel')}
            </Button>
            <Button
              disabled={!review || updateMutation.isPending}
              onClick={() => {
                if (!review) return
                updateMutation.mutate({
                  source: review.item.source,
                  withdrawalId: review.item.id,
                  status: review.nextStatus,
                  adminRemark,
                })
              }}
            >
              {updateMutation.isPending ? (
                <Spinner data-icon='inline-start' />
              ) : null}
              {t('Confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
