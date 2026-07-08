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
import { type FormEvent, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  BadgePercent,
  Building2,
  KeyRound,
  Pencil,
  Plus,
  Trash2,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { formatCompactNumber, formatLogQuota } from '@/lib/format'
import { formatGroupDiscount } from '@/lib/group-discount'
import { useGroupDiscountLabels } from '@/hooks/use-group-discount-labels'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Textarea } from '@/components/ui/textarea'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { PageTransition } from '@/components/page-transition'
import {
  CHANNEL_TYPE_OPTIONS,
  CHANNEL_TYPE_WARNINGS,
  FIELD_PLACEHOLDERS,
} from '@/features/channels/constants'
import {
  getChannelTypeLabel,
  getKeyPromptForType,
} from '@/features/channels/lib/channel-utils'
import { getPerfMetricsGroups } from '@/features/performance-metrics/api'
import {
  formatLatency,
  formatThroughput,
  formatUptimePct,
} from '@/features/performance-metrics/lib/format'
import type { PerfGroupSummary } from '@/features/performance-metrics/types'
import { getPricing } from '@/features/pricing/api'
import {
  createUserOwnedProvider,
  deleteUserOwnedProvider,
  getUserOwnedProviders,
  updateUserOwnedProvider,
} from './api'
import {
  ALL_SUPPLIER_CATEGORY_VALUE,
  ALL_SUPPLIER_VENDOR_VALUE,
  buildSupplierDirectoryData,
  filterSupplierDirectoryItems,
  type SupplierDirectoryCategory,
  type SupplierDirectoryItem,
} from './lib/supplier-directory'
import {
  buildUserOwnedProviderPayload,
  defaultProviderForm,
  providerToFormState,
  type ProviderFormState,
  type UserOwnedProviderRow,
} from './lib/user-owned-provider-form'

export function SupplierDirectory() {
  const { t } = useTranslation()
  const [selectedCategory, setSelectedCategory] = useState(
    ALL_SUPPLIER_CATEGORY_VALUE
  )
  const { data, isLoading } = useQuery({
    queryKey: ['pricing', 'supplier-directory'],
    queryFn: getPricing,
    staleTime: 5 * 60 * 1000,
  })

  const ownedQuery = useQuery({
    queryKey: ['supplier-directory', 'user-owned-providers'],
    queryFn: getUserOwnedProviders,
  })
  const groupPerfQuery = useQuery({
    queryKey: ['perf-metrics-groups', 24],
    queryFn: () => getPerfMetricsGroups(24),
    staleTime: 60 * 1000,
  })
  const groupPerformance = useMemo(
    () =>
      Object.fromEntries(
        (groupPerfQuery.data?.data.groups ?? []).map((item) => [
          item.group,
          item,
        ])
      ),
    [groupPerfQuery.data]
  )

  const directory = useMemo(() => buildSupplierDirectoryData(data), [data])
  const filteredSuppliers = useMemo(
    () =>
      filterSupplierDirectoryItems(directory.items, {
        category: selectedCategory,
        vendor: ALL_SUPPLIER_VENDOR_VALUE,
        search: '',
      }),
    [directory.items, selectedCategory]
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

      <UserOwnedProvidersSection
        providers={ownedQuery.data?.data ?? []}
        isLoading={ownedQuery.isLoading}
        groupPerformance={groupPerformance}
      />

      {directory.items.length === 0 ? (
        <Empty className='min-h-[320px] border'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              <Building2 />
            </EmptyMedia>
            <EmptyTitle>{t('No suppliers found')}</EmptyTitle>
            <EmptyDescription>
              {t('No supplier metadata configured.')}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <>
          <section className='flex flex-col gap-3'>
            <h2 className='font-semibold'>{t('System built-in suppliers')}</h2>
            <SupplierCategoryFilter
              categories={directory.categories}
              selectedCategory={selectedCategory}
              onSelectCategory={setSelectedCategory}
            />
          </section>
          {filteredSuppliers.length === 0 ? (
            <Empty className='min-h-[260px] border'>
              <EmptyHeader>
                <EmptyMedia variant='icon'>
                  <Building2 />
                </EmptyMedia>
                <EmptyTitle>{t('No suppliers found')}</EmptyTitle>
                <EmptyDescription>
                  {t('No supplier metadata configured.')}
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <SupplierCardsGrid
              suppliers={filteredSuppliers}
              groupPerformance={groupPerformance}
            />
          )}
        </>
      )}
    </PageTransition>
  )
}

