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
import { useEffect, useMemo, useRef, useState } from 'react'
import { VChart } from '@visactor/react-vchart'
import { ChartNoAxesColumnIncreasing, Layers3 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { VCHART_OPTION } from '@/lib/vchart'
import { useThemeCustomization } from '@/context/theme-customization-provider'
import { useTheme } from '@/context/theme-provider'
import { processUsageRankingChartData } from '@/features/dashboard/lib'
import type { QuotaDataItem } from '@/features/dashboard/types'

let themeManagerPromise: Promise<
  (typeof import('@visactor/vchart'))['ThemeManager']
> | null = null

interface UsageRankingChartsProps {
  data: QuotaDataItem[]
  loading?: boolean
}

export function UsageRankingCharts(props: UsageRankingChartsProps) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const { customization } = useThemeCustomization()
  const [themeReady, setThemeReady] = useState(false)
  const themeManagerRef = useRef<
    (typeof import('@visactor/vchart'))['ThemeManager'] | null
  >(null)

  useEffect(() => {
    const updateTheme = async () => {
      setThemeReady(false)

      if (!themeManagerPromise) {
        themeManagerPromise = import('@visactor/vchart').then(
          (module) => module.ThemeManager
        )
      }

      const ThemeManager = await themeManagerPromise
      themeManagerRef.current = ThemeManager
      ThemeManager.setCurrentTheme(resolvedTheme === 'dark' ? 'dark' : 'light')
      setThemeReady(true)
    }

    void updateTheme()
  }, [resolvedTheme])

  const chartData = useMemo(
    () =>
      processUsageRankingChartData(
        props.loading ? [] : props.data,
        t,
        customization.preset
      ),
    [props.data, props.loading, t, customization.preset]
  )
  const chartTheme = resolvedTheme === 'dark' ? 'dark' : 'light'
  const chartKey = [
    props.loading ? 'loading' : 'ready',
    props.data.length,
    resolvedTheme,
    customization.preset,
  ].join('-')

  const charts = [
    {
      id: 'model-rank',
      title: t('Model Usage Ranking'),
      icon: ChartNoAxesColumnIncreasing,
      spec: chartData.spec_model_rank,
    },
    {
      id: 'supplier-group',
      title: t('Supplier Group Statistics'),
      icon: Layers3,
      spec: chartData.spec_group_share,
    },
  ]

  return (
    <div className='grid gap-3 lg:grid-cols-2'>
      {charts.map((chart) => {
        const Icon = chart.icon
        return (
          <div key={chart.id} className='overflow-hidden rounded-lg border'>
            <div className='flex items-center justify-between gap-3 border-b px-3 py-2 sm:px-5 sm:py-3'>
              <div className='flex min-w-0 items-center gap-2'>
                <Icon className='text-muted-foreground/60 size-4 shrink-0' />
                <div className='truncate text-sm font-semibold'>
                  {chart.title}
                </div>
              </div>
              <span className='text-muted-foreground shrink-0 text-xs'>
                {t('Total:')} {chartData.totalQuotaDisplay}
              </span>
            </div>
            <div className='h-[320px] p-1.5 sm:h-[360px] sm:p-2'>
              {themeReady && chart.spec && (
                <VChart
                  key={`${chartKey}-${chart.id}`}
                  spec={{
                    ...chart.spec,
                    theme: chartTheme,
                    background: 'transparent',
                  }}
                  option={VCHART_OPTION}
                />
              )}
            </div>
          </div>
        )
      })}
    </div>
  )
}
