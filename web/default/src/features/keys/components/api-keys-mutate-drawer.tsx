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
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { useForm, type SubmitErrorHandler } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import {
  ChevronDown,
  Gauge,
  KeyRound,
  Route,
  Settings2,
  ShieldCheck,
  Sparkles,
  Tags,
  Trash2,
  WalletCards,
  type LucideIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { getUserModels, getUserGroups } from '@/lib/api'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { USER_FACING_GROUP_TERMS } from '@/lib/user-facing-group-terms'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { DateTimePicker } from '@/components/datetime-picker'
import { MultiSelect } from '@/components/multi-select'
import { getPerfMetricsGroups } from '@/features/performance-metrics/api'
import { getPricing, hydratePricingModels } from '@/features/pricing/api'
import { createApiKey, updateApiKey, getApiKey } from '../api'
import { ERROR_MESSAGES, SUCCESS_MESSAGES } from '../constants'
import {
  AUTO_GROUP_VALUE,
  buildApiKeyGroupOptions,
  getApiKeyFormSchema,
  type ApiKeyFormValues,
  type ApiKeyRoutingStrategy,
  getApiKeyFormDefaultValues,
  shouldFallbackApiKeyGroup,
  transformFormDataToPayload,
  transformApiKeyToFormDefaults,
} from '../lib'
import { type ApiKey } from '../types'
import { type ApiKeyGroupOption } from './api-key-group-combobox'
import {
  ApiKeyGroupOrderSelector,
  ApiKeyGroupPickerPopover,
} from './api-key-group-order-selector'
import { useApiKeys } from './api-keys-provider'

type ApiKeyMutateDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: ApiKey
  side?: 'left' | 'right'
  onApiKeyCreated?: (apiKey: { key: string; name: string }) => void
}

const SMART_ROUTING_OPTIONS: Array<{
  value: ApiKeyRoutingStrategy
  label: string
  description: string
  icon: LucideIcon
  tone: string
  selectedTone: string
  iconTone: string
}> = [
  {
    value: 'smart_auto',
    label: 'Smart automatic',
    description:
      'Use the system-maintained ranking that balances price, speed, and recent success rate.',
    icon: Sparkles,
    tone: 'border-sky-200 bg-sky-50/70 hover:bg-sky-50 dark:border-sky-900/60 dark:bg-sky-950/20 dark:hover:bg-sky-950/30',
    selectedTone:
      'has-[[data-state=checked]]:border-sky-500 has-[[data-state=checked]]:bg-sky-50 has-[[data-state=checked]]:ring-sky-500/20 dark:has-[[data-state=checked]]:bg-sky-950/40',
    iconTone: 'text-sky-600 dark:text-sky-400',
  },
  {
    value: 'price_first',
    label: 'Price first',
    description:
      'Prefer lower-cost suppliers in the selected available groups for budget-sensitive usage.',
    icon: Tags,
    tone: 'border-emerald-200 bg-emerald-50/70 hover:bg-emerald-50 dark:border-emerald-900/60 dark:bg-emerald-950/20 dark:hover:bg-emerald-950/30',
    selectedTone:
      'has-[[data-state=checked]]:border-emerald-500 has-[[data-state=checked]]:bg-emerald-50 has-[[data-state=checked]]:ring-emerald-500/20 dark:has-[[data-state=checked]]:bg-emerald-950/40',
    iconTone: 'text-emerald-600 dark:text-emerald-400',
  },
  {
    value: 'speed_first',
    label: 'Speed first',
    description:
      'Prefer suppliers with faster recent response times for latency-sensitive requests.',
    icon: Gauge,
    tone: 'border-amber-200 bg-amber-50/70 hover:bg-amber-50 dark:border-amber-900/60 dark:bg-amber-950/20 dark:hover:bg-amber-950/30',
    selectedTone:
      'has-[[data-state=checked]]:border-amber-500 has-[[data-state=checked]]:bg-amber-50 has-[[data-state=checked]]:ring-amber-500/20 dark:has-[[data-state=checked]]:bg-amber-950/40',
    iconTone: 'text-amber-600 dark:text-amber-400',
  },
  {
    value: 'success_first',
    label: 'Success rate first',
    description:
      'Prefer suppliers with higher recent success rates for stability-sensitive usage.',
    icon: ShieldCheck,
    tone: 'border-teal-200 bg-teal-50/70 hover:bg-teal-50 dark:border-teal-900/60 dark:bg-teal-950/20 dark:hover:bg-teal-950/30',
    selectedTone:
      'has-[[data-state=checked]]:border-teal-500 has-[[data-state=checked]]:bg-teal-50 has-[[data-state=checked]]:ring-teal-500/20 dark:has-[[data-state=checked]]:bg-teal-950/40',
    iconTone: 'text-teal-600 dark:text-teal-400',
  },
]

