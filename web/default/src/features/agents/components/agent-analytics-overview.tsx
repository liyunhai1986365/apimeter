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
import { useMemo, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { VChart } from '@visactor/react-vchart'
import {
  Activity,
  Banknote,
  Bot,
  ChartNoAxesCombined,
  CircleCheck,
  CircleX,
  Coins,
  Users,
  Zap,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { getChartColor } from '@/lib/colors'
import dayjs from '@/lib/dayjs'
import {
  formatNumber,
  formatQuota,
  formatTimestampToDate,
  formatTokens,
  formatUseTime,
} from '@/lib/format'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VCHART_OPTION } from '@/lib/vchart'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { TableEmpty } from '@/components/data-table'
import { TruncatedText } from '@/components/truncated-text'
import { getAgentAnalytics } from '../api'
import type { AgentAnalyticsLogItem, AgentAnalyticsSummary } from '../types'

type RangePreset = 'day' | 'week' | 'month'

const RANGE_SECONDS: Record<RangePreset, number> = {
  day: 86400,
  week: 7 * 86400,
  month: 30 * 86400,
}

function formatSettlementAmount(amount: number, currency: string) {
  const code = currency === 'RMB' ? 'RMB' : 'USD'
  const symbol = code === 'RMB' ? '¥' : '$'
  return `${symbol}${amount.toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 4,
  })} ${code}`
}

function AnalyticsMetric(props: {
  label: string
  value: string
  description: string
  icon: ReactNode
  loading: boolean
}) {
  return (
    <Card size='sm'>
      <CardHeader>
        <CardDescription className='flex items-center gap-2'>
          {props.icon}
          {props.label}
        </CardDescription>
        <CardTitle className='font-mono text-xl font-semibold tabular-nums'>
          {props.loading ? <Skeleton className='h-7 w-24' /> : props.value}
        </CardTitle>
      </CardHeader>
      <CardContent className='text-muted-foreground text-xs'>
        {props.description}
      </CardContent>
    </Card>
  )
}

function buildMetrics(
  summary: AgentAnalyticsSummary | undefined,
  loading: boolean,
  t: (key: string, options?: Record<string, unknown>) => string
) {
  const metrics = [
    {
      label: t('Agent Users'),
      value: formatNumber(summary?.total_users ?? 0),
      description: t('{{active}} active · {{new}} new', {
        active: formatNumber(summary?.active_users ?? 0),
        new: formatNumber(summary?.new_users ?? 0),
      }),
      icon: <Users />,
    },
    {
      label: t('Usage Users'),
      value: formatNumber(summary?.usage_users ?? 0),
      description: t('Users who made requests in this period'),
      icon: <Activity />,
    },
    {
      label: t('Total Requests'),
      value: formatNumber(summary?.total_requests ?? 0),
      description: t('{{success}} successful · {{failed}} failed', {
        success: formatNumber(summary?.successful_requests ?? 0),
        failed: formatNumber(summary?.failed_requests ?? 0),
      }),
      icon: <Zap />,
    },
    {
      label: t('Success Rate'),
      value: `${(summary?.success_rate ?? 0).toFixed(2)}%`,
      description: t('Successful requests divided by all requests'),
      icon: <CircleCheck />,
    },
    {
      label: t('Total Tokens'),
      value: formatTokens(summary?.total_tokens ?? 0),
      description: t('Input and output tokens combined'),
      icon: <Coins />,
    },
    {
      label: t('User Consumption'),
      value: formatQuota(summary?.total_quota ?? 0),
      description: t('Amount charged to agent users'),
      icon: <Banknote />,
    },
    {
      label: t('Agent Profit'),
      value: formatSettlementAmount(
        summary?.profit_amount ?? 0,
        summary?.currency ?? 'USD'
      ),
      description: t('Gross profit generated in this period'),
      icon: <ChartNoAxesCombined />,
    },
  ]
  return metrics.map((item) => ({ ...item, loading }))
}

export function AgentAnalyticsOverview() {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const [range, setRange] = useState<RangePreset>('week')
  const [rangeEnd, setRangeEnd] = useState(() => Math.floor(Date.now() / 1000))
  const timeRange = useMemo(() => {
    return {
      start_timestamp: rangeEnd - RANGE_SECONDS[range],
      end_timestamp: rangeEnd,
    }
  }, [range, rangeEnd])
  const analyticsQuery = useQuery({
    queryKey: ['agent', 'analytics', range, timeRange],
    queryFn: () => getAgentAnalytics(timeRange),
  })
  const analytics = analyticsQuery.data?.data
  const metrics = buildMetrics(analytics?.summary, analyticsQuery.isLoading, t)

  const trendSpec = useMemo(() => {
    const values = (analytics?.trend ?? []).flatMap((item) => {
      const time = dayjs(item.timestamp * 1000).format(
        analytics?.bucket_seconds === 3600 ? 'MM-DD HH:mm' : 'MM-DD'
      )
      return [
        { time, metric: t('Successful Requests'), value: item.requests },
        { time, metric: t('Failed Requests'), value: item.errors },
      ]
    })
    return {
      type: 'line',
      data: [{ id: 'trend', values }],
      xField: 'time',
      yField: 'value',
      seriesField: 'metric',
      color: [getChartColor(0), getChartColor(4)],
      point: { visible: values.length <= 48 },
      legends: { visible: true, orient: 'top' },
      axes: [
        { orient: 'left', min: 0, nice: true },
        { orient: 'bottom', label: { autoRotate: true, autoHide: true } },
      ],
      tooltip: { dimension: { visible: true } },
    }
  }, [analytics, t])

  const modelSpec = useMemo(() => {
    const values = (analytics?.top_models ?? [])
      .slice(0, 8)
      .map((item) => ({ model: item.model_name, requests: item.requests }))
      .reverse()
    return {
      type: 'bar',
      direction: 'horizontal',
      data: [{ id: 'models', values }],
      xField: 'requests',
      yField: 'model',
      color: getChartColor(1),
      bar: { style: { cornerRadius: 4 } },
      axes: [
        { orient: 'bottom', min: 0, nice: true },
        { orient: 'left', label: { autoLimit: true } },
      ],
      tooltip: { mark: { visible: true } },
    }
  }, [analytics])

  return (
    <div className='flex flex-col gap-4'>
      <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
        <div>
          <h2 className='text-base font-semibold'>
            {t('Agent Data Overview')}
          </h2>
          <p className='text-muted-foreground text-sm'>
            {t('Usage and revenue for all users under the current agent.')}
          </p>
        </div>
        <ToggleGroup
          value={[range]}
          onValueChange={(value) => {
            const next = value[0] as RangePreset | undefined
            if (next) {
              setRange(next)
              setRangeEnd(Math.floor(Date.now() / 1000))
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

      <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        {metrics.map((metric) => (
          <AnalyticsMetric key={metric.label} {...metric} />
        ))}
      </div>

      <div className='grid gap-4 xl:grid-cols-[minmax(0,1.45fr)_minmax(320px,0.75fr)]'>
        <Card>
          <CardHeader>
            <CardTitle>{t('Request Trend')}</CardTitle>
            <CardDescription>
              {t('Successful and failed requests over time')}
            </CardDescription>
          </CardHeader>
          <CardContent className='h-80'>
            {analyticsQuery.isLoading ? (
              <Skeleton className='size-full' />
            ) : themeReady ? (
              <VChart
                key={`${range}-${resolvedTheme}`}
                spec={{
                  ...trendSpec,
                  theme: resolvedTheme === 'dark' ? 'dark' : 'light',
                  background: 'transparent',
                }}
                option={VCHART_OPTION}
              />
            ) : null}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t('Top Models')}</CardTitle>
            <CardDescription>{t('Ranked by request count')}</CardDescription>
          </CardHeader>
          <CardContent className='h-80'>
            {analyticsQuery.isLoading ? (
              <Skeleton className='size-full' />
            ) : themeReady ? (
              <VChart
                key={`${range}-${resolvedTheme}-models`}
                spec={{
                  ...modelSpec,
                  theme: resolvedTheme === 'dark' ? 'dark' : 'light',
                  background: 'transparent',
                }}
                option={VCHART_OPTION}
              />
            ) : null}
          </CardContent>
        </Card>
      </div>

      <div className='grid gap-4 xl:grid-cols-2'>
        <Card>
          <CardHeader>
            <CardTitle>{t('Top Users')}</CardTitle>
            <CardDescription>
              {t('Highest request volume users')}
            </CardDescription>
          </CardHeader>
          <CardContent className='px-0'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className='pl-4'>{t('User')}</TableHead>
                  <TableHead>{t('Requests')}</TableHead>
                  <TableHead>{t('Total Tokens')}</TableHead>
                  <TableHead className='pr-4 text-right'>
                    {t('Consumption')}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {analytics?.top_users?.length ? (
                  analytics.top_users.map((user) => (
                    <TableRow key={user.user_id}>
                      <TableCell className='pl-4'>
                        <div className='flex flex-col gap-0.5'>
                          <span className='font-medium'>
                            {user.username || `#${user.user_id}`}
                          </span>
                          <span className='text-muted-foreground text-xs'>
                            #{user.user_id}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell className='font-mono tabular-nums'>
                        {formatNumber(user.requests)}
                        {user.errors > 0 ? (
                          <span className='text-muted-foreground ml-1 text-xs'>
                            / {formatNumber(user.errors)} {t('failed')}
                          </span>
                        ) : null}
                      </TableCell>
                      <TableCell>{formatTokens(user.tokens)}</TableCell>
                      <TableCell className='pr-4 text-right'>
                        {formatQuota(user.quota)}
                      </TableCell>
                    </TableRow>
                  ))
                ) : (
                  <TableEmpty
                    colSpan={4}
                    title={t('No usage data')}
                    description={t(
                      'Usage will appear after agent users make requests.'
                    )}
                    icon={<Users />}
                  />
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t('Recent Logs')}</CardTitle>
            <CardDescription>
              {t('Latest requests from all agent users')}
            </CardDescription>
          </CardHeader>
          <CardContent className='px-0'>
            <AgentAnalyticsLogTable
              logs={analytics?.recent_logs ?? []}
              loading={analyticsQuery.isLoading}
              compact
            />
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

export function AgentAnalyticsLogTable(props: {
  logs: AgentAnalyticsLogItem[]
  loading?: boolean
  compact?: boolean
}) {
  const { t } = useTranslation()
  if (props.loading) {
    return (
      <div className='flex flex-col gap-3 px-4 py-3'>
        {Array.from({ length: 5 }, (_, index) => (
          <Skeleton key={index} className='h-8 w-full' />
        ))}
      </div>
    )
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead className='pl-4'>{t('Time')}</TableHead>
          <TableHead>{t('Status')}</TableHead>
          <TableHead>{t('User')}</TableHead>
          <TableHead>{t('Model')}</TableHead>
          {!props.compact ? <TableHead>{t('Tokens')}</TableHead> : null}
          {!props.compact ? <TableHead>{t('Fail Reason')}</TableHead> : null}
          <TableHead className='pr-4 text-right'>{t('Consumption')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {props.logs.length ? (
          props.logs.map((log) => {
            const tokens = log.prompt_tokens + log.completion_tokens
            return (
              <TableRow key={log.id}>
                <TableCell className='pl-4 font-mono text-xs whitespace-nowrap'>
                  {formatTimestampToDate(log.created_at)}
                </TableCell>
                <TableCell>
                  <Badge variant={log.type === 2 ? 'secondary' : 'destructive'}>
                    {log.type === 2 ? <CircleCheck /> : <CircleX />}
                    {log.type === 2 ? t('Success') : t('Failed')}
                  </Badge>
                </TableCell>
                <TableCell>{log.username || `#${log.user_id}`}</TableCell>
                <TableCell>
                  <div className='flex flex-col gap-0.5'>
                    <span>{log.model_name || '-'}</span>
                    {!props.compact && log.use_time > 0 ? (
                      <span className='text-muted-foreground text-xs'>
                        {formatUseTime(log.use_time)}
                      </span>
                    ) : null}
                  </div>
                </TableCell>
                {!props.compact ? (
                  <TableCell>{formatTokens(tokens)}</TableCell>
                ) : null}
                {!props.compact ? (
                  <TableCell>
                    {log.type === 5 ? (
                      <div className='flex max-w-[360px] items-center gap-2'>
                        {log.status_code ? (
                          <Badge variant='outline'>
                            HTTP {log.status_code}
                          </Badge>
                        ) : null}
                        <TruncatedText
                          text={log.error_message || '-'}
                          maxWidth='max-w-[260px]'
                        />
                      </div>
                    ) : (
                      '-'
                    )}
                  </TableCell>
                ) : null}
                <TableCell className='pr-4 text-right'>
                  {formatQuota(log.quota)}
                </TableCell>
              </TableRow>
            )
          })
        ) : (
          <TableEmpty
            colSpan={props.compact ? 5 : 7}
            title={t('No usage logs')}
            description={t('No matching requests were found in this period.')}
            icon={<Bot />}
          />
        )}
      </TableBody>
    </Table>
  )
}
