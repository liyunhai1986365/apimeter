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
import { AlertTriangle, Save, Settings2, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import { Textarea } from '@/components/ui/textarea'
import { deleteWorkspace, updateWorkspace } from '../api'
import { ERROR_MESSAGES } from '../constants'
import {
  canDeleteWorkspace,
  getWorkspaceAfterDelete,
  normalizeWorkspaceSettingsForm,
} from '../lib/workspace-settings'
import { useApiKeys } from './api-keys-provider'
import { WorkspaceQuotaManagement } from './workspace-quota-management'

type ApiKeyWorkspaceSettingsDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ApiKeyWorkspaceSettingsDialog({
  open,
  onOpenChange,
}: ApiKeyWorkspaceSettingsDialogProps) {
  const { t } = useTranslation()
  const {
    selectedWorkspace,
    workspaces,
    refreshWorkspaces,
    triggerRefresh,
    setSelectedWorkspaceId,
  } = useApiKeys()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [isSaving, setIsSaving] = useState(false)
  const [isDeleting, setIsDeleting] = useState(false)
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false)

  useEffect(() => {
    if (open && selectedWorkspace) {
      setName(selectedWorkspace.name)
      setDescription(selectedWorkspace.description || '')
    }
  }, [open, selectedWorkspace])

  const normalizedForm = useMemo(() => {
    try {
      return normalizeWorkspaceSettingsForm({ name, description })
    } catch {
      return null
    }
  }, [description, name])

  const hasChanges =
    !!normalizedForm &&
    !!selectedWorkspace &&
    (normalizedForm.name !== selectedWorkspace.name ||
      normalizedForm.description !== (selectedWorkspace.description || ''))

  const deleteAllowed = canDeleteWorkspace(selectedWorkspace)

  const handleSave = async () => {
    if (!selectedWorkspace) return

    let form
    try {
      form = normalizeWorkspaceSettingsForm({ name, description })
    } catch {
      toast.error(t('Please enter a workspace name'))
      return
    }

    setIsSaving(true)
    try {
      const res = await updateWorkspace({
        id: selectedWorkspace.id,
        name: form.name,
        description: form.description,
      })
      if (!res.success || !res.data) {
        toast.error(res.message || t('Failed to update workspace'))
        return
      }
      setSelectedWorkspaceId(res.data.id)
      await refreshWorkspaces(res.data.id)
      triggerRefresh()
      toast.success(t('Workspace updated successfully'))
    } catch {
      toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      setIsSaving(false)
    }
  }

  const handleDelete = async () => {
    if (!selectedWorkspace || !deleteAllowed) return

    const nextWorkspaceId = getWorkspaceAfterDelete(
      workspaces,
      selectedWorkspace.id
    )

    setIsDeleting(true)
    try {
      const res = await deleteWorkspace(selectedWorkspace.id)
      if (!res.success) {
        toast.error(res.message || t('Failed to delete workspace'))
        return
      }
      setSelectedWorkspaceId(nextWorkspaceId)
      await refreshWorkspaces(nextWorkspaceId)
      triggerRefresh()
      toast.success(t('Workspace deleted successfully'))
      setDeleteConfirmOpen(false)
      onOpenChange(false)
    } catch {
      toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      setIsDeleting(false)
    }
  }

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-[680px]'>
          <DialogHeader>
            <DialogTitle className='flex items-center gap-2'>
              <Settings2 className='size-4' />
              {t('Workspace settings')}
            </DialogTitle>
            <DialogDescription>
              {t(
                'Update the current workspace profile, quota rule, and lifecycle.'
              )}
            </DialogDescription>
          </DialogHeader>

          {selectedWorkspace ? (
            <div className='space-y-6'>
              <section className='space-y-3'>
                <div>
                  <h3 className='text-sm font-semibold'>
                    {t('Workspace profile')}
                  </h3>
                  <p className='text-muted-foreground mt-1 text-xs'>
                    {t('These details are shown on the API key workspace bar.')}
                  </p>
                </div>

                <div className='grid gap-3 sm:grid-cols-2'>
                  <div className='space-y-2'>
                    <Label htmlFor='workspace-settings-name'>{t('Name')}</Label>
                    <Input
                      id='workspace-settings-name'
                      value={name}
                      onChange={(event) => setName(event.target.value)}
                      placeholder={t('Workspace name')}
                      maxLength={64}
                    />
                  </div>
                  <div className='space-y-2 sm:col-span-2'>
                    <Label htmlFor='workspace-settings-description'>
                      {t('Description')}
                    </Label>
                    <Textarea
                      id='workspace-settings-description'
                      value={description}
                      onChange={(event) => setDescription(event.target.value)}
                      placeholder={t('Optional description')}
                      maxLength={255}
                      className='min-h-20'
                    />
                  </div>
                </div>

                <div className='flex justify-end'>
                  <Button
                    type='button'
                    disabled={!hasChanges || isSaving}
                    onClick={handleSave}
                  >
                    <Save className='size-4' />
                    {isSaving ? t('Saving') : t('Save workspace')}
                  </Button>
                </div>
              </section>

              <Separator />

              <section className='space-y-3'>
                <div>
                  <h3 className='text-sm font-semibold'>
                    {t('Quota management')}
                  </h3>
                  <p className='text-muted-foreground mt-1 text-xs'>
                    {t(
                      'Set a periodic quota reset for all API keys in this workspace.'
                    )}
                  </p>
                </div>
                <WorkspaceQuotaManagement
                  workspaceId={selectedWorkspace.id}
                  embedded
                />
              </section>

              <Separator />

              <section className='space-y-3'>
                <div>
                  <h3 className='text-sm font-semibold'>
                    {t('Delete workspace')}
                  </h3>
                  <p className='text-muted-foreground mt-1 text-xs'>
                    {t(
                      'Deleting a workspace moves its API keys back to the default workspace.'
                    )}
                  </p>
                </div>
                <Alert variant={deleteAllowed ? 'default' : 'destructive'}>
                  <AlertTriangle className='size-4' />
                  <AlertTitle>
                    {deleteAllowed
                      ? t('This action cannot be undone.')
                      : t('Default workspace cannot be deleted')}
                  </AlertTitle>
                  <AlertDescription>
                    {deleteAllowed
                      ? t(
                          'API keys in this workspace will remain available after they are moved.'
                        )
                      : t(
                          'Create or select another workspace if you need to delete it.'
                        )}
                  </AlertDescription>
                </Alert>
                <Button
                  type='button'
                  variant='destructive'
                  disabled={!deleteAllowed}
                  onClick={() => setDeleteConfirmOpen(true)}
                >
                  <Trash2 className='size-4' />
                  {t('Delete current workspace')}
                </Button>
              </section>
            </div>
          ) : (
            <Alert>
              <AlertTitle>{t('No workspace selected')}</AlertTitle>
              <AlertDescription>
                {t('Select a workspace before opening settings.')}
              </AlertDescription>
            </Alert>
          )}
        </DialogContent>
      </Dialog>

      <AlertDialog open={deleteConfirmOpen} onOpenChange={setDeleteConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('Delete current workspace?')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t('This will delete workspace')}{' '}
              <span className='font-semibold'>{selectedWorkspace?.name}</span>
              {t('. API keys will be moved to the default workspace.')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isDeleting}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              disabled={isDeleting}
              className='bg-destructive text-destructive-foreground hover:bg-destructive/90'
            >
              {isDeleting ? t('Deleting...') : t('Delete workspace')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
