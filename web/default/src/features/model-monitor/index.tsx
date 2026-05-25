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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ChevronDown,
  ChevronRight,
  Loader2,
  Play,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  formatNumber,
  formatPercent,
  formatTimestampToDate,
} from '@/lib/format'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { SectionPageLayout } from '@/components/layout'
import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import { removeChannelModel } from '@/features/channels/api'
import {
  listModelMonitorModels,
  testModelChannels,
} from '@/features/models/api'
import type {
  ModelChannelTestItem,
  ModelMonitorChannel,
  ModelMonitorListItem,
} from '@/features/models/types'

const windowOptions = [
  { value: '10m', label: 'Last 10 minutes' },
  { value: '30m', label: 'Last 30 minutes' },
  { value: '1h', label: 'Last 1 hour' },
  { value: '6h', label: 'Last 6 hours' },
  { value: '24h', label: 'Last 24 hours' },
]

function healthVariant(score: number): StatusVariant {
  if (score >= 90) return 'success'
  if (score >= 70) return 'info'
  if (score >= 50) return 'warning'
  return 'danger'
}

function errorVariant(rate: number): StatusVariant {
  if (rate <= 1) return 'success'
  if (rate <= 5) return 'info'
  if (rate <= 15) return 'warning'
  return 'danger'
}

function latencyVariant(p95UseTime: number): StatusVariant {
  if (p95UseTime <= 0) return 'neutral'
  if (p95UseTime <= 3) return 'success'
  if (p95UseTime <= 10) return 'info'
  if (p95UseTime <= 20) return 'warning'
  return 'danger'
}

function formatUseTime(value: number) {
  if (!value) return '-'
  if (value < 1) return `${Math.round(value * 1000)}ms`
  return `${value.toFixed(2)}s`
}

