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
import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  BadgeDollarSign,
  Gift,
  Link2,
  ShieldCheck,
  Sparkles,
  TrendingUp,
  UserPlus,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { useStatus } from '@/hooks/use-status'
import { Button } from '@/components/ui/button'
import { PublicLayout } from '@/components/layout'
import { Footer } from '@/components/layout/components/footer'
import {
  formatInviteRegisterReward,
  formatInviteRewardRatio,
  getInviteRewardConfig,
} from './lib/reward-config'

export function Invite() {
  const { t } = useTranslation()
  const { status } = useStatus()
  const { auth } = useAuthStore()
  const config = getInviteRewardConfig(status)
  const isAuthenticated = !!auth.user
  const ctaHref = isAuthenticated ? '/wallet' : '/sign-up'
  const topupRatioText =
    config.topupRewardRatio > 0
      ? formatInviteRewardRatio(config.topupRewardRatio)
      : t('Configured by admin')
  const limitText =
    config.topupRewardLimit > 0
      ? t('First {{count}} top-ups', { count: config.topupRewardLimit })
      : t('Unlimited top-up rewards')
  const registerRewardText =
    config.inviterRegisterQuota > 0
      ? formatInviteRegisterReward(config.inviterRegisterQuota)
      : t('Configured by admin')

  return (
    <PublicLayout showMainContainer={false}>
      <main className='bg-background overflow-hidden'>
        <section className='relative isolate min-h-[calc(100svh-4rem)] px-4 pt-28 pb-16 sm:px-6 lg:pt-32'>
          <div className='absolute inset-0 -z-10 bg-[radial-gradient(circle_at_20%_10%,rgba(16,185,129,0.24),transparent_28%),radial-gradient(circle_at_78%_18%,rgba(14,165,233,0.18),transparent_28%),linear-gradient(180deg,rgba(16,185,129,0.08),transparent_52%)]' />
          <div className='absolute inset-x-0 top-16 -z-10 h-px bg-gradient-to-r from-transparent via-emerald-400/40 to-transparent' />

          <div className='mx-auto grid max-w-7xl gap-10 lg:grid-cols-[minmax(0,1.06fr)_minmax(360px,0.72fr)] lg:items-center'>
            <div className='max-w-3xl'>
              <div className='mb-5 inline-flex items-center gap-2 rounded-full border border-emerald-500/20 bg-emerald-500/10 px-3 py-1 text-sm font-medium text-emerald-700 dark:text-emerald-200'>
                <Sparkles className='size-4' />
                {t('Invite Reward Program')}
              </div>
              <h1 className='max-w-4xl text-5xl leading-[0.98] font-semibold tracking-normal text-balance sm:text-6xl lg:text-7xl'>
                {t('Turn every referral into recurring rewards.')}
              </h1>
              <p className='text-muted-foreground mt-6 max-w-2xl text-lg leading-8 sm:text-xl'>
                {t(
                  'Share your invite link, help friends get started, and earn rewards when they register and add funds.'
                )}
              </p>
              <div className='mt-8 flex flex-col gap-3 sm:flex-row'>
                <Button
                  size='lg'
                  className='h-12 rounded-full px-6 text-base'
                  render={<Link to={ctaHref} />}
                >
                  {isAuthenticated
                    ? t('Get my invite link')
                    : t('Start inviting')}
                  <ArrowRight className='size-4' />
                </Button>
                <Button
                  variant='outline'
                  size='lg'
                  className='h-12 rounded-full px-6 text-base'
                  render={<Link to='/pricing' />}
                >
                  {t('View pricing')}
                </Button>
              </div>
            </div>

            <div className='relative'>
              <div className='rounded-[2rem] border border-white/30 bg-white/72 p-4 shadow-[0_32px_100px_-40px_rgba(16,185,129,0.55)] backdrop-blur-xl dark:border-white/10 dark:bg-zinc-950/64'>
                <div className='rounded-[1.5rem] bg-gradient-to-br from-emerald-950 via-emerald-800 to-cyan-900 p-6 text-white'>
                  <div className='flex items-center justify-between'>
                    <span className='text-sm font-medium text-emerald-100/80'>
                      {t('Top-up reward')}
                    </span>
                    <BadgeDollarSign className='size-6 text-emerald-200' />
                  </div>
                  <div className='mt-8 text-6xl font-semibold tracking-normal'>
                    {topupRatioText}
                  </div>
                  <p className='mt-3 text-sm leading-6 text-emerald-50/76'>
                    {t(
                      'Earn a percentage of invited users top-ups after the settlement window.'
                    )}
                  </p>
                </div>
                <div className='mt-4 grid gap-3 sm:grid-cols-2'>
                  <MetricCard
                    icon={<TrendingUp className='size-4' />}
                    label={t('Rewarded top-ups')}
                    value={limitText}
                  />
                  <MetricCard
                    icon={<Gift className='size-4' />}
                    label={t('Registration reward')}
                    value={registerRewardText}
                  />
                </div>
              </div>
            </div>
          </div>
        </section>

        <section className='px-4 py-16 sm:px-6'>
          <div className='mx-auto max-w-7xl'>
            <div className='mb-8 max-w-2xl'>
              <h2 className='text-3xl font-semibold tracking-normal'>
                {t('Simple to share. Easy to earn.')}
              </h2>
              <p className='text-muted-foreground mt-3 leading-7'>
                {t(
                  'The reward flow is built for repeat referrals without extra manual work.'
                )}
              </p>
            </div>
            <div className='grid gap-4 md:grid-cols-3'>
              <StepCard
                icon={<Link2 className='size-5' />}
                title={t('Share your link')}
                text={t(
                  'Copy your referral link from the wallet and send it to friends.'
                )}
              />
              <StepCard
                icon={<UserPlus className='size-5' />}
                title={t('Friends register')}
                text={t(
                  'New users join through your invite link and become linked to you.'
                )}
              />
              <StepCard
                icon={<BadgeDollarSign className='size-5' />}
                title={t('Rewards arrive')}
                text={t(
                  'Eligible top-up rewards are automatically credited after 24 hours.'
                )}
              />
            </div>
          </div>
        </section>

        <section className='px-4 py-16 sm:px-6'>
          <div className='mx-auto grid max-w-7xl gap-6 lg:grid-cols-[0.75fr_1fr] lg:items-start'>
            <div>
              <div className='inline-flex items-center gap-2 rounded-full border px-3 py-1 text-sm font-medium'>
                <ShieldCheck className='size-4 text-emerald-600' />
                {t('Program rules')}
              </div>
              <h2 className='mt-5 text-3xl font-semibold tracking-normal'>
                {t('Clear rules for sustainable rewards.')}
              </h2>
            </div>
            <div className='grid gap-3'>
              {[
                config.topupRewardRatio > 0
                  ? t(
                      'Invite a friend to register and top up. You can earn {{ratio}} of their top-up amount as a reward.',
                      { ratio: topupRatioText }
                    )
                  : null,
                config.topupRewardLimit > 0
                  ? t(
                      'Each invited user brings you rewards for their first {{count}} top-ups.',
                      { count: config.topupRewardLimit }
                    )
                  : t(
                      'Each invited user can bring you top-up rewards without a count limit.'
                    ),
                config.inviterRegisterQuota > 0
                  ? t(
                      'Invite a friend to register and instantly receive {{amount}}.',
                      {
                        amount: registerRewardText,
                      }
                    )
                  : null,
                t(
                  'Do not invite yourself with alternate accounts. Violations will result in reward recovery and serious cases may be banned.'
                ),
              ]
                .filter(Boolean)
                .map((rule, index) => (
                  <div
                    key={String(rule)}
                    className='bg-card flex gap-4 rounded-xl border p-4 shadow-sm'
                  >
                    <span className='flex size-7 shrink-0 items-center justify-center rounded-full bg-emerald-500/10 text-sm font-semibold text-emerald-700 dark:text-emerald-200'>
                      {index + 1}
                    </span>
                    <p className='text-muted-foreground leading-7'>{rule}</p>
                  </div>
                ))}
            </div>
          </div>
        </section>

        <section className='px-4 py-16 sm:px-6'>
          <div className='mx-auto max-w-7xl rounded-[2rem] bg-emerald-950 px-6 py-12 text-center text-white sm:px-10'>
            <h2 className='text-3xl font-semibold tracking-normal sm:text-4xl'>
              {t('Ready to grow your rewards?')}
            </h2>
            <p className='mx-auto mt-4 max-w-2xl text-emerald-50/76'>
              {t(
                'Start with one invite link. Every qualified friend can become a new reward stream.'
              )}
            </p>
            <Button
              size='lg'
              className='mt-8 h-12 rounded-full bg-white px-6 text-base text-emerald-950 hover:bg-emerald-50'
              render={<Link to={ctaHref} />}
            >
              {isAuthenticated ? t('Open wallet') : t('Create account')}
              <ArrowRight className='size-4' />
            </Button>
          </div>
        </section>
      </main>
      <Footer />
    </PublicLayout>
  )
}

function MetricCard(props: {
  icon: React.ReactNode
  label: string
  value: string
}) {
  return (
    <div className='bg-background rounded-2xl border p-4'>
      <div className='text-muted-foreground flex items-center gap-2 text-xs font-medium'>
        {props.icon}
        {props.label}
      </div>
      <div className='mt-2 text-lg font-semibold'>{props.value}</div>
    </div>
  )
}

function StepCard(props: {
  icon: React.ReactNode
  title: string
  text: string
}) {
  return (
    <div className='bg-card rounded-2xl border p-5 shadow-sm'>
      <div className='flex size-10 items-center justify-center rounded-full bg-emerald-500/10 text-emerald-700 dark:text-emerald-200'>
        {props.icon}
      </div>
      <h3 className='mt-5 text-lg font-semibold'>{props.title}</h3>
      <p className='text-muted-foreground mt-2 leading-7'>{props.text}</p>
    </div>
  )
}
