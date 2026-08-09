import { type FormEvent, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowLeft01Icon,
  ArrowRight01Icon,
  CustomerSupportIcon,
  Clock01Icon,
  GiftIcon,
  Link01Icon,
  MoneyReceiveCircleIcon,
  MoneySend02Icon,
  UserMultiple02Icon,
  Wallet01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { getSelf } from '@/lib/api'
import {
  formatQuota,
  formatTimestampToDate,
  parseQuotaFromDollars,
  quotaUnitsToDollars,
} from '@/lib/format'
import { useOpenCustomerService } from '@/hooks/use-open-customer-service'
import { useSystemConfig } from '@/hooks/use-system-config'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
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
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
  InputGroupTextarea,
} from '@/components/ui/input-group'
import { Separator } from '@/components/ui/separator'
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { CopyButton } from '@/components/copy-button'
import { SectionPageLayout } from '@/components/layout'
import {
  formatInviteRegisterReward,
  formatInviteRewardRatio,
} from '@/features/invite/lib/reward-config'
import type { AffiliateRewardPolicy } from '@/features/invite/types'
import { useAffiliate } from '@/features/wallet/hooks/use-affiliate'
import { WithdrawalDialog } from '@/features/withdrawals/components/withdrawal-dialog'
import {
  cancelAffiliateWithdrawal,
  getInviteRewardsApiError,
  getAffiliateInvites,
  getAffiliateWithdrawals,
  submitAffiliateWithdrawal,
  transferAffiliateRewards,
} from './api'
import type {
  AffiliateInvitePage,
  AffiliateInviteRecord,
  AffiliateInviteStats,
  AffiliateWithdrawal,
  AffiliateWithdrawalPage,
  AffiliateWithdrawalStatus,
} from './types'

const PAGE_SIZE = 10
const WITHDRAWAL_PAGE_SIZE = 10

