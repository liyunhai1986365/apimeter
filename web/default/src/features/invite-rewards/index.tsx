import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  ArrowLeft01Icon,
  ArrowRight01Icon,
  Clock01Icon,
  GiftIcon,
  Link01Icon,
  MoneyReceiveCircleIcon,
  UserMultiple02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
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
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { CopyButton } from '@/components/copy-button'
import { SectionPageLayout } from '@/components/layout'
import {
  formatInviteRegisterReward,
  formatInviteRewardRatio,
} from '@/features/invite/lib/reward-config'
import type { AffiliateRewardPolicy } from '@/features/invite/types'
import { useAffiliate } from '@/features/wallet/hooks/use-affiliate'
import { getAffiliateInvites } from './api'
import type { AffiliateInviteRecord, AffiliateInviteStats } from './types'

const PAGE_SIZE = 10

export function InviteRewards() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const { affiliateLink, loading: linkLoading } = useAffiliate()
  const inviteQuery = useQuery({
    queryKey: ['affiliate', 'invites', page, PAGE_SIZE],
    queryFn: () => getAffiliateInvites(page, PAGE_SIZE),
    placeholderData: (previousData) => previousData,
  })
  const data = inviteQuery.data
  const pageCount = Math.max(1, Math.ceil((data?.total ?? 0) / PAGE_SIZE))

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Invite Rewards')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto flex w-full max-w-7xl flex-col gap-4 sm:gap-5'>
          <div className='grid items-stretch gap-4 lg:grid-cols-[minmax(0,1.35fr)_minmax(320px,0.65fr)]'>
            <InviteOverviewCard
              affiliateLink={affiliateLink}
              linkLoading={linkLoading}
              stats={data?.stats}
              statsLoading={inviteQuery.isPending}
            />
            <RewardPolicyCard
              policy={data?.affiliate_policy}
              loading={inviteQuery.isPending}
            />
          </div>

          <Card>
            <CardHeader>
              <CardTitle>{t('Invite records')}</CardTitle>
              <CardDescription>
                {t('Rewards are grouped by each invited user.')}
              </CardDescription>
              {data ? (
                <CardAction className='text-muted-foreground text-sm tabular-nums'>
                  {t('{{count}} invited', { count: data.total })}
                </CardAction>
              ) : null}
            </CardHeader>
            <CardContent className='px-0'>
              {inviteQuery.isPending ? (
                <RecordsSkeleton />
              ) : inviteQuery.isError ? (
                <Empty className='border-0 py-12'>
                  <EmptyHeader>
                    <EmptyMedia variant='icon'>
                      <HugeiconsIcon icon={GiftIcon} />
                    </EmptyMedia>
                    <EmptyTitle>
                      {t('Unable to load invite records')}
                    </EmptyTitle>
                    <EmptyDescription>
                      {t('Please try again in a moment.')}
                    </EmptyDescription>
                  </EmptyHeader>
                  <EmptyContent>
                    <Button
                      variant='outline'
                      onClick={() => inviteQuery.refetch()}
                    >
                      {t('Retry')}
                    </Button>
                  </EmptyContent>
                </Empty>
              ) : data?.items.length ? (
                <>
                  <div className='hidden sm:block'>
                    <InviteRecordsTable records={data.items} />
                  </div>
                  <div className='sm:hidden'>
                    <InviteRecordsList records={data.items} />
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
            {data && data.total > PAGE_SIZE ? (
              <CardFooter className='justify-between gap-3'>
                <span className='text-muted-foreground text-xs tabular-nums'>
                  {t('Page {{page}} of {{total}}', {
                    page,
                    total: pageCount,
                  })}
                </span>
                <div className='flex gap-2'>
                  <Button
                    variant='outline'
                    size='sm'
                    disabled={page <= 1 || inviteQuery.isFetching}
                    onClick={() => setPage((current) => current - 1)}
                    aria-label={t('Previous')}
                  >
                    <HugeiconsIcon
                      icon={ArrowLeft01Icon}
                      data-icon='inline-start'
                    />
                    <span className='hidden sm:inline'>{t('Previous')}</span>
                  </Button>
                  <Button
                    variant='outline'
                    size='sm'
                    disabled={page >= pageCount || inviteQuery.isFetching}
                    onClick={() => setPage((current) => current + 1)}
                    aria-label={t('Next')}
                  >
                    <span className='hidden sm:inline'>{t('Next')}</span>
                    <HugeiconsIcon
                      icon={ArrowRight01Icon}
                      data-icon='inline-end'
                    />
                  </Button>
                </div>
              </CardFooter>
            ) : null}
          </Card>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function InviteOverviewCard({
  affiliateLink,
  linkLoading,
  stats,
  statsLoading,
}: {
  affiliateLink: string
  linkLoading: boolean
  stats?: AffiliateInviteStats
  statsLoading: boolean
}) {
  const { t } = useTranslation()
  return (
    <Card className='h-full'>
      <CardHeader className='border-b'>
        <CardTitle className='flex items-center gap-2'>
          <span className='bg-primary/10 text-primary flex size-8 items-center justify-center rounded-lg'>
            <HugeiconsIcon icon={Link01Icon} className='size-4' />
          </span>
          {t('Share your invite link')}
        </CardTitle>
        <CardDescription className='max-w-2xl'>
          {t(
            'Friends who register through this link will appear in your invite records.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='flex flex-1 flex-col gap-5'>
        {linkLoading ? (
          <Skeleton className='h-10 w-full' />
        ) : (
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
        )}

        {statsLoading ? <StatsSkeleton /> : <StatsBand stats={stats} />}
      </CardContent>
    </Card>
  )
}

function StatsBand({ stats }: { stats?: AffiliateInviteStats }) {
  const { t } = useTranslation()
  const items = [
    {
      label: t('Available rewards'),
      value: formatQuota(stats?.available_reward_quota ?? 0),
      icon: GiftIcon,
      emphasized: true,
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
    <div className='bg-border grid gap-px overflow-hidden rounded-lg border sm:grid-cols-2 xl:grid-cols-4'>
      {items.map((item) => (
        <div key={item.label} className='bg-muted/50 min-w-0 p-3'>
          <div className='text-muted-foreground flex items-center gap-1.5 text-xs'>
            <HugeiconsIcon icon={item.icon} className='size-3.5 shrink-0' />
            <span className='truncate'>{item.label}</span>
          </div>
          <div
            className={`mt-2 truncate font-semibold tabular-nums ${item.emphasized ? 'text-primary text-xl' : 'text-lg'}`}
          >
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
}: {
  policy?: AffiliateRewardPolicy
  loading: boolean
}) {
  const { t } = useTranslation()
  const rewardRatio = policy?.topup_reward_ratio ?? 0
  const rewardLimit = policy?.topup_reward_limit ?? 0
  const consumeRewardRatio = policy?.consume_reward_ratio ?? 0
  const topupLimit =
    rewardLimit > 0
      ? t('First {{count}} top-ups', { count: rewardLimit })
      : t('Unlimited top-up rewards')
  const policies = [
    {
      label: t('Registration reward'),
      value: formatInviteRegisterReward(policy?.inviter_reward_quota ?? 0),
      icon: GiftIcon,
    },
    {
      label: t('Invitee registration reward'),
      value: formatInviteRegisterReward(policy?.invitee_reward_quota ?? 0),
      icon: UserMultiple02Icon,
    },
  ]
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
  if (rewardRatio > 0) {
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
    <div className='bg-border grid gap-px overflow-hidden rounded-lg border sm:grid-cols-2 xl:grid-cols-4'>
      {Array.from({ length: 4 }).map((_, index) => (
        <div key={index} className='bg-muted/50 space-y-2 p-3'>
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
