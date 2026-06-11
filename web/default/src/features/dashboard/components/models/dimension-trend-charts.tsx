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
import { BriefcaseBusiness, KeyRound, type LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { computeTimeRange, type TimeGranularity } from '@/lib/time'
import { VCHART_OPTION } from '@/lib/vchart'
import { useThemeCustomization } from '@/context/theme-customization-provider'
import { useTheme } from '@/context/theme-provider'
import { getUsageDimensionTrends } from '@/features/dashboard/api'
import { DEFAULT_TIME_GRANULARITY } from '@/features/dashboard/constants'
import {
  buildQueryParams,
  getDefaultDays,
  processDimensionTrendChartData,
} from '@/features/dashboard/lib'
import type {
  DashboardFilters,
  QuotaDataItem,
  UsageDimensionTrendItem,
} from '@/features/dashboard/types'

let themeManagerPromise: Promise<
  (typeof import('@visactor/vchart'))['ThemeManager']
> | null = null

interface DimensionTrendChartsProps {
  filters?: DashboardFilters
  timeReferenceData?: QuotaDataItem[]
  timeGranularity?: TimeGranularity
}

type DimensionTrendCardProps = {
  data: UsageDimensionTrendItem[]
  loading?: boolean
  referenceTimestamps?: number[]
  timeGranularity: TimeGranularity
  dimension: 'workspace' | 'token'
  title: string
  icon: LucideIcon
}

function DimensionTrendCard(props: DimensionTrendCardProps) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const { customization } = useThemeCustomization()
  const [themeReady, setThemeReady] = useState(false)
  const themeManagerRef = useRef<
    (typeof import('@visactor/vchart'))['ThemeManager'] | null
  >(null)
  const Icon = props.icon

  useEffect(() => {
    const updateTheme = async () => {
      setThemeReady(false)

      if (!themeManagerPromise) {
        themeManagerPromise = import('@visactor/vchart').then(
          (m) => m.ThemeManager
        )
      }

      const ThemeManager = await themeManagerPromise
      themeManagerRef.current = ThemeManager
      ThemeManager.setCurrentTheme(resolvedTheme === 'dark' ? 'dark' : 'light')
      setThemeReady(true)
    }

    updateTheme()
  }, [resolvedTheme])

  const chartData = useMemo(
    () =>
      processDimensionTrendChartData(
        props.loading ? [] : props.data,
        props.dimension,
        props.timeGranularity,
        t,
        customization.preset,
        props.referenceTimestamps
      ),
    [
      props.data,
      props.dimension,
      props.loading,
      props.referenceTimestamps,
      props.timeGranularity,
      t,
      customization.preset,
    ]
  )

  const spec = chartData.spec_trend
  const chartKey = [
    props.dimension,
    props.loading ? 'loading' : 'ready',
    props.data.length,
    resolvedTheme,
    customization.preset,
  ].join('-')

  return (
    <div className='overflow-hidden rounded-lg border'>
      <div className='flex items-center justify-between gap-3 border-b px-3 py-2 sm:px-5 sm:py-3'>
        <div className='flex min-w-0 items-center gap-2'>
          <Icon className='text-muted-foreground/60 size-4 shrink-0' />
          <div className='truncate text-sm font-semibold'>{props.title}</div>
        </div>
        <span className='text-muted-foreground shrink-0 text-xs'>
          {t('Total:')} {chartData.totalQuotaDisplay}
        </span>
      </div>
      <div className='h-[280px] p-1.5 sm:h-[340px] sm:p-2'>
        {themeReady && spec && (
          <VChart
            key={chartKey}
            spec={{
              ...spec,
              theme: resolvedTheme === 'dark' ? 'dark' : 'light',
              background: 'transparent',
            }}
            option={VCHART_OPTION}
          />
        )}
      </div>
    </div>
  )
}

export function DimensionTrendCharts(props: DimensionTrendChartsProps) {
  const { t } = useTranslation()
  const [data, setData] = useState<UsageDimensionTrendItem[]>([])
  const [loading, setLoading] = useState(true)
  const timeGranularity = props.timeGranularity ?? DEFAULT_TIME_GRANULARITY
  const referenceTimestamps = useMemo(
    () =>
      Array.from(
        new Set(
          (props.timeReferenceData || [])
            .map((item) => Number(item.created_at) || 0)
            .filter((timestamp) => timestamp > 0)
        )
      ),
    [props.timeReferenceData]
  )

  useEffect(() => {
    const abortController = new AbortController()

    const timeRange = computeTimeRange(
      getDefaultDays(props.filters?.time_granularity),
      props.filters?.start_timestamp,
      props.filters?.end_timestamp
    )

    const loadData = async () => {
      await Promise.resolve()
      if (abortController.signal.aborted) return
      setLoading(true)
      try {
        const { username: _username, ...selfFilters } = props.filters ?? {}
        const res = await getUsageDimensionTrends(
          buildQueryParams(timeRange, selfFilters),
          false
        )
        if (abortController.signal.aborted) return
        setData(res?.data || [])
      } catch {
        if (abortController.signal.aborted) return
        setData([])
      } finally {
        if (!abortController.signal.aborted) {
          setLoading(false)
        }
      }
    }

    void loadData()

    return () => {
      abortController.abort()
    }
  }, [props.filters])

  return (
    <div className='space-y-3'>
      <DimensionTrendCard
        data={data}
        loading={loading}
        referenceTimestamps={referenceTimestamps}
        timeGranularity={timeGranularity}
        dimension='workspace'
        title={t('Workspace Usage Trend')}
        icon={BriefcaseBusiness}
      />
      <DimensionTrendCard
        data={data}
        loading={loading}
        referenceTimestamps={referenceTimestamps}
        timeGranularity={timeGranularity}
        dimension='token'
        title={t('Token Usage Trend')}
        icon={KeyRound}
      />
    </div>
  )
}