function SupplierCategoryFilter(props: {
  categories: SupplierDirectoryCategory[]
  selectedCategory: string
  onSelectCategory: (category: string) => void
}) {
  const { t } = useTranslation()

  return (
    <div className='flex flex-wrap items-center gap-2'>
      {props.categories.map((category) => {
        const isSelected = props.selectedCategory === category.value
        return (
          <Button
            key={category.value}
            type='button'
            size='sm'
            variant={isSelected ? 'default' : 'outline'}
            onClick={() => props.onSelectCategory(category.value)}
            aria-pressed={isSelected}
          >
            {category.value === ALL_SUPPLIER_CATEGORY_VALUE
              ? t('All')
              : t(category.label)}
            <span className='font-mono text-xs tabular-nums opacity-80'>
              {category.count}
            </span>
          </Button>
        )
      })}
    </div>
  )
}

function UserOwnedProvidersSection(props: {
  providers: UserOwnedProviderRow[]
  isLoading: boolean
  groupPerformance: Record<string, PerfGroupSummary>
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingProvider, setEditingProvider] =
    useState<UserOwnedProviderRow | null>(null)
  const [form, setForm] = useState<ProviderFormState>(defaultProviderForm)
  const totalStats = useMemo(
    () =>
      props.providers.reduce(
        (total, provider) => ({
          requestCount:
            total.requestCount + (provider.stats?.request_count ?? 0),
          quota: total.quota + (provider.stats?.quota ?? 0),
        }),
        { requestCount: 0, quota: 0 }
      ),
    [props.providers]
  )

  const resetDialog = () => {
    setDialogOpen(false)
    setEditingProvider(null)
    setForm(defaultProviderForm)
  }

  const openCreateDialog = () => {
    setEditingProvider(null)
    setForm(defaultProviderForm)
    setDialogOpen(true)
  }

  const openEditDialog = (provider: UserOwnedProviderRow) => {
    setEditingProvider(provider)
    setForm(providerToFormState(provider))
    setDialogOpen(true)
  }

  const createMutation = useMutation({
    mutationFn: createUserOwnedProvider,
    onSuccess: (res) => {
      if (!res.success) {
        toast.error(res.message || t('Failed to create provider'))
        return
      }
      toast.success(t('Provider created successfully'))
      resetDialog()
      queryClient.invalidateQueries({
        queryKey: ['supplier-directory', 'user-owned-providers'],
      })
    },
    onError: () => toast.error(t('Failed to create provider')),
  })

  const updateMutation = useMutation({
    mutationFn: ({
      id,
      payload,
    }: {
      id: number
      payload: Parameters<typeof updateUserOwnedProvider>[1]
    }) => updateUserOwnedProvider(id, payload),
    onSuccess: (res) => {
      if (!res.success) {
        toast.error(res.message || t('Failed to update provider'))
        return
      }
      toast.success(t('Provider updated successfully'))
      resetDialog()
      queryClient.invalidateQueries({
        queryKey: ['supplier-directory', 'user-owned-providers'],
      })
    },
    onError: () => toast.error(t('Failed to update provider')),
  })

  const deleteMutation = useMutation({
    mutationFn: deleteUserOwnedProvider,
    onSuccess: (res) => {
      if (!res.success) {
        toast.error(res.message || t('Failed to delete provider'))
        return
      }
      toast.success(t('Provider deleted successfully'))
      queryClient.invalidateQueries({
        queryKey: ['supplier-directory', 'user-owned-providers'],
      })
    },
    onError: () => toast.error(t('Failed to delete provider')),
  })

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const name = form.name.trim()
    const key = form.key.trim()
    const isEditing = editingProvider !== null
    const payload = buildUserOwnedProviderPayload(form, {
      allowBlankKey: isEditing,
    })
    if (!name || (!isEditing && !key) || !payload.channel.models) {
      toast.error(t('Name, API key, and models are required'))
      return
    }
    if (editingProvider) {
      updateMutation.mutate({ id: editingProvider.id, payload })
      return
    }
    createMutation.mutate(payload)
  }

  return (
    <section className='flex flex-col gap-3'>
      <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
        <div className='min-w-0'>
          <h2 className='font-semibold'>{t('My Providers')}</h2>
          <p className='text-muted-foreground text-xs'>
            {t('Use your own upstream keys without wallet billing.')}
          </p>
        </div>
        <div className='flex flex-wrap items-center gap-2 sm:justify-end'>
          {props.providers.length > 0 && (
            <div className='bg-muted/30 flex items-center gap-3 rounded-md border px-3 py-1.5 text-xs'>
              <span className='text-muted-foreground'>
                {t('Estimated spend')}
              </span>
              <span className='font-semibold tabular-nums'>
                {formatLogQuota(totalStats.quota)}
              </span>
              <span className='text-muted-foreground'>
                {formatCompactNumber(totalStats.requestCount)} {t('Requests')}
              </span>
            </div>
          )}
          <Button size='sm' onClick={openCreateDialog}>
            <Plus data-icon='inline-start' />
            {t('Create Provider')}
          </Button>
        </div>
      </div>

      <div>
        {props.isLoading ? (
          <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>
            {Array.from({ length: 2 }).map((_, index) => (
              <Skeleton key={index} className='h-44 rounded-lg' />
            ))}
          </div>
        ) : props.providers.length === 0 ? (
          <div className='text-muted-foreground rounded-lg border px-4 py-6 text-sm'>
            {t('No custom providers yet.')}
          </div>
        ) : (
          <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>
            {props.providers.map((provider) => (
              <UserOwnedProviderCard
                key={provider.id}
                provider={provider}
                performance={props.groupPerformance[provider.group]}
                isDeleting={deleteMutation.isPending}
                onEdit={() => openEditDialog(provider)}
                onDelete={() => deleteMutation.mutate(provider.id)}
              />
            ))}
          </div>
        )}
      </div>

      <Dialog
        open={dialogOpen}
        onOpenChange={(open) => {
          if (open) {
            setDialogOpen(true)
          } else {
            resetDialog()
          }
        }}
      >
        <DialogContent className='sm:max-w-lg'>
          <DialogHeader>
            <DialogTitle>
              {editingProvider ? t('Edit Provider') : t('Create Provider')}
            </DialogTitle>
            <DialogDescription>
              {editingProvider
                ? t('Update this private provider configuration.')
                : t(
                    'Create a private provider backed by your own upstream key.'
                  )}
            </DialogDescription>
          </DialogHeader>
          <form className='grid gap-4' onSubmit={submit}>
            <div className='grid gap-2'>
              <Label htmlFor='provider-name'>{t('Provider Name')}</Label>
              <Input
                id='provider-name'
                value={form.name}
                onChange={(event) =>
                  setForm((prev) => ({ ...prev, name: event.target.value }))
                }
              />
            </div>
            <div className='grid gap-2'>
              <Label htmlFor='provider-type'>{t('Channel Type')}</Label>
              <select
                id='provider-type'
                className='border-input bg-background h-9 rounded-md border px-3 text-sm outline-none'
                value={form.type}
                onChange={(event) =>
                  setForm((prev) => ({
                    ...prev,
                    type: Number(event.target.value),
                  }))
                }
              >
                {CHANNEL_TYPE_OPTIONS.map((option) => (
                  <option key={option.value} value={option.value}>
                    {t(option.label)}
                  </option>
                ))}
              </select>
            </div>
            <ProviderAccessFields
              form={form}
              isEditing={Boolean(editingProvider)}
              setForm={setForm}
            />
            <div className='grid gap-2'>
              <Label htmlFor='provider-models'>{t('Models')}</Label>
              <Textarea
                id='provider-models'
                value={form.models}
                placeholder='gpt-4o, gpt-4.1'
                onChange={(event) =>
                  setForm((prev) => ({ ...prev, models: event.target.value }))
                }
              />
            </div>
            <DialogFooter>
              <Button type='button' variant='outline' onClick={resetDialog}>
                {t('Cancel')}
              </Button>
              <Button
                type='submit'
                disabled={createMutation.isPending || updateMutation.isPending}
              >
                <KeyRound data-icon='inline-start' />
                {editingProvider ? t('Update Provider') : t('Create Provider')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </section>
  )
}

