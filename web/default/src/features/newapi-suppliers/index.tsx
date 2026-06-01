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
import type { ReactNode } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  CheckCircle2,
  CircleAlert,
  Coins,
  Edit,
  Plus,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { SectionPageLayout } from '@/components/layout'
import { formatQuota, formatTimestamp } from '@/lib/format'
import { normalizePagedData } from '@/lib/paged-response'
import {
  checkNewAPISupplier,
  createNewAPISupplier,
  deleteNewAPISupplier,
  getLocalGroups,
  listNewAPISuppliers,
  queryNewAPISupplierBalance,
  updateNewAPISupplier,
} from './api'
import { SupplierChannelConfigDialog } from './components/supplier-channel-config-dialog'
import type { NewAPISupplier, NewAPISupplierGroupSnapshot } from './types'

const queryKey = ['newapi-suppliers']

const emptyForm = {
  name: '',
  base_url: '',
  auth_mode: 'password' as 'password' | 'access_token',
  username: '',
  password: '',
  access_token: '',
  upstream_user_id: '',
  default_local_group: 'default',
  tag: '',
  remark: '',
}

type SupplierForm = typeof emptyForm

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
      }))
  } catch {
    return []
  }
}

function statusBadge(status: number, t: (key: string) => string) {
  if (status === 1) {
    return <Badge variant='secondary'>{t('Enabled')}</Badge>
  }
  if (status === 2) {
    return <Badge variant='destructive'>{t('Error')}</Badge>
  }
  return <Badge variant='outline'>{t('Disabled')}</Badge>
}

