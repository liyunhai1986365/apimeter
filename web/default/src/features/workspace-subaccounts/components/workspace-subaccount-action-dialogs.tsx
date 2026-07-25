import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
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
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Spinner } from '@/components/ui/spinner'
import { PasswordInput } from '@/components/password-input'
import {
  deleteWorkspaceSubaccount,
  resetWorkspaceSubaccountPassword,
} from '../api'
import type { WorkspaceSubaccountSummary } from '../types'

export type WorkspaceSubaccountAction = 'reset-password' | 'delete' | null

interface WorkspaceSubaccountActionDialogsProps {
  action: WorkspaceSubaccountAction
  account: WorkspaceSubaccountSummary | null
  onClose: () => void
  onSaved: () => void
}

export function WorkspaceSubaccountActionDialogs({
  action,
  account,
  onClose,
  onSaved,
}: WorkspaceSubaccountActionDialogsProps) {
  const { t } = useTranslation()
  const [password, setPassword] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  useEffect(() => {
    if (action === 'reset-password') setPassword('')
  }, [action, account?.id])

  const handleResetPassword = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!account) return
    if (password.length < 8 || password.length > 20) {
      toast.error(t('Password must be between 8 and 20 characters'))
      return
    }

    setIsSubmitting(true)
    try {
      const result = await resetWorkspaceSubaccountPassword(
        account.id,
        password
      )
      if (!result.success) {
        toast.error(result.message || t('Failed to reset password'))
        return
      }
      toast.success(t('Temporary password reset successfully'))
      onClose()
      onSaved()
    } catch {
      toast.error(t('Failed to reset password'))
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleDelete = async () => {
    if (!account) return
    setIsSubmitting(true)
    try {
      const result = await deleteWorkspaceSubaccount(account.id)
      if (!result.success) {
        toast.error(result.message || t('Failed to delete workspace account'))
        return
      }
      toast.success(t('Workspace account deleted successfully'))
      onClose()
      onSaved()
    } catch {
      toast.error(t('Failed to delete workspace account'))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <>
      <Dialog
        open={action === 'reset-password'}
        onOpenChange={(open) => !open && onClose()}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('Reset temporary password')}</DialogTitle>
            <DialogDescription>
              {t('Set a temporary password for {{username}}.', {
                username: account?.username || '',
              })}
            </DialogDescription>
          </DialogHeader>
          <form
            id='workspace-account-reset-form'
            onSubmit={handleResetPassword}
          >
            <Field>
              <FieldLabel htmlFor='workspace-account-reset-password'>
                {t('Temporary password')}
              </FieldLabel>
              <PasswordInput
                id='workspace-account-reset-password'
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
                  'The account will be required to change it at the next sign-in.'
                )}
              </FieldDescription>
            </Field>
          </form>
          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              onClick={onClose}
              disabled={isSubmitting}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='submit'
              form='workspace-account-reset-form'
              disabled={isSubmitting}
            >
              {isSubmitting && <Spinner data-icon='inline-start' />}
              {t('Reset password')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={action === 'delete'}
        onOpenChange={(open) => !open && onClose()}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('Delete workspace account?')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'This removes {{username}} and revokes its workspace access. API keys remain owned by the main account. This action cannot be undone.',
                { username: account?.username || '' }
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isSubmitting}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              variant='destructive'
              onClick={handleDelete}
              disabled={isSubmitting}
            >
              {isSubmitting && <Spinner data-icon='inline-start' />}
              {t('Delete account')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
