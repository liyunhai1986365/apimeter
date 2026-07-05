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
import { useForm } from 'react-hook-form'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { RefreshCw, Route, Save } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  getRoutingStrategySnapshots,
  refreshRoutingStrategySnapshots,
  saveRoutingStrategyOverride,
} from '../api'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import type { ModelSettings, RoutingStrategy } from '../types'

type RoutingStrategyFormValues = Pick<
  ModelSettings,
  | 'routing_strategy_setting.enabled'
  | 'routing_strategy_setting.update_interval_minutes'
  | 'routing_strategy_setting.window_hours'
  | 'routing_strategy_setting.min_request_count'
  | 'routing_strategy_setting.smart_price_weight'
  | 'routing_strategy_setting.smart_speed_weight'
  | 'routing_strategy_setting.smart_success_weight'
  | 'routing_strategy_setting.excluded_groups'
  | 'routing_strategy_setting.pinned_groups'
>

const strategyLabels: Record<RoutingStrategy, string> = {
  smart_auto: 'Smart automatic',
  price_first: 'Price first',
  speed_first: 'Speed first',
  success_first: 'Success rate first',
}

type Props = {
  defaultValues: RoutingStrategyFormValues
}

function parseGroups(raw: string) {
  try {
    const value = JSON.parse(raw)
    return Array.isArray(value)
      ? value.filter((item): item is string => typeof item === 'string')
      : []
  } catch {
    return []
  }
}

function parseEditableGroups(value: string) {
  return value
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean)
}

