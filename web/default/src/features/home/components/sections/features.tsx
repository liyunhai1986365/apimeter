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
import { useTranslation } from 'react-i18next'
import { AnimateInView } from '@/components/animate-in-view'
import { COPY_CARDS, TRUST_CARDS } from '../../landing-data'

interface FeaturesProps {
  className?: string
}

export function Features(_props: FeaturesProps) {
  const { t } = useTranslation()

  return (
    <section className='relative z-10 px-6 py-20 md:py-28'>
      <div className='mx-auto max-w-6xl'>
        <AnimateInView className='mb-10 max-w-2xl'>
          <p className='text-muted-foreground mb-3 text-xs font-medium tracking-widest uppercase'>
            {t('Platform capabilities')}
          </p>
          <h2 className='text-2xl leading-tight font-bold tracking-tight md:text-3xl'>
            {t('Connect, route, bill, and govern model traffic')}
          </h2>
          <p className='text-muted-foreground mt-4 text-sm leading-relaxed md:text-base'>
            {t(
              'Manage credentials, routing rules, billing controls, request logs, and access policies from one operational surface.'
            )}
          </p>
        </AnimateInView>

        <div className='grid gap-4 md:grid-cols-2 lg:grid-cols-4'>
          {COPY_CARDS.map((card, index) => {
            const Icon = card.icon
            return (
              <AnimateInView
                key={card.titleKey}
                delay={index * 80}
                animation='scale-in'
                className='border-border/70 bg-background rounded-2xl border p-5 shadow-sm transition-colors duration-200 hover:bg-muted/20'
              >
                <div
                  className={`mb-4 flex size-10 items-center justify-center rounded-xl ${card.accentClassName}`}
                >
                  <Icon className='size-5' />
                </div>
                <h3 className='text-base font-semibold'>{t(card.titleKey)}</h3>
                <p className='text-muted-foreground mt-2 text-sm leading-relaxed'>
                  {t(card.descriptionKey)}
                </p>
              </AnimateInView>
            )
          })}
        </div>

        <div className='border-border/70 bg-muted/20 mt-6 rounded-3xl border p-5 md:p-6'>
          <div className='mb-5 flex flex-col gap-2 md:flex-row md:items-end md:justify-between'>
            <div>
              <p className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
                {t('Enterprise trust')}
              </p>
              <h3 className='mt-1 text-lg font-semibold'>
                {t('Controls that survive real traffic')}
              </h3>
            </div>
            <p className='text-muted-foreground max-w-lg text-sm leading-relaxed'>
              {t(
                'Inspired by the NTTD homepage emphasis on payment, registration, and channel monitoring, but expressed through this project’s operational console.'
              )}
            </p>
          </div>

          <div className='grid gap-3 md:grid-cols-2 lg:grid-cols-4'>
            {TRUST_CARDS.map((card) => {
              const Icon = card.icon
              return (
                <div
                  key={card.titleKey}
                  className='bg-background/80 border-border/60 rounded-2xl border p-4'
                >
                  <div
                    className={`mb-3 flex size-8 items-center justify-center rounded-lg ${card.accentClassName}`}
                  >
                    <Icon className='size-4' />
                  </div>
                  <h4 className='text-sm font-semibold'>{t(card.titleKey)}</h4>
                  <p className='text-muted-foreground mt-2 text-xs leading-relaxed'>
                    {t(card.descriptionKey)}
                  </p>
                </div>
              )
            })}
          </div>
        </div>
      </div>
    </section>
  )
}