export function InviteRewards() {
  const { t } = useTranslation()
  const { systemName, currency } = useSystemConfig()
  const openCustomerService = useOpenCustomerService()
  const queryClient = useQueryClient()
  const setUser = useAuthStore((state) => state.auth.setUser)
  const [page, setPage] = useState(1)
  const [withdrawalPage, setWithdrawalPage] = useState(1)
  const [transferOpen, setTransferOpen] = useState(false)
  const [withdrawalOpen, setWithdrawalOpen] = useState(false)
  const [recordsTab, setRecordsTab] = useState('invites')
  const { affiliateLink, loading: linkLoading } = useAffiliate()
  const inviteQuery = useQuery({
    queryKey: ['affiliate', 'invites', page, PAGE_SIZE],
    queryFn: () => getAffiliateInvites(page, PAGE_SIZE),
    placeholderData: (previousData) => previousData,
  })
  const withdrawalsQuery = useQuery({
    queryKey: [
      'affiliate',
      'withdrawals',
      withdrawalPage,
      WITHDRAWAL_PAGE_SIZE,
    ],
    queryFn: () =>
      getAffiliateWithdrawals(withdrawalPage, WITHDRAWAL_PAGE_SIZE),
    placeholderData: (previousData) => previousData,
  })

  const refreshRewardAccount = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['affiliate', 'invites'] }),
      queryClient.invalidateQueries({
        queryKey: ['affiliate', 'withdrawals'],
      }),
    ])
    const self = await getSelf()
    if (self?.success && self.data) {
      setUser(self.data)
    }
  }

  const transferMutation = useMutation({
    mutationFn: transferAffiliateRewards,
    onSuccess: async () => {
      setTransferOpen(false)
      toast.success(t('Rewards transferred to balance'))
      await refreshRewardAccount()
    },
    onError: (error) => {
      toast.error(getInviteRewardsApiError(error, t('Transfer failed')))
    },
  })
  const withdrawalMutation = useMutation({
    mutationFn: submitAffiliateWithdrawal,
    onSuccess: async () => {
      setWithdrawalOpen(false)
      setWithdrawalPage(1)
      setRecordsTab('withdrawals')
      toast.success(t('Withdrawal submitted'))
      await refreshRewardAccount()
    },
    onError: (error) => {
      toast.error(
        getInviteRewardsApiError(error, t('Failed to submit withdrawal'))
      )
    },
  })
  const cancelWithdrawalMutation = useMutation({
    mutationFn: cancelAffiliateWithdrawal,
    onSuccess: async () => {
      toast.success(t('Withdrawal cancelled'))
      await refreshRewardAccount()
    },
    onError: (error) => {
      toast.error(
        getInviteRewardsApiError(error, t('Failed to cancel withdrawal'))
      )
    },
  })
  const data = inviteQuery.data
  const pageCount = Math.max(1, Math.ceil((data?.total ?? 0) / PAGE_SIZE))
  const withdrawals = withdrawalsQuery.data
  const withdrawalPageCount = Math.max(
    1,
    Math.ceil((withdrawals?.total ?? 0) / WITHDRAWAL_PAGE_SIZE)
  )
  const availableRewardQuota = data?.stats.available_reward_quota ?? 0
  const minimumRewardActionQuota =
    data?.minimum_reward_action_quota ?? currency.quotaPerUnit

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Invite Rewards')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <div className='mx-auto flex w-full max-w-7xl flex-col gap-4 sm:gap-5'>
            <div className='grid items-stretch gap-4 lg:grid-cols-[minmax(0,1.35fr)_minmax(320px,0.65fr)]'>
              <InviteOverviewCard
                siteName={systemName}
                affiliateLink={affiliateLink}
                linkLoading={linkLoading}
                stats={data?.stats}
                statsLoading={inviteQuery.isPending}
                availableQuota={availableRewardQuota}
                accountLoading={!data}
                minimumQuota={minimumRewardActionQuota}
                onTransfer={() => setTransferOpen(true)}
                onWithdraw={() => setWithdrawalOpen(true)}
              />
              <RewardPolicyCard
                policy={data?.affiliate_policy}
                loading={inviteQuery.isPending}
                onApply={openCustomerService}
              />
            </div>

            <RewardRecordsCard
              activeTab={recordsTab}
              onTabChange={setRecordsTab}
              inviteData={data}
              inviteLoading={inviteQuery.isPending}
              inviteError={inviteQuery.isError}
              inviteFetching={inviteQuery.isFetching}
              invitePage={page}
              invitePageCount={pageCount}
              onInviteRetry={() => inviteQuery.refetch()}
              onInvitePageChange={setPage}
              withdrawalData={withdrawals}
              withdrawalLoading={withdrawalsQuery.isPending}
              withdrawalError={withdrawalsQuery.isError}
              withdrawalFetching={withdrawalsQuery.isFetching}
              withdrawalPage={withdrawalPage}
              withdrawalPageCount={withdrawalPageCount}
              cancellingId={
                cancelWithdrawalMutation.isPending
                  ? cancelWithdrawalMutation.variables
                  : undefined
              }
              onWithdrawalRetry={() => withdrawalsQuery.refetch()}
              onWithdrawalPageChange={setWithdrawalPage}
              onCancelWithdrawal={(id) => cancelWithdrawalMutation.mutate(id)}
            />
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <RewardTransferDialog
        key={transferOpen ? 'transfer-open' : 'transfer-closed'}
        open={transferOpen}
        onOpenChange={setTransferOpen}
        availableQuota={availableRewardQuota}
        minimumQuota={minimumRewardActionQuota}
        pending={transferMutation.isPending}
        onSubmit={(quota) => transferMutation.mutate(quota)}
      />
      <WithdrawalDialog
        key={withdrawalOpen ? 'withdraw-open' : 'withdraw-closed'}
        open={withdrawalOpen}
        onOpenChange={setWithdrawalOpen}
        availableAmount={quotaUnitsToDollars(availableRewardQuota)}
        minimumAmount={quotaUnitsToDollars(minimumRewardActionQuota)}
        formatAmount={(amount) => formatQuota(parseQuotaFromDollars(amount))}
        pending={withdrawalMutation.isPending}
        onSubmit={(amount, accountInfo) =>
          withdrawalMutation.mutate({
            amount_quota: parseQuotaFromDollars(amount),
            account_info: accountInfo,
          })
        }
      />
    </>
  )
}

