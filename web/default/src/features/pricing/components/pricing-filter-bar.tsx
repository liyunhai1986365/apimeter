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
import { cn } from '@/lib/utils'
import {
  MODEL_CATEGORIES,
  getModelCategoryLabels,
} from '../constants'
import type { PricingModel } from '../types'

type CategoryTab = {
  value: string
  label: string
  count: number
}

export interface PricingFilterBarProps {
  categoryFilter: string
  onCategoryChange: (value: string) => void
  models: PricingModel[]
  className?: string
}

function countBy(
  models: PricingModel[],
  predicate: (model: PricingModel) => boolean
): number {
  return models.reduce((count, model) => count + (predicate(model) ? 1 : 0), 0)
}

export function PricingFilterBar(props: PricingFilterBarProps) {
  const { t } = useTranslation()
  const categoryLabels = getModelCategoryLabels(t)
  const categoryTabs: CategoryTab[] = [
    {
      value: MODEL_CATEGORIES.ALL,
      label: categoryLabels[MODEL_CATEGORIES.ALL],
      count: props.models.length,
    },
    ...Object.entries(categoryLabels)
      .filter(([value]) => value !== MODEL_CATEGORIES.ALL)
      .map(([value, label]) => ({
        value,
        label,
        count: countBy(
          props.models,
          (model) => (model.category || MODEL_CATEGORIES.TEXT) === value
        ),
      })),
  ]

  return (
    <nav
      aria-label={t('Model Category')}
      className={cn(
        'bg-background/95 rounded-xl border p-2 shadow-sm backdrop-blur',
        props.className
      )}
    >
      <div className='flex gap-1 overflow-x-auto'>
        {categoryTabs.map((tab) => {
          const active = props.categoryFilter === tab.value
          return (
            <button
              key={tab.value}
              type='button'
              onClick={() => props.onCategoryChange(tab.value)}
              aria-pressed={active}
              className={cn(
                'inline-flex h-9 shrink-0 cursor-pointer items-center gap-2 rounded-lg px-3 text-sm font-medium transition-colors',
                'focus-visible:ring-ring/50 focus-visible:ring-2 focus-visible:outline-none',
                active
                  ? 'bg-muted text-foreground ring-border/80 ring-1'
                  : 'text-muted-foreground hover:bg-muted/70 hover:text-foreground'
              )}
            >
              <span>{tab.label}</span>
              <span
                className={cn(
                  'rounded-md px-1.5 py-0.5 text-[10px] tabular-nums',
                  active
                    ? 'bg-background text-muted-foreground'
                    : 'bg-muted text-muted-foreground'
                )}
              >
                {tab.count}
              </span>
            </button>
          )
        })}
      </div>
    </nav>
  )
}