type ApiKeyFormSectionProps = {
  title: string
  description: string
  icon: LucideIcon
  children: ReactNode
}

function ApiKeyFormSection(props: ApiKeyFormSectionProps) {
  const Icon = props.icon

  return (
    <section className='bg-card rounded-lg border'>
      <div className='flex items-center gap-2.5 border-b px-3 py-2.5 sm:gap-3 sm:px-4 sm:py-3'>
        <div className='bg-muted text-muted-foreground flex size-8 shrink-0 items-center justify-center rounded-lg border sm:size-10'>
          <Icon className='size-4 sm:size-5' />
        </div>
        <div className='min-w-0'>
          <h3 className='text-sm leading-none font-medium'>{props.title}</h3>
          <p className='text-muted-foreground mt-0.5 text-xs sm:mt-1'>
            {props.description}
          </p>
        </div>
      </div>
      <div className='space-y-3 p-3 sm:space-y-4 sm:p-4'>{props.children}</div>
    </section>
  )
}

function getImageResponseFormatLabel(
  value: ApiKeyFormValues['image_response_format'],
  t: ReturnType<typeof useTranslation>['t']
) {
  switch (value) {
    case 'url':
      return t('Force URL')
    case 'b64_json':
      return t('Force base64')
    case 'follow_request':
    default:
      return t('Follow request or endpoint')
  }
}

function getImageResponseFormatDescription(
  value: ApiKeyFormValues['image_response_format'],
  t: ReturnType<typeof useTranslation>['t']
) {
  switch (value) {
    case 'url':
      return t(
        'Force this key to return image results as URL fields when possible.'
      )
    case 'b64_json':
      return t(
        'Force this key to return image results as base64 fields when possible.'
      )
    case 'follow_request':
    default:
      return t(
        'Do not force a token-level image format; follow the request, channel override, or endpoint default.'
      )
  }
}

function getImageStoreStrategyLabel(
  value: ApiKeyFormValues['image_store_strategy'],
  t: ReturnType<typeof useTranslation>['t']
) {
  switch (value) {
    case 'only_store_base64':
      return t('Store base64 only')
    case 'force_store_url_and_base64':
      return t('Store URL and base64')
    case 'default':
    default:
      return t('Default storage')
  }
}

function getImageStoreStrategyDescription(
  value: ApiKeyFormValues['image_store_strategy'],
  t: ReturnType<typeof useTranslation>['t']
) {
  switch (value) {
    case 'only_store_base64':
      return t(
        'When returning URL format, store base64 image data through the gateway; endpoint URLs are returned directly.'
      )
    case 'force_store_url_and_base64':
      return t(
        'When returning URL format, store both endpoint URLs and base64 image data through the gateway.'
      )
    case 'default':
    default:
      return t(
        'Use the default image handling policy; base64 image data is stored by the gateway when returning URL format.'
      )
  }
}

