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
import { ArrowRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { AnimateInView } from '@/components/animate-in-view'

interface CTAProps {
  className?: string
  isAuthenticated?: boolean
}

export function CTA(props: CTAProps) {
  const { t } = useTranslation()

  if (props.isAuthenticated) {
    return null
  }

  return (
    <section className='relative z-10 px-6 py-20 md:py-28'>
      <AnimateInView animation='scale-in'>
        <div className='mx-auto max-w-6xl overflow-hidden rounded-3xl bg-slate-950 p-6 text-white shadow-2xl shadow-slate-950/20 md:p-10'>
          <div className='grid gap-8 md:grid-cols-[1fr_auto] md:items-center'>
            <div>
              <p className='mb-3 text-xs font-medium tracking-widest text-cyan-200 uppercase'>
                {t('Ready for your own gateway')}
              </p>
              <h2 className='max-w-2xl text-2xl leading-tight font-bold tracking-tight md:text-4xl'>
                {t('Launch a cleaner AI API business surface without hiding the product.')}
              </h2>
              <p className='mt-5 max-w-2xl text-sm leading-relaxed text-slate-300 md:text-base'>
                {t(
                  'Turn registration, payment, channel monitoring, pricing, and API compatibility into a homepage that feels like the console customers will actually use.'
                )}
              </p>
            </div>
            <div className='flex flex-col gap-3 sm:flex-row md:flex-col'>
              <Button
                className='group h-11 bg-white text-slate-950 hover:bg-slate-200'
                render={<Link to='/sign-up' />}
              >
                {t('Start routing now')}
                <ArrowRight className='size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
              </Button>
              <Button
                variant='outline'
                className='h-11 border-white/20 bg-transparent text-white hover:bg-white/10 hover:text-white'
                render={<Link to='/pricing' />}
              >
                {t('Explore model pricing')}
              </Button>
            </div>
          </div>
        </div>
      </AnimateInView>
    </section>
  )
}
