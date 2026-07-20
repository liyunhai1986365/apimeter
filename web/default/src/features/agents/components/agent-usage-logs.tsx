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
import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, RotateCcw, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { listAgentAnalyticsLogs } from '../api'
import { AgentAnalyticsLogTable } from './agent-analytics-overview'

type RangePreset = 'day' | 'week' | 'month'

const RANGE_SECONDS: Record<RangePreset, number> = {
  day: 86400,
  week: 7 * 86400,
  month: 30 * 86400,
}

export function AgentUsageLogs() {
  const { t } = useTranslation()
  const [range, setRange] = useState<RangePreset>('week')
  const [rangeEnd, setRangeEnd] = useState(() => Math.floor(Date.now() / 1000))
  const [page, setPage] = useState(1)
  const [logType, setLogType] = useState('all')
  const [username, setUsername] = useState('')
  const [modelName, setModelName] = useState('')
  const [requestId, setRequestId] = useState('')
  const [appliedFilters, setAppliedFilters] = useState({
    username: '',
    modelName: '',
    requestId: '',
  })
  const timeRange = useMemo(() => {
    return {
      start_timestamp: rangeEnd - RANGE_SECONDS[range],
      end_timestamp: rangeEnd,
    }
  }, [range, rangeEnd])
  const logsQuery = useQuery({
    queryKey: [
      'agent',
      'analytics-logs',
      range,
      page,
      logType,
      appliedFilters,
      timeRange,
    ],
    queryFn: () =>
      listAgentAnalyticsLogs({
        ...timeRange,
        p: page,
        page_size: 20,
        ...(logType === 'all' ? {} : { type: Number(logType) }),
        ...(appliedFilters.username
          ? { username: appliedFilters.username }
          : {}),
        ...(appliedFilters.modelName
          ? { model_name: appliedFilters.modelName }
          : {}),
        ...(appliedFilters.requestId
          ? { request_id: appliedFilters.requestId }
          : {}),
      }),
  })
  const pageData = logsQuery.data?.data
  const totalPages = Math.max(1, Math.ceil((pageData?.total ?? 0) / 20))

  const applyFilters = () => {
    setPage(1)
    setRangeEnd(Math.floor(Date.now() / 1000))
    setAppliedFilters({
      username: username.trim(),
      modelName: modelName.trim(),
      requestId: requestId.trim(),
    })
  }
  const resetFilters = () => {
    setUsername('')
    setModelName('')
    setRequestId('')
    setLogType('all')
    setPage(1)
    setAppliedFilters({ username: '', modelName: '', requestId: '' })
  }

  return (
    <div className='flex flex-col gap-4'>
      <div className='flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between'>
        <div>
          <h2 className='text-base font-semibold'>{t('Agent Usage Logs')}</h2>
          <p className='text-muted-foreground text-sm'>
            {t('Inspect requests from every user under the current agent.')}
          </p>
        </div>
        <ToggleGroup
          value={[range]}
          onValueChange={(value) => {
            const next = value[0] as RangePreset | undefined
            if (next) {
              setRange(next)
              setRangeEnd(Math.floor(Date.now() / 1000))
              setPage(1)
            }
          }}
          variant='outline'
          size='sm'
          aria-label={t('Time Range')}
        >
          <ToggleGroupItem value='day'>{t('24 hours')}</ToggleGroupItem>
          <ToggleGroupItem value='week'>{t('7 days')}</ToggleGroupItem>
          <ToggleGroupItem value='month'>{t('30 days')}</ToggleGroupItem>
        </ToggleGroup>
      </div>

      <div className='grid gap-2 md:grid-cols-2 xl:grid-cols-[160px_repeat(3,minmax(0,1fr))_auto]'>
        <Select
          value={logType}
          onValueChange={(value) => {
            if (value) {
              setLogType(value)
              setPage(1)
            }
          }}
        >
          <SelectTrigger className='w-full'>
            <SelectValue placeholder={t('Log Type')} />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value='all'>{t('All Results')}</SelectItem>
              <SelectItem value='2'>{t('Success')}</SelectItem>
              <SelectItem value='5'>{t('Failed')}</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
        <Input
          value={username}
          onChange={(event) => setUsername(event.target.value)}
          placeholder={t('Username')}
          onKeyDown={(event) => {
            if (event.key === 'Enter') applyFilters()
          }}
        />
        <Input
          value={modelName}
          onChange={(event) => setModelName(event.target.value)}
          placeholder={t('Model')}
          onKeyDown={(event) => {
            if (event.key === 'Enter') applyFilters()
          }}
        />
        <Input
          value={requestId}
          onChange={(event) => setRequestId(event.target.value)}
          placeholder={t('Request ID')}
          onKeyDown={(event) => {
            if (event.key === 'Enter') applyFilters()
          }}
        />
        <div className='flex gap-2'>
          <Button onClick={applyFilters}>
            <Search data-icon='inline-start' />
            {t('Search')}
          </Button>
          <Button variant='outline' size='icon' onClick={resetFilters}>
            <RotateCcw />
            <span className='sr-only'>{t('Reset')}</span>
          </Button>
        </div>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t('Request Logs')}</CardTitle>
          <CardDescription>
            {t('{{count}} matching records', {
              count: pageData?.total ?? 0,
            })}
          </CardDescription>
        </CardHeader>
        <CardContent className='px-0'>
          <AgentAnalyticsLogTable
            logs={pageData?.items ?? []}
            loading={logsQuery.isLoading || logsQuery.isFetching}
          />
        </CardContent>
      </Card>

      <div className='flex items-center justify-between gap-3'>
        <p className='text-muted-foreground text-sm'>
          {t('Page {{page}} of {{total}}', { page, total: totalPages })}
        </p>
        <div className='flex gap-2'>
          <Button
            variant='outline'
            size='icon'
            disabled={page <= 1 || logsQuery.isFetching}
            onClick={() => setPage((current) => Math.max(1, current - 1))}
          >
            <ChevronLeft />
            <span className='sr-only'>{t('Previous')}</span>
          </Button>
          <Button
            variant='outline'
            size='icon'
            disabled={page >= totalPages || logsQuery.isFetching}
            onClick={() => setPage((current) => current + 1)}
          >
            <ChevronRight />
            <span className='sr-only'>{t('Next')}</span>
          </Button>
        </div>
      </div>
    </div>
  )
}
