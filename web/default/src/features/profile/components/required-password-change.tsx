import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { Key01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { getSelf } from '@/lib/api'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Spinner } from '@/components/ui/spinner'
import { PasswordInput } from '@/components/password-input'
import { updateUserProfile } from '../api'

export function RequiredPasswordChange() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { auth } = useAuthStore()
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault()

    if (!currentPassword) {
      toast.error(t('Please enter your current password'))
      return
    }
    if (newPassword.length < 8 || newPassword.length > 20) {
      toast.error(t('Password must be between 8 and 20 characters'))
      return
    }
    if (newPassword === currentPassword) {
      toast.error(t('New password must be different from current password'))
      return
    }
    if (newPassword !== confirmPassword) {
      toast.error(t('Passwords do not match'))
      return
    }

    setIsSubmitting(true)
    try {
      const result = await updateUserProfile({
        original_password: currentPassword,
        password: newPassword,
      })
      if (!result.success) {
        toast.error(result.message || t('Failed to change password'))
        return
      }

      const self = await getSelf()
      if (self?.success && self.data) {
        auth.setUser(self.data)
      } else if (auth.user) {
        auth.setUser({ ...auth.user, must_change_password: false })
      }

      toast.success(t('Password changed successfully'))
      await navigate({ to: '/keys', replace: true })
    } catch {
      toast.error(t('Failed to change password'))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <main className='flex min-h-full w-full items-center justify-center p-4 sm:p-6'>
      <Card className='w-full max-w-md'>
        <CardHeader>
          <div className='bg-muted text-muted-foreground mb-2 flex size-9 items-center justify-center rounded-md border'>
            <HugeiconsIcon icon={Key01Icon} />
          </div>
          <CardTitle>{t('Change your temporary password')}</CardTitle>
          <CardDescription>
            {t('Set a new password before accessing your assigned workspaces.')}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form id='required-password-change-form' onSubmit={handleSubmit}>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor='required-current-password'>
                  {t('Current Password')}
                </FieldLabel>
                <PasswordInput
                  id='required-current-password'
                  value={currentPassword}
                  onChange={(event) => setCurrentPassword(event.target.value)}
                  autoComplete='current-password'
                  disabled={isSubmitting}
                  required
                />
              </Field>
              <Field>
                <FieldLabel htmlFor='required-new-password'>
                  {t('New Password')}
                </FieldLabel>
                <PasswordInput
                  id='required-new-password'
                  value={newPassword}
                  onChange={(event) => setNewPassword(event.target.value)}
                  autoComplete='new-password'
                  disabled={isSubmitting}
                  minLength={8}
                  maxLength={20}
                  required
                />
                <FieldDescription>
                  {t('Password must be between 8 and 20 characters')}
                </FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor='required-confirm-password'>
                  {t('Confirm New Password')}
                </FieldLabel>
                <PasswordInput
                  id='required-confirm-password'
                  value={confirmPassword}
                  onChange={(event) => setConfirmPassword(event.target.value)}
                  autoComplete='new-password'
                  disabled={isSubmitting}
                  required
                />
              </Field>
            </FieldGroup>
          </form>
        </CardContent>
        <CardFooter className='justify-end'>
          <Button
            type='submit'
            form='required-password-change-form'
            disabled={isSubmitting}
          >
            {isSubmitting && <Spinner data-icon='inline-start' />}
            {isSubmitting ? t('Changing...') : t('Change Password')}
          </Button>
        </CardFooter>
      </Card>
    </main>
  )
}
