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
import { useCallback, useEffect, useState } from 'react'
import type { KeyboardEvent } from 'react'
import { useIsFetching, useQueryClient } from '@tanstack/react-query'
import { getRouteApi, useNavigate } from '@tanstack/react-router'
import { type Table } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { useIsAdmin } from '@/hooks/use-admin'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { DataTableToolbar } from '@/components/data-table'
import { FilterCombobox } from '@/components/filter-combobox'
import { useLogFilterOptions } from '../hooks/use-log-filter-options'
import { getDefaultTimeRange } from '../lib/utils'
import type { CommonLogFilters } from '../types'
import { CompactDateTimeRangePicker } from './compact-date-time-range-picker'

const route = getRouteApi('/_authenticated/usage-logs/$section')
const ALL_CHANNEL_FILTER_VALUE = '__all_channels__'
const ALL_ACTION_VALUE = '__all_actions__'
const ALL_FINAL_STATUS_VALUE = '__all_final_status__'
const actionOptions = ['retry', 'failover', 'skip_retry']
const finalStatusOptions = ['pending', 'succeeded', 'failed']

export function RetryRouteEventsFilterBar<TData>(props: {
  table: Table<TData>
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const searchParams = route.useSearch()
  const isAdmin = useIsAdmin()
  const fetchingLogs = useIsFetching({ queryKey: ['logs'] })
  const [filters, setFilters] = useState<CommonLogFilters>(() => {
    const { start, end } = getDefaultTimeRange()
    return { startTime: start, endTime: end }
  })
  const [action, setAction] = useState('')
  const [finalStatus, setFinalStatus] = useState('')
  const { channelOptions } = useLogFilterOptions({
    channel: filters.channel,
    includeGroups: false,
    includeChannels: isAdmin,
  })

  useEffect(() => {
    const { start, end } = getDefaultTimeRange()
    setFilters({
      startTime: searchParams.startTime
        ? new Date(searchParams.startTime)
        : start,
      endTime: searchParams.endTime ? new Date(searchParams.endTime) : end,
      ...(searchParams.channel
        ? { channel: String(searchParams.channel) }
        : {}),
      ...(searchParams.model ? { model: String(searchParams.model) } : {}),
      ...(searchParams.requestId
        ? { requestId: String(searchParams.requestId) }
        : {}),
      ...(searchParams.group ? { group: String(searchParams.group) } : {}),
    })
    setAction(searchParams.filter ? String(searchParams.filter) : '')
    setFinalStatus(searchParams.status ? String(searchParams.status) : '')
  }, [
    searchParams.startTime,
    searchParams.endTime,
    searchParams.channel,
    searchParams.model,
    searchParams.requestId,
    searchParams.group,
    searchParams.filter,
    searchParams.status,
  ])

  const handleChange = useCallback(
    (field: keyof CommonLogFilters, value: Date | string | undefined) => {
      setFilters((prev) => ({ ...prev, [field]: value }))
    },
    []
  )

  const handleApply = useCallback(() => {
    navigate({
      to: '/usage-logs/$section',
      params: { section: 'retry-route-events' },
      search: {
        page: 1,
        ...(filters.startTime
          ? { startTime: filters.startTime.getTime() }
          : {}),
        ...(filters.endTime ? { endTime: filters.endTime.getTime() } : {}),
        ...(filters.model ? { model: filters.model } : {}),
        ...(filters.channel ? { channel: filters.channel } : {}),
        ...(filters.group ? { group: filters.group } : {}),
        ...(filters.requestId ? { requestId: filters.requestId } : {}),
        ...(action ? { filter: action } : {}),
        ...(finalStatus ? { status: finalStatus } : {}),
      },
    })
    queryClient.invalidateQueries({ queryKey: ['logs'] })
  }, [action, filters, finalStatus, navigate, queryClient])

  const handleReset = useCallback(() => {
    const { start, end } = getDefaultTimeRange()
    setFilters({ startTime: start, endTime: end })
    setAction('')
    setFinalStatus('')
    navigate({
      to: '/usage-logs/$section',
      params: { section: 'retry-route-events' },
      search: {
        page: 1,
        startTime: start.getTime(),
        endTime: end.getTime(),
      },
    })
    queryClient.invalidateQueries({ queryKey: ['logs'] })
  }, [navigate, queryClient])

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === 'Enter') handleApply()
    },
    [handleApply]
  )

  const inputClass = 'w-full sm:w-[180px] lg:w-[200px]'
  const hasAdditionalFilters =
    !!filters.model ||
    !!filters.channel ||
    !!filters.group ||
    !!filters.requestId ||
    !!action ||
    !!finalStatus

  return (
    <DataTableToolbar
      table={props.table}
      customSearch={
        <CompactDateTimeRangePicker
          start={filters.startTime}
          end={filters.endTime}
          onChange={({ start, end }) => {
            handleChange('startTime', start)
            handleChange('endTime', end)
          }}
          className='w-full sm:w-[340px]'
        />
      }
      additionalSearch={
        <>
          <Input
            placeholder={t('Request ID')}
            value={filters.requestId || ''}
            onChange={(e) => handleChange('requestId', e.target.value)}
            onKeyDown={handleKeyDown}
            className={inputClass}
          />
          <Input
            placeholder={t('Model Name')}
            value={filters.model || ''}
            onChange={(e) => handleChange('model', e.target.value)}
            onKeyDown={handleKeyDown}
            className={inputClass}
          />
          <Select
            value={action || ALL_ACTION_VALUE}
            onValueChange={(value) =>
              setAction(value === ALL_ACTION_VALUE ? '' : (value ?? ''))
            }
          >
            <SelectTrigger className={inputClass}>
              <SelectValue placeholder={t('All Actions')} />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                <SelectItem value={ALL_ACTION_VALUE}>
                  {t('All Actions')}
                </SelectItem>
                {actionOptions.map((value) => (
                  <SelectItem key={value} value={value}>
                    {t(value)}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <Select
            value={finalStatus || ALL_FINAL_STATUS_VALUE}
            onValueChange={(value) =>
              setFinalStatus(
                value === ALL_FINAL_STATUS_VALUE ? '' : (value ?? '')
              )
            }
          >
            <SelectTrigger className={inputClass}>
              <SelectValue placeholder={t('All Statuses')} />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                <SelectItem value={ALL_FINAL_STATUS_VALUE}>
                  {t('All Statuses')}
                </SelectItem>
                {finalStatusOptions.map((value) => (
                  <SelectItem key={value} value={value}>
                    {t(value)}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <Input
            placeholder={t('Target Group')}
            value={filters.group || ''}
            onChange={(e) => handleChange('group', e.target.value)}
            onKeyDown={handleKeyDown}
            className={inputClass}
          />
          {isAdmin && (
            <FilterCombobox
              options={channelOptions}
              value={filters.channel}
              allValue={ALL_CHANNEL_FILTER_VALUE}
              allLabel={t('All Channels')}
              placeholder={t('Source Channel')}
              onValueChange={(value) => handleChange('channel', value)}
              className={inputClass}
            />
          )}
        </>
      }
      hasAdditionalFilters={hasAdditionalFilters}
      onSearch={handleApply}
      searchLoading={fetchingLogs > 0}
      onReset={handleReset}
    />
  )
}
