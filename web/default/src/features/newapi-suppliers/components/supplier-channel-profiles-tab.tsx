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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, Pencil, RefreshCw, Search, TestTube } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
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
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { CHANNEL_TYPE_OPTIONS } from '@/features/channels/constants'
import { getChannelTypeLabel } from '@/features/channels/lib/channel-utils'
import { formatTimestamp } from '@/lib/format'
import { normalizePagedData } from '@/lib/paged-response'
import {
  getLocalGroups,
  listNewAPISupplierChannelProfileModels,
  listNewAPISupplierChannelProfiles,
  syncNewAPISupplierChannelProfile,
  testAllNewAPISupplierChannelProfiles,
  testNewAPISupplierChannelProfileModels,
  testNewAPISupplierChannelProfileModel,
  updateNewAPISupplierChannelProfile,
} from '../api'
import type {
  NewAPISupplierChannelProfile,
  NewAPISupplierChannelProfileUpdateRequest,
} from '../types'

type ProfileEditForm = {
  local_group: string
  channel_type: string
  channel_name_template: string
  tag: string
  weight: string
  priority: string
  auto_ban: boolean
  balance_threshold: string
  channel_ratio: string
  sync_mode: string
  sync_status: string
  channel_status: string
}

type ProfileFilters = {
  supplier: string
  upstream_group: string
  local_group: string
  managed_channel: string
  sync_status: string
  channel_status: string
  model: string
}

const emptyProfileFilters: ProfileFilters = {
  supplier: '',
  upstream_group: '',
  local_group: '',
  managed_channel: 'all',
  sync_status: 'all',
  channel_status: 'all',
  model: '',
}

const managedChannelFilterLabels: Record<string, string> = {
  all: 'All Managed Channels',
  linked: 'Linked',
  unlinked: 'Unlinked',
}

const syncStatusLabels: Record<string, string> = {
  all: 'All Sync Status',
  pending: 'Pending',
  synced: 'Synced',
  created: 'Created',
  failed: 'Failed',
  disabled: 'Disabled',
}

const channelStatusLabels: Record<string, string> = {
  all: 'All Channel Status',
  available: 'Available',
  unavailable: 'Unavailable',
}

const syncModeLabels: Record<string, string> = {
  managed: 'Managed',
  manual: 'Manual',
  ignored: 'Ignored',
}

function buildLocalGroupOptions(groups: string[], current: string) {
  return Array.from(
    new Set(
      [...groups, current]
        .map((group) => group.trim())
        .filter((group) => group.length > 0)
    )
  )
}

function translatedSelectLabel(
  labels: Record<string, string>,
  value: unknown,
  t: (key: string) => string
) {
  const key = String(value ?? '')
  return t(labels[key] ?? 'Unknown')
}

function translatedChannelTypeLabel(
  value: unknown,
  t: (key: string) => string
) {
  const type = Number(value)
  return t(Number.isFinite(type) ? getChannelTypeLabel(type) : 'Unknown')
}

