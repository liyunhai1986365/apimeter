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
import { useSystemConfig } from '@/hooks/use-system-config'
import { AnimateInView } from '@/components/animate-in-view'
import { MAINLAND_STEP_CARDS, STEP_CARDS } from '../../landing-data'

export function HowItWorks() {
  const { t } = useTranslation()
  const { mainlandChinaPresentationEnabled } = useSystemConfig()
  const steps = mainlandChinaPresentationEnabled
    ? MAINLAND_STEP_CARDS
    : STEP_CARDS

  return (
    <section className='border-border/40 bg-muted/20 relative z-10 border-y px-6 py-20 md:py-28'>
      <div className='mx-auto max-w-6xl'>
        <AnimateInView className='mb-10 text-center md:mb-14'>
          <p className='text-muted-foreground mb-3 text-xs font-medium tracking-widest uppercase'>
            {t('Operating model')}
          </p>
          <h2 className='text-2xl font-bold tracking-tight md:text-3xl'>
            {t('From upstream keys to production traffic in three moves')}
          </h2>
        </AnimateInView>

        <div className='grid gap-4 md:grid-cols-3'>
          {steps.map((step, index) => {
            const Icon = step.icon
            return (
              <AnimateInView
                key={step.titleKey}
                delay={index * 120}
                animation='fade-up'
                className='border-border/70 bg-background relative rounded-2xl border p-6 shadow-sm'
              >
                <div className='mb-6 flex items-center justify-between gap-4'>
                  <div className='bg-primary/10 text-primary flex size-11 items-center justify-center rounded-xl'>
                    <Icon className='size-5' />
                  </div>
                  <span className='text-muted-foreground text-3xl font-semibold tabular-nums'>
                    0{index + 1}
                  </span>
                </div>
                <h3 className='text-base font-semibold'>{t(step.titleKey)}</h3>
                <p className='text-muted-foreground mt-2 text-sm leading-relaxed'>
                  {t(step.descriptionKey)}
                </p>
              </AnimateInView>
            )
          })}
        </div>
      </div>
    </section>
  )
}