function UserOwnedProviderCard(props: {
  provider: UserOwnedProviderRow
  performance?: PerfGroupSummary
  isDeleting: boolean
  onEdit: () => void
  onDelete: () => void
}) {
  const { t } = useTranslation()
  const models = getModelList(props.provider.models)
  const modelCount = getModelCount(props.provider.models)

  return (
    <Card size='sm' className='min-h-44'>
      <CardHeader>
        <CardTitle className='min-w-0 truncate'>
          {props.provider.name}
        </CardTitle>
        <CardDescription className='truncate'>
          {props.provider.group}
        </CardDescription>
        <CardAction className='flex gap-1'>
          <Button
            variant='ghost'
            size='icon-sm'
            onClick={props.onEdit}
            aria-label={t('Edit')}
          >
            <Pencil />
          </Button>
          <Button
            variant='ghost'
            size='icon-sm'
            disabled={props.isDeleting}
            onClick={props.onDelete}
            aria-label={t('Delete')}
          >
            <Trash2 />
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className='flex flex-1 flex-col gap-3'>
        <div className='flex flex-wrap items-center gap-2'>
          <Badge variant='outline' className='font-normal'>
            {t(getChannelTypeLabel(props.provider.type))}
          </Badge>
          <SupplierPerformancePills performance={props.performance} />
        </div>
        <div className='grid grid-cols-2 gap-2'>
          <SupplierMetric
            label={t('Models')}
            value={<ModelNamesTooltip count={modelCount} models={models} />}
          />
          <SupplierMetric
            label={t('Estimated spend')}
            value={formatLogQuota(props.provider.stats?.quota ?? 0)}
          />
        </div>
      </CardContent>
      <SupplierPerformanceFooter performance={props.performance} />
    </Card>
  )
}