export function RoutingStrategySection(props: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const queryClient = useQueryClient()
  const [editingKey, setEditingKey] = useState('')
  const [editingGroups, setEditingGroups] = useState('')
  const form = useForm<RoutingStrategyFormValues>({
    defaultValues: props.defaultValues,
  })

  const snapshotsQuery = useQuery({
    queryKey: ['routing-strategy-snapshots'],
    queryFn: getRoutingStrategySnapshots,
  })

  const refreshMutation = useMutation({
    mutationFn: refreshRoutingStrategySnapshots,
    onSuccess: (data) => {
      if (data.success) {
        toast.success(t('Routing strategies refreshed'))
        queryClient.invalidateQueries({
          queryKey: ['routing-strategy-snapshots'],
        })
      } else {
        toast.error(data.message || t('Refresh failed'))
      }
    },
  })

  const overrideMutation = useMutation({
    mutationFn: saveRoutingStrategyOverride,
    onSuccess: (data) => {
      if (data.success) {
        toast.success(t('Manual routing order saved'))
        setEditingKey('')
        queryClient.invalidateQueries({
          queryKey: ['routing-strategy-snapshots'],
        })
      } else {
        toast.error(data.message || t('Save failed'))
      }
    },
  })

  const snapshots = snapshotsQuery.data?.data?.snapshots ?? []
  const groupedSnapshots = useMemo(() => {
    const map = new Map<string, typeof snapshots>()
    for (const snapshot of snapshots) {
      const key = snapshot.user_group || 'default'
      map.set(key, [...(map.get(key) ?? []), snapshot])
    }
    return [...map.entries()].sort(([a], [b]) => a.localeCompare(b))
  }, [snapshots])

  const onSubmit = async (data: RoutingStrategyFormValues) => {
    const entries = Object.entries(data) as [string, unknown][]
    const updates = entries.filter(
      ([key, value]) =>
        value !==
        (props.defaultValues[key as keyof RoutingStrategyFormValues] as unknown)
    )
    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }
    for (const [key, value] of updates) {
      await updateOption.mutateAsync({
        key,
        value: value as string | number | boolean,
      })
    }
    toast.success(t('Saved successfully'))
  }

  return (
    <SettingsSection
      title={t('Routing Strategies')}
      description={t(
        'Maintain system-generated supplier rankings used by smart routing API keys.'
      )}
    >
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(onSubmit)}
          className='flex flex-col gap-4'
        >
          <Card>
            <CardHeader>
              <CardTitle className='flex items-center gap-2 text-base'>
                <Route className='size-4' />
                {t('Smart routing settings')}
              </CardTitle>
              <CardDescription>
                {t(
                  'These settings control scheduled ranking refreshes and the Smart automatic scoring weights.'
                )}
              </CardDescription>
            </CardHeader>
            <CardContent className='grid gap-4 md:grid-cols-2'>
              <FormField
                control={form.control}
                name='routing_strategy_setting.enabled'
                render={({ field }) => (
                  <FormItem className='flex items-center justify-between gap-3 rounded-lg border p-3 md:col-span-2'>
                    <div>
                      <FormLabel>{t('Enable scheduled refresh')}</FormLabel>
                      <FormDescription>
                        {t(
                          'Automatically refresh maintained routing rankings.'
                        )}
                      </FormDescription>
                    </div>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
              {[
                [
                  'routing_strategy_setting.update_interval_minutes',
                  'Refresh interval minutes',
                ],
                ['routing_strategy_setting.window_hours', 'Stats window hours'],
                [
                  'routing_strategy_setting.min_request_count',
                  'Minimum request count',
                ],
                [
                  'routing_strategy_setting.smart_price_weight',
                  'Smart price weight',
                ],
                [
                  'routing_strategy_setting.smart_speed_weight',
                  'Smart speed weight',
                ],
                [
                  'routing_strategy_setting.smart_success_weight',
                  'Smart success weight',
                ],
              ].map(([name, label]) => (
                <FormField
                  key={name}
                  control={form.control}
                  name={name as keyof RoutingStrategyFormValues}
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t(label)}</FormLabel>
                      <FormControl>
                        <Input
                          type='number'
                          step={name.includes('weight') ? '0.05' : '1'}
                          value={String(field.value ?? '')}
                          onChange={(event) =>
                            field.onChange(Number(event.target.value))
                          }
                        />
                      </FormControl>
                    </FormItem>
                  )}
                />
              ))}
              <FormField
                control={form.control}
                name='routing_strategy_setting.excluded_groups'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Excluded groups')}</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder='group-a,group-b' />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Comma-separated groups excluded from generated rankings.'
                      )}
                    </FormDescription>
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='routing_strategy_setting.pinned_groups'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Pinned groups')}</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder='group-a,group-b' />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Comma-separated groups pinned to the front in order.'
                      )}
                    </FormDescription>
                  </FormItem>
                )}
              />
              <div className='flex gap-2 md:col-span-2'>
                <Button type='submit' disabled={updateOption.isPending}>
                  <Save className='size-4' />
                  {t('Save settings')}
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  disabled={refreshMutation.isPending}
                  onClick={() => refreshMutation.mutate()}
                >
                  <RefreshCw className='size-4' />
                  {t('Refresh now')}
                </Button>
              </div>
            </CardContent>
          </Card>
        </form>
      </Form>

      <div className='grid gap-3'>
        {groupedSnapshots.length === 0 ? (
          <Card>
            <CardContent className='text-muted-foreground flex min-h-28 items-center justify-center text-sm'>
              {t('No routing strategy snapshots yet.')}
            </CardContent>
          </Card>
        ) : (
          groupedSnapshots.map(([userGroup, items]) => (
            <Card key={userGroup}>
              <CardHeader>
                <CardTitle className='text-base'>{userGroup}</CardTitle>
                <CardDescription>
                  {t('Maintained rankings for this user group.')}
                </CardDescription>
              </CardHeader>
              <CardContent className='grid gap-3 lg:grid-cols-2'>
                {items.map((snapshot) => {
                  const key = `${snapshot.strategy}:${snapshot.user_group}`
                  const groups = parseGroups(snapshot.groups)
                  const isEditing = editingKey === key
                  return (
                    <div key={key} className='rounded-lg border p-3'>
                      <div className='mb-2 flex items-start justify-between gap-3'>
                        <div>
                          <div className='flex items-center gap-2'>
                            <h4 className='text-sm font-medium'>
                              {t(strategyLabels[snapshot.strategy])}
                            </h4>
                            {snapshot.manual_override && (
                              <Badge variant='secondary'>
                                {t('Manual override')}
                              </Badge>
                            )}
                          </div>
                          <p className='text-muted-foreground text-xs'>
                            {snapshot.updated_at
                              ? t('Updated at {{time}}', {
                                  time: new Date(
                                    snapshot.updated_at * 1000
                                  ).toLocaleString(),
                                })
                              : t('Not updated yet')}
                          </p>
                        </div>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          onClick={() => {
                            setEditingKey(isEditing ? '' : key)
                            setEditingGroups(groups.join('\n'))
                          }}
                        >
                          {isEditing ? t('Cancel') : t('Edit order')}
                        </Button>
                      </div>
                      {isEditing ? (
                        <div className='flex flex-col gap-2'>
                          <Textarea
                            value={editingGroups}
                            onChange={(event) =>
                              setEditingGroups(event.target.value)
                            }
                            rows={6}
                          />
                          <Button
                            type='button'
                            size='sm'
                            disabled={overrideMutation.isPending}
                            onClick={() =>
                              overrideMutation.mutate({
                                strategy: snapshot.strategy,
                                user_group: snapshot.user_group,
                                groups: parseEditableGroups(editingGroups),
                                manual_override: true,
                              })
                            }
                          >
                            {t('Save manual order')}
                          </Button>
                        </div>
                      ) : (
                        <div className='flex flex-wrap gap-1.5'>
                          {groups.slice(0, 16).map((group, index) => (
                            <Badge
                              key={`${group}-${index}`}
                              variant='outline'
                              className='font-normal'
                            >
                              {index + 1}. {group}
                            </Badge>
                          ))}
                          {groups.length > 16 && (
                            <Badge variant='secondary'>
                              +{groups.length - 16}
                            </Badge>
                          )}
                        </div>
                      )}
                    </div>
                  )
                })}
              </CardContent>
            </Card>
          ))
        )}
      </div>
    </SettingsSection>
  )
}
