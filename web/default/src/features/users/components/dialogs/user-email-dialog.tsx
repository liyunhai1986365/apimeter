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
import { Send } from 'lucide-react'
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
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { sendUserEmail } from '../../api'
import { ERROR_MESSAGES } from '../../constants'
import { type User } from '../../types'

interface UserEmailDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  user: User
}

export function UserEmailDialog({
  open,
  onOpenChange,
  user,
}: UserEmailDialogProps) {
  const { t } = useTranslation()
  const [subject, setSubject] = useState('')
  const [content, setContent] = useState('')
  const [isSending, setIsSending] = useState(false)

  useEffect(() => {
    if (!open) {
      setSubject('')
      setContent('')
    }
  }, [open])

  const handleSubmit = async () => {
    const trimmedSubject = subject.trim()
    const trimmedContent = content.trim()
    if (!trimmedSubject || !trimmedContent) {
      toast.error(t('Please enter email subject and content'))
      return
    }

    setIsSending(true)
    try {
      const result = await sendUserEmail(user.id, {
        subject: trimmedSubject,
        content: trimmedContent,
      })
      if (result.success) {
        toast.success(t('Email sent successfully'))
        onOpenChange(false)
      } else {
        toast.error(result.message || t('Failed to send email'))
      }
    } catch (_error) {
      toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      setIsSending(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle className='flex items-center gap-2'>
            <Send size={18} />
            {t('Send Email')}
          </DialogTitle>
          <DialogDescription>
            {t(
              'Send a custom email to {{email}} using the system SMTP settings.',
              {
                email: user.email || user.username,
              }
            )}
          </DialogDescription>
        </DialogHeader>

        <FieldGroup>
          <Field>
            <FieldLabel htmlFor={`user-email-subject-${user.id}`}>
              {t('Email Subject')}
            </FieldLabel>
            <Input
              id={`user-email-subject-${user.id}`}
              value={subject}
              onChange={(event) => setSubject(event.target.value)}
              placeholder={t('Enter email subject')}
              maxLength={200}
            />
          </Field>

          <Field>
            <FieldLabel htmlFor={`user-email-content-${user.id}`}>
              {t('Email Content')}
            </FieldLabel>
            <Textarea
              id={`user-email-content-${user.id}`}
              value={content}
              onChange={(event) => setContent(event.target.value)}
              placeholder={t('Enter email content')}
              className='min-h-40 resize-y'
            />
          </Field>
        </FieldGroup>

        <DialogFooter className='grid grid-cols-2 gap-2 sm:flex'>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={isSending}
          >
            {t('Cancel')}
          </Button>
          <Button type='button' onClick={handleSubmit} disabled={isSending}>
            <Send data-icon='inline-start' />
            {isSending ? t('Sending...') : t('Send')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