function RewardTransferDialog({
  open,
  onOpenChange,
  availableQuota,
  minimumQuota,
  pending,
  onSubmit,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  availableQuota: number
  minimumQuota: number
  pending: boolean
  onSubmit: (amountQuota: number) => void
}) {
  const { t } = useTranslation()
  const [amount, setAmount] = useState(() =>
    String(quotaUnitsToDollars(minimumQuota))
  )
  const numericAmount = Number(amount)
  const amountQuota = parseQuotaFromDollars(numericAmount)
  const amountError =
    amount.trim() === '' || !Number.isFinite(numericAmount) || amountQuota <= 0
      ? t('Amount must be greater than 0')
      : amountQuota < minimumQuota
        ? t('Minimum action amount: {{amount}}', {
            amount: formatQuota(minimumQuota),
          })
        : amountQuota > availableQuota
          ? t('Amount exceeds available rewards')
          : null
  const amountInvalid = amountError !== null
  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (amountInvalid || pending) return
    onSubmit(amountQuota)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-md'>
        <form
          className='flex flex-col gap-4'
          onSubmit={handleSubmit}
          noValidate
        >
          <DialogHeader>
            <DialogTitle className='flex items-center gap-2'>
              <HugeiconsIcon icon={Wallet01Icon} className='size-5' />
              {t('Transfer to balance')}
            </DialogTitle>
            <DialogDescription>
              {t('Move rewards from the reward account to your main balance.')}
            </DialogDescription>
          </DialogHeader>

          <FieldGroup className='py-2'>
            <Field
              data-invalid={amountInvalid || undefined}
              data-disabled={pending || undefined}
            >
              <FieldLabel htmlFor='transfer-reward-amount'>
                {t('Amount')}
              </FieldLabel>
              <Input
                id='transfer-reward-amount'
                type='number'
                min={quotaUnitsToDollars(minimumQuota)}
                max={quotaUnitsToDollars(availableQuota)}
                step='any'
                value={amount}
                onChange={(event) => setAmount(event.target.value)}
                aria-invalid={amountInvalid || undefined}
                disabled={pending}
                autoFocus
              />
              <FieldDescription>
                {t('Available: {{amount}} · Minimum: {{minimum}}', {
                  amount: formatQuota(availableQuota),
                  minimum: formatQuota(minimumQuota),
                })}
              </FieldDescription>
              {amountError ? <FieldError>{amountError}</FieldError> : null}
            </Field>
          </FieldGroup>

          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              onClick={() => onOpenChange(false)}
              disabled={pending}
            >
              {t('Cancel')}
            </Button>
            <Button type='submit' disabled={amountInvalid || pending}>
              {pending ? <Spinner data-icon='inline-start' /> : null}
              {t('Confirm transfer')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function RewardRecordsCard({
  activeTab,
  onTabChange,
  inviteData,
  inviteLoading,
  inviteError,
  inviteFetching,
  invitePage,
  invitePageCount,
  onInviteRetry,
  onInvitePageChange,
  withdrawalData,
  withdrawalLoading,
  withdrawalError,
  withdrawalFetching,
  withdrawalPage,
  withdrawalPageCount,
  cancellingId,
  onWithdrawalRetry,
  onWithdrawalPageChange,
  onCancelWithdrawal,
}: {
  activeTab: string
  onTabChange: (tab: string) => void
  inviteData?: AffiliateInvitePage
  inviteLoading: boolean
  inviteError: boolean
  inviteFetching: boolean
  invitePage: number
  invitePageCount: number
  onInviteRetry: () => void
  onInvitePageChange: (page: number) => void
  withdrawalData?: AffiliateWithdrawalPage
  withdrawalLoading: boolean
  withdrawalError: boolean
  withdrawalFetching: boolean
  withdrawalPage: number
  withdrawalPageCount: number
  cancellingId?: number
  onWithdrawalRetry: () => void
  onWithdrawalPageChange: (page: number) => void
  onCancelWithdrawal: (id: number) => void
}) {
  const { t } = useTranslation()

  return (
    <Card>
      <Tabs value={activeTab} onValueChange={onTabChange} className='gap-0'>
        <CardHeader className='border-b'>
          <CardTitle>{t('Reward records')}</CardTitle>
          <CardAction className='col-span-2 col-start-1 row-span-1 row-start-2 mt-2 justify-self-stretch sm:col-span-1 sm:col-start-2 sm:row-start-1 sm:mt-0 sm:justify-self-end'>
            <TabsList className='grid w-full grid-cols-2 sm:w-auto'>
              <TabsTrigger value='invites'>
                {t('Invites')}
                {inviteData ? (
                  <Badge variant='secondary' className='hidden sm:inline-flex'>
                    {inviteData.total}
                  </Badge>
                ) : null}
              </TabsTrigger>
              <TabsTrigger value='withdrawals'>
                {t('Withdrawals')}
                {withdrawalData ? (
                  <Badge variant='secondary' className='hidden sm:inline-flex'>
                    {withdrawalData.total}
                  </Badge>
                ) : null}
              </TabsTrigger>
            </TabsList>
          </CardAction>
        </CardHeader>

        <TabsContent value='invites'>
          <CardContent className='px-0'>
            {inviteLoading ? (
              <RecordsSkeleton />
            ) : inviteError ? (
              <Empty className='border-0 py-12'>
                <EmptyHeader>
                  <EmptyMedia variant='icon'>
                    <HugeiconsIcon icon={GiftIcon} />
                  </EmptyMedia>
                  <EmptyTitle>{t('Unable to load invite records')}</EmptyTitle>
                  <EmptyDescription>
                    {t('Please try again in a moment.')}
                  </EmptyDescription>
                </EmptyHeader>
                <EmptyContent>
                  <Button variant='outline' onClick={onInviteRetry}>
                    {t('Retry')}
                  </Button>
                </EmptyContent>
              </Empty>
            ) : inviteData?.items.length ? (
              <>
                <div className='hidden sm:block'>
                  <InviteRecordsTable records={inviteData.items} />
                </div>
                <div className='sm:hidden'>
                  <InviteRecordsList records={inviteData.items} />
                </div>
              </>
            ) : (
              <Empty className='border-0 py-12'>
                <EmptyHeader>
                  <EmptyMedia variant='icon'>
                    <HugeiconsIcon icon={UserMultiple02Icon} />
                  </EmptyMedia>
                  <EmptyTitle>{t('No invited users yet')}</EmptyTitle>
                  <EmptyDescription>
                    {t(
                      'Share your invite link to see registrations and rewards here.'
                    )}
                  </EmptyDescription>
                </EmptyHeader>
              </Empty>
            )}
          </CardContent>
          {inviteData && inviteData.total > PAGE_SIZE ? (
            <RecordsPagination
              page={invitePage}
              pageCount={invitePageCount}
              fetching={inviteFetching}
              onPageChange={onInvitePageChange}
            />
          ) : null}
        </TabsContent>

        <TabsContent value='withdrawals'>
          <CardContent className='px-0'>
            {withdrawalLoading ? (
              <RecordsSkeleton />
            ) : withdrawalError ? (
              <Empty className='border-0 py-10'>
                <EmptyHeader>
                  <EmptyMedia variant='icon'>
                    <HugeiconsIcon icon={MoneySend02Icon} />
                  </EmptyMedia>
                  <EmptyTitle>
                    {t('Unable to load withdrawal records')}
                  </EmptyTitle>
                </EmptyHeader>
                <EmptyContent>
                  <Button variant='outline' onClick={onWithdrawalRetry}>
                    {t('Retry')}
                  </Button>
                </EmptyContent>
              </Empty>
            ) : withdrawalData?.items.length ? (
              <>
                <div className='hidden sm:block'>
                  <WithdrawalTable
                    records={withdrawalData.items}
                    cancellingId={cancellingId}
                    onCancel={onCancelWithdrawal}
                  />
                </div>
                <div className='sm:hidden'>
                  <WithdrawalList
                    records={withdrawalData.items}
                    cancellingId={cancellingId}
                    onCancel={onCancelWithdrawal}
                  />
                </div>
              </>
            ) : (
              <Empty className='border-0 py-10'>
                <EmptyHeader>
                  <EmptyMedia variant='icon'>
                    <HugeiconsIcon icon={MoneySend02Icon} />
                  </EmptyMedia>
                  <EmptyTitle>{t('No Withdrawals')}</EmptyTitle>
                  <EmptyDescription>
                    {t('Submitted withdrawals will appear here.')}
                  </EmptyDescription>
                </EmptyHeader>
              </Empty>
            )}
          </CardContent>
          {withdrawalData && withdrawalData.total > WITHDRAWAL_PAGE_SIZE ? (
            <RecordsPagination
              page={withdrawalPage}
              pageCount={withdrawalPageCount}
              fetching={withdrawalFetching}
              onPageChange={onWithdrawalPageChange}
            />
          ) : null}
        </TabsContent>
      </Tabs>
    </Card>
  )
}

function RecordsPagination({
  page,
  pageCount,
  fetching,
  onPageChange,
}: {
  page: number
  pageCount: number
  fetching: boolean
  onPageChange: (page: number) => void
}) {
  const { t } = useTranslation()
  return (
    <CardFooter className='justify-between gap-3'>
      <span className='text-muted-foreground text-xs tabular-nums'>
        {t('Page {{page}} of {{total}}', { page, total: pageCount })}
      </span>
      <div className='flex gap-2'>
        <Button
          variant='outline'
          size='icon-sm'
          disabled={page <= 1 || fetching}
          onClick={() => onPageChange(page - 1)}
          aria-label={t('Previous')}
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} />
        </Button>
        <Button
          variant='outline'
          size='icon-sm'
          disabled={page >= pageCount || fetching}
          onClick={() => onPageChange(page + 1)}
          aria-label={t('Next')}
        >
          <HugeiconsIcon icon={ArrowRight01Icon} />
        </Button>
      </div>
    </CardFooter>
  )
}

function WithdrawalTable({
  records,
  cancellingId,
  onCancel,
}: {
  records: AffiliateWithdrawal[]
  cancellingId?: number
  onCancel: (id: number) => void
}) {
  const { t } = useTranslation()
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead className='pl-4'>{t('Amount')}</TableHead>
          <TableHead>{t('Status')}</TableHead>
          <TableHead>{t('Withdrawal Account')}</TableHead>
          <TableHead>{t('Created At')}</TableHead>
          <TableHead className='pr-4 text-right'>{t('Actions')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {records.map((record) => (
          <TableRow key={record.id}>
            <TableCell className='pl-4 font-medium tabular-nums'>
              {formatQuota(record.amount_quota)}
            </TableCell>
            <TableCell>
              <div className='flex flex-col items-start gap-1'>
                <WithdrawalStatusBadge status={record.status} />
                {record.admin_remark ? (
                  <span className='text-muted-foreground max-w-56 text-xs break-words'>
                    {record.admin_remark}
                  </span>
                ) : null}
              </div>
            </TableCell>
            <TableCell className='text-muted-foreground max-w-72 truncate'>
              {record.account_info}
            </TableCell>
            <TableCell className='text-muted-foreground whitespace-nowrap'>
              {formatTimestampToDate(record.created_at)}
            </TableCell>
            <TableCell className='pr-4 text-right'>
              {record.status === 'pending' ? (
                <Button
                  variant='ghost'
                  size='sm'
                  disabled={cancellingId !== undefined}
                  onClick={() => onCancel(record.id)}
                >
                  {cancellingId === record.id ? <Spinner /> : null}
                  {t('Cancel')}
                </Button>
              ) : (
                <span className='text-muted-foreground'>-</span>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

function WithdrawalList({
  records,
  cancellingId,
  onCancel,
}: {
  records: AffiliateWithdrawal[]
  cancellingId?: number
  onCancel: (id: number) => void
}) {
  const { t } = useTranslation()
  return (
    <div className='divide-y'>
      {records.map((record) => (
        <div className='space-y-3 px-4 py-4' key={record.id}>
          <div className='flex items-center justify-between gap-3'>
            <span className='font-semibold tabular-nums'>
              {formatQuota(record.amount_quota)}
            </span>
            <WithdrawalStatusBadge status={record.status} />
          </div>
          <div className='text-muted-foreground text-xs break-words'>
            {record.account_info}
          </div>
          {record.admin_remark ? (
            <div className='text-muted-foreground text-xs break-words'>
              {t('Admin Remark')}: {record.admin_remark}
            </div>
          ) : null}
          <div className='flex items-center justify-between gap-3'>
            <span className='text-muted-foreground text-xs'>
              {formatTimestampToDate(record.created_at)}
            </span>
            {record.status === 'pending' ? (
              <Button
                variant='ghost'
                size='sm'
                disabled={cancellingId !== undefined}
                onClick={() => onCancel(record.id)}
              >
                {cancellingId === record.id ? <Spinner /> : null}
                {t('Cancel')}
              </Button>
            ) : null}
          </div>
        </div>
      ))}
    </div>
  )
}

function WithdrawalStatusBadge({
  status,
}: {
  status: AffiliateWithdrawalStatus
}) {
  const { t } = useTranslation()
  const labels: Record<AffiliateWithdrawalStatus, string> = {
    pending: t('Pending review'),
    approved: t('Approved'),
    paid: t('Paid'),
    rejected: t('Rejected'),
    cancelled: t('Cancelled'),
  }
  const variant =
    status === 'paid'
      ? 'default'
      : status === 'approved'
        ? 'secondary'
        : status === 'rejected'
          ? 'destructive'
          : 'outline'
  return <Badge variant={variant}>{labels[status]}</Badge>
}

function InviteOverviewCard({
  siteName,
  affiliateLink,
  linkLoading,
  stats,
  statsLoading,
  availableQuota,
  accountLoading,
  minimumQuota,
  onTransfer,
  onWithdraw,
}: {
  siteName: string
  affiliateLink: string
  linkLoading: boolean
  stats?: AffiliateInviteStats
  statsLoading: boolean
  availableQuota: number
  accountLoading: boolean
  minimumQuota: number
  onTransfer: () => void
  onWithdraw: () => void
}) {
  const { t } = useTranslation()
  const inviteCode = getAffiliateCode(affiliateLink)
  const referralSiteName = getReferralSiteName(siteName)
  const referralMessage = t(
    `🎁 Recommend friends to register for {{siteName}} and enjoy free AI credits upon registration! Discounts starting at 90% off

Use my exclusive invite code [{{inviteCode}}], or click the link to register directly. Both of us can receive credit rewards.

The platform supports leading AI models including GPT-5, Claude, Gemini, and DeepSeek. One API key can access all models at excellent prices.

Register now 👉 {{affiliateLink}}`,
    { siteName: referralSiteName, inviteCode, affiliateLink }
  )
  return (
    <Card className='h-full'>
      <CardHeader className='border-b'>
        <CardTitle className='flex items-center gap-2'>
          <span className='bg-primary/10 text-primary flex size-8 items-center justify-center rounded-lg'>
            <HugeiconsIcon icon={Wallet01Icon} className='size-4' />
          </span>
          {t('Reward account')}
        </CardTitle>
        {minimumQuota > 0 ? (
          <CardDescription>
            {t('Minimum action amount: {{amount}}', {
              amount: formatQuota(minimumQuota),
            })}
          </CardDescription>
        ) : null}
        <CardAction className='col-span-2 col-start-1 row-span-1 row-start-3 mt-3 justify-self-stretch sm:col-span-1 sm:col-start-2 sm:row-span-2 sm:row-start-1 sm:mt-0 sm:justify-self-end'>
          <div className='flex flex-wrap gap-2'>
            <Button
              variant='outline'
              size='sm'
              disabled={accountLoading}
              onClick={onTransfer}
            >
              <HugeiconsIcon icon={Wallet01Icon} data-icon='inline-start' />
              {t('Transfer to balance')}
            </Button>
            <Button size='sm' disabled={accountLoading} onClick={onWithdraw}>
              <HugeiconsIcon icon={MoneySend02Icon} data-icon='inline-start' />
              {t('Request withdrawal')}
            </Button>
          </div>
        </CardAction>
      </CardHeader>
      <CardContent className='flex flex-1 flex-col gap-5'>
        {statsLoading || accountLoading ? (
          <StatsSkeleton />
        ) : (
          <StatsBand stats={stats} availableQuota={availableQuota} />
        )}

        <Separator />

        <div className='mt-auto flex flex-col gap-3'>
          <div className='flex items-center gap-2'>
            <span className='bg-primary/10 text-primary flex size-8 items-center justify-center rounded-lg'>
              <HugeiconsIcon icon={Link01Icon} className='size-4' />
            </span>
            <span className='font-medium'>{t('Share your invite link')}</span>
          </div>
          {linkLoading ? (
            <>
              <Skeleton className='h-48 w-full' />
              <Skeleton className='h-10 w-full' />
            </>
          ) : (
            <>
              <InputGroup className='h-auto items-stretch'>
                <InputGroupTextarea
                  value={referralMessage}
                  readOnly
                  aria-label={t('Referral message')}
                  className='max-h-none min-h-44 overflow-y-auto text-sm leading-6 sm:max-h-56'
                />
                <InputGroupAddon
                  align='block-end'
                  className='justify-end border-t'
                >
                  <CopyButton
                    value={referralMessage}
                    size='sm'
                    tooltip={t('Copy to clipboard')}
                    aria-label={t('Copy to clipboard')}
                  >
                    {t('Copy')}
                  </CopyButton>
                </InputGroupAddon>
              </InputGroup>
              <div className='flex flex-col gap-2'>
                <span className='text-muted-foreground text-xs'>
                  {t('Invite link')}
                </span>
                <InputGroup className='h-10'>
                  <InputGroupInput
                    value={affiliateLink}
                    readOnly
                    aria-label={t('Invite link')}
                    className='font-mono text-xs sm:text-sm'
                  />
                  <InputGroupAddon align='inline-end'>
                    <CopyButton
                      value={affiliateLink}
                      size='icon'
                      tooltip={t('Copy referral link')}
                      aria-label={t('Copy referral link')}
                    />
                  </InputGroupAddon>
                </InputGroup>
              </div>
            </>
          )}
        </div>
      </CardContent>
    </Card>
  )
}

function getAffiliateCode(affiliateLink: string) {
  if (!affiliateLink) return ''
  try {
    return new URL(affiliateLink).searchParams.get('aff') ?? ''
  } catch {
    return ''
  }
}

function getReferralSiteName(siteName: string) {
  const normalizedName = siteName.trim()
  if (!normalizedName || /api$/i.test(normalizedName)) return normalizedName
  return `${normalizedName} API`
}

function StatsBand({
  stats,
  availableQuota,
}: {
  stats?: AffiliateInviteStats
  availableQuota: number
}) {
  const { t } = useTranslation()
  const items = [
    {
      label: t('Available rewards'),
      value: formatQuota(availableQuota),
      icon: Wallet01Icon,
    },
    {
      label: t('Invited friends'),
      value: String(stats?.invite_count ?? 0),
      icon: UserMultiple02Icon,
    },
    {
      label: t('Pending settlement'),
      value: formatQuota(stats?.pending_topup_reward_quota ?? 0),
      icon: Clock01Icon,
    },
    {
      label: t('Total rewards earned'),
      value: formatQuota(stats?.total_reward_quota ?? 0),
      icon: MoneyReceiveCircleIcon,
    },
  ]

  return (
    <div className='bg-border grid grid-cols-2 gap-px overflow-hidden rounded-lg border sm:grid-cols-4'>
      {items.map((item) => (
        <div key={item.label} className='bg-muted/50 min-w-0 p-3'>
          <div className='text-muted-foreground flex items-center gap-1.5 text-xs'>
            <HugeiconsIcon icon={item.icon} className='size-3.5 shrink-0' />
            <span className='truncate'>{item.label}</span>
          </div>
          <div className='mt-2 truncate text-lg font-semibold tabular-nums'>
            {item.value}
          </div>
        </div>
      ))}
    </div>
  )
}

function RewardPolicyCard({
  policy,
  loading,
  onApply,
}: {
  policy?: AffiliateRewardPolicy
  loading: boolean
  onApply: () => void
}) {
  const { t } = useTranslation()
  const rewardRatio = policy?.topup_reward_ratio ?? 0
  const rewardLimit = policy?.topup_reward_limit ?? 0
  const consumeRewardRatio = policy?.consume_reward_ratio ?? 0
  const inviterRewardQuota = policy?.inviter_reward_quota ?? 0
  const inviteeRewardQuota = policy?.invitee_reward_quota ?? 0
  const topupLimit =
    rewardLimit > 0
      ? t('First {{count}} top-ups', { count: rewardLimit })
      : t('Unlimited top-up rewards')
  const policies = []
  if (inviterRewardQuota > 0) {
    policies.push({
      label: t('Registration reward'),
      value: formatInviteRegisterReward(inviterRewardQuota),
      icon: GiftIcon,
    })
  }
  if (inviteeRewardQuota > 0) {
    policies.push({
      label: t('Invitee registration reward'),
      value: formatInviteRegisterReward(inviteeRewardQuota),
      icon: UserMultiple02Icon,
    })
  }
  if (rewardRatio > 0) {
    policies.push({
      label: t('Top-up reward'),
      value: formatInviteRewardRatio(rewardRatio),
      icon: MoneyReceiveCircleIcon,
    })
    policies.push({
      label: t('Rewarded top-ups'),
      value: topupLimit,
      icon: UserMultiple02Icon,
    })
  }
  if (consumeRewardRatio > 0) {
    policies.push({
      label: t('Consumption reward'),
      value: formatInviteRewardRatio(consumeRewardRatio),
      icon: MoneyReceiveCircleIcon,
    })
  }
  if (rewardRatio > 0 || consumeRewardRatio > 0) {
    policies.push({
      label: t('Reward settlement'),
      value: t('24 hours'),
      icon: Clock01Icon,
    })
  }

  return (
    <Card className='h-full'>
      <CardHeader className='border-b'>
        <CardTitle>{t('Reward Rules')}</CardTitle>
        <CardDescription>
          {t('Rewards are calculated automatically under the current policy.')}
        </CardDescription>
        <CardAction>
          <Badge variant='secondary'>
            {policy?.uses_default_role
              ? t('System default')
              : policy?.role_name || t('System default')}
          </Badge>
        </CardAction>
      </CardHeader>
      <CardContent className='flex flex-1 flex-col gap-4'>
        {loading ? (
          <PolicySkeleton />
        ) : (
          <div className='divide-y'>
            {policies.map((policy) => (
              <div
                key={policy.label}
                className='flex min-h-14 items-center justify-between gap-4 py-2.5 first:pt-0 last:pb-0'
              >
                <div className='text-muted-foreground flex min-w-0 items-center gap-2 text-sm'>
                  <HugeiconsIcon
                    icon={policy.icon}
                    className='size-4 shrink-0'
                  />
                  <span>{policy.label}</span>
                </div>
                <span className='max-w-[58%] text-right text-sm font-semibold tabular-nums'>
                  {policy.value}
                </span>
              </div>
            ))}
          </div>
        )}
        <Alert className='bg-muted/40 mt-auto border-0'>
          <AlertDescription className='text-xs leading-5'>
            {t(
              'Do not invite yourself with alternate accounts. Violations will result in reward recovery and serious cases may be banned.'
            )}
          </AlertDescription>
        </Alert>
      </CardContent>
      <CardFooter>
        <Button className='w-full' variant='outline' onClick={onApply}>
          <HugeiconsIcon icon={CustomerSupportIcon} data-icon='inline-start' />
          {t('Apply to become a brand partner')}
        </Button>
      </CardFooter>
    </Card>
  )
}

function InviteRecordsTable({ records }: { records: AffiliateInviteRecord[] }) {
  const { t } = useTranslation()
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead className='pl-4'>{t('Invited user')}</TableHead>
          <TableHead>{t('Joined')}</TableHead>
          <TableHead className='text-right'>{t('Rewarded top-ups')}</TableHead>
          <TableHead className='text-right'>{t('Rewards received')}</TableHead>
          <TableHead className='pr-4 text-right'>
            {t('Pending settlement')}
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {records.map((record) => (
          <TableRow key={record.invitee_id}>
            <TableCell className='pl-4'>
              <UserIdentity record={record} />
            </TableCell>
            <TableCell className='text-muted-foreground'>
              {formatTimestampToDate(record.created_at)}
            </TableCell>
            <TableCell className='text-right tabular-nums'>
              {record.reward_count}
            </TableCell>
            <TableCell className='text-right font-medium tabular-nums'>
              {formatQuota(record.completed_reward_quota)}
            </TableCell>
            <TableCell className='pr-4 text-right tabular-nums'>
              {formatQuota(record.pending_reward_quota)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

function InviteRecordsList({ records }: { records: AffiliateInviteRecord[] }) {
  const { t } = useTranslation()
  return (
    <div className='flex flex-col'>
      {records.map((record, index) => (
        <div key={record.invitee_id}>
          {index > 0 ? <Separator /> : null}
          <div className='flex flex-col gap-3 px-4 py-4'>
            <div className='flex items-start justify-between gap-3'>
              <UserIdentity record={record} />
              <span className='text-muted-foreground shrink-0 text-xs'>
                {formatTimestampToDate(record.created_at)}
              </span>
            </div>
            <div className='grid grid-cols-3 gap-2'>
              <RecordMetric
                label={t('Rewarded top-ups')}
                value={String(record.reward_count)}
              />
              <RecordMetric
                label={t('Rewards received')}
                value={formatQuota(record.completed_reward_quota)}
              />
              <RecordMetric
                label={t('Pending settlement')}
                value={formatQuota(record.pending_reward_quota)}
              />
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}

function UserIdentity({ record }: { record: AffiliateInviteRecord }) {
  const name = record.display_name || record.username
  return (
    <div className='min-w-0'>
      <div className='truncate font-medium'>{name}</div>
      {record.display_name ? (
        <div className='text-muted-foreground truncate text-xs'>
          @{record.username}
        </div>
      ) : null}
    </div>
  )
}

function RecordMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className='min-w-0'>
      <div className='text-muted-foreground truncate text-xs'>{label}</div>
      <div className='mt-1 truncate text-sm font-medium tabular-nums'>
        {value}
      </div>
    </div>
  )
}

function StatsSkeleton() {
  return (
    <div className='bg-border grid grid-cols-2 gap-px overflow-hidden rounded-lg border sm:grid-cols-4'>
      {Array.from({ length: 4 }).map((_, index) => (
        <div key={index} className='bg-muted/50 flex flex-col gap-2 p-3'>
          <Skeleton className='h-3 w-24' />
          <Skeleton className='h-6 w-20' />
        </div>
      ))}
    </div>
  )
}

function PolicySkeleton() {
  return (
    <div className='divide-y'>
      {Array.from({ length: 4 }).map((_, index) => (
        <div
          key={index}
          className='flex min-h-14 items-center justify-between gap-4 py-2.5 first:pt-0 last:pb-0'
        >
          <Skeleton className='h-4 w-28' />
          <Skeleton className='h-5 w-20' />
        </div>
      ))}
    </div>
  )
}

function RecordsSkeleton() {
  return (
    <div className='flex flex-col gap-4 px-4 py-5'>
      {Array.from({ length: 4 }).map((_, index) => (
        <div key={index} className='flex items-center justify-between gap-4'>
          <div className='flex flex-col gap-2'>
            <Skeleton className='h-4 w-28' />
            <Skeleton className='h-3 w-20' />
          </div>
          <Skeleton className='h-5 w-24' />
        </div>
      ))}
    </div>
  )
}
