/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Add01Icon,
  Delete02Icon,
  Edit02Icon,
  Mail01Icon,
  Megaphone01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { formatTimestampToDate } from '@/lib/format'
import { normalizePagedData } from '@/lib/paged-response'
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
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
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
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldTitle,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Markdown } from '@/components/ui/markdown'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { DateTimePicker } from '@/components/datetime-picker'
import { StatusBadge } from '@/components/status-badge'
import {
  ANNOUNCEMENT_TYPE_OPTIONS,
  getAnnouncementTypeOption,
  type AnnouncementType,
} from '@/features/dashboard/lib/announcement-categories'
import {
  createAgentAnnouncement,
  deleteAgentAnnouncement,
  listAgentAnnouncements,
  sendAgentAnnouncementEmail,
  updateAgentAnnouncement,
} from '../api'
import type { AgentAnnouncement, AgentAnnouncementInput } from '../types'

type AnnouncementDraft = {
  title: string
  content: string
  type: AnnouncementType
  extra: string
  publishAt?: Date
  enabled: boolean
  sendEmail: boolean
}

type DraftErrors = Partial<Record<'title' | 'content' | 'publishAt', string>>

function newAnnouncementDraft(): AnnouncementDraft {
  return {
    title: '',
    content: '',
    type: 'general',
    extra: '',
    publishAt: new Date(),
    enabled: true,
    sendEmail: false,
  }
}

