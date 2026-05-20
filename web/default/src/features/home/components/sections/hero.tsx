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
import { ArrowRight, CirclePlay } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { LOGO_RAIL_PROVIDERS } from '../../landing-data'
import { ProviderLogoMarquee } from '../provider-logo-marquee'

interface HeroProps {
  isAuthenticated?: boolean
}

export function Hero(props: HeroProps) {
  const { t } = useTranslation()

  return (
    <section className='relative z-10 overflow-hidden px-6 pt-24 pb-14 md:pt-32 md:pb-18'>
      <div
        aria-hidden='true'
        className='pointer-events-none absolute inset-0 -z-10 bg-[radial-gradient(circle_at_16%_18%,rgba(14,165,233,0.13),transparent_28%),radial-gradient(circle_at_84%_8%,rgba(16,185,129,0.11),transparent_24%),linear-gradient(to_bottom,var(--background),var(--muted)_180%,var(--background))]'
      />
      <div
        aria-hidden='true'
        className='absolute inset-0 -z-10 bg-[linear-gradient(to_right,var(--border)_1px,transparent_1px),linear-gradient(to_bottom,var(--border)_1px,transparent_1px)] [mask-image:radial-gradient(ellipse_60%_50%_at_50%_30%,black_20%,transparent_100%)] bg-[size:4rem_4rem] opacity-[0.08]'
      />

      <div className='mx-auto flex max-w-5xl flex-col items-center text-center'>
        <div
          className='landing-animate-fade-up border-border/70 bg-background/80 text-muted-foreground inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-medium shadow-sm'
          style={{ animationDelay: '0ms' }}
        >
          <span className='size-2 rounded-full bg-emerald-500' />
          {t('API platform optimized for Agents')}
        </div>
        <h1
          className='landing-animate-fade-up mt-6 max-w-4xl text-[clamp(2.4rem,6.4vw,5.6rem)] leading-[1.02] font-bold tracking-tight'
          style={{ animationDelay: '80ms' }}
        >
          {t('Models unified API gateway')}
        </h1>
        <p
          className='landing-animate-fade-up text-muted-foreground mt-6 max-w-2xl text-base leading-relaxed opacity-0 md:text-lg'
          style={{ animationDelay: '160ms' }}
        >
          {t(
            'Unify OpenAI, Claude, Gemini, DeepSeek and private channels with shared keys, billing, fallback routing, and live request visibility.'
          )}
        </p>

        <div
          className='landing-animate-fade-up mt-8 flex flex-col items-center justify-center gap-3 opacity-0 sm:flex-row'
          style={{ animationDelay: '240ms' }}
        >
          {props.isAuthenticated ? (
            <Button
              size='lg'
              className='group h-11'
              render={<Link to='/dashboard' />}
            >
              {t('Go to Dashboard')}
              <ArrowRight className='size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
            </Button>
          ) : (
            <>
              <Button
                size='lg'
                className='group h-11'
                render={<Link to='/sign-up' />}
              >
                {t('Start routing now')}
                <ArrowRight className='size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
              </Button>
              <Button
                size='lg'
                variant='outline'
                className='h-11'
                render={<Link to='/pricing' />}
              >
                <CirclePlay className='size-4' />
                {t('Explore model pricing')}
              </Button>
            </>
          )}
        </div>

        <div
          className='landing-animate-fade-up mt-12 w-full opacity-0'
          style={{ animationDelay: '320ms' }}
        >
          <p className='text-muted-foreground mb-6 text-xs font-medium tracking-wide uppercase'>
            {t('Connected model providers')}
          </p>
          <ProviderLogoMarquee providers={LOGO_RAIL_PROVIDERS} />
        </div>

        <div
          className='landing-animate-fade-up mt-12 grid w-full gap-3 opacity-0 sm:grid-cols-3'
          style={{ animationDelay: '420ms' }}
        >
          {[
            {
              label: t('Channel monitor'),
              value: t('health-aware routing'),
            },
            {
              label: t('Payment ready'),
              value: t('quota and recharge flows'),
            },
            {
              label: t('Developer contract'),
              value: t('OpenAI, Claude, Gemini compatible'),
            },
          ].map((item) => (
            <div
              key={item.label}
              className='border-border/70 bg-background/70 rounded-2xl border p-4 text-center shadow-sm'
            >
              <p className='text-muted-foreground text-xs font-medium uppercase'>
                {item.label}
              </p>
              <p className='mt-1 text-sm font-semibold'>{item.value}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
