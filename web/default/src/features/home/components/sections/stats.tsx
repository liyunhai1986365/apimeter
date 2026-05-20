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
import { METRICS } from '../../landing-data'

interface StatsProps {
  className?: string
}

export function Stats(_props: StatsProps) {
  const { t } = useTranslation()

  return (
    <div className='border-border/40 bg-muted/20 relative z-10 border-y'>
      <div className='mx-auto max-w-6xl px-6 py-8 md:py-10'>
        <div className='grid grid-cols-2 gap-4 md:grid-cols-4'>
          {METRICS.map((metric) => (
            <div
              key={metric.labelKey}
              className='bg-background/70 border-border/60 rounded-2xl border p-5 text-center shadow-sm'
            >
              <span className='text-2xl font-bold tracking-tight md:text-3xl'>
                {metric.value}
              </span>
              <span className='text-muted-foreground mt-1.5 text-xs'>
                {t(metric.labelKey)}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
