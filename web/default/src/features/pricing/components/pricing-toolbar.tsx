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
import { useCallback, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  ArrowUpDownIcon,
  GridViewIcon,
  Table01Icon,
  Tick02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon, type IconSvgElement } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { useIsAdmin } from '@/hooks/use-admin'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { getPricingUserGroups } from '../api'
import {
  VIEW_MODES,
  getSortLabels,
  type SortOption,
  type ViewMode,
} from '../constants'
import type {
  PricingGroupDisplayConfig,
  PricingModel,
  TokenUnit,
} from '../types'
import { DownloadQuotationButton } from './download-quotation-button'
import { SearchBar } from './search-bar'

type SegmentOption = {
  value: string
  label?: string
  icon?: IconSvgElement
  tooltip?: string
}

export interface PricingToolbarProps {
  filteredCount: number
  totalCount?: number
  searchValue: string
  onSearchChange: (value: string) => void
  onClearSearch: () => void
  sortBy: string
  onSortChange: (value: string) => void
  tokenUnit: TokenUnit
  onTokenUnitChange: (value: TokenUnit) => void
  viewMode: ViewMode
  onViewModeChange: (value: ViewMode) => void
  hasActiveFilters: boolean
  quotationModels: PricingModel[]
  priceRate: number
  usdExchangeRate: number
  userGroup?: string
  onUserGroupChange: (value: string) => void
  usableGroup: Record<string, string | { desc?: string; ratio?: number }>
  groupDisplay?: PricingGroupDisplayConfig
}

function SegmentedControl(props: {
  options: SegmentOption[]
  value: string
  onChange: (value: string) => void
  ariaLabel: string
}) {
  return (
    <div
      role='group'
      aria-label={props.ariaLabel}
      className='bg-muted/50 inline-flex h-7 items-center rounded-md border p-0.5'
    >
      {props.options.map((option) => {
        const isActive = option.value === props.value
        const button = (
          <button
            key={option.value}
            type='button'
            onClick={() => props.onChange(option.value)}
            aria-pressed={isActive}
            className={cn(
              'inline-flex h-full items-center justify-center rounded-sm text-xs font-medium transition-all',
              option.icon && !option.label ? 'w-6' : 'gap-1 px-2.5',
              isActive
                ? 'bg-background text-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground'
            )}
          >
            {option.icon && <HugeiconsIcon icon={option.icon} />}
            {option.label}
          </button>
        )

        if (!option.tooltip) {
          return button
        }

        return (
          <Tooltip key={option.value}>
            <TooltipTrigger render={button}></TooltipTrigger>
            <TooltipContent side='bottom' className='text-xs'>
              {option.tooltip}
            </TooltipContent>
          </Tooltip>
        )
      })}
    </div>
  )
}

