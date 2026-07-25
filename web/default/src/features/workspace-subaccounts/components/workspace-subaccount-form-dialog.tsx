import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
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
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { PasswordInput } from '@/components/password-input'
import type { Workspace } from '@/features/keys/types'
import { createWorkspaceSubaccount, updateWorkspaceSubaccount } from '../api'
import type { WorkspaceSubaccountSummary } from '../types'

interface WorkspaceSubaccountFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  account: WorkspaceSubaccountSummary | null
  workspaces: Workspace[]
  fixedWorkspace?: Workspace | null
  onSaved: () => void
}

export function WorkspaceSubaccountFormDialog({
  open,
  onOpenChange,
  account,
  workspaces,
  fixedWorkspace = null,
  onSaved,
}: WorkspaceSubaccountFormDialogProps) {
  const { t } = useTranslation()
  const isEditing = account !== null
  const [username, setUsername] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [workspaceIds, setWorkspaceIds] = useState<number[]>([])
  const [isSubmitting, setIsSubmitting] = useState(false)

  useEffect(() => {
    if (!open) return
    setUsername(account?.username || '')
    setDisplayName(account?.display_name || '')
    setEmail(account?.email || '')
    setPassword('')
    setWorkspaceIds(fixedWorkspace ? [fixedWorkspace.id] : [])
  }, [account, fixedWorkspace, open])

  const assignableWorkspaces = workspaces.filter(
    (workspace) => !workspace.is_default
  )

  const toggleWorkspace = (workspaceId: number, checked: boolean) => {
    setWorkspaceIds((current) =>
      checked
        ? [...new Set([...current, workspaceId])]
        : current.filter((id) => id !== workspaceId)
    )
  }

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault()
    const trimmedUsername = username.trim()
    const trimmedDisplayName = displayName.trim()

    if (!trimmedUsername) {
      toast.error(t('Please enter a username'))
      return
    }
    if (!trimmedDisplayName) {
      toast.error(t('Please enter a display name'))
      return
    }
    if (!isEditing && (password.length < 8 || password.length > 20)) {
      toast.error(t('Password must be between 8 and 20 characters'))
      return
    }

    setIsSubmitting(true)
    try {
      const result = isEditing
        ? await updateWorkspaceSubaccount(account.id, {
            display_name: trimmedDisplayName,
            email: email.trim(),
          })
        : await createWorkspaceSubaccount({
            username: trimmedUsername,
            display_name: trimmedDisplayName,
            email: email.trim(),
            password,
            workspace_ids: workspaceIds,
          })

      if (!result.success) {
        toast.error(
          result.message ||
            t(
              isEditing
                ? 'Failed to update workspace account'
                : 'Failed to create workspace account'
            )
        )
        return
      }

      toast.success(
        t(
          isEditing
            ? 'Workspace account updated successfully'
            : 'Workspace account created successfully'
        )
      )
      onOpenChange(false)
      onSaved()
    } catch {
      toast.error(
        t(
          isEditing
            ? 'Failed to update workspace account'
            : 'Failed to create workspace account'
        )
      )
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>
            {t(
              isEditing ? 'Edit workspace account' : 'Create workspace account'
            )}
          </DialogTitle>
          <DialogDescription>
            {t(
              isEditing
                ? 'Update the account profile. Workspace access is granted from each workspace.'
                : 'Create a limited account and optionally grant access to available workspaces.'
            )}
          </DialogDescription>
        </DialogHeader>

        <form id='workspace-subaccount-form' onSubmit={handleSubmit}>
          <FieldGroup>
            <Field data-disabled={isEditing || undefined}>
              <FieldLabel htmlFor='workspace-account-username'>
                {t('Username')}
              </FieldLabel>
              <Input
                id='workspace-account-username'
                value={username}
                onChange={(event) => setUsername(event.target.value)}
                disabled={isEditing || isSubmitting}
                maxLength={20}
                autoComplete='off'
                required
              />
            </Field>

            <Field>
              <FieldLabel htmlFor='workspace-account-display-name'>
                {t('Display Name')}
              </FieldLabel>
              <Input
                id='workspace-account-display-name'
                value={displayName}
                onChange={(event) => setDisplayName(event.target.value)}
                disabled={isSubmitting}
                maxLength={20}
                required
              />
            </Field>

            <Field>
              <FieldLabel htmlFor='workspace-account-email'>
                {t('Email')}
              </FieldLabel>
              <Input
                id='workspace-account-email'
                type='email'
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                disabled={isSubmitting}
                autoComplete='off'
              />
            </Field>

            {!isEditing && (
              <>
                <Field>
                  <FieldLabel htmlFor='workspace-account-password'>
                    {t('Temporary password')}
                  </FieldLabel>
                  <PasswordInput
                    id='workspace-account-password'
                    value={password}
                    onChange={(event) => setPassword(event.target.value)}
                    disabled={isSubmitting}
                    minLength={8}
                    maxLength={20}
                    autoComplete='new-password'
                    required
                  />
                  <FieldDescription>
                    {t(
                      'The account must change this password after the first sign-in.'
                    )}
                  </FieldDescription>
                </Field>

                <FieldSet>
                  <FieldLegend variant='label'>
                    {t('Initial workspaces')}
                  </FieldLegend>
                  <FieldDescription>
                    {fixedWorkspace
                      ? t('This subaccount will be added to {{workspace}}.', {
                          workspace: fixedWorkspace.name,
                        })
                      : t(
                          'You can change workspace access later in workspace settings.'
                        )}
                  </FieldDescription>
                  {!fixedWorkspace && (
                    <FieldGroup data-slot='checkbox-group' className='gap-3'>
                      {assignableWorkspaces.length > 0 ? (
                        assignableWorkspaces.map((workspace) => (
                          <Field key={workspace.id} orientation='horizontal'>
                            <Checkbox
                              id={`workspace-account-workspace-${workspace.id}`}
                              checked={workspaceIds.includes(workspace.id)}
                              onCheckedChange={(checked) =>
                                toggleWorkspace(workspace.id, checked === true)
                              }
                              disabled={isSubmitting}
                            />
                            <FieldLabel
                              htmlFor={`workspace-account-workspace-${workspace.id}`}
                              className='font-normal'
                            >
                              {workspace.name}
                            </FieldLabel>
                          </Field>
                        ))
                      ) : (
                        <FieldDescription>
                          {t('No workspace is available.')}
                        </FieldDescription>
                      )}
                    </FieldGroup>
                  )}
                </FieldSet>
              </>
            )}
          </FieldGroup>
        </form>

        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={isSubmitting}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='submit'
            form='workspace-subaccount-form'
            disabled={isSubmitting}
          >
            {isSubmitting && <Spinner data-icon='inline-start' />}
            {t(isEditing ? 'Save changes' : 'Create account')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