function ProviderAccessFields(props: {
  form: ProviderFormState
  isEditing: boolean
  setForm: React.Dispatch<React.SetStateAction<ProviderFormState>>
}) {
  const { t } = useTranslation()
  const { form, isEditing, setForm } = props
  const currentType = form.type

  return (
    <div className='grid gap-4 rounded-lg border p-3'>
      <div className='flex items-center gap-2'>
        <KeyRound className='text-muted-foreground size-4' />
        <h3 className='text-sm font-medium'>{t('API Access')}</h3>
      </div>

      {CHANNEL_TYPE_WARNINGS[currentType] && (
        <div className='text-warning rounded-md border px-3 py-2 text-xs'>
          {t(CHANNEL_TYPE_WARNINGS[currentType])}
        </div>
      )}

      {currentType === 1 && (
        <TextInputField
          id='provider-openai-org'
          label={t('OpenAI Organization')}
          value={form.openaiOrganization}
          placeholder='org-...'
          onChange={(value) =>
            setForm((prev) => ({ ...prev, openaiOrganization: value }))
          }
        />
      )}

      {currentType === 3 && (
        <>
          <TextInputField
            id='provider-azure-endpoint'
            label={t('AZURE_OPENAI_ENDPOINT *')}
            value={form.baseUrl}
            placeholder='https://docs-test-001.openai.azure.com'
            onChange={(value) =>
              setForm((prev) => ({ ...prev, baseUrl: value }))
            }
          />
          <TextInputField
            id='provider-azure-version'
            label={t('Default API Version *')}
            value={form.other}
            placeholder='2025-04-01-preview'
            onChange={(value) => setForm((prev) => ({ ...prev, other: value }))}
          />
          <TextInputField
            id='provider-azure-responses-version'
            label={t('Responses API Version')}
            value={form.azureResponsesVersion}
            placeholder='preview'
            onChange={(value) =>
              setForm((prev) => ({ ...prev, azureResponsesVersion: value }))
            }
          />
        </>
      )}

      {currentType === 8 && (
        <TextInputField
          id='provider-custom-base-url'
          label={`${t('Full Base URL (supports')} {${t('model')}} ${t(
            'variable) *'
          )}`}
          value={form.baseUrl}
          placeholder='https://api.openai.com/v1/chat/completions'
          onChange={(value) => setForm((prev) => ({ ...prev, baseUrl: value }))}
        />
      )}

      {currentType === 18 && (
        <TextInputField
          id='provider-xunfei-version'
          label={t('Model Version *')}
          value={form.other}
          placeholder='v2.1'
          onChange={(value) => setForm((prev) => ({ ...prev, other: value }))}
        />
      )}

      {currentType === 20 && (
        <CheckboxField
          id='provider-openrouter-enterprise'
          label={t('Enterprise Account')}
          checked={form.openrouterEnterprise}
          onChange={(checked) =>
            setForm((prev) => ({ ...prev, openrouterEnterprise: checked }))
          }
        />
      )}

      {currentType === 21 && (
        <TextInputField
          id='provider-knowledge-base-id'
          label={t('Knowledge Base ID *')}
          value={form.other}
          placeholder='123456'
          onChange={(value) => setForm((prev) => ({ ...prev, other: value }))}
        />
      )}

      {currentType === 22 && (
        <TextInputField
          id='provider-fastgpt-base-url'
          label={t('Private Deployment URL')}
          value={form.baseUrl}
          placeholder='https://fastgpt.run/api/openapi'
          onChange={(value) => setForm((prev) => ({ ...prev, baseUrl: value }))}
        />
      )}

      {currentType === 33 && (
        <SelectField
          id='provider-aws-key-type'
          label={t('AWS Key Format')}
          value={form.awsKeyType}
          options={[
            { value: 'ak_sk', label: t('AccessKey / SecretAccessKey') },
            { value: 'api_key', label: t('API Key') },
          ]}
          onChange={(value) =>
            setForm((prev) => ({
              ...prev,
              awsKeyType: value === 'api_key' ? 'api_key' : 'ak_sk',
            }))
          }
        />
      )}

      {currentType === 36 && (
        <TextInputField
          id='provider-suno-base-url'
          label={t('API Base URL (Important: Not Chat API) *')}
          value={form.baseUrl}
          placeholder='https://api.example.com'
          onChange={(value) => setForm((prev) => ({ ...prev, baseUrl: value }))}
        />
      )}

      {currentType === 39 && (
        <TextInputField
          id='provider-cloudflare-account-id'
          label={t('Account ID *')}
          value={form.other}
          placeholder='d6b5da8hk1awo8nap34ube6gh'
          onChange={(value) => setForm((prev) => ({ ...prev, other: value }))}
        />
      )}

      {currentType === 41 && (
        <>
          <SelectField
            id='provider-vertex-key-type'
            label={t('Vertex AI Key Format')}
            value={form.vertexKeyType}
            options={[
              { value: 'json', label: t('JSON') },
              { value: 'api_key', label: t('API Key') },
            ]}
            onChange={(value) =>
              setForm((prev) => ({
                ...prev,
                vertexKeyType: value === 'api_key' ? 'api_key' : 'json',
              }))
            }
          />
          <TextareaField
            id='provider-vertex-region'
            label={t('Deployment Region *')}
            value={form.other}
            placeholder='us-central1'
            onChange={(value) => setForm((prev) => ({ ...prev, other: value }))}
          />
        </>
      )}

      {currentType === 45 && (
        <SelectField
          id='provider-volcengine-base-url'
          label={t('API Base URL *')}
          value={form.baseUrl || 'https://ark.cn-beijing.volces.com'}
          options={[
            {
              value: 'https://ark.cn-beijing.volces.com',
              label: 'https://ark.cn-beijing.volces.com',
            },
            {
              value: 'https://ark.ap-southeast.bytepluses.com',
              label: 'https://ark.ap-southeast.bytepluses.com',
            },
            { value: 'doubao-coding-plan', label: t('Doubao Coding Plan') },
          ]}
          onChange={(value) => setForm((prev) => ({ ...prev, baseUrl: value }))}
        />
      )}

      {currentType === 49 && (
        <TextInputField
          id='provider-coze-agent-id'
          label={t('Agent ID *')}
          value={form.other}
          placeholder='7342866812345'
          onChange={(value) => setForm((prev) => ({ ...prev, other: value }))}
        />
      )}

      {shouldShowGeneralBaseUrl(currentType) && (
        <TextInputField
          id='provider-base-url'
          label={t('Base URL')}
          value={form.baseUrl}
          placeholder={t(FIELD_PLACEHOLDERS.BASE_URL)}
          onChange={(value) => setForm((prev) => ({ ...prev, baseUrl: value }))}
        />
      )}

      <TextareaField
        id='provider-key'
        label={t('API Key *')}
        value={form.key}
        placeholder={getProviderKeyPlaceholder({
          type: currentType,
          awsKeyType: form.awsKeyType,
          isEditing,
          t,
        })}
        rows={currentType === 41 && form.vertexKeyType === 'json' ? 6 : 4}
        onChange={(value) => setForm((prev) => ({ ...prev, key: value }))}
      />
      {isEditing && (
        <p className='text-muted-foreground -mt-2 text-xs'>
          {t('Enter new key to update, or leave empty to keep current key')}
        </p>
      )}
    </div>
  )
}

