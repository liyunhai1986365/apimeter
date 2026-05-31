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
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getPerfMetricsSummary } from '@/features/performance-metrics/api'
import { getLobeIcon } from '@/lib/lobe-icon'
import { DEFAULT_PRICING_PAGE_SIZE, DEFAULT_TOKEN_UNIT } from '../constants'
import type { PricingModel, TokenUnit } from '../types'
import { ModelCard } from './model-card'
import type { ModelPerfBadgeData } from './model-perf-badge'

export interface ModelCardGridProps {
  models: PricingModel[]
  onModelClick: (modelName: string) => void
  priceRate?: number
  usdExchangeRate?: number
  tokenUnit?: TokenUnit
  showRechargePrice?: boolean
}

export function ModelCardGrid(props: ModelCardGridProps) {
  const { t } = useTranslation()
  const pageSize = DEFAULT_PRICING_PAGE_SIZE
  const loadMoreRef = useRef<HTMLDivElement | null>(null)
  const [visibleCount, setVisibleCount] = useState(pageSize)
  const tokenUnit = props.tokenUnit ?? DEFAULT_TOKEN_UNIT
  const hasMore = visibleCount < props.models.length

  const perfQuery = useQuery({
    queryKey: ['perf-metrics-summary', 24],
    queryFn: () => getPerfMetricsSummary(24),
    staleTime: 60 * 1000,
    retry: false,
  })

  const vendorGroups = useMemo(() => {
    type VendorGroup = {
      key: string
      name: string
      description?: string
      icon?: string
      sortOrder: number
      models: PricingModel[]
      totalCount: number
    }

    const totalCounts = new Map<string, number>()
    for (const model of props.models) {
      const vendorName = model.vendor_name || t('Other Providers')
      const key = model.vendor_id ? String(model.vendor_id) : `unknown-${vendorName}`
      totalCounts.set(key, (totalCounts.get(key) || 0) + 1)
    }

    const groups = new Map<string, VendorGroup>()
    for (const model of props.models) {
      const vendorId = model.vendor_id ? String(model.vendor_id) : 'unknown'
      const vendorName = model.vendor_name || t('Other Providers')
      const key = model.vendor_id ? vendorId : `unknown-${vendorName}`
      const existing = groups.get(key)
      if (existing) {
        existing.models.push(model)
        continue
      }
      groups.set(key, {
        key,
        name: vendorName,
        description: model.vendor_description,
        icon: model.vendor_icon,
        sortOrder: model.vendor_sort_order ?? Number.MAX_SAFE_INTEGER,
        models: [model],
        totalCount: totalCounts.get(key) || 1,
      })
    }

    const sortedGroups = Array.from(groups.values()).sort((a, b) => {
      if (a.sortOrder === b.sortOrder) return a.name.localeCompare(b.name)
      return a.sortOrder - b.sortOrder
    })

    let remaining = visibleCount
    const visibleGroups: VendorGroup[] = []
    for (const group of sortedGroups) {
      if (remaining <= 0) break
      visibleGroups.push({
        ...group,
        models: group.models.slice(0, remaining),
      })
      remaining -= group.models.length
    }
    return visibleGroups
  }, [props.models, t, visibleCount])

  const visibleModelCount = useMemo(
    () =>
      vendorGroups.reduce((count, group) => count + group.models.length, 0),
    [vendorGroups]
  )

  const perfMap = useMemo(() => {
    const map = new Map<string, ModelPerfBadgeData>()
    for (const model of perfQuery.data?.data?.models ?? []) {
      map.set(model.model_name, model)
    }
    return map
  }, [perfQuery.data])

  useEffect(() => {
    setVisibleCount(pageSize)
  }, [pageSize, props.models])

  useEffect(() => {
    const marker = loadMoreRef.current
    if (!marker || !hasMore) return

    const observer = new IntersectionObserver(
      (entries) => {
        if (!entries[0]?.isIntersecting) return
        setVisibleCount((current) =>
          Math.min(current + pageSize, props.models.length)
        )
      },
      { rootMargin: '320px 0px' }
    )

    observer.observe(marker)
    return () => observer.disconnect()
  }, [hasMore, pageSize, props.models.length])

  if (props.models.length === 0) {
    return null
  }

  return (
    <div className='space-y-4 sm:space-y-5'>
      <div className='space-y-5'>
        {vendorGroups.map((group) => {
          const icon = group.icon ? getLobeIcon(group.icon, 32) : null
          return (
            <section key={group.key} className='scroll-mt-28 space-y-3'>
              <div className='sticky top-16 z-20 -mx-1 rounded-lg border bg-background/95 px-3 py-2 shadow-sm backdrop-blur supports-[backdrop-filter]:bg-background/85 sm:top-20 sm:px-4 sm:py-3'>
                <div className='flex min-w-0 items-center justify-between gap-3'>
                  <div className='flex min-w-0 items-center gap-3'>
                    <div className='bg-muted/60 flex size-10 shrink-0 items-center justify-center rounded-lg'>
                      {icon || (
                        <span className='text-muted-foreground text-sm font-bold'>
                          {group.name.charAt(0).toUpperCase()}
                        </span>
                      )}
                    </div>
                    <div className='min-w-0'>
                      <h2 className='truncate text-sm font-semibold sm:text-base'>
                        {group.name}
                      </h2>
                      {group.description && (
                        <p className='text-muted-foreground mt-0.5 line-clamp-1 text-xs'>
                          {group.description}
                        </p>
                      )}
                    </div>
                  </div>
                  <div className='text-muted-foreground shrink-0 text-xs font-medium tabular-nums'>
                    {t('{{count}} models', { count: group.totalCount })}
                  </div>
                </div>
              </div>

              <div className='grid grid-cols-1 gap-3 sm:gap-4 md:grid-cols-2 lg:grid-cols-4'>
                {group.models.map((model) => (
                  <ModelCard
                    key={model.id ?? model.model_name}
                    model={model}
                    tokenUnit={tokenUnit}
                    priceRate={props.priceRate}
                    usdExchangeRate={props.usdExchangeRate}
                    showRechargePrice={props.showRechargePrice}
                    perf={perfMap.get(model.model_name || '')}
                    onClick={() => props.onModelClick(model.model_name || '')}
                  />
                ))}
              </div>
            </section>
          )
        })}
      </div>

      <div
        ref={loadMoreRef}
        className='text-muted-foreground flex min-h-12 items-center justify-center border-t px-4 py-3 text-center text-sm'
      >
        <div>
          {hasMore ? t('Scroll to load more models') : t('All models loaded')}
          <span className='ml-2 tabular-nums'>
            {visibleModelCount.toLocaleString()} /{' '}
            {props.models.length.toLocaleString()}
          </span>
        </div>
      </div>
    </div>
  )
}
