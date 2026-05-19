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
import type { ReactNode } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { CheckCircle2, Layers3, TestTube2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { CHANNEL_TYPE_OPTIONS } from '@/features/channels/constants'
import { getChannelTypeLabel } from '@/features/channels/lib/channel-utils'
import {
  configureNewAPISupplierChannels,
  getLocalGroups,
  testNewAPISupplierModel,
} from '../api'
import {
  applySupplierModelTestResults,
  type SupplierModelTestResult,
} from '../supplier-model-test'
import {
  buildSupplierModelProviderFilters,
  selectSupplierProviderModels,
} from '../supplier-model-provider-filter'
import type {
  ConfigureItem,
  NewAPISupplier,
  NewAPISupplierGroupSnapshot,
} from '../types'

function parseGroupModels(raw: string): NewAPISupplierGroupSnapshot[] {
  if (!raw?.trim()) return []
  try {
    const parsed = JSON.parse(raw) as NewAPISupplierGroupSnapshot[]
    if (!Array.isArray(parsed)) return []
    return parsed
      .filter((item) => item.group && item.source === 'pricing')
      .map((item) => ({
        group: item.group,
        models: Array.isArray(item.models) ? item.models : [],
        source: item.source,
        ratio: item.ratio || '',
        desc: item.desc || '',
        model_providers: item.model_providers || {},
      }))
  } catch {
    return []
  }
}

type SupplierChannelConfigDialogProps = {
  open: boolean
  supplier: NewAPISupplier | null
  onOpenChange: (open: boolean) => void
  onConfigured: () => void
}

type ModelTestStatus = 'testing' | 'success' | 'error'

type ModelTestState = {
  status: ModelTestStatus
  time?: number
  message?: string
}

export function SupplierChannelConfigDialog({
  open,
  supplier,
  onOpenChange,
  onConfigured,
}: SupplierChannelConfigDialogProps) {
  const { t } = useTranslation()
  const [selectedGroup, setSelectedGroup] = useState('')
  const [localGroup, setLocalGroup] = useState('')
  const [channelType, setChannelType] = useState(1)
  const [channelName, setChannelName] = useState('')
  const [channelNameTouched, setChannelNameTouched] = useState(false)
  const initializedKeyRef = useRef('')
  const [selectedModels, setSelectedModels] = useState<Record<string, boolean>>(
    {}
  )
  const [modelTestStates, setModelTestStates] = useState<
    Record<string, ModelTestState>
  >({})
  const [isTestingModels, setIsTestingModels] = useState(false)

  const { data: localGroupsData } = useQuery({
    queryKey: ['groups'],
    queryFn: getLocalGroups,
    enabled: open,
    staleTime: 5 * 60 * 1000,
  })

  const localGroups = useMemo(() => {
    const values = localGroupsData?.data ?? []
    const current = localGroup.trim()
    return Array.from(new Set([...values, ...(current ? [current] : [])]))
  }, [localGroupsData, localGroup])

  const groups = useMemo(
    () =>
      supplier?.model_source === 'pricing'
        ? parseGroupModels(supplier?.group_models_json ?? '')
        : [],
    [supplier]
  )

  const activeGroup = useMemo(
    () => groups.find((item) => item.group === selectedGroup) ?? null,
    [groups, selectedGroup]
  )

  const activeGroupModels = useMemo(
    () => activeGroup?.models ?? [],
    [activeGroup]
  )

  const selectedModelNames = useMemo(
    () => activeGroupModels.filter((model) => Boolean(selectedModels[model])),
    [activeGroupModels, selectedModels]
  )

  const providerFilters = useMemo(
    () =>
      buildSupplierModelProviderFilters(
        activeGroupModels,
        activeGroup?.model_providers
      ),
    [activeGroup, activeGroupModels]
  )

  const defaultChannelName = useMemo(() => {
    if (!supplier || !activeGroup) return ''
    const ratio = activeGroup.ratio || t('Not available')
    return `${activeGroup.group} - ${ratio} - ${t(getChannelTypeLabel(channelType))} - ${supplier.name}`
  }, [activeGroup, channelType, supplier, t])

  useEffect(() => {
    if (!open) {
      initializedKeyRef.current = ''
      setChannelNameTouched(false)
      return
    }
    if (!supplier || groups.length === 0) return
    const initializeKey = `${supplier.id}:${supplier.group_models_json}`
    if (initializedKeyRef.current === initializeKey) return
    initializedKeyRef.current = initializeKey
    const firstGroup = groups[0]
    setSelectedGroup(firstGroup.group)
    setChannelType(supplier?.channel_type || 1)
    setChannelName('')
    setChannelNameTouched(false)
    const preferred = supplier?.default_local_group?.trim()
    const availableLocalGroups = localGroupsData?.data ?? []
    const fallback =
      availableLocalGroups.find((group) => group === preferred) ||
      availableLocalGroups.find((group) => group === 'default') ||
      availableLocalGroups[0] ||
      preferred ||
      firstGroup.group ||
      'default'
    setLocalGroup(fallback)
    setSelectedModels(
      Object.fromEntries(firstGroup.models.map((model) => [model, true]))
    )
    setModelTestStates({})
  }, [groups, localGroupsData, open, supplier])

  useEffect(() => {
    if (!open || !activeGroup) return
    setSelectedModels(
      Object.fromEntries(activeGroup.models.map((model) => [model, true]))
    )
    setModelTestStates({})
  }, [activeGroup, open])

  useEffect(() => {
    if (!open || channelNameTouched) return
    setChannelName(defaultChannelName)
  }, [channelNameTouched, defaultChannelName, open])

  const configureMutation = useMutation({
    mutationFn: ({ id, items }: { id: number; items: ConfigureItem[] }) =>
      configureNewAPISupplierChannels(id, items),
    onSuccess: (res) => {
      if (!res.success) return
      toast.success(t('Group channel configured successfully'))
      onConfigured()
    },
  })

  const handleGroupChange = (group: string | null) => {
    if (!group) return
    setSelectedGroup(group)
    const preferred = supplier?.default_local_group?.trim()
    const fallback =
      localGroups.find((item) => item === preferred) ||
      localGroups.find((item) => item === 'default') ||
      localGroups[0] ||
      preferred ||
      group ||
      'default'
    setLocalGroup(fallback)
  }

  const toggleAll = (checked: boolean) => {
    setSelectedModels(
      Object.fromEntries(activeGroupModels.map((model) => [model, checked]))
    )
  }

  const selectProviderModels = (models: string[]) => {
    setSelectedModels(selectSupplierProviderModels(activeGroupModels, models))
    setModelTestStates({})
  }

  const submit = () => {
    if (!supplier || !activeGroup) return
    if (selectedModelNames.length === 0) {
      toast.error(t('Please select at least one model'))
      return
    }
    configureMutation.mutate({
      id: supplier.id,
      items: [
        {
          upstream_group: activeGroup.group,
          local_group: localGroup.trim() || activeGroup.group,
          channel_type: channelType,
          channel_name: channelName.trim() || defaultChannelName,
          models: selectedModelNames,
        },
      ],
    })
  }

  const testModel = () => {
    if (!supplier || !activeGroup) return
    const modelsToTest = activeGroupModels
    if (modelsToTest.length === 0) {
      toast.error(t('No models in this group'))
      return
    }
    void testSelectedModels({
      supplierId: supplier.id,
      upstreamGroup: activeGroup.group,
      models: modelsToTest,
      channelType,
    })
  }

  const testSelectedModels = async ({
    supplierId,
    upstreamGroup,
    models,
    channelType,
  }: {
    supplierId: number
    upstreamGroup: string
    models: string[]
    channelType: number
  }) => {
    setIsTestingModels(true)
    setModelTestStates((prev) => {
      const next = { ...prev }
      for (const model of models) {
        next[model] = { status: 'testing' }
      }
      return next
    })
    const results: SupplierModelTestResult[] = []
    try {
      for (const model of models) {
        try {
          const res = await testNewAPISupplierModel(supplierId, {
            upstream_group: upstreamGroup,
            model,
            channel_type: channelType,
          })
          results.push({ model, success: res.success })
          setModelTestStates((prev) => ({
            ...prev,
            [model]: {
              status: res.success ? 'success' : 'error',
              time: res.time,
              message: res.success ? undefined : res.message,
            },
          }))
        } catch (error) {
          results.push({ model, success: false })
          setModelTestStates((prev) => ({
            ...prev,
            [model]: {
              status: 'error',
              message:
                error instanceof Error ? error.message : t('Model test failed'),
            },
          }))
        }
      }
      setSelectedModels((prev) => applySupplierModelTestResults(prev, results))
      const failedCount = results.filter((result) => !result.success).length
      const passedCount = results.length - failedCount
      if (failedCount > 0) {
        toast.error(
          t(
            '{{failed}} model(s) failed and were automatically deselected. {{passed}} passed.',
            {
              failed: failedCount,
              passed: passedCount,
            }
          )
        )
      } else {
        toast.success(
          t('All {{count}} selected model(s) passed the test.', {
            count: passedCount,
          })
        )
      }
      onConfigured()
    } finally {
      setIsTestingModels(false)
    }
  }

  const allChecked =
    activeGroupModels.length > 0 &&
    selectedModelNames.length === activeGroupModels.length

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-h-[85vh] overflow-y-auto sm:max-w-3xl'>
        <DialogHeader>
          <DialogTitle>{t('Configure Channels')}</DialogTitle>
          <DialogDescription>
            {supplier
              ? t('Configure one upstream group into a local channel')
              : t('Select a supplier before configuring channels')}
          </DialogDescription>
        </DialogHeader>

        {!groups.length ? (
          <div className='border-border bg-muted/30 rounded-md border px-4 py-8 text-center'>
            <Layers3 className='text-muted-foreground mx-auto mb-3 h-8 w-8' />
            <div className='font-medium'>{t('No groups available')}</div>
            <div className='text-muted-foreground mt-1 text-sm'>
              {supplier?.model_source && supplier.model_source !== 'pricing'
                ? t('Run supplier check first to load groups and models')
                : t('Run supplier check first to load groups and models')}
            </div>
          </div>
        ) : (
          <div className='space-y-5'>
            <div className='grid gap-4 sm:grid-cols-3'>
              <Field label={t('Upstream Group')}>
                <Select value={selectedGroup} onValueChange={handleGroupChange}>
                  <SelectTrigger className='w-full'>
                    <SelectValue placeholder={t('Select a group')} />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      {groups.map((group) => (
                        <SelectItem key={group.group} value={group.group}>
                          <span>{group.group}</span>
                          <span className='text-muted-foreground text-xs'>
                            {group.ratio
                              ? `${t('Ratio')}: ${group.ratio} · `
                              : ''}
                            {group.models.length} {t('Models')}
                          </span>
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>

              <Field label={t('Channel Type')}>
                <Select
                  value={String(channelType)}
                  onValueChange={(value) => {
                    const next = Number(value)
                    if (Number.isInteger(next) && next > 0) {
                      setChannelType(next)
                    }
                  }}
                >
                  <SelectTrigger className='w-full'>
                    <SelectValue placeholder={t('Select channel type')} />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      {CHANNEL_TYPE_OPTIONS.map((option) => (
                        <SelectItem
                          key={option.value}
                          value={String(option.value)}
                        >
                          {t(option.label)}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>

              <Field label={t('Local Group')}>
                <Select
                  value={localGroup}
                  onValueChange={(value) => value && setLocalGroup(value)}
                >
                  <SelectTrigger className='w-full'>
                    <SelectValue placeholder={t('Select a group')} />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      {localGroups.map((group) => (
                        <SelectItem key={group} value={group}>
                          {group}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>
            </div>

            <Field label={t('Channel Name')}>
              <Input
                value={channelName}
                onChange={(event) => {
                  setChannelNameTouched(true)
                  setChannelName(event.target.value)
                }}
                placeholder={defaultChannelName}
              />
            </Field>

            <div className='flex flex-wrap items-center gap-2'>
              <Badge variant='outline'>
                {t('Selected Models')}: {selectedModelNames.length}
              </Badge>
              <Badge variant='secondary'>
                {t('Channel Type')}: {t(getChannelTypeLabel(channelType))}
              </Badge>
              <Badge variant={activeGroup?.ratio ? 'secondary' : 'outline'}>
                {t('Upstream Group Ratio')}:{' '}
                {activeGroup?.ratio || t('Not available')}
              </Badge>
              {activeGroup?.source && (
                <Badge variant='secondary'>
                  {t('Model Source')}:{' '}
                  {activeGroup.source === 'pricing'
                    ? t('Model Square')
                    : activeGroup.source}
                </Badge>
              )}
            </div>

            <Separator />

            <div className='space-y-3'>
              <div className='flex items-center justify-between gap-3'>
                <div>
                  <div className='font-medium'>
                    {t('Models in Selected Group')}
                  </div>
                  <div className='text-muted-foreground text-sm'>
                    {activeGroup?.group} · {activeGroupModels.length}{' '}
                    {t('Models')}
                    {activeGroup?.ratio
                      ? ` · ${t('Ratio')}: ${activeGroup.ratio}`
                      : ''}
                  </div>
	                  {activeGroup?.desc && (
	                    <div className='text-muted-foreground text-sm'>
	                      {activeGroup.desc}
	                    </div>
	                  )}
	                </div>
                <label className='flex items-center gap-2 text-sm'>
                  <Checkbox
                    checked={allChecked}
                    disabled={activeGroupModels.length === 0 || isTestingModels}
                    onCheckedChange={(value) => toggleAll(Boolean(value))}
                  />
	                  {t('Select all models')}
	                </label>
	              </div>

	              {providerFilters.length > 0 && (
	                <div className='space-y-2'>
	                  <div className='text-muted-foreground text-xs font-medium'>
	                    {t('Filter by model provider')}
	                  </div>
	                  <div className='flex flex-wrap gap-2'>
	                    {providerFilters.map((filter) => (
	                      <Button
	                        key={filter.provider}
	                        type='button'
	                        variant='outline'
	                        size='sm'
	                        disabled={isTestingModels}
	                        onClick={() => selectProviderModels(filter.models)}
	                      >
	                        {filter.provider}
	                        <Badge
	                          variant='secondary'
	                          className='ml-1 h-4 rounded px-1 text-[10px]'
	                        >
	                          {filter.models.length}
	                        </Badge>
	                      </Button>
	                    ))}
	                  </div>
	                </div>
	              )}

	              {activeGroupModels.length ? (
	                <div
                  key={activeGroup?.group}
                  className='grid gap-2 sm:grid-cols-2 lg:grid-cols-3'
                >
                  {activeGroupModels.map((model) =>
                    (() => {
                      const state = modelTestStates[model]
                      return (
                        <label
                          key={model}
                          className={cn(
                            'border-border flex min-w-0 items-center gap-2 rounded-md border px-2 py-1.5 text-sm',
                            selectedModels[model] && 'border-primary',
                            state?.status === 'error' && 'border-destructive',
                            state?.status === 'success' && 'border-emerald-500'
                          )}
                        >
                          <Checkbox
                            checked={Boolean(selectedModels[model])}
                            disabled={isTestingModels}
                            onCheckedChange={(value) =>
                              setSelectedModels((prev) => ({
                                ...prev,
                                [model]: Boolean(value),
                              }))
                            }
                          />
	                          <span className='min-w-0 flex-1 truncate'>
	                            <span className='block truncate'>{model}</span>
	                            {activeGroup?.model_providers?.[model]?.length ? (
	                              <span className='text-muted-foreground block truncate text-xs'>
	                                {activeGroup.model_providers[model].join(', ')}
	                              </span>
	                            ) : null}
	                          </span>
                          {state?.status === 'testing' && (
                            <Badge variant='outline' className='shrink-0'>
                              {t('Testing...')}
                            </Badge>
                          )}
                          {state?.status === 'success' && (
                            <Badge variant='secondary' className='shrink-0'>
                              {t('Passed')}
                              {typeof state.time === 'number'
                                ? ` · ${state.time.toFixed(2)}s`
                                : ''}
                            </Badge>
                          )}
                          {state?.status === 'error' && (
                            <Badge
                              variant='destructive'
                              className='max-w-[120px] shrink-0 truncate'
                              title={state.message}
                            >
                              {t('Failed')}
                            </Badge>
                          )}
                        </label>
                      )
                    })()
                  )}
                </div>
              ) : (
                <div className='border-border text-muted-foreground rounded-md border border-dashed px-4 py-8 text-center text-sm'>
                  {t('No models in this group')}
                </div>
              )}
            </div>
          </div>
        )}

        <DialogFooter>
          <Button
            onClick={testModel}
            variant='outline'
            disabled={activeGroupModels.length === 0 || isTestingModels}
          >
            <TestTube2 className='h-4 w-4' />
            {isTestingModels ? t('Testing...') : t('Test All Models')}
          </Button>
          <Button
            onClick={submit}
            disabled={
              activeGroupModels.length === 0 ||
              configureMutation.isPending ||
              selectedModelNames.length === 0
            }
          >
            <CheckCircle2 className='h-4 w-4' />
            {t('Configure This Group')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function Field(props: { label: string; children: ReactNode }) {
  return (
    <div className='grid gap-2'>
      <Label>{props.label}</Label>
      {props.children}
    </div>
  )
}