function shouldShowGeneralBaseUrl(type: number) {
  return ![3, 8, 22, 36, 45].includes(type)
}

function getProviderKeyPlaceholder(params: {
  type: number
  awsKeyType: 'ak_sk' | 'api_key'
  isEditing: boolean
  t: (key: string) => string
}) {
  if (params.isEditing) return params.t('Leave blank to keep the existing key')
  if (params.type === 33) {
    return params.awsKeyType === 'api_key'
      ? params.t('Enter API Key, format: APIKey|Region')
      : params.t('Enter key, format: AccessKey|SecretAccessKey|Region')
  }
  return params.t(getKeyPromptForType(params.type))
}

function TextInputField(props: {
  id: string
  label: string
  value: string
  placeholder?: string
  onChange: (value: string) => void
}) {
  return (
    <div className='grid gap-2'>
      <Label htmlFor={props.id}>{props.label}</Label>
      <Input
        id={props.id}
        value={props.value}
        placeholder={props.placeholder}
        onChange={(event) => props.onChange(event.target.value)}
      />
    </div>
  )
}

function TextareaField(props: {
  id: string
  label: string
  value: string
  placeholder?: string
  rows?: number
  onChange: (value: string) => void
}) {
  return (
    <div className='grid gap-2'>
      <Label htmlFor={props.id}>{props.label}</Label>
      <Textarea
        id={props.id}
        value={props.value}
        placeholder={props.placeholder}
        rows={props.rows}
        onChange={(event) => props.onChange(event.target.value)}
      />
    </div>
  )
}

