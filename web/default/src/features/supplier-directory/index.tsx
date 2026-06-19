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
import { FormEvent, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  BadgePercent,
  Building2,
  ChevronDown,
  KeyRound,
  Pencil,
  Plus,
  Trash2,
} from 'lucide-react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import { formatGroupDiscount } from '@/lib/group-discount'
import { useGroupDiscountLabels } from '@/hooks/use-group-discount-labels'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
import { getPricing } from '@/features/pricing/api'
import { formatCompactNumber, formatLogQuota } from '@/lib/format'
import {
  createUserOwnedProvider,
  deleteUserOwnedProvider,
  getUserOwnedProviders,
  updateUserOwnedProvider,
} from './api'
import {
  buildSupplierDirectoryData,
  type SupplierDirectoryItem,
} from './lib/supplier-directory'
import {
  buildUserOwnedProviderPayload,
  defaultProviderForm,
  providerToFormState,
  type ProviderFormState,
  type UserOwnedProviderRow,
} from './lib/user-owned-provider-form'

type SupplierCategorySection = {
  id: string
  name: string
  suppliers: SupplierDirectoryItem[]
}

export function SupplierDirectory() {
  const { t } = useTranslation()
  const { data, isLoading } = useQuery({
    queryKey: ['pricing', 'supplier-directory'],
    queryFn: getPricing,
    staleTime: 5 * 60 * 1000,
  })

  const ownedQuery = useQuery({
    queryKey: ['supplier-directory', 'user-owned-providers'],
    queryFn: getUserOwnedProviders,
  })

  const directory = useMemo(() => buildSupplierDirectoryData(data), [data])
  const sections = useMemo(
    () => groupSuppliersByCategory(directory.items),
    [directory.items]
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
      />

      {sections.length === 0 ? (
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
        <SupplierCategorySections sections={sections} />
      )}
    </PageTransition>
  )
}

function groupSuppliersByCategory(
  suppliers: SupplierDirectoryItem[]
): SupplierCategorySection[] {
  const sectionMap = new Map<string, SupplierCategorySection>()

  for (const supplier of suppliers) {
    const existing = sectionMap.get(supplier.categoryId)
    if (existing) {
      existing.suppliers.push(supplier)
      continue
    }
    sectionMap.set(supplier.categoryId, {
      id: supplier.categoryId,
      name: supplier.categoryName,
      suppliers: [supplier],
    })
  }

  return [...sectionMap.values()]
}

