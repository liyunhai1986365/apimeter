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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { createWorkspace } from '../api'
import { ERROR_MESSAGES } from '../constants'
import { useApiKeys } from './api-keys-provider'

type ApiKeyWorkspaceCreateDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ApiKeyWorkspaceCreateDialog({
  open,
  onOpenChange,
}: ApiKeyWorkspaceCreateDialogProps) {
  const { t } = useTranslation()
  const { refreshWorkspaces, setSelectedWorkspaceId, triggerRefresh } =
    useApiKeys()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  useEffect(() => {
    if (!open) {
      setName('')
      setDescription('')
    }
  }, [open])

  const handleSubmit = async () => {
    const trimmedName = name.trim()
    if (!trimmedName) {
      toast.error(t('Please enter a workspace name'))
      return
    }
    setIsSubmitting(true)
    try {
      const res = await createWorkspace({
        name: trimmedName,
        description: description.trim(),
      })
      if (res.success && res.data) {
        setSelectedWorkspaceId(res.data.id)
        await refreshWorkspaces(res.data.id)
        triggerRefresh()
        toast.success(t('Workspace created successfully'))
        onOpenChange(false)
      } else {
        toast.error(res.message || t('Failed to create workspace'))
      }
    } catch {
      toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('Create Workspace')}</DialogTitle>
          <DialogDescription>
            {t(
              'Create a workspace to organize API keys by project or purpose.'
            )}
          </DialogDescription>
        </DialogHeader>
        <div className='space-y-3'>
          <div className='space-y-1.5'>
            <label className='text-sm font-medium'>{t('Name')}</label>
            <Input
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder={t('Workspace name')}
              maxLength={64}
            />
          </div>
          <div className='space-y-1.5'>
            <label className='text-sm font-medium'>{t('Description')}</label>
            <Textarea
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              placeholder={t('Optional description')}
              maxLength={255}
              className='min-h-20'
            />
          </div>
        </div>
        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            disabled={isSubmitting}
            onClick={() => onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button type='button' disabled={isSubmitting} onClick={handleSubmit}>
            {isSubmitting ? t('Creating...') : t('Create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