function SelectField(props: {
  id: string
  label: string
  value: string
  options: Array<{ value: string; label: string }>
  onChange: (value: string) => void
}) {
  return (
    <div className='grid gap-2'>
      <Label htmlFor={props.id}>{props.label}</Label>
      <select
        id={props.id}
        className='border-input bg-background h-9 rounded-md border px-3 text-sm outline-none'
        value={props.value}
        onChange={(event) => props.onChange(event.target.value)}
      >
        {props.options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </div>
  )
}

function CheckboxField(props: {
  id: string
  label: string
  checked: boolean
  onChange: (checked: boolean) => void
}) {
  return (
    <label
      htmlFor={props.id}
      className='flex items-center justify-between gap-3 rounded-md border px-3 py-2 text-sm'
    >
      <span>{props.label}</span>
      <input
        id={props.id}
        type='checkbox'
        checked={props.checked}
        onChange={(event) => props.onChange(event.target.checked)}
      />
    </label>
  )
}

function SupplierCardsGrid(props: {
  suppliers: SupplierDirectoryItem[]
  groupPerformance: Record<string, PerfGroupSummary>
}) {
  return (
    <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4'>
      {props.suppliers.map((supplier) => (
        <SupplierCard
          key={supplier.group}
          supplier={supplier}
          performance={
            supplier.performance ?? props.groupPerformance[supplier.group]
          }
        />
      ))}
    </div>
  )
}

function SupplierCard({
  supplier,
  performance,
}: {
  supplier: SupplierDirectoryItem
  performance?: PerfGroupSummary
}) {
  const { t } = useTranslation()
  const discountLabels = useGroupDiscountLabels()
  const discount = formatGroupDiscount(supplier.ratio, discountLabels)

  return (
    <Card size='sm' className='hover:bg-muted/20 min-h-56 transition-colors'>
      <CardHeader>
        <CardTitle className='min-w-0 truncate'>{supplier.group}</CardTitle>
        <CardDescription className='line-clamp-2 min-h-10'>
          {supplier.description
            ? t(supplier.description)
            : t('No description available.')}
        </CardDescription>
        <CardAction>
          <Badge variant='outline' className='font-normal'>
            {t(supplier.categoryName)}
          </Badge>
        </CardAction>
      </CardHeader>
      <CardContent className='flex flex-1 flex-col gap-4'>
        <div className='flex flex-wrap items-center gap-2'>
          {discount ? (
            <Badge className='border-info bg-info text-info-foreground'>
              <BadgePercent data-icon='inline-start' />
              {discount}
            </Badge>
          ) : (
            <Badge
              variant='outline'
              className='text-muted-foreground font-normal'
            >
              {t('No active discounts')}
            </Badge>
          )}
        </div>
      </CardContent>
      <SupplierPerformanceFooter performance={performance} />
    </Card>
  )
}

function SupplierMetric(props: { label: string; value: React.ReactNode }) {
  return (
    <div className='bg-muted/30 rounded-md border px-3 py-2'>
      <div className='text-muted-foreground text-xs'>{props.label}</div>
      <div className='text-lg font-semibold tabular-nums'>{props.value}</div>
    </div>
  )
}

function ModelNamesTooltip(props: { count: number; models: string[] }) {
  const { t } = useTranslation()

  if (props.count === 0) {
    return <span>{props.count}</span>
  }

  return (
    <TooltipProvider delay={120}>
      <Tooltip>
        <TooltipTrigger
          render={
            <button
              type='button'
              className='hover:text-primary font-inherit focus-visible:ring-ring/50 rounded-sm text-left tabular-nums underline-offset-4 outline-none hover:underline focus-visible:ring-2'
              aria-label={t('Models')}
            >
              {props.count}
            </button>
          }
        />
        <TooltipContent side='top' className='max-w-80'>
          <div className='flex max-w-72 flex-col gap-1.5'>
            <div className='font-medium'>{t('Models')}</div>
            <div className='flex flex-wrap gap-1'>
              {props.models.map((model) => (
                <span
                  key={model}
                  className='bg-background/15 rounded px-1.5 py-0.5 font-mono'
                >
                  {model}
                </span>
              ))}
            </div>
          </div>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

function SupplierPerformancePills(props: { performance?: PerfGroupSummary }) {
  if (!props.performance || props.performance.request_count <= 0) {
    return null
  }

  return (
    <>
      <Badge variant='outline' className='font-mono font-normal tabular-nums'>
        TPS {formatThroughput(props.performance.avg_tps)}
      </Badge>
      <Badge variant='outline' className='font-mono font-normal tabular-nums'>
        TTFT {formatLatency(props.performance.avg_ttft_ms)}
      </Badge>
    </>
  )
}

function SupplierPerformanceFooter(props: { performance?: PerfGroupSummary }) {
  const { t } = useTranslation()

  if (!props.performance || props.performance.request_count <= 0) {
    return (
      <CardFooter className='text-muted-foreground justify-between gap-3 text-xs'>
        {t('No performance data available')}
      </CardFooter>
    )
  }

  return (
    <CardFooter className='flex-col items-stretch gap-3'>
      <div className='grid grid-cols-4 gap-2 text-xs'>
        <PerformanceMetric
          label='TTFT'
          value={formatLatency(props.performance.avg_ttft_ms)}
        />
        <PerformanceMetric
          label={t('Latency short')}
          value={formatLatency(props.performance.avg_latency_ms)}
        />
        <PerformanceMetric
          label='TPS'
          value={formatThroughput(props.performance.avg_tps)}
        />
        <PerformanceMetric
          label={t('Success rate')}
          value={formatUptimePct(props.performance.success_rate)}
        />
      </div>
    </CardFooter>
  )
}

function PerformanceMetric(props: { label: string; value: string }) {
  return (
    <div className='min-w-0'>
      <div className='text-muted-foreground truncate'>{props.label}</div>
      <div className='truncate font-mono font-semibold tabular-nums'>
        {props.value}
      </div>
    </div>
  )
}

function getModelList(value: string | undefined) {
  return (value || '')
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
}

function getModelCount(value: string | undefined) {
  return (value || '')
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean).length
}

function SupplierDirectorySkeleton() {
  return (
    <div className='flex min-h-0 flex-1 flex-col gap-4 overflow-auto p-4 lg:p-6'>
      <div className='flex items-end justify-between gap-3'>
        <div className='flex flex-col gap-2'>
          <Skeleton className='h-7 w-40' />
        </div>
      </div>
      <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4'>
        {Array.from({ length: 8 }).map((_, index) => (
          <Skeleton key={index} className='h-56 rounded-lg' />
        ))}
      </div>
    </div>
  )
}
