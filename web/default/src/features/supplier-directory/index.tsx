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
import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  BadgePercent,
  Building2,
  ChevronDown,
  SlidersHorizontal,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { formatGroupDiscount } from '@/lib/group-discount'
import { useGroupDiscountLabels } from '@/hooks/use-group-discount-labels'
import { Badge } from '@/components/ui/badge'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { PageTransition } from '@/components/page-transition'
import { getPricing } from '@/features/pricing/api'
import {
  buildSupplierDirectoryData,
  type SupplierDirectoryItem,
} from './lib/supplier-directory'

type SupplierCategorySection = {
  id: string
  name: string
  suppliers: SupplierDirectoryItem[]
}

export function SupplierDirectory() {
  const { t } = useTranslation()
  const { data, isLoading } = useQuery({
    queryKey: ['pricing', 'supplier-directory'],
    queryFn: getPricing,
    staleTime: 5 * 60 * 1000,
  })

  const directory = useMemo(() => buildSupplierDirectoryData(data), [data])
  const sections = useMemo(
    () => groupSuppliersByCategory(directory.items),
    [directory.items]
  )

  if (isLoading) {
    return <SupplierDirectorySkeleton />
  }

  return (
    <PageTransition className='flex min-h-0 flex-1 flex-col gap-4 overflow-auto p-4 lg:p-6'>
      <header className='flex items-center gap-2'>
        <Building2 className='text-muted-foreground size-5' />
        <div className='min-w-0'>
          <h1 className='text-2xl font-semibold tracking-tight'>
            {t('Suppliers')}
          </h1>
        </div>
      </header>

      {sections.length === 0 ? (
        <Empty className='min-h-[320px] border'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              <SlidersHorizontal />
            </EmptyMedia>
            <EmptyTitle>{t('No suppliers found')}</EmptyTitle>
            <EmptyDescription>
              {t('No supplier metadata configured.')}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <SupplierCategorySections sections={sections} />
      )}
    </PageTransition>
  )
}

function groupSuppliersByCategory(
  suppliers: SupplierDirectoryItem[]
): SupplierCategorySection[] {
  const sectionMap = new Map<string, SupplierCategorySection>()

  for (const supplier of suppliers) {
    const existing = sectionMap.get(supplier.categoryId)
    if (existing) {
      existing.suppliers.push(supplier)
      continue
    }
    sectionMap.set(supplier.categoryId, {
      id: supplier.categoryId,
      name: supplier.categoryName,
      suppliers: [supplier],
    })
  }

  return [...sectionMap.values()]
}

function SupplierCategorySections(props: {
  sections: SupplierCategorySection[]
}) {
  const { t } = useTranslation()

  return (
    <div className='flex flex-col gap-3'>
      {props.sections.map((section) => (
        <details
          key={section.id}
          open
          className='group overflow-hidden rounded-lg border bg-card'
        >
          <summary className='flex cursor-pointer list-none items-center justify-between gap-3 px-4 py-3 transition-colors hover:bg-muted/30'>
            <div className='flex min-w-0 items-center gap-2'>
              <ChevronDown className='text-muted-foreground size-4 shrink-0 transition-transform group-open:rotate-180' />
              <h2 className='truncate text-base font-semibold'>
                {t(section.name)}
              </h2>
            </div>
            <Badge variant='secondary' className='font-normal tabular-nums'>
              {section.suppliers.length}
            </Badge>
          </summary>
          <div className='divide-y border-t'>
            {section.suppliers.map((supplier) => (
              <SupplierRow key={supplier.group} supplier={supplier} />
            ))}
          </div>
        </details>
      ))}
    </div>
  )
}

function SupplierRow({ supplier }: { supplier: SupplierDirectoryItem }) {
  const { t } = useTranslation()
  const discountLabels = useGroupDiscountLabels()
  const discount = formatGroupDiscount(supplier.ratio, discountLabels)

  return (
    <div className='grid gap-2 px-4 py-3 text-sm transition-colors hover:bg-muted/30 md:grid-cols-[minmax(10rem,0.8fr)_minmax(14rem,1.4fr)_auto] md:items-center'>
      <div className='min-w-0 truncate font-medium'>{supplier.group}</div>
      <div className='text-muted-foreground min-w-0 truncate text-xs'>
        {supplier.description ? t(supplier.description) : '-'}
      </div>
      <div className='flex justify-start md:justify-end'>
        {discount ? (
          <Badge className='border-info bg-info text-info-foreground'>
            <BadgePercent data-icon='inline-start' />
            {discount}
          </Badge>
        ) : (
          <Badge variant='outline' className='text-muted-foreground font-normal'>
            {t('No active discounts')}
          </Badge>
        )}
      </div>
    </div>
  )
}

function SupplierDirectorySkeleton() {
  return (
    <div className='flex min-h-0 flex-1 flex-col gap-4 overflow-auto p-4 lg:p-6'>
      <div className='flex items-end justify-between gap-3'>
        <div className='flex flex-col gap-2'>
          <Skeleton className='h-7 w-40' />
        </div>
      </div>
      <div className='flex flex-col gap-3'>
        {Array.from({ length: 8 }).map((_, index) => (
          <Skeleton key={index} className='h-14 rounded-lg' />
        ))}
      </div>
    </div>
  )
}
