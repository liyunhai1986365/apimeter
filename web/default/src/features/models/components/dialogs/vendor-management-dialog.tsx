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
import { useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowDown,
  ArrowUp,
  Loader2,
  Pencil,
  Plus,
  Save,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { getLobeIcon } from '@/lib/lobe-icon'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ScrollArea } from '@/components/ui/scroll-area'
import { StatusBadge } from '@/components/status-badge'
import { getVendors, updateVendorSortOrder } from '../../api'
import { modelsQueryKeys, vendorsQueryKeys } from '../../lib'
import type { Vendor } from '../../types'
import { useModels } from '../models-provider'

type VendorManagementDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

function reorder(items: Vendor[], fromIndex: number, toIndex: number) {
  const next = [...items]
  const [item] = next.splice(fromIndex, 1)
  if (!item) return items
  next.splice(toIndex, 0, item)
  return next.map((vendor, index) => ({
    ...vendor,
    sort_order: (index + 1) * 10,
  }))
}

export function VendorManagementDialog(props: VendorManagementDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { setOpen, setCurrentVendor } = useModels()
  const [vendors, setVendors] = useState<Vendor[]>([])
  const [isSaving, setIsSaving] = useState(false)

  const vendorsQuery = useQuery({
    queryKey: vendorsQueryKeys.list({ page_size: 1000 }),
    queryFn: () => getVendors({ page_size: 1000 }),
    enabled: props.open,
  })

  const remoteVendors = useMemo(
    () => vendorsQuery.data?.data?.items ?? [],
    [vendorsQuery.data]
  )

  useEffect(() => {
    if (props.open) {
      setVendors(remoteVendors)
    }
  }, [props.open, remoteVendors])

  const hasChanges = useMemo(() => {
    if (vendors.length !== remoteVendors.length) return true
    return vendors.some(
      (vendor, index) =>
        vendor.id !== remoteVendors[index]?.id ||
        (vendor.sort_order || 0) !== (remoteVendors[index]?.sort_order || 0)
    )
  }, [remoteVendors, vendors])

  const moveVendor = (index: number, direction: -1 | 1) => {
    const target = index + direction
    if (target < 0 || target >= vendors.length) return
    setVendors((current) => reorder(current, index, target))
  }

  const handleCreate = () => {
    setCurrentVendor(null)
    setOpen('create-vendor')
  }

  const handleEdit = (vendor: Vendor) => {
    setCurrentVendor(vendor)
    setOpen('update-vendor')
  }

  const handleSave = async () => {
    setIsSaving(true)
    try {
      const response = await updateVendorSortOrder(
        vendors.map((vendor, index) => ({
          id: vendor.id,
          sort_order: vendor.sort_order || (index + 1) * 10,
        }))
      )
      if (!response.success) {
        toast.error(response.message || 'Operation failed')
        return
      }
      toast.success(t('Vendor order saved'))
      queryClient.invalidateQueries({ queryKey: vendorsQueryKeys.lists() })
      queryClient.invalidateQueries({ queryKey: modelsQueryKeys.lists() })
      queryClient.invalidateQueries({ queryKey: ['pricing'] })
    } catch (error: unknown) {
      toast.error((error as Error)?.message || 'Operation failed')
    } finally {
      setIsSaving(false)
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[86vh] sm:max-w-3xl'>
        <DialogHeader>
          <DialogTitle>{t('Manage Vendors')}</DialogTitle>
          <DialogDescription>
            {t(
              'Control provider information and the order used by model management and the model square.'
            )}
          </DialogDescription>
        </DialogHeader>

        <div className='flex items-center justify-between gap-3'>
          <div className='text-muted-foreground text-xs'>
            {t('{{count}} vendors', { count: vendors.length })}
          </div>
          <Button type='button' size='sm' onClick={handleCreate}>
            <Plus className='size-4' />
            {t('Create Vendor')}
          </Button>
        </div>

        <ScrollArea className='max-h-[54vh] rounded-lg border'>
          <div className='divide-y'>
            {vendorsQuery.isLoading ? (
              <div className='text-muted-foreground flex h-32 items-center justify-center text-sm'>
                <Loader2 className='mr-2 size-4 animate-spin' />
                {t('Loading...')}
              </div>
            ) : vendors.length === 0 ? (
              <div className='text-muted-foreground flex h-32 items-center justify-center text-sm'>
                {t('No vendors found')}
              </div>
            ) : (
              vendors.map((vendor, index) => {
                const icon = vendor.icon ? getLobeIcon(vendor.icon, 28) : null
                return (
                  <div
                    key={vendor.id}
                    className='grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3 p-3'
                  >
                    <div className='bg-muted/60 flex size-10 items-center justify-center rounded-lg'>
                      {icon || (
                        <span className='text-muted-foreground text-sm font-semibold'>
                          {vendor.name.charAt(0).toUpperCase()}
                        </span>
                      )}
                    </div>
                    <div className='min-w-0'>
                      <div className='flex min-w-0 flex-wrap items-center gap-2'>
                        <span className='truncate text-sm font-semibold'>
                          {vendor.name}
                        </span>
                        <StatusBadge
                          label={`${t('Sort')} ${vendor.sort_order || 0}`}
                          variant='neutral'
                          size='sm'
                          copyable={false}
                        />
                      </div>
                      <p className='text-muted-foreground mt-1 line-clamp-1 text-xs'>
                        {vendor.description || t('No description available.')}
                      </p>
                    </div>
                    <div className='flex items-center gap-1'>
                      <Button
                        type='button'
                        variant='ghost'
                        size='icon-sm'
                        disabled={index === 0}
                        onClick={() => moveVendor(index, -1)}
                        title={t('Move up')}
                      >
                        <ArrowUp className='size-4' />
                      </Button>
                      <Button
                        type='button'
                        variant='ghost'
                        size='icon-sm'
                        disabled={index === vendors.length - 1}
                        onClick={() => moveVendor(index, 1)}
                        title={t('Move down')}
                      >
                        <ArrowDown className='size-4' />
                      </Button>
                      <Button
                        type='button'
                        variant='ghost'
                        size='icon-sm'
                        onClick={() => handleEdit(vendor)}
                        title={t('Edit')}
                      >
                        <Pencil className='size-4' />
                      </Button>
                    </div>
                  </div>
                )
              })
            )}
          </div>
        </ScrollArea>

        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            onClick={() => props.onOpenChange(false)}
          >
            {t('Close')}
          </Button>
          <Button
            type='button'
            onClick={handleSave}
            disabled={!hasChanges || isSaving}
          >
            {isSaving ? (
              <Loader2 className='size-4 animate-spin' />
            ) : (
              <Save className='size-4' />
            )}
            {t('Save order')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