export function AgentAnnouncementManager() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<AgentAnnouncement | null>(null)
  const [draft, setDraft] = useState<AnnouncementDraft>(newAnnouncementDraft)
  const [errors, setErrors] = useState<DraftErrors>({})
  const [deleteTarget, setDeleteTarget] = useState<AgentAnnouncement | null>(
    null
  )
  const [emailTarget, setEmailTarget] = useState<AgentAnnouncement | null>(null)

  const announcementsQuery = useQuery({
    queryKey: ['agent', 'announcements'],
    queryFn: () => listAgentAnnouncements(),
  })
  const announcementsPage = useMemo(
    () => normalizePagedData<AgentAnnouncement>(announcementsQuery.data),
    [announcementsQuery.data]
  )
  const announcements = announcementsPage.items

  const typeItems = useMemo(
    () =>
      ANNOUNCEMENT_TYPE_OPTIONS.map((option) => ({
        value: option.value,
        label: t(option.label),
      })),
    [t]
  )

  const refreshAnnouncements = () =>
    queryClient.invalidateQueries({ queryKey: ['agent', 'announcements'] })

  const saveMutation = useMutation({
    mutationFn: async (input: {
      id?: number
      announcement: AgentAnnouncementInput
      sendEmail: boolean
    }) => {
      const saved = input.id
        ? await updateAgentAnnouncement({ id: input.id, ...input.announcement })
        : await createAgentAnnouncement(input.announcement)
      const announcement = saved.data
      let email
      let emailError: unknown
      if (input.sendEmail && announcement) {
        try {
          email = await sendAgentAnnouncementEmail(announcement.id)
        } catch (error) {
          emailError = error
        }
      }
      return { email, emailError }
    },
    onSuccess: ({ email, emailError }) => {
      if (emailError) {
        toast.error(t('Announcement saved, but email delivery failed'))
      } else if (email?.data) {
        toast.success(
          t('Agent announcement email sent to {{sent}} of {{total}} users', {
            sent: email.data.sent,
            total: email.data.total,
          })
        )
      } else {
        toast.success(
          editing
            ? t('Agent announcement updated')
            : t('Agent announcement created')
        )
      }
      setDialogOpen(false)
      refreshAnnouncements()
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const deleteMutation = useMutation({
    mutationFn: deleteAgentAnnouncement,
    onSuccess: () => {
      toast.success(t('Agent announcement deleted'))
      setDeleteTarget(null)
      refreshAnnouncements()
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const emailMutation = useMutation({
    mutationFn: sendAgentAnnouncementEmail,
    onSuccess: (result) => {
      toast.success(
        t('Agent announcement email sent to {{sent}} of {{total}} users', {
          sent: result.data?.sent ?? 0,
          total: result.data?.total ?? 0,
        })
      )
      setEmailTarget(null)
      refreshAnnouncements()
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const openCreateDialog = () => {
    setEditing(null)
    setDraft(newAnnouncementDraft())
    setErrors({})
    setDialogOpen(true)
  }

  const openEditDialog = (announcement: AgentAnnouncement) => {
    setEditing(announcement)
    setDraft({
      title: announcement.title,
      content: announcement.content,
      type: announcement.type,
      extra: announcement.extra || '',
      publishAt: new Date(announcement.publish_at * 1000),
      enabled: announcement.enabled,
      sendEmail: false,
    })
    setErrors({})
    setDialogOpen(true)
  }

  const submitAnnouncement = (event: React.FormEvent) => {
    event.preventDefault()
    const nextErrors: DraftErrors = {}
    if (!draft.title.trim()) nextErrors.title = t('Title is required')
    if (!draft.content.trim()) nextErrors.content = t('Content is required')
    if (!draft.publishAt) nextErrors.publishAt = t('Publish date is required')
    setErrors(nextErrors)
    if (Object.keys(nextErrors).length > 0 || !draft.publishAt) return

    saveMutation.mutate({
      id: editing?.id,
      announcement: {
        title: draft.title.trim(),
        content: draft.content.trim(),
        type: draft.type,
        extra: draft.extra.trim(),
        publish_at: Math.floor(draft.publishAt.getTime() / 1000),
        enabled: draft.enabled,
      },
      sendEmail: draft.sendEmail,
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Agent Announcements')}</CardTitle>
        <CardDescription>
          {t(
            'Publish notices on this agent site and email only this agent’s enabled users.'
          )}
        </CardDescription>
        <CardAction>
          <Button size='sm' onClick={openCreateDialog}>
            <HugeiconsIcon icon={Add01Icon} data-icon='inline-start' />
            {t('Create Announcement')}
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent>
        <div className='overflow-x-auto rounded-lg border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Title')}</TableHead>
                <TableHead>{t('Type')}</TableHead>
                <TableHead>{t('Visibility')}</TableHead>
                <TableHead>{t('Publish Date')}</TableHead>
                <TableHead>{t('Latest email delivery')}</TableHead>
                <TableHead className='text-right'>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {announcements.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={6}>
                    <Empty>
                      <EmptyHeader>
                        <EmptyMedia variant='icon'>
                          <HugeiconsIcon icon={Megaphone01Icon} />
                        </EmptyMedia>
                        <EmptyTitle>{t('No Agent Announcements')}</EmptyTitle>
                        <EmptyDescription>
                          {t(
                            'Create an announcement for users visiting this agent site.'
                          )}
                        </EmptyDescription>
                      </EmptyHeader>
                      <EmptyContent>
                        <Button size='sm' onClick={openCreateDialog}>
                          <HugeiconsIcon
                            icon={Add01Icon}
                            data-icon='inline-start'
                          />
                          {t('Create Announcement')}
                        </Button>
                      </EmptyContent>
                    </Empty>
                  </TableCell>
                </TableRow>
              ) : (
                announcements.map((announcement) => {
                  const typeOption = getAnnouncementTypeOption(
                    announcement.type
                  )
                  return (
                    <TableRow key={announcement.id}>
                      <TableCell className='max-w-72'>
                        <div className='flex min-w-0 flex-col gap-1'>
                          <span className='truncate font-medium'>
                            {announcement.title}
                          </span>
                          <span className='text-muted-foreground truncate text-xs'>
                            {announcement.content}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell>
                        <StatusBadge
                          label={t(typeOption.label)}
                          variant={typeOption.badgeVariant}
                          copyable={false}
                        />
                      </TableCell>
                      <TableCell>
                        <StatusBadge
                          label={
                            announcement.enabled
                              ? t('Visible on agent site')
                              : t('Hidden')
                          }
                          variant={announcement.enabled ? 'success' : 'neutral'}
                          copyable={false}
                        />
                      </TableCell>
                      <TableCell>
                        {formatTimestampToDate(announcement.publish_at)}
                      </TableCell>
                      <TableCell>
                        {announcement.last_email_at > 0 ? (
                          <div className='flex flex-col gap-1'>
                            <span className='tabular-nums'>
                              {t('{{sent}} of {{total}} sent', {
                                sent: announcement.last_email_sent,
                                total: announcement.last_email_total,
                              })}
                            </span>
                            <span className='text-muted-foreground text-xs'>
                              {formatTimestampToDate(
                                announcement.last_email_at
                              )}
                            </span>
                          </div>
                        ) : (
                          <span className='text-muted-foreground'>
                            {t('Not sent')}
                          </span>
                        )}
                      </TableCell>
                      <TableCell>
                        <div className='flex justify-end gap-1'>
                          <Button
                            variant='ghost'
                            size='icon-sm'
                            aria-label={t('Send announcement email')}
                            onClick={() => setEmailTarget(announcement)}
                          >
                            <HugeiconsIcon icon={Mail01Icon} />
                          </Button>
                          <Button
                            variant='ghost'
                            size='icon-sm'
                            aria-label={t('Edit Announcement')}
                            onClick={() => openEditDialog(announcement)}
                          >
                            <HugeiconsIcon icon={Edit02Icon} />
                          </Button>
                          <Button
                            variant='ghost'
                            size='icon-sm'
                            aria-label={t('Delete Announcement')}
                            onClick={() => setDeleteTarget(announcement)}
                          >
                            <HugeiconsIcon icon={Delete02Icon} />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  )
                })
              )}
            </TableBody>
          </Table>
        </div>
      </CardContent>
      <CardFooter>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Email recipients are limited to enabled users assigned to this agent; duplicate email addresses are sent once.'
          )}
        </p>
      </CardFooter>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className='flex max-h-[calc(100dvh-2rem)] flex-col sm:max-w-3xl'>
          <DialogHeader>
            <DialogTitle>
              {editing
                ? t('Edit Agent Announcement')
                : t('Create Agent Announcement')}
            </DialogTitle>
            <DialogDescription>
              {t(
                'The announcement is isolated to this agent. Email delivery uses the same saved content.'
              )}
            </DialogDescription>
          </DialogHeader>
          <form
            className='flex min-h-0 flex-1 flex-col gap-4'
            onSubmit={submitAnnouncement}
          >
            <div className='min-h-0 flex-1 overflow-y-auto pr-1'>
              <FieldGroup>
                <Field data-invalid={Boolean(errors.title)}>
                  <FieldLabel htmlFor='agent-announcement-title'>
                    {t('Title')}
                  </FieldLabel>
                  <Input
                    id='agent-announcement-title'
                    value={draft.title}
                    maxLength={120}
                    aria-invalid={Boolean(errors.title)}
                    onChange={(event) =>
                      setDraft((current) => ({
                        ...current,
                        title: event.target.value,
                      }))
                    }
                    placeholder={t('Enter announcement title')}
                  />
                  <FieldDescription>
                    {t('This title is also used as the email subject.')}
                  </FieldDescription>
                  <FieldError>{errors.title}</FieldError>
                </Field>

                <Field data-invalid={Boolean(errors.content)}>
                  <FieldLabel htmlFor='agent-announcement-content'>
                    {t('Content')}
                  </FieldLabel>
                  <Tabs defaultValue='write'>
                    <TabsList>
                      <TabsTrigger value='write'>{t('Write')}</TabsTrigger>
                      <TabsTrigger value='preview'>{t('Preview')}</TabsTrigger>
                    </TabsList>
                    <TabsContent value='write'>
                      <Textarea
                        id='agent-announcement-content'
                        value={draft.content}
                        maxLength={10000}
                        rows={10}
                        aria-invalid={Boolean(errors.content)}
                        onChange={(event) =>
                          setDraft((current) => ({
                            ...current,
                            content: event.target.value,
                          }))
                        }
                        placeholder={t(
                          'Enter announcement content (supports Markdown)'
                        )}
                      />
                    </TabsContent>
                    <TabsContent value='preview'>
                      <div className='bg-muted/20 min-h-56 rounded-lg border p-4'>
                        {draft.content ? (
                          <Markdown>{draft.content}</Markdown>
                        ) : (
                          <p className='text-muted-foreground text-sm'>
                            {t('Markdown preview')}
                          </p>
                        )}
                      </div>
                    </TabsContent>
                  </Tabs>
                  <FieldError>{errors.content}</FieldError>
                </Field>

                <FieldGroup className='grid gap-5 sm:grid-cols-2'>
                  <Field>
                    <FieldLabel htmlFor='agent-announcement-type'>
                      {t('Type')}
                    </FieldLabel>
                    <Select
                      items={typeItems}
                      value={draft.type}
                      onValueChange={(value) =>
                        value &&
                        setDraft((current) => ({
                          ...current,
                          type: value as AnnouncementType,
                        }))
                      }
                    >
                      <SelectTrigger
                        id='agent-announcement-type'
                        className='w-full'
                      >
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          {typeItems.map((item) => (
                            <SelectItem key={item.value} value={item.value}>
                              {item.label}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </Field>
                  <Field data-invalid={Boolean(errors.publishAt)}>
                    <FieldLabel>{t('Publish Date')}</FieldLabel>
                    <DateTimePicker
                      value={draft.publishAt}
                      onChange={(publishAt) =>
                        setDraft((current) => ({ ...current, publishAt }))
                      }
                    />
                    <FieldDescription>
                      {t(
                        'Future announcements become visible at the selected time.'
                      )}
                    </FieldDescription>
                    <FieldError>{errors.publishAt}</FieldError>
                  </Field>
                </FieldGroup>

                <Field>
                  <FieldLabel htmlFor='agent-announcement-extra'>
                    {t('Extra')}
                  </FieldLabel>
                  <Input
                    id='agent-announcement-extra'
                    value={draft.extra}
                    maxLength={100}
                    onChange={(event) =>
                      setDraft((current) => ({
                        ...current,
                        extra: event.target.value,
                      }))
                    }
                    placeholder={t('Optional short note')}
                  />
                </Field>

                <Field orientation='horizontal'>
                  <FieldContent>
                    <FieldTitle>{t('Show on agent site')}</FieldTitle>
                    <FieldDescription>
                      {t(
                        'Only enabled announcements are shown on this agent domain.'
                      )}
                    </FieldDescription>
                  </FieldContent>
                  <Switch
                    id='agent-announcement-enabled'
                    checked={draft.enabled}
                    onCheckedChange={(enabled) =>
                      setDraft((current) => ({ ...current, enabled }))
                    }
                  />
                  <FieldLabel
                    htmlFor='agent-announcement-enabled'
                    className='sr-only'
                  >
                    {t('Show on agent site')}
                  </FieldLabel>
                </Field>

                <Field orientation='horizontal'>
                  <Checkbox
                    id='agent-announcement-send-email'
                    checked={draft.sendEmail}
                    onCheckedChange={(checked) =>
                      setDraft((current) => ({
                        ...current,
                        sendEmail: checked === true,
                      }))
                    }
                  />
                  <FieldContent>
                    <FieldLabel htmlFor='agent-announcement-send-email'>
                      {t('Email agent users after saving')}
                    </FieldLabel>
                    <FieldDescription>
                      {t(
                        'Send this saved announcement now to this agent’s eligible email recipients.'
                      )}
                    </FieldDescription>
                  </FieldContent>
                </Field>
              </FieldGroup>
            </div>
            <DialogFooter>
              <Button
                type='button'
                variant='outline'
                disabled={saveMutation.isPending}
                onClick={() => setDialogOpen(false)}
              >
                {t('Cancel')}
              </Button>
              <Button type='submit' disabled={saveMutation.isPending}>
                {saveMutation.isPending ? (
                  <Spinner data-icon='inline-start' />
                ) : null}
                {draft.sendEmail ? t('Save and Send Email') : t('Save')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('Delete Agent Announcement')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'This removes the announcement from this agent site. Previously sent emails cannot be recalled.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteMutation.isPending}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              variant='destructive'
              disabled={deleteMutation.isPending}
              onClick={() =>
                deleteTarget && deleteMutation.mutate(deleteTarget.id)
              }
            >
              {t('Delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog
        open={Boolean(emailTarget)}
        onOpenChange={(open) => !open && setEmailTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('Send Agent Announcement Email')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'Send “{{title}}” now to all eligible users assigned to this agent?',
                { title: emailTarget?.title ?? '' }
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={emailMutation.isPending}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={emailMutation.isPending}
              onClick={() =>
                emailTarget && emailMutation.mutate(emailTarget.id)
              }
            >
              {emailMutation.isPending ? (
                <Spinner data-icon='inline-start' />
              ) : null}
              {t('Send Email')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  )
}