export function PricingToolbar(props: PricingToolbarProps) {
  const { t } = useTranslation()
  const isAdmin = useIsAdmin()
  const { onUserGroupChange, userGroup } = props
  const sortLabels = getSortLabels(t)
  const {
    data: userGroups = [],
    isLoading: isLoadingUserGroups,
    isError: didUserGroupsFail,
  } = useQuery({
    queryKey: ['pricing', 'user-groups'],
    queryFn: getPricingUserGroups,
    enabled: isAdmin,
    staleTime: 5 * 60 * 1000,
  })

  useEffect(() => {
    if (didUserGroupsFail) toast.error(t('Unable to load user groups'))
  }, [didUserGroupsFail, t])

  useEffect(() => {
    if (
      !isAdmin ||
      isLoadingUserGroups ||
      userGroups.length === 0 ||
      (userGroup && userGroups.includes(userGroup))
    ) {
      return
    }
    onUserGroupChange(userGroups[0])
  }, [isAdmin, isLoadingUserGroups, onUserGroupChange, userGroup, userGroups])

  const handleTokenUnitChange = useCallback(
    (value: string) => props.onTokenUnitChange(value as TokenUnit),
    [props]
  )

  const handleViewModeChange = useCallback(
    (value: string) => props.onViewModeChange(value as ViewMode),
    [props]
  )

  return (
    <div className='bg-background/95 rounded-lg border px-2.5 py-2 shadow-xs'>
      <div className='flex flex-col gap-2 lg:flex-row lg:items-center lg:justify-between'>
        <div className='flex min-w-0 flex-col gap-1.5 sm:flex-row sm:items-center'>
          <div className='text-muted-foreground flex shrink-0 items-baseline gap-1 text-xs'>
            <span className='text-foreground font-semibold tabular-nums'>
              {props.filteredCount.toLocaleString()}
            </span>
            <span>{props.filteredCount === 1 ? t('model') : t('models')}</span>
            {props.hasActiveFilters && props.totalCount && (
              <span className='text-muted-foreground/60 text-xs'>
                / {props.totalCount.toLocaleString()}
              </span>
            )}
          </div>

          <SearchBar
            value={props.searchValue}
            onChange={props.onSearchChange}
            onClear={props.onClearSearch}
            placeholder={t('Search model name, provider, endpoint, or tag...')}
            className='w-full sm:w-[20rem] lg:w-[24rem]'
          />
        </div>

        <div className='flex flex-wrap items-center gap-1.5'>
          {isAdmin && (
            <Select
              value={userGroup ?? ''}
              onValueChange={(value) => {
                if (value) onUserGroupChange(value)
              }}
              disabled={isLoadingUserGroups || userGroups.length === 0}
            >
              <SelectTrigger
                size='sm'
                className='h-7 w-[12rem] rounded-md text-xs'
                aria-label={t('User group')}
                title={t('User group')}
              >
                <span className='text-muted-foreground'>
                  {t('User group')}:
                </span>
                <SelectValue
                  placeholder={
                    isLoadingUserGroups
                      ? t('Loading user groups...')
                      : t('Select a user group')
                  }
                />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {userGroups.map((group) => (
                    <SelectItem key={group} value={group}>
                      {group}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          )}

          <DownloadQuotationButton
            models={props.quotationModels}
            tokenUnit={props.tokenUnit}
            priceRate={props.priceRate}
            usdExchangeRate={props.usdExchangeRate}
            userGroup={props.userGroup}
            usableGroup={props.usableGroup}
            groupDisplay={props.groupDisplay}
            hasActiveFilters={
              props.hasActiveFilters || Boolean(props.searchValue.trim())
            }
          />

          <div className='hidden items-center sm:flex'>
            <SegmentedControl
              options={[
                { value: 'M', label: '/1M' },
                { value: 'K', label: '/1K' },
              ]}
              value={props.tokenUnit}
              onChange={handleTokenUnitChange}
              ariaLabel={t('Token unit')}
            />
          </div>

          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  className='h-7 gap-1.5 rounded-md px-2.5 text-xs'
                />
              }
            >
              <HugeiconsIcon icon={ArrowUpDownIcon} data-icon='inline-start' />
              <span>{sortLabels[props.sortBy as SortOption] || t('Sort')}</span>
            </DropdownMenuTrigger>
            <DropdownMenuContent align='end' className='w-44'>
              <DropdownMenuGroup>
                {Object.entries(sortLabels).map(([value, label]) => (
                  <DropdownMenuItem
                    key={value}
                    onClick={() => props.onSortChange(value)}
                    className='gap-2'
                  >
                    <HugeiconsIcon
                      icon={Tick02Icon}
                      className={cn(
                        'shrink-0',
                        props.sortBy === value ? 'opacity-100' : 'opacity-0'
                      )}
                    />
                    {label}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>

          <SegmentedControl
            options={[
              {
                value: VIEW_MODES.CARD,
                icon: GridViewIcon,
                tooltip: t('Card view'),
              },
              {
                value: VIEW_MODES.TABLE,
                icon: Table01Icon,
                tooltip: t('Table view'),
              },
            ]}
            value={props.viewMode}
            onChange={handleViewModeChange}
            ariaLabel={t('View mode')}
          />
        </div>
      </div>
    </div>
  )
}
