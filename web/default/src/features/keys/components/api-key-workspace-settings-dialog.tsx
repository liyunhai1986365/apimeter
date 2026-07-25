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
  AddTeamIcon,
  Alert02Icon,
  Delete02Icon,
  FloppyDiskIcon,
  Settings02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
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
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import {
  getWorkspaceSubaccounts,
  setWorkspaceAccess,
} from '@/features/workspace-subaccounts/api'
import { WorkspaceSubaccountFormDialog } from '@/features/workspace-subaccounts/components/workspace-subaccount-form-dialog'
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
  const queryClient = useQueryClient()
  const isWorkspaceSubaccount = useAuthStore(
    (state) => state.auth.user?.workspace_subaccount === true
  )
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
  const [isSavingAccess, setIsSavingAccess] = useState(false)
  const [accessUserIds, setAccessUserIds] = useState<number[]>([])
  const [subaccountDialogOpen, setSubaccountDialogOpen] = useState(false)
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false)

  useEffect(() => {
    if (open && selectedWorkspace) {
      setName(selectedWorkspace.name)
      setDescription(selectedWorkspace.description || '')
      setAccessUserIds(selectedWorkspace.access_users.map((user) => user.id))
    }
  }, [open, selectedWorkspace])

  const subaccountsQuery = useQuery({
    queryKey: ['workspace-subaccounts'],
    queryFn: getWorkspaceSubaccounts,
    enabled: open && !isWorkspaceSubaccount,
  })

  const subaccounts = subaccountsQuery.data?.data || []

  const hasAccessChanges = useMemo(() => {
    if (!selectedWorkspace) return false
    const currentIds = selectedWorkspace.access_users
      .map((user) => user.id)
      .sort((left, right) => left - right)
    const nextIds = [...accessUserIds].sort((left, right) => left - right)
    return currentIds.join(',') !== nextIds.join(',')
  }, [accessUserIds, selectedWorkspace])

  const toggleAccessUser = (userId: number, checked: boolean) => {
    setAccessUserIds((current) =>
      checked
        ? [...new Set([...current, userId])]
        : current.filter((id) => id !== userId)
    )
  }

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

  const handleSaveAccess = async () => {
    if (!selectedWorkspace || selectedWorkspace.is_default) return

    setIsSavingAccess(true)
    try {
      const result = await setWorkspaceAccess(
        selectedWorkspace.id,
        accessUserIds
      )
      if (!result.success) {
        toast.error(result.message || t('Failed to update workspace access'))
        return
      }
      await refreshWorkspaces(selectedWorkspace.id)
      toast.success(t('Workspace access updated successfully'))
    } catch {
      toast.error(t('Failed to update workspace access'))
    } finally {
      setIsSavingAccess(false)
    }
  }

  const handleCreateSubaccount = () => {
    onOpenChange(false)
    setSubaccountDialogOpen(true)
  }

  const handleSubaccountSaved = () => {
    void queryClient.invalidateQueries({ queryKey: ['workspace-subaccounts'] })
    void refreshWorkspaces(selectedWorkspace?.id)
    onOpenChange(true)
  }

  if (isWorkspaceSubaccount) return null

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-[680px]'>
          <DialogHeader>
            <DialogTitle className='flex items-center gap-2'>
              <HugeiconsIcon icon={Settings02Icon} />
              {t('Workspace settings')}
            </DialogTitle>
            <DialogDescription>
              {t(
                'Update the current workspace profile, access, quota rule, and lifecycle.'
              )}
            </DialogDescription>
          </DialogHeader>

          {selectedWorkspace ? (
            <div className='flex flex-col gap-6'>
              <section className='flex flex-col gap-3'>
                <div>
                  <h3 className='text-sm font-semibold'>
                    {t('Workspace profile')}
                  </h3>
                  <p className='text-muted-foreground mt-1 text-xs'>
                    {t('These details are shown on the API key workspace bar.')}
                  </p>
                </div>

                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor='workspace-settings-name'>
                      {t('Name')}
                    </FieldLabel>
                    <Input
                      id='workspace-settings-name'
                      value={name}
                      onChange={(event) => setName(event.target.value)}
                      placeholder={t('Workspace name')}
                      maxLength={64}
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor='workspace-settings-description'>
                      {t('Description')}
                    </FieldLabel>
                    <Textarea
                      id='workspace-settings-description'
                      value={description}
                      onChange={(event) => setDescription(event.target.value)}
                      placeholder={t('Optional description')}
                      maxLength={255}
                      className='min-h-20'
                    />
                  </Field>
                </FieldGroup>

                <div className='flex justify-end'>
                  <Button
                    type='button'
                    disabled={!hasChanges || isSaving}
                    onClick={handleSave}
                  >
                    <HugeiconsIcon
                      icon={FloppyDiskIcon}
                      data-icon='inline-start'
                    />
                    {isSaving ? t('Saving') : t('Save workspace')}
                  </Button>
                </div>
              </section>

              <Separator />

              <section className='flex flex-col gap-3'>
                <div>
                  <h3 className='text-sm font-semibold'>
                    {t('Workspace access')}
                  </h3>
                  <p className='text-muted-foreground mt-1 text-xs'>
                    {t(
                      'Add or remove subaccounts that can access this workspace and its API keys.'
                    )}
                  </p>
                </div>
                {selectedWorkspace.is_default ? (
                  <Alert>
                    <AlertTitle>
                      {t(
                        'Default workspace is only accessible to the main account'
                      )}
                    </AlertTitle>
                    <AlertDescription>
                      {t(
                        'Move API keys to another workspace before granting access.'
                      )}
                    </AlertDescription>
                  </Alert>
                ) : (
                  <div className='flex flex-col gap-3'>
                    <FieldSet>
                      <FieldLegend variant='label'>
                        {t('Workspace members')}
                      </FieldLegend>
                      <FieldDescription>
                        {t('{{count}} subaccounts selected', {
                          count: accessUserIds.length,
                        })}
                      </FieldDescription>
                      <FieldGroup
                        data-slot='checkbox-group'
                        className='max-h-56 gap-3 overflow-y-auto pr-1'
                      >
                        {subaccountsQuery.isLoading ? (
                          <FieldDescription className='flex items-center gap-2'>
                            <Spinner />
                            {t('Loading...')}
                          </FieldDescription>
                        ) : subaccounts.length > 0 ? (
                          subaccounts.map((account) => {
                            const isSelected = accessUserIds.includes(
                              account.id
                            )
                            const wasAssigned =
                              selectedWorkspace.access_users.some(
                                (user) => user.id === account.id
                              )
                            const isDisabled =
                              account.status !== 1 && !wasAssigned
                            return (
                              <Field
                                key={account.id}
                                orientation='horizontal'
                                data-disabled={isDisabled || undefined}
                              >
                                <Checkbox
                                  id={`workspace-member-${account.id}`}
                                  checked={isSelected}
                                  onCheckedChange={(checked) =>
                                    toggleAccessUser(
                                      account.id,
                                      checked === true
                                    )
                                  }
                                  disabled={isSavingAccess || isDisabled}
                                />
                                <FieldLabel
                                  htmlFor={`workspace-member-${account.id}`}
                                  className='min-w-0 flex-col items-start gap-0.5 font-normal'
                                >
                                  <span className='truncate'>
                                    {account.display_name || account.username}
                                  </span>
                                  <span className='text-muted-foreground truncate text-xs'>
                                    @{account.username}
                                    {account.status !== 1
                                      ? ` (${t('Disabled')})`
                                      : ''}
                                  </span>
                                </FieldLabel>
                              </Field>
                            )
                          })
                        ) : (
                          <FieldDescription>
                            {t('No subaccounts yet.')}
                          </FieldDescription>
                        )}
                      </FieldGroup>
                    </FieldSet>
                    <div className='flex flex-col gap-2 sm:flex-row sm:justify-between'>
                      <Button
                        type='button'
                        variant='outline'
                        onClick={handleCreateSubaccount}
                      >
                        <HugeiconsIcon
                          icon={AddTeamIcon}
                          data-icon='inline-start'
                        />
                        {t('Add subaccount')}
                      </Button>
                      <Button
                        type='button'
                        disabled={!hasAccessChanges || isSavingAccess}
                        onClick={handleSaveAccess}
                      >
                        {isSavingAccess && <Spinner data-icon='inline-start' />}
                        {t('Save members')}
                      </Button>
                    </div>
                  </div>
                )}
              </section>

              <Separator />

              <section className='flex flex-col gap-3'>
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

              <section className='flex flex-col gap-3'>
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
                  <HugeiconsIcon icon={Alert02Icon} />
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
                  <HugeiconsIcon icon={Delete02Icon} data-icon='inline-start' />
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

      <WorkspaceSubaccountFormDialog
        open={subaccountDialogOpen}
        onOpenChange={setSubaccountDialogOpen}
        account={null}
        workspaces={workspaces}
        fixedWorkspace={selectedWorkspace}
        onSaved={handleSubaccountSaved}
      />
    </>
  )
}