export function NewAPISuppliers() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [formOpen, setFormOpen] = useState(false)
  const [configOpen, setConfigOpen] = useState(false)
  const [editing, setEditing] = useState<NewAPISupplier | null>(null)
  const [configuring, setConfiguring] = useState<NewAPISupplier | null>(null)
  const [form, setForm] = useState<SupplierForm>(emptyForm)

  const { data, isLoading } = useQuery({
    queryKey,
    queryFn: () => listNewAPISuppliers({ p: 1, page_size: 100 }),
  })

  const { data: localGroupsData } = useQuery({
    queryKey: ['groups'],
    queryFn: getLocalGroups,
    staleTime: 5 * 60 * 1000,
  })

  const suppliers = useMemo(() => normalizePagedData(data).items, [data])
  const localGroups = useMemo(() => {
    const current = form.default_local_group.trim()
    const values = localGroupsData?.data ?? []
    return Array.from(new Set([...values, ...(current ? [current] : [])]))
  }, [form.default_local_group, localGroupsData])

  const saveMutation = useMutation({
    mutationFn: async () => {
      const { auth_mode: authMode, ...basePayload } = form
      const payload = {
        ...basePayload,
        tag: form.tag || form.name,
        username: authMode === 'password' ? form.username : '',
        password: authMode === 'password' ? form.password : '',
        access_token: authMode === 'access_token' ? form.access_token : '',
        upstream_user_id:
          authMode === 'access_token' ? Number(form.upstream_user_id) || 0 : 0,
      }
      if (editing) {
        return updateNewAPISupplier({ ...payload, id: editing.id })
      }
      return createNewAPISupplier(payload)
    },
    onSuccess: (res) => {
      if (!res.success) return
      toast.success(t('Supplier saved successfully'))
      setFormOpen(false)
      setEditing(null)
      setForm(emptyForm)
      queryClient.invalidateQueries({ queryKey })
    },
  })

  const checkMutation = useMutation({
    mutationFn: checkNewAPISupplier,
    onSuccess: (res) => {
      if (!res.success) return
      toast.success(t('Supplier check completed'))
      queryClient.invalidateQueries({ queryKey })
    },
  })

  const balanceMutation = useMutation({
    mutationFn: queryNewAPISupplierBalance,
    onSuccess: (res) => {
      if (!res.success) return
      toast.success(t('Supplier balance updated'))
      queryClient.invalidateQueries({ queryKey })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: deleteNewAPISupplier,
    onSuccess: (res) => {
      if (!res.success) return
      toast.success(t('Supplier deleted successfully'))
      queryClient.invalidateQueries({ queryKey })
    },
  })

  const openCreate = () => {
    setEditing(null)
    setForm(emptyForm)
    setFormOpen(true)
  }

  const openEdit = (supplier: NewAPISupplier) => {
    setEditing(supplier)
    setForm({
      name: supplier.name,
      base_url: supplier.base_url,
      auth_mode: supplier.access_token ? 'access_token' : 'password',
      username: supplier.username,
      password: '',
      access_token: '',
      upstream_user_id: supplier.upstream_user_id
        ? String(supplier.upstream_user_id)
        : '',
      default_local_group: supplier.default_local_group || 'default',
      tag: supplier.tag || supplier.name,
      remark: supplier.remark || '',
    })
    setFormOpen(true)
  }

  const openConfigure = (supplier: NewAPISupplier) => {
    if (supplier.model_source !== 'pricing') {
      toast.error(t('Run supplier check first to load groups and models'))
      return
    }
    setConfiguring(supplier)
    setConfigOpen(true)
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('Supplier Management')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t('Manage upstream NewAPI suppliers and configure channels')}
        </SectionPageLayout.Description>
        <SectionPageLayout.Actions>
          <Button onClick={openCreate} size='sm'>
            <Plus className='h-4 w-4' />
            {t('Add Supplier')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <Card>
            <CardContent className='p-0'>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Supplier')}</TableHead>
                    <TableHead>{t('Status')}</TableHead>
                    <TableHead>{t('Balance')}</TableHead>
                    <TableHead>{t('Groups / Models')}</TableHead>
                    <TableHead>{t('Channels')}</TableHead>
                    <TableHead>{t('Last Check')}</TableHead>
                    <TableHead className='text-right'>{t('Actions')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {suppliers.map((supplier) => {
                    const groupModels = parseGroupModels(
                      supplier.group_models_json
                    )
                    const modelCount = groupModels.reduce(
                      (sum, item) => sum + item.models.length,
                      0
                    )
                    return (
                      <TableRow key={supplier.id}>
                        <TableCell className='min-w-[220px]'>
                          <div className='space-y-1'>
                            <div className='font-medium'>{supplier.name}</div>
                            <div className='text-muted-foreground max-w-[320px] truncate text-xs'>
                              {supplier.base_url}
                            </div>
                            {supplier.last_error && (
                              <div className='text-destructive flex items-center gap-1 text-xs'>
                                <CircleAlert className='h-3 w-3' />
                                {supplier.last_error}
                              </div>
                            )}
                          </div>
                        </TableCell>
                        <TableCell>{statusBadge(supplier.status, t)}</TableCell>
                        <TableCell>
                          <div className='space-y-1'>
                            <div>{formatQuota(supplier.quota || 0)}</div>
                            <div className='text-muted-foreground text-xs'>
                              {t('Used')}: {formatQuota(supplier.used_quota || 0)}
                            </div>
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className='space-y-1'>
                            <div>
                              {groupModels.length} {t('Groups')} / {modelCount}{' '}
                              {t('Models')}
                            </div>
                            {groupModels.length > 0 && (
                              <div className='flex max-w-[320px] flex-wrap gap-1'>
                                {groupModels.slice(0, 3).map((group) => (
                                  <Badge
                                    key={group.group}
                                    variant='outline'
                                    className='font-normal'
                                  >
                                    {group.group}
                                    {group.ratio && (
                                      <span className='text-muted-foreground ml-1'>
                                        {t('Ratio')}: {group.ratio}
                                      </span>
                                    )}
                                  </Badge>
                                ))}
                                {groupModels.length > 3 && (
                                  <Badge
                                    variant='outline'
                                    className='font-normal'
                                  >
                                    +{groupModels.length - 3}
                                  </Badge>
                                )}
                              </div>
                            )}
                            <div className='text-muted-foreground text-xs'>
                              {supplier.model_source === 'pricing'
                                ? t('Model Square')
                                : supplier.model_source || t('Not checked')}
                            </div>
                          </div>
                        </TableCell>
                        <TableCell>{supplier.bound_channel_count || 0}</TableCell>
                        <TableCell>
                          {supplier.last_check_time
                            ? formatTimestamp(supplier.last_check_time)
                            : t('Never')}
                        </TableCell>
                        <TableCell>
                          <div className='flex justify-end gap-2'>
                            <Button
                              size='icon-sm'
                              variant='outline'
                              onClick={() => balanceMutation.mutate(supplier.id)}
                              disabled={balanceMutation.isPending}
                            >
                              <Coins className='h-4 w-4' />
                              <span className='sr-only'>
                                {t('Query Balance')}
                              </span>
                            </Button>
                            <Button
                              size='icon-sm'
                              variant='outline'
                              onClick={() => checkMutation.mutate(supplier.id)}
                              disabled={checkMutation.isPending}
                            >
                              <RefreshCw className='h-4 w-4' />
                              <span className='sr-only'>{t('Check')}</span>
                            </Button>
                            <Button
                              size='icon-sm'
                              variant='outline'
                              onClick={() => openConfigure(supplier)}
                              disabled={
                                supplier.model_source !== 'pricing' ||
                                groupModels.length === 0
                              }
                            >
                              <CheckCircle2 className='h-4 w-4' />
                              <span className='sr-only'>
                                {t('Configure Channels')}
                              </span>
                            </Button>
                            <Button
                              size='icon-sm'
                              variant='outline'
                              onClick={() => openEdit(supplier)}
                            >
                              <Edit className='h-4 w-4' />
                              <span className='sr-only'>{t('Edit')}</span>
                            </Button>
                            <Button
                              size='icon-sm'
                              variant='destructive'
                              onClick={() => deleteMutation.mutate(supplier.id)}
                              disabled={deleteMutation.isPending}
                            >
                              <Trash2 className='h-4 w-4' />
                              <span className='sr-only'>{t('Delete')}</span>
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    )
                  })}
                  {!isLoading && suppliers.length === 0 && (
                    <TableRow>
                      <TableCell
                        className='text-muted-foreground h-24 text-center'
                        colSpan={7}
                      >
                        {t('No Suppliers Found')}
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <Dialog open={formOpen} onOpenChange={setFormOpen}>
        <DialogContent className='sm:max-w-lg'>
          <DialogHeader>
            <DialogTitle>
              {editing ? t('Edit Supplier') : t('Add Supplier')}
            </DialogTitle>
            <DialogDescription>
              {t('Connect an upstream NewAPI account')}
            </DialogDescription>
          </DialogHeader>
          <div className='grid gap-4'>
            <Field label={t('Supplier Name')}>
              <Input
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
              />
            </Field>
            <Field label='Base URL'>
              <Input
                placeholder='https://upstream.example.com'
                value={form.base_url}
                onChange={(e) =>
                  setForm({ ...form, base_url: e.target.value })
                }
              />
            </Field>
            <Field label={t('Authentication Method')}>
              <Select
                value={form.auth_mode}
                onValueChange={(value) =>
                  setForm({
                    ...form,
                    auth_mode: value as SupplierForm['auth_mode'],
                  })
                }
              >
                <SelectTrigger className='w-full'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectItem value='password'>
                    {t('Username and password')}
                  </SelectItem>
                  <SelectItem value='access_token'>
                    {t('System access token')}
                  </SelectItem>
                </SelectContent>
              </Select>
            </Field>
            {form.auth_mode === 'password' ? (
              <div className='grid gap-4 sm:grid-cols-2'>
                <Field label={t('Username')}>
                  <Input
                    value={form.username}
                    onChange={(e) =>
                      setForm({ ...form, username: e.target.value })
                    }
                  />
                </Field>
                <Field label={t('Password')}>
                  <Input
                    type='password'
                    placeholder={
                      editing
                        ? t('Leave empty to keep existing password')
                        : undefined
                    }
                    value={form.password}
                    onChange={(e) =>
                      setForm({ ...form, password: e.target.value })
                    }
                  />
                </Field>
              </div>
            ) : (
              <div className='grid gap-4 sm:grid-cols-2'>
                <Field label={t('System access token')}>
                  <Input
                    type='password'
                    placeholder={
                      editing
                        ? t('Leave empty to keep existing token')
                        : t('Paste the upstream system access token')
                    }
                    value={form.access_token}
                    onChange={(e) =>
                      setForm({ ...form, access_token: e.target.value })
                    }
                  />
                </Field>
                <Field label={t('Upstream User ID')}>
                  <Input
                    inputMode='numeric'
                    placeholder={t('Enter upstream user ID')}
                    value={form.upstream_user_id}
                    onChange={(e) =>
                      setForm({ ...form, upstream_user_id: e.target.value })
                    }
                  />
                </Field>
              </div>
            )}
            <div className='grid gap-4 sm:grid-cols-2'>
              <Field label={t('Default Local Group')}>
                <Select
                  value={form.default_local_group}
                  onValueChange={(value) =>
                    value &&
                    setForm({ ...form, default_local_group: value })
                  }
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
              <Field label='Tag'>
                <Input
                  value={form.tag}
                  onChange={(e) => setForm({ ...form, tag: e.target.value })}
                />
              </Field>
            </div>
            <Field label={t('Remark')}>
              <Textarea
                value={form.remark}
                onChange={(e) => setForm({ ...form, remark: e.target.value })}
              />
            </Field>
          </div>
          <DialogFooter>
            <Button
              onClick={() => saveMutation.mutate()}
              disabled={saveMutation.isPending}
            >
              {t('Save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <SupplierChannelConfigDialog
        open={configOpen}
        supplier={configuring}
        onOpenChange={(open) => {
          setConfigOpen(open)
          if (!open) setConfiguring(null)
        }}
        onConfigured={() => queryClient.invalidateQueries({ queryKey })}
      />
    </>
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