export function SupplierChannelProfilesTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [filters, setFilters] = useState<ProfileFilters>(emptyProfileFilters)
  const [submittedFilters, setSubmittedFilters] =
    useState<ProfileFilters>(emptyProfileFilters)
  const [selectedProfileIds, setSelectedProfileIds] = useState<number[]>([])
  const [selectedModelNames, setSelectedModelNames] = useState<string[]>([])
  const [selectedProfile, setSelectedProfile] =
    useState<NewAPISupplierChannelProfile | null>(null)
  const [editingProfile, setEditingProfile] =
    useState<NewAPISupplierChannelProfile | null>(null)
  const [editForm, setEditForm] = useState<ProfileEditForm>(() =>
    profileToEditForm(null)
  )

  const { data, isLoading } = useQuery({
    queryKey: ['newapi-supplier-channel-profiles', submittedFilters],
    queryFn: () =>
      listNewAPISupplierChannelProfiles({
        p: 1,
        page_size: 100,
        supplier: submittedFilters.supplier || undefined,
        upstream_group: submittedFilters.upstream_group || undefined,
        local_group: submittedFilters.local_group || undefined,
        managed_channel:
          submittedFilters.managed_channel === 'all'
            ? undefined
            : submittedFilters.managed_channel === 'linked'
              ? 'linked'
              : 'unlinked',
        sync_status:
          submittedFilters.sync_status === 'all'
            ? undefined
            : submittedFilters.sync_status,
        channel_status:
          submittedFilters.channel_status === 'all'
            ? undefined
            : submittedFilters.channel_status,
        model: submittedFilters.model || undefined,
      }),
  })

  const { data: localGroupsData } = useQuery({
    queryKey: ['groups'],
    queryFn: getLocalGroups,
    staleTime: 5 * 60 * 1000,
  })

  const profiles = useMemo(() => normalizePagedData(data).items, [data])
  const localGroups = useMemo(
    () => localGroupsData?.data ?? [],
    [localGroupsData]
  )

  const { data: modelsData } = useQuery({
    queryKey: ['newapi-supplier-channel-profile-models', selectedProfile?.id],
    queryFn: () =>
      listNewAPISupplierChannelProfileModels(Number(selectedProfile?.id)),
    enabled: Boolean(selectedProfile?.id),
  })

  const profileModels = useMemo(() => modelsData?.data ?? [], [modelsData])
  const allVisibleProfilesSelected =
    profiles.length > 0 &&
    profiles.every((profile) => selectedProfileIds.includes(profile.id))
  const allVisibleModelsSelected =
    profileModels.length > 0 &&
    profileModels.every((item) => selectedModelNames.includes(item.model_name))

  useEffect(() => {
    setEditForm(profileToEditForm(editingProfile))
  }, [editingProfile])

  useEffect(() => {
    setSelectedProfileIds((prev) => {
      const visibleIds = new Set(profiles.map((profile) => profile.id))
      return prev.filter((id) => visibleIds.has(id))
    })
  }, [profiles])

  useEffect(() => {
    setSelectedModelNames([])
  }, [selectedProfile?.id])

  useEffect(() => {
    setSelectedModelNames((prev) => {
      const modelNames = new Set(profileModels.map((item) => item.model_name))
      return prev.filter((name) => modelNames.has(name))
    })
  }, [profileModels])

  const updateMutation = useMutation({
    mutationFn: ({
      profileId,
      data,
    }: {
      profileId: number
      data: NewAPISupplierChannelProfileUpdateRequest
    }) => updateNewAPISupplierChannelProfile(profileId, data),
    onSuccess: (res) => {
      if (!res.success) {
        toast.error(res.message || t('Operation failed'))
        return
      }
      toast.success(t('Supplier channel profile saved'))
      setEditingProfile(null)
      queryClient.invalidateQueries({
        queryKey: ['newapi-supplier-channel-profiles'],
      })
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t('Operation failed'))
    },
  })

  const inlineUpdateMutation = useMutation({
    mutationFn: ({
      profileId,
      data,
    }: {
      profileId: number
      data: NewAPISupplierChannelProfileUpdateRequest
    }) => updateNewAPISupplierChannelProfile(profileId, data),
    onSuccess: (res) => {
      if (!res.success) {
        toast.error(res.message || t('Operation failed'))
        return
      }
      toast.success(t('Supplier channel profile saved'))
      queryClient.invalidateQueries({
        queryKey: ['newapi-supplier-channel-profiles'],
      })
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t('Operation failed'))
    },
  })

  const testModelMutation = useMutation({
    mutationFn: ({ profileId, model }: { profileId: number; model: string }) =>
      testNewAPISupplierChannelProfileModel(profileId, {
        model,
        endpoint_type: selectedProfile?.endpoint_type,
        stream: false,
      }),
    onSuccess: (res) => {
      if (!res.success) {
        toast.error(res.message || t('Model test failed'))
      } else {
        toast.success(t('Model test passed'))
      }
      queryClient.invalidateQueries({
        queryKey: ['newapi-supplier-channel-profile-models', selectedProfile?.id],
      })
      queryClient.invalidateQueries({
        queryKey: ['newapi-supplier-channel-profiles'],
      })
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t('Model test failed'))
    },
  })

  const testProfileMutation = useMutation({
    mutationFn: ({
      profileId,
      models,
    }: {
      profileId: number
      models?: string[]
    }) =>
      testNewAPISupplierChannelProfileModels(profileId, {
        stream: false,
        models,
      }),
    onSuccess: (res) => {
      if (!res.success || !res.data) {
        toast.error(res.message || t('Channel test failed'))
        return
      }
      toast.success(
        t('Channel test completed: {{passed}} passed, {{failed}} failed', {
          passed: res.data.passed,
          failed: res.data.failed,
        })
      )
      queryClient.invalidateQueries({
        queryKey: ['newapi-supplier-channel-profile-models', selectedProfile?.id],
      })
      queryClient.invalidateQueries({
        queryKey: ['newapi-supplier-channel-profiles'],
      })
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t('Channel test failed'))
    },
  })

  const testAllProfilesMutation = useMutation({
    mutationFn: (profileIds?: number[]) =>
      testAllNewAPISupplierChannelProfiles({
        stream: false,
        profile_ids: profileIds?.length ? profileIds : undefined,
      }),
    onSuccess: (res) => {
      if (!res.success || !res.data) {
        toast.error(res.message || t('All channel tests failed'))
        return
      }
      toast.success(
        t('All channel tests completed: {{passed}} passed, {{failed}} failed', {
          passed: res.data.passed,
          failed: res.data.failed,
        })
      )
      queryClient.invalidateQueries({
        queryKey: ['newapi-supplier-channel-profile-models'],
      })
      queryClient.invalidateQueries({
        queryKey: ['newapi-supplier-channel-profiles'],
      })
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('All channel tests failed')
      )
    },
  })

  const syncProfileMutation = useMutation({
    mutationFn: (profileId: number) =>
      syncNewAPISupplierChannelProfile(profileId),
    onSuccess: (res) => {
      if (!res.success) {
        toast.error(res.message || t('Supplier channel sync failed'))
        return
      }
      toast.success(t('Supplier channel synced'))
      queryClient.invalidateQueries({
        queryKey: ['newapi-supplier-channel-profiles'],
      })
    },
    onError: (error) => {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Supplier channel sync failed')
      )
    },
  })

  const submitEdit = () => {
    if (!editingProfile) return
    updateMutation.mutate({
      profileId: editingProfile.id,
      data: editFormToRequest(editForm),
    })
  }

  const applyFilters = () => {
    setSubmittedFilters({
      ...filters,
      supplier: filters.supplier.trim(),
      upstream_group: filters.upstream_group.trim(),
      local_group: filters.local_group.trim(),
      model: filters.model.trim(),
    })
  }

  const toggleAllVisibleProfiles = (checked: boolean) => {
    const visibleIds = profiles.map((profile) => profile.id)
    setSelectedProfileIds((prev) => {
      if (checked) {
        return Array.from(new Set([...prev, ...visibleIds]))
      }
      return prev.filter((id) => !visibleIds.includes(id))
    })
  }

  const toggleProfile = (profileId: number, checked: boolean) => {
    setSelectedProfileIds((prev) =>
      checked
        ? Array.from(new Set([...prev, profileId]))
        : prev.filter((id) => id !== profileId)
    )
  }

  const toggleAllVisibleModels = (checked: boolean) => {
    const modelNames = profileModels.map((item) => item.model_name)
    setSelectedModelNames((prev) => {
      if (checked) {
        return Array.from(new Set([...prev, ...modelNames]))
      }
      return prev.filter((name) => !modelNames.includes(name))
    })
  }

  const toggleModel = (modelName: string, checked: boolean) => {
    setSelectedModelNames((prev) =>
      checked
        ? Array.from(new Set([...prev, modelName]))
        : prev.filter((name) => name !== modelName)
    )
  }

  const updateProfileChannelTypeInline = (
    profile: NewAPISupplierChannelProfile,
    value: string
  ) => {
    const channelType = Number(value)
    if (!Number.isFinite(channelType) || channelType === profile.channel_type) {
      return
    }
    inlineUpdateMutation.mutate({
      profileId: profile.id,
      data: {
        ...profileToUpdateRequest(profile),
        channel_type: channelType,
      },
    })
  }

  const updateProfileChannelStatusInline = (
    profile: NewAPISupplierChannelProfile,
    value: string
  ) => {
    if (!value || value === profile.channel_status) {
      return
    }
    inlineUpdateMutation.mutate({
      profileId: profile.id,
      data: {
        ...profileToUpdateRequest(profile),
        channel_status: value,
      },
    })
  }

  const updateProfileLocalGroupInline = (
    profile: NewAPISupplierChannelProfile,
    value: string
  ) => {
    const localGroup = value.trim()
    if (!localGroup || localGroup === profile.local_group) {
      return
    }
    inlineUpdateMutation.mutate({
      profileId: profile.id,
      data: {
        ...profileToUpdateRequest(profile),
        local_group: localGroup,
      },
    })
  }

  return (
    <>
      <div className='space-y-3'>
        <div className='flex flex-wrap items-center gap-2'>
          <Input
            className='w-full sm:w-72'
            value={filters.model}
            placeholder={t('Filter by model name')}
            onChange={(event) =>
              setFilters((prev) => ({ ...prev, model: event.target.value }))
            }
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                applyFilters()
              }
            }}
          />
          <Input
            className='w-full sm:w-52'
            value={filters.supplier}
            placeholder={t('Supplier ID or name')}
            onChange={(event) =>
              setFilters((prev) => ({ ...prev, supplier: event.target.value }))
            }
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                applyFilters()
              }
            }}
          />
          <Input
            className='w-full sm:w-44'
            value={filters.upstream_group}
            placeholder={t('Upstream Group')}
            onChange={(event) =>
              setFilters((prev) => ({
                ...prev,
                upstream_group: event.target.value,
              }))
            }
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                applyFilters()
              }
            }}
          />
          <Input
            className='w-full sm:w-44'
            value={filters.local_group}
            placeholder={t('Local Group')}
            onChange={(event) =>
              setFilters((prev) => ({
                ...prev,
                local_group: event.target.value,
              }))
            }
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                applyFilters()
              }
            }}
          />
          <Select
            value={filters.managed_channel}
            onValueChange={(value) =>
              setFilters((prev) => ({
                ...prev,
                managed_channel: value ?? 'all',
              }))
            }
          >
            <SelectTrigger className='w-full sm:w-44'>
              <SelectValue>
                {(value) =>
                  translatedSelectLabel(managedChannelFilterLabels, value, t)
                }
              </SelectValue>
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectItem value='all'>{t('All Managed Channels')}</SelectItem>
              <SelectItem value='linked'>{t('Linked')}</SelectItem>
              <SelectItem value='unlinked'>{t('Unlinked')}</SelectItem>
            </SelectContent>
          </Select>
          <Select
            value={filters.sync_status}
            onValueChange={(value) =>
              setFilters((prev) => ({ ...prev, sync_status: value ?? 'all' }))
            }
          >
            <SelectTrigger className='w-full sm:w-40'>
              <SelectValue>
                {(value) => translatedSelectLabel(syncStatusLabels, value, t)}
              </SelectValue>
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectItem value='all'>{t('All Sync Status')}</SelectItem>
              <SelectItem value='pending'>{t('Pending')}</SelectItem>
              <SelectItem value='synced'>{t('Synced')}</SelectItem>
              <SelectItem value='created'>{t('Created')}</SelectItem>
              <SelectItem value='failed'>{t('Failed')}</SelectItem>
              <SelectItem value='disabled'>{t('Disabled')}</SelectItem>
            </SelectContent>
          </Select>
          <Select
            value={filters.channel_status}
            onValueChange={(value) =>
              setFilters((prev) => ({
                ...prev,
                channel_status: value ?? 'all',
              }))
            }
          >
            <SelectTrigger className='w-full sm:w-44'>
              <SelectValue>
                {(value) => translatedSelectLabel(channelStatusLabels, value, t)}
              </SelectValue>
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectItem value='all'>{t('All Channel Status')}</SelectItem>
              <SelectItem value='available'>{t('Available')}</SelectItem>
              <SelectItem value='unavailable'>{t('Unavailable')}</SelectItem>
            </SelectContent>
          </Select>
          <Button onClick={applyFilters}>
            <Search className='h-4 w-4' />
            {t('Search')}
          </Button>
          <Button
            variant='outline'
            onClick={() => testAllProfilesMutation.mutate(selectedProfileIds)}
            disabled={
              testAllProfilesMutation.isPending ||
              selectedProfileIds.length === 0
            }
          >
            {testAllProfilesMutation.isPending ? (
              <Loader2 className='h-4 w-4 animate-spin' />
            ) : (
              <TestTube className='h-4 w-4' />
            )}
            {t('Test Selected Channels')}
          </Button>
          <Button
            variant='outline'
            onClick={() => testAllProfilesMutation.mutate(undefined)}
            disabled={testAllProfilesMutation.isPending || profiles.length === 0}
          >
            {testAllProfilesMutation.isPending ? (
              <Loader2 className='h-4 w-4 animate-spin' />
            ) : (
              <TestTube className='h-4 w-4' />
            )}
            {t('Test All Channels')}
          </Button>
        </div>
        <Card>
          <CardContent className='p-0'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className='w-10'>
                    <Checkbox
                      checked={allVisibleProfilesSelected}
                      onCheckedChange={(value) =>
                        toggleAllVisibleProfiles(Boolean(value))
                      }
                      aria-label={t('Select all channels')}
                    />
                  </TableHead>
                  <TableHead>{t('Supplier')}</TableHead>
                  <TableHead>{t('Upstream Group')}</TableHead>
                  <TableHead>{t('Ratio')}</TableHead>
                  <TableHead>{t('Channel Type')}</TableHead>
                  <TableHead>{t('Local Group')}</TableHead>
                  <TableHead>{t('Supported Models')}</TableHead>
                  <TableHead>{t('Linked Channel')}</TableHead>
                  <TableHead>{t('Sync Status')}</TableHead>
                  <TableHead>{t('Channel Status')}</TableHead>
                  <TableHead>{t('Last Check')}</TableHead>
                  <TableHead className='text-right'>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {profiles.map((profile) => (
                  <TableRow key={profile.id}>
                    <TableCell>
                      <Checkbox
                        checked={selectedProfileIds.includes(profile.id)}
                        onCheckedChange={(value) =>
                          toggleProfile(profile.id, Boolean(value))
                        }
                        aria-label={t('Select channel {{id}}', {
                          id: profile.id,
                        })}
                      />
                    </TableCell>
                    <TableCell>
                      <div className='space-y-1'>
                        <div className='font-medium'>
                          {profile.supplier_name_snapshot}
                        </div>
                        <div className='text-muted-foreground max-w-[260px] truncate text-xs'>
                          {profile.base_url_snapshot}
                        </div>
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className='space-y-1'>
                        <div>{profile.upstream_group}</div>
                        {profile.upstream_group_desc && (
                          <div className='text-muted-foreground max-w-[220px] truncate text-xs'>
                            {profile.upstream_group_desc}
                          </div>
                        )}
                      </div>
                    </TableCell>
                    <TableCell>{profile.upstream_group_ratio || '-'}</TableCell>
                    <TableCell>
                      <Select
                        value={String(profile.channel_type)}
                        onValueChange={(value) =>
                          updateProfileChannelTypeInline(profile, value || '')
                        }
                        disabled={inlineUpdateMutation.isPending}
                      >
                        <SelectTrigger className='h-8 w-36'>
                          <SelectValue>
                            {(value) => translatedChannelTypeLabel(value, t)}
                          </SelectValue>
                        </SelectTrigger>
                        <SelectContent alignItemWithTrigger={false}>
                          {CHANNEL_TYPE_OPTIONS.map((option) => (
                            <SelectItem
                              key={option.value}
                              value={String(option.value)}
                            >
                              {t(option.label)}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <div className='text-muted-foreground mt-1 text-xs'>
                        {profile.endpoint_type}
                      </div>
                    </TableCell>
                    <TableCell>
                      <Select
                        value={profile.local_group}
                        disabled={inlineUpdateMutation.isPending}
                        onValueChange={(value) =>
                          updateProfileLocalGroupInline(profile, value || '')
                        }
                      >
                        <SelectTrigger className='h-8 w-36'>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent alignItemWithTrigger={false}>
                          {buildLocalGroupOptions(
                            localGroups,
                            profile.local_group
                          ).map((group) => (
                            <SelectItem key={group} value={group}>
                              {group}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </TableCell>
                    <TableCell>
                      <ModelSummary models={profile.model_names ?? []} />
                    </TableCell>
                    <TableCell>
                      {profile.channel_id ? (
                        <Badge variant='secondary'>#{profile.channel_id}</Badge>
                      ) : (
                        <Badge variant='outline'>{t('Not created')}</Badge>
                      )}
                    </TableCell>
                    <TableCell>
                      {profileStatusBadge(profile.sync_status, t)}
                    </TableCell>
                    <TableCell>
                      <Select
                        value={profile.channel_status || 'available'}
                        onValueChange={(value) =>
                          updateProfileChannelStatusInline(profile, value || '')
                        }
                        disabled={inlineUpdateMutation.isPending}
                      >
                        <SelectTrigger className='h-8 w-32'>
                          <SelectValue>
                            {(value) =>
                              translatedSelectLabel(
                                channelStatusLabels,
                                value,
                                t
                              )
                            }
                          </SelectValue>
                        </SelectTrigger>
                        <SelectContent alignItemWithTrigger={false}>
                          <SelectItem value='available'>
                            {t('Available')}
                          </SelectItem>
                          <SelectItem value='unavailable'>
                            {t('Unavailable')}
                          </SelectItem>
                        </SelectContent>
                      </Select>
                    </TableCell>
                    <TableCell>
                      {profile.last_checked_at
                        ? formatTimestamp(profile.last_checked_at)
                        : t('Never')}
                    </TableCell>
                    <TableCell className='text-right'>
                      <div className='flex justify-end gap-2'>
                        <Button
                          size='sm'
                          variant='outline'
                          onClick={() => setEditingProfile(profile)}
                        >
                          <Pencil className='h-4 w-4' />
                          {t('Edit')}
                        </Button>
                        <Button
                          size='sm'
                          variant='outline'
                          onClick={() => setSelectedProfile(profile)}
                        >
                          <TestTube className='h-4 w-4' />
                          {t('Test')}
                        </Button>
                        <Button
                          size='sm'
                          variant='outline'
                          disabled={syncProfileMutation.isPending}
                          onClick={() => syncProfileMutation.mutate(profile.id)}
                        >
                          {syncProfileMutation.isPending ? (
                            <Loader2 className='h-4 w-4 animate-spin' />
                          ) : (
                            <RefreshCw className='h-4 w-4' />
                          )}
                          {t('Sync')}
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
                {!isLoading && profiles.length === 0 && (
                  <TableRow>
                    <TableCell
                      className='text-muted-foreground h-24 text-center'
                      colSpan={12}
                    >
                      {t('No supplier channel profiles found')}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>

      <Dialog
        open={Boolean(selectedProfile)}
        onOpenChange={(open) => {
          if (!open) setSelectedProfile(null)
        }}
      >
        <DialogContent className='max-h-[85vh] overflow-y-auto sm:max-w-4xl'>
          <DialogHeader>
            <div className='flex flex-wrap items-start justify-between gap-3'>
              <div>
                <DialogTitle>{t('Profile Models')}</DialogTitle>
                <DialogDescription>
                  {selectedProfile?.supplier_name_snapshot} /{' '}
                  {selectedProfile?.upstream_group}
                </DialogDescription>
              </div>
              <Button
                size='sm'
                variant='outline'
                disabled={!selectedProfile || testProfileMutation.isPending}
                onClick={() => {
                  if (!selectedProfile) return
                  testProfileMutation.mutate({ profileId: selectedProfile.id })
                }}
              >
                {testProfileMutation.isPending ? (
                  <Loader2 className='h-4 w-4 animate-spin' />
                ) : (
                  <TestTube className='h-4 w-4' />
                )}
                {t('Test This Channel')}
              </Button>
              <Button
                size='sm'
                variant='outline'
                disabled={
                  !selectedProfile ||
                  testProfileMutation.isPending ||
                  selectedModelNames.length === 0
                }
                onClick={() => {
                  if (!selectedProfile) return
                  testProfileMutation.mutate({
                    profileId: selectedProfile.id,
                    models: selectedModelNames,
                  })
                }}
              >
                {testProfileMutation.isPending ? (
                  <Loader2 className='h-4 w-4 animate-spin' />
                ) : (
                  <TestTube className='h-4 w-4' />
                )}
                {t('Test Selected Models')}
              </Button>
            </div>
          </DialogHeader>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className='w-10'>
                  <Checkbox
                    checked={allVisibleModelsSelected}
                    onCheckedChange={(value) =>
                      toggleAllVisibleModels(Boolean(value))
                    }
                    aria-label={t('Select all models')}
                  />
                </TableHead>
                <TableHead>{t('Model')}</TableHead>
                <TableHead>{t('Provider')}</TableHead>
                <TableHead>{t('Availability')}</TableHead>
                <TableHead>{t('Last Test')}</TableHead>
                <TableHead className='text-right'>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {profileModels.map((item) => (
                <TableRow key={item.id}>
                  <TableCell>
                    <Checkbox
                      checked={selectedModelNames.includes(item.model_name)}
                      onCheckedChange={(value) =>
                        toggleModel(item.model_name, Boolean(value))
                      }
                      aria-label={t('Select model {{model}}', {
                        model: item.model_name,
                      })}
                    />
                  </TableCell>
                  <TableCell className='font-medium'>
                    {item.model_name}
                  </TableCell>
                  <TableCell>{item.model_provider || '-'}</TableCell>
                  <TableCell>
                    {modelStatusBadge(item.available_status, t)}
                  </TableCell>
                  <TableCell>
                    {item.last_test_time
                      ? formatTimestamp(item.last_test_time)
                      : t('Never')}
                    {item.last_error && (
                      <div className='text-destructive max-w-[300px] truncate text-xs'>
                        {item.last_error}
                      </div>
                    )}
                  </TableCell>
                  <TableCell className='text-right'>
                    <Button
                      size='sm'
                      variant='outline'
                      disabled={testModelMutation.isPending}
                      onClick={() => {
                        if (!selectedProfile) return
                        testModelMutation.mutate({
                          profileId: selectedProfile.id,
                          model: item.model_name,
                        })
                      }}
                    >
                      {testModelMutation.isPending ? (
                        <Loader2 className='h-4 w-4 animate-spin' />
                      ) : (
                        <TestTube className='h-4 w-4' />
                      )}
                      {t('Test')}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </DialogContent>
      </Dialog>

      <Dialog
        open={Boolean(editingProfile)}
        onOpenChange={(open) => {
          if (!open) setEditingProfile(null)
        }}
      >
        <DialogContent className='max-h-[85vh] overflow-y-auto sm:max-w-2xl'>
          <DialogHeader>
            <DialogTitle>{t('Edit Supplier Channel Profile')}</DialogTitle>
            <DialogDescription>
              {editingProfile?.supplier_name_snapshot} /{' '}
              {editingProfile?.upstream_group}
            </DialogDescription>
          </DialogHeader>
          <div className='grid gap-4 sm:grid-cols-2'>
            <Field label={t('Local Group')}>
              <Input
                value={editForm.local_group}
                onChange={(event) =>
                  setEditForm((prev) => ({
                    ...prev,
                    local_group: event.target.value,
                  }))
                }
              />
            </Field>
            <Field label={t('Channel Type')}>
              <Select
                value={editForm.channel_type}
                onValueChange={(value) =>
                  setEditForm((prev) => ({
                    ...prev,
                    channel_type: value || prev.channel_type,
                  }))
                }
              >
                <SelectTrigger className='w-full'>
                  <SelectValue>
                    {(value) => translatedChannelTypeLabel(value, t)}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  {CHANNEL_TYPE_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={String(option.value)}>
                      {t(option.label)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field label={t('Channel Status')}>
              <Select
                value={editForm.channel_status}
                onValueChange={(value) =>
                  setEditForm((prev) => ({
                    ...prev,
                    channel_status: value || prev.channel_status,
                  }))
                }
              >
                <SelectTrigger className='w-full'>
                  <SelectValue>
                    {(value) =>
                      translatedSelectLabel(channelStatusLabels, value, t)
                    }
                  </SelectValue>
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectItem value='available'>{t('Available')}</SelectItem>
                  <SelectItem value='unavailable'>{t('Unavailable')}</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <Field label={t('Sync Status')}>
              <Select
                value={editForm.sync_status}
                onValueChange={(value) =>
                  setEditForm((prev) => ({
                    ...prev,
                    sync_status: value || prev.sync_status,
                  }))
                }
              >
                <SelectTrigger className='w-full'>
                  <SelectValue>
                    {(value) => translatedSelectLabel(syncStatusLabels, value, t)}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectItem value='pending'>{t('Pending')}</SelectItem>
                  <SelectItem value='synced'>{t('Synced')}</SelectItem>
                  <SelectItem value='created'>{t('Created')}</SelectItem>
                  <SelectItem value='failed'>{t('Failed')}</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <Field label={t('Sync Mode')}>
              <Select
                value={editForm.sync_mode}
                onValueChange={(value) =>
                  setEditForm((prev) => ({
                    ...prev,
                    sync_mode: value || prev.sync_mode,
                  }))
                }
              >
                <SelectTrigger className='w-full'>
                  <SelectValue>
                    {(value) => translatedSelectLabel(syncModeLabels, value, t)}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectItem value='managed'>{t('Managed')}</SelectItem>
                  <SelectItem value='manual'>{t('Manual')}</SelectItem>
                  <SelectItem value='ignored'>{t('Ignored')}</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <Field label={t('Channel Ratio')}>
              <Input
                value={editForm.channel_ratio}
                type='number'
                step='0.0001'
                min='0'
                onChange={(event) =>
                  setEditForm((prev) => ({
                    ...prev,
                    channel_ratio: event.target.value,
                  }))
                }
              />
            </Field>
            <Field label={t('Weight')}>
              <Input
                value={editForm.weight}
                type='number'
                min='0'
                onChange={(event) =>
                  setEditForm((prev) => ({
                    ...prev,
                    weight: event.target.value,
                  }))
                }
              />
            </Field>
            <Field label={t('Priority')}>
              <Input
                value={editForm.priority}
                type='number'
                onChange={(event) =>
                  setEditForm((prev) => ({
                    ...prev,
                    priority: event.target.value,
                  }))
                }
              />
            </Field>
            <Field label={t('Balance Threshold')}>
              <Input
                value={editForm.balance_threshold}
                type='number'
                min='0'
                onChange={(event) =>
                  setEditForm((prev) => ({
                    ...prev,
                    balance_threshold: event.target.value,
                  }))
                }
              />
            </Field>
            <Field label={t('Tag')}>
              <Input
                value={editForm.tag}
                onChange={(event) =>
                  setEditForm((prev) => ({ ...prev, tag: event.target.value }))
                }
              />
            </Field>
            <Field className='sm:col-span-2' label={t('Channel Name Template')}>
              <Input
                value={editForm.channel_name_template}
                onChange={(event) =>
                  setEditForm((prev) => ({
                    ...prev,
                    channel_name_template: event.target.value,
                  }))
                }
              />
            </Field>
            <div className='flex items-center gap-3 sm:col-span-2'>
              <Switch
                checked={editForm.auto_ban}
                onCheckedChange={(checked) =>
                  setEditForm((prev) => ({ ...prev, auto_ban: checked }))
                }
              />
              <Label>{t('Auto Ban')}</Label>
            </div>
          </div>
          <DialogFooter>
            <Button variant='outline' onClick={() => setEditingProfile(null)}>
              {t('Cancel')}
            </Button>
            <Button onClick={submitEdit} disabled={updateMutation.isPending}>
              {updateMutation.isPending && (
                <Loader2 className='h-4 w-4 animate-spin' />
              )}
              {t('Save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

function Field({
  label,
  className,
  children,
}: {
  label: string
  className?: string
  children: ReactNode
}) {
  return (
    <div className={className}>
      <Label className='mb-2 block text-xs'>{label}</Label>
      {children}
    </div>
  )
}

function ModelSummary({ models }: { models: string[] }) {
  const normalizedModels = models.filter(Boolean)
  if (normalizedModels.length === 0) {
    return <span className='text-muted-foreground text-xs'>-</span>
  }
  const fullText = normalizedModels.join(', ')
  const preview = normalizedModels.slice(0, 3).join(', ')
  const extraCount = normalizedModels.length - 3
  return (
    <div
      className='max-w-[260px] truncate text-xs'
      title={fullText}
    >
      {preview}
      {extraCount > 0 && (
        <span className='text-muted-foreground'>
          {' '}
          {`+${extraCount}`}
        </span>
      )}
    </div>
  )
}

function profileStatusBadge(status: string, t: (key: string) => string) {
  if (status === 'created' || status === 'synced') {
    return (
      <Badge variant='secondary'>
        {t(status === 'created' ? 'Created' : 'Synced')}
      </Badge>
    )
  }
  if (status === 'failed') {
    return <Badge variant='destructive'>{t('Failed')}</Badge>
  }
  return <Badge variant='outline'>{t('Pending')}</Badge>
}

function modelStatusBadge(status: string, t: (key: string) => string) {
  if (status === 'available') {
    return <Badge variant='secondary'>{t('Available')}</Badge>
  }
  if (status === 'unavailable') {
    return <Badge variant='destructive'>{t('Unavailable')}</Badge>
  }
  return <Badge variant='outline'>{t('Unknown')}</Badge>
}

function profileToEditForm(
  profile: NewAPISupplierChannelProfile | null
): ProfileEditForm {
  return {
    local_group: profile?.local_group ?? '',
    channel_type: String(profile?.channel_type ?? 1),
    channel_name_template:
      profile?.channel_name_template ??
      '{group} - {ratio} - {channel_type} - {supplier}',
    tag: profile?.tag ?? '',
    weight:
      profile?.weight === undefined || profile?.weight === null
        ? ''
        : String(profile.weight),
    priority:
      profile?.priority === undefined || profile?.priority === null
        ? ''
        : String(profile.priority),
    auto_ban: profile?.auto_ban !== 0,
    balance_threshold: String(profile?.balance_threshold ?? 0),
    channel_ratio:
      profile?.channel_ratio === undefined || profile?.channel_ratio === null
        ? ''
        : String(profile.channel_ratio),
    sync_mode: profile?.sync_mode || 'managed',
    sync_status: profile?.sync_status || 'pending',
    channel_status: profile?.channel_status || 'available',
  }
}

function editFormToRequest(
  form: ProfileEditForm
): NewAPISupplierChannelProfileUpdateRequest {
  return {
    local_group: form.local_group.trim(),
    channel_type: Number(form.channel_type) || 1,
    channel_name_template: form.channel_name_template.trim(),
    tag: form.tag.trim(),
    weight: form.weight.trim() === '' ? undefined : Number(form.weight),
    priority: form.priority.trim() === '' ? undefined : Number(form.priority),
    auto_ban: form.auto_ban ? 1 : 0,
    balance_threshold: Number(form.balance_threshold) || 0,
    channel_ratio:
      form.channel_ratio.trim() === '' ? undefined : Number(form.channel_ratio),
    sync_mode: form.sync_mode,
    sync_status: form.sync_status,
    channel_status: form.channel_status,
  }
}

function profileToUpdateRequest(
  profile: NewAPISupplierChannelProfile
): NewAPISupplierChannelProfileUpdateRequest {
  return {
    local_group: profile.local_group,
    channel_type: profile.channel_type,
    channel_name_template: profile.channel_name_template,
    tag: profile.tag,
    weight: profile.weight,
    priority: profile.priority,
    auto_ban: profile.auto_ban,
    balance_threshold: profile.balance_threshold,
    channel_ratio: profile.channel_ratio,
    sync_mode: profile.sync_mode,
    sync_status: profile.sync_status,
    channel_status: profile.channel_status || 'available',
  }
}