export function ApiKeysMutateDrawer({
  open,
  onOpenChange,
  currentRow,
  side = 'right',
  onApiKeyCreated,
}: ApiKeyMutateDrawerProps) {
  const { t } = useTranslation()
  const groupTerms = USER_FACING_GROUP_TERMS
  const isUpdate = !!currentRow
  const { triggerRefresh, selectedWorkspaceId } = useApiKeys()
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [advancedOpen, setAdvancedOpen] = useState(false)
  // Fetch models
  const { data: modelsData } = useQuery({
    queryKey: ['user-models'],
    queryFn: getUserModels,
    staleTime: 5 * 60 * 1000, // Cache for 5 minutes
  })

  // Fetch groups
  const { data: groupsData } = useQuery({
    queryKey: ['user-groups'],
    queryFn: getUserGroups,
    staleTime: 5 * 60 * 1000,
  })

  const { data: pricingData } = useQuery({
    queryKey: ['pricing', 'api-key-group-filters'],
    queryFn: getPricing,
    staleTime: 5 * 60 * 1000,
  })
  const { data: groupPerfData } = useQuery({
    queryKey: ['perf-metrics-groups', 24],
    queryFn: () => getPerfMetricsGroups(24),
    staleTime: 60 * 1000,
  })

  const models = modelsData?.data || []
  const groupsRaw = groupsData?.data || {}
  const pricingModels = useMemo(
    () => (pricingData ? hydratePricingModels(pricingData) : []),
    [pricingData]
  )
  const groupPerformance = useMemo(
    () =>
      Object.fromEntries(
        (groupPerfData?.data.groups ?? []).map((item) => [item.group, item])
      ),
    [groupPerfData]
  )
  const schema = getApiKeyFormSchema(t)

  const form = useForm<ApiKeyFormValues>({
    resolver: zodResolver(schema),
    defaultValues: getApiKeyFormDefaultValues(),
  })
  const watchedGroupChain = form.watch('group_chain')
  const watchedModelLimits = form.watch('model_limits')
  const groups: ApiKeyGroupOption[] = buildApiKeyGroupOptions(
    groupsRaw,
    true,
    currentRow?.group || watchedGroupChain?.[0],
    {
      models: pricingModels,
      modelLimits: watchedModelLimits,
    }
  )

  // Load existing data when updating
  useEffect(() => {
    if (open && isUpdate && currentRow) {
      getApiKey(currentRow.id).then((result) => {
        if (result.success && result.data) {
          form.reset(transformApiKeyToFormDefaults(result.data))
        }
      })
    } else if (open && !isUpdate) {
      form.reset(getApiKeyFormDefaultValues())
    }
  }, [open, isUpdate, currentRow, form])

  // Remove unavailable groups after the supplier list loads. Manual routing may
  // intentionally stay empty until the user chooses a supplier.
  useEffect(() => {
    if ((form.getValues('routing_mode') || 'smart') === 'smart') return
    if (groups.length === 0) return
    const currentChain = form.getValues('group_chain') || []
    const cleanChain = currentChain.filter(
      (group) => !shouldFallbackApiKeyGroup(group, groups)
    )
    if (cleanChain.length !== currentChain.length) {
      form.setValue('group_chain', cleanChain)
    }
  }, [groups, form])

  const onSubmit = async (data: ApiKeyFormValues) => {
    setIsSubmitting(true)
    try {
      const basePayload = transformFormDataToPayload(data)

      if (isUpdate && currentRow) {
        const result = await updateApiKey({
          ...basePayload,
          id: currentRow.id,
        })
        if (result.success) {
          toast.success(t(SUCCESS_MESSAGES.API_KEY_UPDATED))
          onOpenChange(false)
          triggerRefresh()
        } else {
          toast.error(result.message || t(ERROR_MESSAGES.UPDATE_FAILED))
        }
      } else {
        // Create mode - handle batch creation
        const count = data.tokenCount || 1
        let successCount = 0
        let firstCreatedKey = ''
        let firstCreatedName = ''

        for (let i = 0; i < count; i++) {
          const tokenName =
            i === 0 && data.name
              ? data.name
              : `${data.name || 'default'}-${Math.random().toString(36).slice(2, 8)}`
          const result = await createApiKey({
            ...basePayload,
            workspace_id: selectedWorkspaceId || undefined,
            name: tokenName,
          })
          if (result.success) {
            successCount++
            if (successCount === 1 && result.data?.key) {
              firstCreatedKey = result.data.key
              firstCreatedName = result.data.name || tokenName
            }
          } else {
            toast.error(result.message || t(ERROR_MESSAGES.CREATE_FAILED))
            break
          }
        }

        if (successCount > 0) {
          toast.success(
            t('Successfully created {{count}} API Key(s)', {
              count: successCount,
            })
          )
          onOpenChange(false)
          triggerRefresh()
          if (successCount === 1 && firstCreatedKey) {
            onApiKeyCreated?.({
              key: firstCreatedKey,
              name: firstCreatedName,
            })
          }
        }
      }
    } catch (_error) {
      toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      setIsSubmitting(false)
    }
  }

  const onInvalid: SubmitErrorHandler<ApiKeyFormValues> = () => {
    toast.error(t('Please fix the highlighted fields before saving'))
  }

  const handleSetExpiry = (months: number, days: number, hours: number) => {
    if (months === 0 && days === 0 && hours === 0) {
      form.setValue('expired_time', undefined)
      return
    }

    const now = new Date()
    now.setMonth(now.getMonth() + months)
    now.setDate(now.getDate() + days)
    now.setHours(now.getHours() + hours)

    form.setValue('expired_time', now)
  }

  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'
  const quotaLabel = t('Quota ({{currency}})', { currency: currencyLabel })
  const quotaPlaceholder = tokensOnly
    ? t('Enter quota in tokens')
    : t('Enter quota in {{currency}}', { currency: currencyLabel })
  const unlimitedQuota = form.watch('unlimited_quota')
  const imageResponseFormat = form.watch('image_response_format')

  useEffect(() => {
    if (imageResponseFormat !== 'url') {
      form.setValue('image_store_strategy', 'default')
    }
  }, [form, imageResponseFormat])

  return (
    <Sheet
      open={open}
      onOpenChange={(v) => {
        onOpenChange(v)
        if (!v) {
          form.reset()
        }
      }}
    >
      <SheetContent
        side={side}
        className='bg-background flex !h-dvh !w-screen max-w-none gap-0 overflow-hidden p-0 sm:!w-full sm:!max-w-[620px]'
      >
        <SheetHeader className='bg-background border-b px-4 py-3 text-start sm:px-5 sm:py-4'>
          <SheetTitle className='text-base sm:text-lg'>
            {isUpdate ? t('Update API Key') : t('Create API Key')}
          </SheetTitle>
          <SheetDescription className='pr-6 text-xs sm:text-sm'>
            {isUpdate
              ? t('Update the API key by providing necessary info.')
              : t('Add a new API key by providing necessary info.')}{' '}
            {t("Click save when you're done.")}
          </SheetDescription>
        </SheetHeader>
        <Form {...form}>
          <form
            id='api-key-form'
            onSubmit={form.handleSubmit(onSubmit, onInvalid)}
            className='min-h-0 flex-1 space-y-3 overflow-y-auto overscroll-contain px-3 py-3 sm:space-y-4 sm:px-4 sm:py-4'
          >
            <ApiKeyFormSection
              title={t('Basic Information')}
              description={t('Set API key basic information')}
              icon={KeyRound}
            >
              <FormField
                control={form.control}
                name='name'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Name')}</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder={t('Enter a name')} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='routing_mode'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Supplier routing')}</FormLabel>
                    <FormControl>
                      <Tabs
                        value={field.value || 'smart'}
                        onValueChange={(value) => {
                          field.onChange(value)
                          if (value === 'smart') {
                            form.setValue('group_chain', [AUTO_GROUP_VALUE])
                          } else {
                            form.setValue('routing_excluded_groups', [])
                          }
                          if (
                            value !== 'smart' &&
                            (form.getValues('group_chain') || []).includes(
                              AUTO_GROUP_VALUE
                            )
                          ) {
                            form.setValue('group_chain', [])
                          }
                        }}
                        className='gap-3'
                      >
                        <TabsList className='grid h-auto w-full grid-cols-2'>
                          <TabsTrigger value='smart' className='gap-1.5'>
                            <Route className='size-4' />
                            {t('Smart routing')}
                          </TabsTrigger>
                          <TabsTrigger value='manual' className='gap-1.5'>
                            <Settings2 className='size-4' />
                            {t('Specify suppliers')}
                          </TabsTrigger>
                        </TabsList>
                        <TabsContent value='smart' className='mt-0'>
                          <FormField
                            control={form.control}
                            name='routing_strategy'
                            render={({ field: strategyField }) => (
                              <FormItem>
                                <FormControl>
                                  <RadioGroup
                                    value={strategyField.value || 'smart_auto'}
                                    onValueChange={strategyField.onChange}
                                    className='grid gap-2 sm:grid-cols-2'
                                  >
                                    {SMART_ROUTING_OPTIONS.map((option) => {
                                      const Icon = option.icon
                                      return (
                                        <Label
                                          key={option.value}
                                          htmlFor={`api-key-routing-${option.value}`}
                                          className={cn(
                                            'flex cursor-pointer items-start gap-2.5 rounded-lg border p-2.5 transition-colors has-[[data-state=checked]]:ring-2',
                                            option.tone,
                                            option.selectedTone
                                          )}
                                        >
                                          <RadioGroupItem
                                            id={`api-key-routing-${option.value}`}
                                            value={option.value}
                                            className='mt-0.5'
                                          />
                                          <Icon
                                            className={cn(
                                              'mt-0.5 size-4 shrink-0',
                                              option.iconTone
                                            )}
                                          />
                                          <span className='min-w-0'>
                                            <span className='block text-sm font-medium'>
                                              {t(option.label)}
                                            </span>
                                            <span className='text-muted-foreground mt-0.5 block text-xs leading-4'>
                                              {t(option.description)}
                                            </span>
                                          </span>
                                        </Label>
                                      )
                                    })}
                                  </RadioGroup>
                                </FormControl>
                                <FormDescription>
                                  {t(
                                    'The system periodically updates supplier rankings. This key reads the latest maintained ranking for the selected strategy.'
                                  )}
                                </FormDescription>
                                <FormField
                                  control={form.control}
                                  name='routing_excluded_groups'
                                  render={({ field: excludedField }) => (
                                    <FormItem>
                                      <FormLabel>
                                        {t('Ignored suppliers')}
                                      </FormLabel>
                                      <FormControl>
                                        <div className='space-y-2'>
                                          {(excludedField.value || []).length >
                                            0 && (
                                            <div className='flex flex-wrap gap-1.5'>
                                              {(excludedField.value || []).map(
                                                (group) => {
                                                  const option = groups.find(
                                                    (item) =>
                                                      item.value === group
                                                  )
                                                  return (
                                                    <Badge
                                                      key={group}
                                                      variant='secondary'
                                                      className='gap-1 rounded-md px-2 py-1'
                                                    >
                                                      {option?.label || group}
                                                      <Button
                                                        type='button'
                                                        variant='ghost'
                                                        size='icon-sm'
                                                        className='size-auto p-0'
                                                        onClick={() =>
                                                          excludedField.onChange(
                                                            (
                                                              excludedField.value ||
                                                              []
                                                            ).filter(
                                                              (item) =>
                                                                item !== group
                                                            )
                                                          )
                                                        }
                                                        aria-label={t('Remove')}
                                                      >
                                                        <Trash2 className='size-3' />
                                                      </Button>
                                                    </Badge>
                                                  )
                                                }
                                              )}
                                            </div>
                                          )}
                                          <ApiKeyGroupPickerPopover
                                            options={groups}
                                            selectedValues={
                                              excludedField.value || []
                                            }
                                            onApply={(values) =>
                                              excludedField.onChange([
                                                ...(excludedField.value || []),
                                                ...values.filter(
                                                  (value) =>
                                                    !(
                                                      excludedField.value || []
                                                    ).includes(value)
                                                ),
                                              ])
                                            }
                                            groupDisplay={
                                              pricingData?.group_display
                                            }
                                            vendors={pricingData?.vendors}
                                            models={pricingModels}
                                            groupPerformance={groupPerformance}
                                            triggerLabel={t(
                                              'Select suppliers to ignore'
                                            )}
                                            applyLabel={t(
                                              'Ignore selected suppliers'
                                            )}
                                          />
                                        </div>
                                      </FormControl>
                                      <FormDescription>
                                        {t(
                                          'Ignored suppliers are removed from this smart routing key only.'
                                        )}
                                      </FormDescription>
                                      <FormMessage />
                                    </FormItem>
                                  )}
                                />
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                        </TabsContent>
                        <TabsContent value='manual' className='mt-0'>
                          <FormField
                            control={form.control}
                            name='group_chain'
                            render={({ field: groupField }) => (
                              <FormItem>
                                <FormControl>
                                  <ApiKeyGroupOrderSelector
                                    options={groups}
                                    value={groupField.value}
                                    onChange={groupField.onChange}
                                    groupDisplay={pricingData?.group_display}
                                    vendors={pricingData?.vendors}
                                    models={pricingModels}
                                    groupPerformance={groupPerformance}
                                  />
                                </FormControl>
                                <FormDescription>
                                  {t(groupTerms.requestOrderDescription)}
                                </FormDescription>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                        </TabsContent>
                      </Tabs>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='expired_time'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Expiration Time')}</FormLabel>
                    <div className='grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center'>
                      <FormControl>
                        <DateTimePicker
                          value={field.value}
                          onChange={field.onChange}
                          placeholder={t('Never expires')}
                          className='min-w-0 [&_input[type=time]]:w-24 sm:[&_input[type=time]]:w-32'
                        />
                      </FormControl>
                      <div className='grid grid-cols-4 gap-2 sm:flex'>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          className='px-2 text-xs sm:px-3 sm:text-sm'
                          onClick={() => handleSetExpiry(0, 0, 0)}
                        >
                          {t('Never')}
                        </Button>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          className='px-2 text-xs sm:px-3 sm:text-sm'
                          onClick={() => handleSetExpiry(1, 0, 0)}
                        >
                          {t('1 Month')}
                        </Button>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          className='px-2 text-xs sm:px-3 sm:text-sm'
                          onClick={() => handleSetExpiry(0, 1, 0)}
                        >
                          {t('1 Day')}
                        </Button>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          className='px-2 text-xs sm:px-3 sm:text-sm'
                          onClick={() => handleSetExpiry(0, 0, 1)}
                        >
                          {t('1 Hour')}
                        </Button>
                      </div>
                    </div>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {!isUpdate && (
                <FormField
                  control={form.control}
                  name='tokenCount'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Quantity')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='number'
                          min='1'
                          placeholder={t('Number of keys to create')}
                          onChange={(e) =>
                            field.onChange(parseInt(e.target.value, 10) || 1)
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Create multiple API keys at once (random suffix will be added to names)'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
            </ApiKeyFormSection>

            <ApiKeyFormSection
              title={t('Quota Settings')}
              description={t('Set quota amount and limits')}
              icon={WalletCards}
            >
              {!unlimitedQuota && (
                <FormField
                  control={form.control}
                  name='remain_quota_dollars'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{quotaLabel}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='number'
                          step={tokensOnly ? 1 : 0.01}
                          placeholder={quotaPlaceholder}
                          onChange={(e) =>
                            field.onChange(parseFloat(e.target.value) || 0)
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {tokensOnly
                          ? t('Enter the quota amount in tokens')
                          : t('Enter the quota amount in {{currency}}', {
                              currency: currencyLabel,
                            })}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}

              <FormField
                control={form.control}
                name='unlimited_quota'
                render={({ field }) => (
                  <FormItem className='flex min-h-16 flex-row items-center justify-between gap-3 rounded-lg border px-3 py-2.5 sm:min-h-20 sm:gap-4 sm:px-4 sm:py-3'>
                    <div className='space-y-0.5'>
                      <FormLabel className='text-sm'>
                        {t('Unlimited Quota')}
                      </FormLabel>
                      <FormDescription className='text-xs'>
                        {t('Enable unlimited quota for this API key')}
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
            </ApiKeyFormSection>

            <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
              <section className='bg-card rounded-lg border'>
                <CollapsibleTrigger
                  render={
                    <button
                      type='button'
                      className='hover:bg-muted/50 flex w-full items-center gap-2.5 px-3 py-2.5 text-left transition-colors sm:gap-3 sm:px-4 sm:py-3'
                    />
                  }
                >
                  <div className='bg-muted text-muted-foreground flex size-8 shrink-0 items-center justify-center rounded-lg border sm:size-10'>
                    <Settings2 className='size-4 sm:size-5' />
                  </div>
                  <div className='min-w-0 flex-1'>
                    <h3 className='text-sm leading-none font-medium'>
                      {t('Advanced Settings')}
                    </h3>
                    <p className='text-muted-foreground mt-1 text-xs'>
                      {t('Set API key access restrictions')}
                    </p>
                  </div>
                  <ChevronDown
                    className={cn(
                      'text-muted-foreground size-4 shrink-0 transition-transform',
                      advancedOpen && 'rotate-180'
                    )}
                  />
                </CollapsibleTrigger>
                <CollapsibleContent>
                  <div className='space-y-3 border-t p-3 sm:space-y-4 sm:p-4'>
                    <FormField
                      control={form.control}
                      name='model_limits'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('Model Limits')}</FormLabel>
                          <FormControl>
                            <MultiSelect
                              options={models.map((m) => ({
                                label: m,
                                value: m,
                              }))}
                              selected={field.value}
                              onChange={field.onChange}
                              placeholder={t(
                                'Select models (empty for allow all)'
                              )}
                            />
                          </FormControl>
                          <FormDescription>
                            {t('Limit which models can be used with this key')}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />

                    <FormField
                      control={form.control}
                      name='allow_ips'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>
                            {t('IP Whitelist (supports CIDR)')}
                          </FormLabel>
                          <FormControl>
                            <Textarea
                              {...field}
                              className='min-h-20 resize-none'
                              placeholder={t(
                                'One IP per line (empty for no restriction)'
                              )}
                              rows={3}
                            />
                          </FormControl>
                          <FormDescription>
                            {t(
                              'Do not over-trust this feature. IP may be spoofed. Please use with nginx, CDN and other gateways.'
                            )}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />

                    <div className='grid gap-3 sm:grid-cols-2'>
                      <FormField
                        control={form.control}
                        name='image_response_format'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Image response format')}</FormLabel>
                            <Select
                              value={field.value || 'follow_request'}
                              onValueChange={field.onChange}
                            >
                              <FormControl>
                                <SelectTrigger>
                                  <SelectValue
                                    placeholder={t(
                                      'Select image response format'
                                    )}
                                  >
                                    {getImageResponseFormatLabel(
                                      field.value,
                                      t
                                    )}
                                  </SelectValue>
                                </SelectTrigger>
                              </FormControl>
                              <SelectContent>
                                <SelectItem value='follow_request'>
                                  {t('Follow request or endpoint')}
                                </SelectItem>
                                <SelectItem value='url'>
                                  {t('Force URL')}
                                </SelectItem>
                                <SelectItem value='b64_json'>
                                  {t('Force base64')}
                                </SelectItem>
                              </SelectContent>
                            </Select>
                            <FormDescription>
                              {getImageResponseFormatDescription(
                                field.value,
                                t
                              )}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />

                      {imageResponseFormat === 'url' && (
                        <FormField
                          control={form.control}
                          name='image_store_strategy'
                          render={({ field }) => (
                            <FormItem>
                              <FormLabel>
                                {t('Image storage strategy')}
                              </FormLabel>
                              <Select
                                value={field.value || 'default'}
                                onValueChange={field.onChange}
                              >
                                <FormControl>
                                  <SelectTrigger>
                                    <SelectValue
                                      placeholder={t(
                                        'Select image storage strategy'
                                      )}
                                    >
                                      {getImageStoreStrategyLabel(
                                        field.value,
                                        t
                                      )}
                                    </SelectValue>
                                  </SelectTrigger>
                                </FormControl>
                                <SelectContent>
                                  <SelectItem value='default'>
                                    {t('Default storage')}
                                  </SelectItem>
                                  <SelectItem value='only_store_base64'>
                                    {t('Store base64 only')}
                                  </SelectItem>
                                  <SelectItem value='force_store_url_and_base64'>
                                    {t('Store URL and base64')}
                                  </SelectItem>
                                </SelectContent>
                              </Select>
                              <FormDescription>
                                {getImageStoreStrategyDescription(
                                  field.value,
                                  t
                                )}
                              </FormDescription>
                              <FormMessage />
                            </FormItem>
                          )}
                        />
                      )}
                    </div>
                  </div>
                </CollapsibleContent>
              </section>
            </Collapsible>
          </form>
        </Form>
        <SheetFooter className='bg-background grid grid-cols-2 gap-2 border-t px-3 py-3 sm:flex sm:flex-row sm:justify-end sm:px-5 sm:py-4'>
          <SheetClose
            render={<Button variant='outline' className='w-full sm:w-auto' />}
          >
            {t('Close')}
          </SheetClose>
          <Button
            type='button'
            onClick={form.handleSubmit(onSubmit)}
            disabled={isSubmitting}
            className='w-full sm:w-auto'
          >
            {isSubmitting ? t('Saving...') : t('Save changes')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