function UserOwnedProvidersSection(props: {
  providers: UserOwnedProviderRow[]
  isLoading: boolean
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
          requestCount: total.requestCount + (provider.stats?.request_count ?? 0),
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
    <section className='rounded-lg border bg-card'>
      <div className='flex flex-col gap-3 border-b px-4 py-3 sm:flex-row sm:items-center sm:justify-between'>
        <div className='min-w-0'>
          <h2 className='font-semibold'>{t('My Providers')}</h2>
          <p className='text-muted-foreground text-xs'>
            {t('Use your own upstream keys without wallet billing.')}
          </p>
        </div>
        <div className='flex flex-wrap items-center gap-2 sm:justify-end'>
          {props.providers.length > 0 && (
            <div className='flex items-center gap-3 rounded-md border bg-muted/30 px-3 py-1.5 text-xs'>
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
            <Plus data-icon='start' />
            {t('Create Provider')}
          </Button>
        </div>
      </div>

      <div className='divide-y'>
        {props.isLoading ? (
          Array.from({ length: 2 }).map((_, index) => (
            <Skeleton key={index} className='m-4 h-14 rounded-md' />
          ))
        ) : props.providers.length === 0 ? (
          <div className='text-muted-foreground px-4 py-6 text-sm'>
            {t('No custom providers yet.')}
          </div>
        ) : (
          props.providers.map((provider) => (
            <div
              key={provider.id}
              className='grid gap-2 px-4 py-3 text-sm md:grid-cols-[minmax(10rem,0.8fr)_minmax(8rem,0.5fr)_minmax(14rem,1fr)_minmax(9rem,0.55fr)_auto] md:items-center'
            >
              <div className='min-w-0'>
                <div className='truncate font-medium'>{provider.name}</div>
                <div className='text-muted-foreground truncate text-xs'>
                  {provider.group}
                </div>
              </div>
              <Badge variant='outline' className='w-fit font-normal'>
                {t(getChannelTypeLabel(provider.type))}
              </Badge>
              <div className='text-muted-foreground min-w-0 truncate text-xs'>
                {provider.models || '-'}
              </div>
              <div className='flex min-w-0 flex-col gap-0.5 md:items-end'>
                <div className='font-medium tabular-nums'>
                  {formatLogQuota(provider.stats?.quota ?? 0)}
                </div>
                <div className='text-muted-foreground text-xs'>
                  {t('Estimated spend')} ·{' '}
                  {formatCompactNumber(provider.stats?.request_count ?? 0)}{' '}
                  {t('Requests')}
                </div>
              </div>
              <div className='flex justify-start gap-1 md:justify-end'>
                <Button
                  variant='ghost'
                  size='icon-sm'
                  onClick={() => openEditDialog(provider)}
                  aria-label={t('Edit')}
                >
                  <Pencil />
                </Button>
                <Button
                  variant='ghost'
                  size='icon-sm'
                  disabled={deleteMutation.isPending}
                  onClick={() => deleteMutation.mutate(provider.id)}
                  aria-label={t('Delete')}
                >
                  <Trash2 />
                </Button>
              </div>
            </div>
          ))
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
                : t('Create a private provider backed by your own upstream key.')}
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
              <Button
                type='button'
                variant='outline'
                onClick={resetDialog}
              >
                {t('Cancel')}
              </Button>
              <Button
                type='submit'
                disabled={createMutation.isPending || updateMutation.isPending}
              >
                <KeyRound data-icon='start' />
                {editingProvider ? t('Update Provider') : t('Create Provider')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </section>
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

function SupplierCategorySections(props: {
  sections: SupplierCategorySection[]
}) {
  const { t } = useTranslation()

  return (
    <div className='flex flex-col gap-3'>
      {props.sections.map((section) => (
        <details
          key={section.id}
          open
          className='group overflow-hidden rounded-lg border bg-card'
        >
          <summary className='flex cursor-pointer list-none items-center justify-between gap-3 px-4 py-3 transition-colors hover:bg-muted/30'>
            <div className='flex min-w-0 items-center gap-2'>
              <ChevronDown className='text-muted-foreground size-4 shrink-0 transition-transform group-open:rotate-180' />
              <h2 className='truncate text-base font-semibold'>
                {t(section.name)}
              </h2>
            </div>
            <Badge variant='secondary' className='font-normal tabular-nums'>
              {section.suppliers.length}
            </Badge>
          </summary>
          <div className='divide-y border-t'>
            {section.suppliers.map((supplier) => (
              <SupplierRow key={supplier.group} supplier={supplier} />
            ))}
          </div>
        </details>
      ))}
    </div>
  )
}

function SupplierRow({ supplier }: { supplier: SupplierDirectoryItem }) {
  const { t } = useTranslation()
  const discountLabels = useGroupDiscountLabels()
  const discount = formatGroupDiscount(supplier.ratio, discountLabels)

  return (
    <div className='grid gap-2 px-4 py-3 text-sm transition-colors hover:bg-muted/30 md:grid-cols-[minmax(10rem,0.8fr)_minmax(14rem,1.4fr)_auto] md:items-center'>
      <div className='min-w-0 truncate font-medium'>{supplier.group}</div>
      <div className='text-muted-foreground min-w-0 truncate text-xs'>
        {supplier.description ? t(supplier.description) : '-'}
      </div>
      <div className='flex justify-start md:justify-end'>
        {discount ? (
          <Badge className='border-info bg-info text-info-foreground'>
            <BadgePercent data-icon='inline-start' />
            {discount}
          </Badge>
        ) : (
          <Badge variant='outline' className='text-muted-foreground font-normal'>
            {t('No active discounts')}
          </Badge>
        )}
      </div>
    </div>
  )
}

function SupplierDirectorySkeleton() {
  return (
    <div className='flex min-h-0 flex-1 flex-col gap-4 overflow-auto p-4 lg:p-6'>
      <div className='flex items-end justify-between gap-3'>
        <div className='flex flex-col gap-2'>
          <Skeleton className='h-7 w-40' />
        </div>
      </div>
      <div className='flex flex-col gap-3'>
        {Array.from({ length: 8 }).map((_, index) => (
          <Skeleton key={index} className='h-14 rounded-lg' />
        ))}
      </div>
    </div>
  )
}
