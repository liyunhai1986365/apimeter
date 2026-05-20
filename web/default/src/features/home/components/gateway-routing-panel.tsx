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
import { ArrowRight, CheckCircle2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { CONTROL_SIGNALS } from '../landing-data'

const ROUTE_STEPS = ['Client app', 'Gateway', 'Upstreams'] as const

export function GatewayRoutingPanel() {
  const { t } = useTranslation()

  return (
    <div className='border-border/70 bg-background/95 relative overflow-hidden rounded-xl border p-5 shadow-xl shadow-slate-950/10 dark:shadow-black/30'>
      <div className='mb-4 flex items-center justify-between gap-4'>
        <div>
          <p className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
            {t('Live routing fabric')}
          </p>
          <h3 className='mt-1 text-base font-semibold'>
            {t('Your app speaks one API. The platform handles the rest.')}
          </h3>
        </div>
        <div className='border-border bg-muted/30 rounded-full border px-3 py-1 text-xs font-medium'>
          {t('OpenAI compatible')}
        </div>
      </div>

      <div className='grid gap-3 sm:grid-cols-[1fr_auto_1fr_auto_1fr] sm:items-center'>
        {ROUTE_STEPS.map((step, index) => (
          <div key={step} className='flex items-center gap-3'>
            <div className='bg-muted/40 border-border/70 flex min-h-20 flex-1 items-center justify-center rounded-lg border px-4 py-3 text-center'>
              <div>
                <p className='text-muted-foreground text-[11px] font-medium uppercase'>
                  {step}
                </p>
                <p className='mt-1 text-sm font-semibold'>
                  {index === 0
                    ? t('SaaS backend')
                    : index === 1
                      ? t('Keys, routing, pricing, logs')
                      : t('OpenAI, Claude, Gemini compatible')}
                </p>
              </div>
            </div>
            {index < ROUTE_STEPS.length - 1 && (
              <ArrowRight className='text-muted-foreground hidden size-4 sm:block' />
            )}
          </div>
        ))}
      </div>

      <div className='mt-4 grid gap-2 sm:grid-cols-3'>
        {CONTROL_SIGNALS.map((signal) => {
          const Icon = signal.icon
          return (
            <div
              key={signal.labelKey}
              className='bg-muted/20 border-border/70 flex items-center justify-between gap-3 rounded-lg border px-3 py-2'
            >
              <span className='text-muted-foreground flex items-center gap-2 text-xs'>
                <Icon className='size-3.5' />
                {t(signal.labelKey)}
              </span>
              <span className='text-xs font-semibold tabular-nums'>
                {signal.value}
              </span>
            </div>
          )
        })}
      </div>

      <div className='text-muted-foreground mt-4 flex items-center gap-2 text-xs'>
        <CheckCircle2 className='size-4 text-emerald-500' />
        <span>{t('Health-aware routing and fallback are already visible here.')}</span>
      </div>
    </div>
  )
}
