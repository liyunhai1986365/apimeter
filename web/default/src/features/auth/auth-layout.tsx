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
  Activity,
  ArrowRight,
  BadgeCheck,
  Gauge,
  Network,
  ShieldCheck,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useStatus } from '@/hooks/use-status'
import { useSystemConfig } from '@/hooks/use-system-config'
import { Skeleton } from '@/components/ui/skeleton'
import { LegalFirstUseDialog } from './components/legal-first-use-dialog'

type AuthLayoutProps = {
  children: React.ReactNode
}

const platformHighlights = [
  {
    icon: Network,
    title: 'Unified model gateway',
    description:
      'Access major AI providers through one consistent API surface.',
  },
  {
    icon: ShieldCheck,
    title: 'Enterprise-grade controls',
    description: 'Built-in keys, groups, quotas, rate limits, and audit logs.',
  },
  {
    icon: Gauge,
    title: 'Smart routing and failover',
    description: 'Route traffic across channels with health-aware reliability.',
  },
]

const platformStats = [
  { value: '40+', label: 'Upstream providers' },
  { value: '24/7', label: 'Relay observability' },
  { value: '3', label: 'Database engines' },
]

export function AuthLayout({ children }: AuthLayoutProps) {
  const { t } = useTranslation()
  const { systemName, logo, loading } = useSystemConfig()
  const { status } = useStatus()

  return (
    <div className='from-background via-background to-muted/70 relative min-h-dvh overflow-hidden bg-radial-[at_18%_12%] from-blue-100/70 dark:from-blue-950/35'>
      <div className='pointer-events-none absolute inset-0 overflow-hidden'>
        <div className='absolute -top-28 left-8 h-72 w-72 rounded-full bg-blue-500/15 blur-3xl dark:bg-blue-500/10' />
        <div className='absolute top-1/4 right-8 h-80 w-80 rounded-full bg-cyan-400/15 blur-3xl dark:bg-cyan-400/10' />
        <div className='absolute -bottom-28 left-1/3 h-72 w-72 rounded-full bg-orange-400/10 blur-3xl dark:bg-orange-400/10' />
        <div className='absolute inset-y-0 left-0 hidden w-3/5 bg-[linear-gradient(115deg,rgba(37,99,235,0.10)_0,rgba(37,99,235,0.04)_38%,transparent_70%)] lg:block' />
        <div className='absolute top-24 left-[12%] hidden h-px w-[42%] bg-gradient-to-r from-transparent via-blue-500/30 to-transparent lg:block' />
        <div className='absolute bottom-32 left-[18%] hidden h-px w-[32%] bg-gradient-to-r from-transparent via-cyan-500/25 to-transparent lg:block' />
      </div>

      <Link
        to='/'
        className='focus-visible:ring-ring/50 absolute top-4 left-4 z-20 flex min-h-11 items-center gap-2 rounded-full border border-white/50 bg-white/75 px-3 py-2 shadow-sm backdrop-blur-xl transition-opacity hover:opacity-85 focus-visible:ring-3 focus-visible:outline-none sm:top-6 sm:left-8 dark:border-white/10 dark:bg-black/30'
      >
        <div className='relative h-8 w-8'>
          {loading ? (
            <Skeleton className='absolute inset-0 rounded-full' />
          ) : (
            <img
              src={logo}
              alt={t('Logo')}
              className='h-8 w-8 rounded-full object-cover'
            />
          )}
        </div>
        {loading ? (
          <Skeleton className='h-6 w-24' />
        ) : (
          <h1 className='text-base font-semibold sm:text-lg'>{systemName}</h1>
        )}
      </Link>

      <main className='relative z-10 mx-auto flex min-h-dvh w-full max-w-7xl items-center px-4 py-20 sm:px-6 lg:px-8'>
        <div className='grid w-full items-center gap-10 lg:grid-cols-[minmax(0,1.08fr)_minmax(420px,0.92fr)] lg:gap-12 xl:gap-16'>
          <section className='hidden max-w-2xl lg:block'>
            <div className='inline-flex items-center gap-2 rounded-full border border-blue-500/20 bg-blue-500/10 px-3 py-1.5 text-sm font-medium text-blue-700 dark:text-blue-200'>
              <BadgeCheck className='size-4' aria-hidden='true' />
              {t('Enterprise AI API gateway')}
            </div>

            <div className='mt-8 space-y-5'>
              <h2 className='max-w-2xl text-5xl leading-tight font-semibold text-slate-950 dark:text-white'>
                {t('One secure console for every AI model workflow.')}
              </h2>
              <p className='text-muted-foreground max-w-xl text-lg leading-8'>
                {t(
                  'Connect providers, manage users, control spend, and keep every request observable from one production-ready platform.'
                )}
              </p>
            </div>

            <div className='mt-10 grid gap-3'>
              {platformHighlights.map((item) => {
                const Icon = item.icon

                return (
                  <div
                    key={item.title}
                    className='group grid grid-cols-[44px_1fr] items-start gap-4 rounded-lg border border-white/60 bg-white/70 p-4 shadow-sm backdrop-blur-md transition duration-200 hover:border-blue-500/30 hover:bg-white/90 dark:border-white/10 dark:bg-white/5 dark:hover:bg-white/[0.08]'
                  >
                    <div className='flex size-11 items-center justify-center rounded-lg bg-blue-600 text-white shadow-sm shadow-blue-600/25'>
                      <Icon className='size-5' aria-hidden='true' />
                    </div>
                    <div>
                      <h3 className='flex items-center gap-2 font-semibold text-slate-950 dark:text-white'>
                        {t(item.title)}
                        <ArrowRight
                          className='size-4 text-blue-600 opacity-0 transition duration-200 group-hover:translate-x-0.5 group-hover:opacity-100'
                          aria-hidden='true'
                        />
                      </h3>
                      <p className='text-muted-foreground mt-1 text-sm leading-6'>
                        {t(item.description)}
                      </p>
                    </div>
                  </div>
                )
              })}
            </div>

            <div className='mt-8 grid max-w-xl grid-cols-3 gap-3'>
              {platformStats.map((item) => (
                <div
                  key={item.label}
                  className='rounded-lg border border-white/60 bg-white/60 px-4 py-3 backdrop-blur-md dark:border-white/10 dark:bg-white/5'
                >
                  <div className='flex items-center gap-2 text-2xl font-semibold text-slate-950 dark:text-white'>
                    {item.value}
                    {item.value === '24/7' ? (
                      <Activity
                        className='size-4 text-emerald-500'
                        aria-hidden='true'
                      />
                    ) : null}
                  </div>
                  <div className='text-muted-foreground mt-1 text-xs font-medium'>
                    {t(item.label)}
                  </div>
                </div>
              ))}
            </div>
          </section>

          <div className='mx-auto w-full max-w-[480px] lg:mr-0'>
            <div className='border-border/70 bg-background/95 rounded-lg border p-5 shadow-2xl shadow-blue-950/10 backdrop-blur-xl sm:p-8 dark:shadow-black/30'>
              {children}
            </div>
          </div>
        </div>
      </main>

      <LegalFirstUseDialog status={status} />
    </div>
  )
}