export function ModelMonitor() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [keyword, setKeyword] = useState('')
  const [windowValue, setWindowValue] = useState('10m')
  const [page, setPage] = useState(1)
  const [selectedModelIds, setSelectedModelIds] = useState<Set<number>>(
    () => new Set()
  )
  const [expandedModelIds, setExpandedModelIds] = useState<Set<number>>(
    () => new Set()
  )
  const [removeTarget, setRemoveTarget] = useState<{
    channel: ModelMonitorChannel
    modelName: string
  } | null>(null)
  const pageSize = 10

  const queryKey = ['model-monitor', { keyword, windowValue, page, pageSize }]
  const query = useQuery({
    queryKey,
    queryFn: () =>
      listModelMonitorModels({
        keyword,
        window: windowValue,
        p: page,
        page_size: pageSize,
      }),
    staleTime: 30 * 1000,
  })

  const data = query.data?.data
  const items = useMemo(() => data?.items || [], [data?.items])
  const total = data?.total || 0
  const pageCount = Math.max(1, Math.ceil(total / pageSize))
  const selectedCount = selectedModelIds.size
  const allPageSelected =
    items.length > 0 &&
    items.every((item) => selectedModelIds.has(item.model_id))
  const somePageSelected = items.some((item) =>
    selectedModelIds.has(item.model_id)
  )

  const testMutation = useMutation({
    mutationFn: async (modelIDs: number[]) => {
      const results = []
      for (const modelID of modelIDs) {
        results.push(await testModelChannels(modelID))
      }
      return results
    },
    onSuccess: (responses) => {
      const failed = responses.filter((response) => !response.success)
      if (failed.length === 0) {
        toast.success(t('Channel test completed'))
      } else {
        toast.error(t('Some channel tests failed'))
      }
      queryClient.invalidateQueries({ queryKey: ['model-monitor'] })
    },
    onError: () => toast.error(t('Failed to test channels')),
  })

  const removeModelMutation = useMutation({
    mutationFn: async ({
      channelID,
      modelName,
    }: {
      channelID: number
      modelName: string
    }) => {
      const response = await removeChannelModel(channelID, modelName)
      if (!response.success) {
        throw new Error(response.message || t('Failed to remove model'))
      }
      return response
    },
    onSuccess: () => {
      toast.success(t('Model removed from channel'))
      setRemoveTarget(null)
      queryClient.invalidateQueries({ queryKey: ['model-monitor'] })
      queryClient.invalidateQueries({ queryKey: ['channels'] })
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Failed to remove model')
      )
    },
  })

  const togglePageSelection = (checked: boolean) => {
    setSelectedModelIds((prev) => {
      const next = new Set(prev)
      for (const item of items) {
        if (checked) {
          next.add(item.model_id)
        } else {
          next.delete(item.model_id)
        }
      }
      return next
    })
  }

  const toggleModelSelection = (modelID: number, checked: boolean) => {
    setSelectedModelIds((prev) => {
      const next = new Set(prev)
      if (checked) {
        next.add(modelID)
      } else {
        next.delete(modelID)
      }
      return next
    })
  }

  const toggleExpanded = (modelID: number) => {
    setExpandedModelIds((prev) => {
      const next = new Set(prev)
      if (next.has(modelID)) {
        next.delete(modelID)
      } else {
        next.add(modelID)
      }
      return next
    })
  }

  const handleTestSelected = () => {
    const modelIDs = [...selectedModelIds]
    if (modelIDs.length === 0) return
    testMutation.mutate(modelIDs)
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Model Monitor')}</SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t('Monitor all model-square models using recent historical logs.')}
        </SectionPageLayout.Description>
        <SectionPageLayout.Actions>
          <Button
            variant='outline'
            size='sm'
            onClick={() => query.refetch()}
            disabled={query.isFetching}
          >
            {query.isFetching ? (
              <Loader2 className='h-4 w-4 animate-spin' />
            ) : (
              <RefreshCw className='h-4 w-4' />
            )}
            {t('Refresh')}
          </Button>
          <Button
            size='sm'
            disabled={selectedCount === 0 || testMutation.isPending}
            onClick={handleTestSelected}
          >
            {testMutation.isPending ? (
              <Loader2 className='h-4 w-4 animate-spin' />
            ) : (
              <Play className='h-4 w-4' />
            )}
            {t('Test Selected Models')}
            {selectedCount > 0 ? ` (${selectedCount})` : ''}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='space-y-4'>
            <div className='flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between'>
              <div className='flex flex-col gap-3 sm:flex-row sm:items-center'>
                <Input
                  value={keyword}
                  onChange={(event) => {
                    setKeyword(event.target.value)
                    setPage(1)
                  }}
                  placeholder={t('Filter by model name...')}
                  className='sm:w-72'
                />
                <Select
                  items={windowOptions.map((option) => ({
                    value: option.value,
                    label: t(option.label),
                  }))}
                  value={windowValue}
                  onValueChange={(value) => {
                    if (value) {
                      setWindowValue(value)
                      setPage(1)
                    }
                  }}
                >
                  <SelectTrigger className='w-[190px]'>
                    <SelectValue placeholder={t('Last 10 minutes')} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {windowOptions.map((option) => (
                        <SelectItem key={option.value} value={option.value}>
                          {t(option.label)}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </div>
              {data?.window && (
                <span className='text-muted-foreground text-xs'>
                  {formatTimestampToDate(data.window.start_timestamp)}
                  {' - '}
                  {formatTimestampToDate(data.window.end_timestamp)}
                </span>
              )}
            </div>

            <div className='overflow-hidden rounded-md border'>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className='w-10'>
                      <Checkbox
                        checked={allPageSelected}
                        indeterminate={!allPageSelected && somePageSelected}
                        onCheckedChange={(checked) =>
                          togglePageSelection(Boolean(checked))
                        }
                        aria-label={t('Select all')}
                      />
                    </TableHead>
                    <TableHead>{t('Model')}</TableHead>
                    <TableHead>{t('Health')}</TableHead>
                    <TableHead>{t('Error Rate')}</TableHead>
                    <TableHead>{t('Latency')}</TableHead>
                    <TableHead>{t('Requests')}</TableHead>
                    <TableHead>{t('Channels')}</TableHead>
                    <TableHead>{t('Latest Test')}</TableHead>
                    <TableHead className='text-right'>{t('Actions')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {query.isLoading ? (
                    <TableRow>
                      <TableCell colSpan={9} className='h-32 text-center'>
                        <Loader2 className='mx-auto h-5 w-5 animate-spin' />
                      </TableCell>
                    </TableRow>
                  ) : items.length ? (
                    items.map((item) => (
                      <MonitorRow
                        key={item.model_id}
                        item={item}
                        selected={selectedModelIds.has(item.model_id)}
                        expanded={expandedModelIds.has(item.model_id)}
                        isTesting={
                          testMutation.isPending &&
                          testMutation.variables?.includes(item.model_id)
                        }
                        onSelect={(checked) =>
                          toggleModelSelection(item.model_id, checked)
                        }
                        onToggleExpanded={() => toggleExpanded(item.model_id)}
                        onTest={() => testMutation.mutate([item.model_id])}
                        onRemoveModel={(channel) =>
                          setRemoveTarget({
                            channel,
                            modelName: item.model_name,
                          })
                        }
                        removingChannelID={
                          removeModelMutation.isPending
                            ? removeModelMutation.variables?.channelID
                            : undefined
                        }
                      />
                    ))
                  ) : (
                    <TableRow>
                      <TableCell
                        colSpan={9}
                        className='text-muted-foreground h-32 text-center text-sm'
                      >
                        {t('No models found')}
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </div>

            <div className='flex items-center justify-between text-sm'>
              <span className='text-muted-foreground'>
                {t('Total')}: {formatNumber(total)}
              </span>
              <div className='flex items-center gap-2'>
                <Button
                  variant='outline'
                  size='sm'
                  disabled={page <= 1}
                  onClick={() => setPage((prev) => Math.max(1, prev - 1))}
                >
                  {t('Previous')}
                </Button>
                <span className='text-muted-foreground text-xs'>
                  {page} / {pageCount}
                </span>
                <Button
                  variant='outline'
                  size='sm'
                  disabled={page >= pageCount}
                  onClick={() =>
                    setPage((prev) => Math.min(pageCount, prev + 1))
                  }
                >
                  {t('Next')}
                </Button>
              </div>
            </div>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>
      <ConfirmDialog
        open={removeTarget !== null}
        onOpenChange={(open) => {
          if (!open && !removeModelMutation.isPending) {
            setRemoveTarget(null)
          }
        }}
        title={t('Remove failed model from channel?')}
        desc={
          removeTarget
            ? t(
                'This will edit channel "{{channel}}" and remove model "{{model}}" from its model list.',
                {
                  channel: removeTarget.channel.channel_name,
                  model: removeTarget.modelName,
                }
              )
            : ''
        }
        confirmText={t('Remove')}
        destructive
        isLoading={removeModelMutation.isPending}
        handleConfirm={() => {
          if (!removeTarget) return
          removeModelMutation.mutate({
            channelID: removeTarget.channel.channel_id,
            modelName: removeTarget.modelName,
          })
        }}
      />
    </>
  )
}

function MonitorRow({
  item,
  selected,
  expanded,
  isTesting,
  onSelect,
  onToggleExpanded,
  onTest,
  onRemoveModel,
  removingChannelID,
}: {
  item: ModelMonitorListItem
  selected: boolean
  expanded: boolean
  isTesting: boolean
  onSelect: (checked: boolean) => void
  onToggleExpanded: () => void
  onTest: () => void
  onRemoveModel: (channel: ModelMonitorChannel) => void
  removingChannelID?: number
}) {
  const { t } = useTranslation()
  const latestTestByChannel = useMemo(
    () => new Map(item.latest_tests?.map((test) => [test.channel_id, test])),
    [item.latest_tests]
  )
  const latestTest = item.latest_tests?.[0]
  const successfulTests =
    item.latest_tests?.filter((test) => test.success).length || 0

  return (
    <>
      <TableRow className={cn(expanded && 'bg-muted/30')}>
        <TableCell>
          <Checkbox
            checked={selected}
            onCheckedChange={(checked) => onSelect(Boolean(checked))}
            aria-label={t('Select model')}
          />
        </TableCell>
        <TableCell>
          <button
            type='button'
            onClick={onToggleExpanded}
            className='flex min-w-0 items-start gap-2 text-left'
          >
            <span className='text-muted-foreground mt-0.5'>
              {expanded ? (
                <ChevronDown className='h-4 w-4' />
              ) : (
                <ChevronRight className='h-4 w-4' />
              )}
            </span>
            <span className='flex min-w-0 flex-col gap-1'>
              <span className='font-mono text-sm font-medium'>
                {item.model_name}
              </span>
            </span>
          </button>
        </TableCell>
        <TableCell>
          <StatusBadge
            label={`${item.summary.health_score}`}
            variant={healthVariant(item.summary.health_score)}
            copyable={false}
          />
        </TableCell>
        <TableCell>
          <MetricStack
            primary={formatPercent(item.summary.error_rate)}
            secondary={`${formatNumber(item.summary.error_requests)} ${t('errors')}`}
            variant={errorVariant(item.summary.error_rate)}
          />
        </TableCell>
        <TableCell>
          <MetricStack
            primary={formatUseTime(item.summary.avg_use_time)}
            secondary={`P95 ${formatUseTime(item.summary.p95_use_time)}`}
            variant={latencyVariant(item.summary.p95_use_time)}
          />
        </TableCell>
        <TableCell>
          <MetricStack
            primary={formatNumber(item.summary.total_requests)}
            secondary={`${formatNumber(item.summary.success_requests)} ${t('success')}`}
          />
        </TableCell>
        <TableCell>{formatNumber(item.configured_channels)}</TableCell>
        <TableCell>
          <LatestTestSummary
            latestTest={latestTest}
            successCount={successfulTests}
            totalCount={item.latest_tests.length}
          />
        </TableCell>
        <TableCell className='text-right'>
          <Button
            size='sm'
            variant='outline'
            onClick={onTest}
            disabled={isTesting}
          >
            {isTesting ? (
              <Loader2 className='h-4 w-4 animate-spin' />
            ) : (
              <Play className='h-4 w-4' />
            )}
            {t('Test Channels')}
          </Button>
        </TableCell>
      </TableRow>
      {expanded && (
        <TableRow>
          <TableCell colSpan={9} className='bg-muted/20 p-0'>
            <ChannelStatsTable
              channels={item.channels || []}
              latestTestByChannel={latestTestByChannel}
              onRemoveModel={onRemoveModel}
              removingChannelID={removingChannelID}
            />
          </TableCell>
        </TableRow>
      )}
    </>
  )
}

function ChannelStatsTable({
  channels,
  latestTestByChannel,
  onRemoveModel,
  removingChannelID,
}: {
  channels: ModelMonitorChannel[]
  latestTestByChannel: Map<number, ModelChannelTestItem>
  onRemoveModel: (channel: ModelMonitorChannel) => void
  removingChannelID?: number
}) {
  const { t } = useTranslation()

  if (channels.length === 0) {
    return (
      <div className='text-muted-foreground px-12 py-6 text-sm'>
        {t('No configured channels')}
      </div>
    )
  }

  return (
    <div className='px-10 py-3'>
      <div className='bg-background overflow-hidden rounded-md border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Channel')}</TableHead>
              <TableHead>{t('Health')}</TableHead>
              <TableHead>{t('Error Rate')}</TableHead>
              <TableHead>{t('Latency')}</TableHead>
              <TableHead>{t('Requests')}</TableHead>
              <TableHead>{t('Latest Error')}</TableHead>
              <TableHead>{t('Latest Test')}</TableHead>
              <TableHead className='text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {channels.map((channel) => {
              const latestTest = latestTestByChannel.get(channel.channel_id)
              const canRemove = latestTest && !latestTest.success
              const isRemoving = removingChannelID === channel.channel_id
              return (
                <TableRow key={channel.channel_id}>
                  <TableCell>
                    <div className='flex flex-col gap-1'>
                      <span className='font-medium'>
                        {channel.channel_name}
                      </span>
                      <span className='text-muted-foreground text-xs'>
                        ID {channel.channel_id} · {t('Group')}:{' '}
                        {channel.group || '-'}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell>
                    <StatusBadge
                      label={`${channel.health_score}`}
                      variant={healthVariant(channel.health_score)}
                      copyable={false}
                    />
                  </TableCell>
                  <TableCell>
                    <MetricStack
                      primary={formatPercent(channel.error_rate)}
                      secondary={`${formatNumber(channel.error_requests)} ${t('errors')}`}
                      variant={errorVariant(channel.error_rate)}
                    />
                  </TableCell>
                  <TableCell>
                    <MetricStack
                      primary={formatUseTime(channel.avg_use_time)}
                      secondary={`P95 ${formatUseTime(channel.p95_use_time)}`}
                      variant={latencyVariant(channel.p95_use_time)}
                    />
                  </TableCell>
                  <TableCell>
                    <MetricStack
                      primary={formatNumber(channel.total_requests)}
                      secondary={`${formatNumber(channel.success_requests)} ${t('success')}`}
                    />
                  </TableCell>
                  <TableCell className='max-w-[280px]'>
                    {channel.latest_error ? (
                      <div className='flex flex-col gap-1 text-xs'>
                        <span className='line-clamp-2'>
                          {channel.latest_error}
                        </span>
                        <span className='text-muted-foreground'>
                          {channel.latest_error_code || '-'}
                          {channel.latest_error_at
                            ? ` · ${formatTimestampToDate(channel.latest_error_at)}`
                            : ''}
                        </span>
                      </div>
                    ) : (
                      <span className='text-muted-foreground text-xs'>
                        {t('No recent errors')}
                      </span>
                    )}
                  </TableCell>
                  <TableCell>
                    <ChannelTestSummary test={latestTest} />
                  </TableCell>
                  <TableCell className='text-right'>
                    {canRemove ? (
                      <Button
                        size='sm'
                        variant='destructive'
                        onClick={() => onRemoveModel(channel)}
                        disabled={isRemoving}
                      >
                        {isRemoving ? (
                          <Loader2 className='h-4 w-4 animate-spin' />
                        ) : (
                          <Trash2 className='h-4 w-4' />
                        )}
                        {t('Remove')}
                      </Button>
                    ) : (
                      <span className='text-muted-foreground text-xs'>-</span>
                    )}
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </div>
    </div>
  )
}

function MetricStack({
  primary,
  secondary,
  variant,
}: {
  primary: string
  secondary: string
  variant?: StatusVariant
}) {
  return (
    <div className='flex flex-col gap-1 text-xs'>
      {variant ? (
        <StatusBadge label={primary} variant={variant} copyable={false} />
      ) : (
        <span className='text-sm font-medium'>{primary}</span>
      )}
      <span className='text-muted-foreground'>{secondary}</span>
    </div>
  )
}

function LatestTestSummary({
  latestTest,
  successCount,
  totalCount,
}: {
  latestTest?: ModelChannelTestItem
  successCount: number
  totalCount: number
}) {
  const { t } = useTranslation()
  if (!latestTest || totalCount === 0) {
    return (
      <span className='text-muted-foreground text-xs'>{t('Not tested')}</span>
    )
  }
  return (
    <div className='flex flex-col gap-1 text-xs'>
      <StatusBadge
        label={`${successCount}/${totalCount} ${t('passed')}`}
        variant={successCount === totalCount ? 'success' : 'warning'}
        copyable={false}
      />
      <span className='text-muted-foreground'>
        {latestTest.created_at
          ? formatTimestampToDate(latestTest.created_at)
          : '-'}
      </span>
    </div>
  )
}

function ChannelTestSummary({ test }: { test?: ModelChannelTestItem }) {
  const { t } = useTranslation()
  if (!test) {
    return (
      <span className='text-muted-foreground text-xs'>{t('Not tested')}</span>
    )
  }
  return (
    <div className='flex flex-col gap-1 text-xs'>
      <StatusBadge
        label={test.success ? t('Passed') : t('Failed')}
        variant={test.success ? 'success' : 'danger'}
        copyable={false}
      />
      <span className='text-muted-foreground'>
        {formatUseTime(test.response_time)}
        {test.created_at ? ` · ${formatTimestampToDate(test.created_at)}` : ''}
      </span>
    </div>
  )
}
