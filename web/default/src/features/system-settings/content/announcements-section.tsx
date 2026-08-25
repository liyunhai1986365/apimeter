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
import * as z from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import {
  Add01Icon,
  Delete02Icon,
  Edit02Icon,
  FloppyDiskIcon,
  Mail01Icon,
  ViewIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import dayjs from '@/lib/dayjs'
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
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Markdown } from '@/components/ui/markdown'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
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
import { ANNOUNCEMENT_TYPE_OPTIONS } from '@/features/dashboard/lib/announcement-categories'
import { sendAnnouncementEmail } from '../api'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  planAnnouncementPublication,
  type Announcement,
} from './announcement-publication'

type AnnouncementsSectionProps = {
  enabled: boolean
  data: string
}

const announcementSchema = z
  .object({
    title: z
      .string()
      .min(1, 'Title is required')
      .max(120, 'Title must be less than 120 characters'),
    content: z
      .string()
      .min(1, 'Content is required')
      .max(10000, 'Content must be less than 10000 characters'),
    publishDate: z.string().min(1, 'Publish date is required'),
    type: z.enum([
      'product_update',
      'system_maintenance',
      'model_release',
      'pricing_update',
      'incident',
      'general',
    ]),
    extra: z
      .string()
      .max(100, 'Extra must be less than 100 characters')
      .optional(),
    audience: z.enum(['all', 'main_site']),
    displayOnFrontend: z.boolean(),
    sendEmail: z.boolean(),
  })
  .refine((values) => values.displayOnFrontend || values.sendEmail, {
    message: 'Select at least one delivery channel',
    path: ['displayOnFrontend'],
  })

type AnnouncementFormValues = z.infer<typeof announcementSchema>

const typeOptions = ANNOUNCEMENT_TYPE_OPTIONS

export function AnnouncementsSection({
  enabled,
  data,
}: AnnouncementsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [announcements, setAnnouncements] = useState<Announcement[]>([])
  const [isEnabled, setIsEnabled] = useState(enabled)
  const [hasChanges, setHasChanges] = useState(false)
  const [selectedIds, setSelectedIds] = useState<number[]>([])
  const [showDialog, setShowDialog] = useState(false)
  const [showDeleteDialog, setShowDeleteDialog] = useState(false)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [editingAnnouncement, setEditingAnnouncement] =
    useState<Announcement | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<'single' | 'batch'>('single')

  const form = useForm<AnnouncementFormValues>({
    resolver: zodResolver(announcementSchema),
    defaultValues: {
      title: '',
      content: '',
      publishDate: new Date().toISOString(),
      type: 'general',
      extra: '',
      audience: 'all',
      displayOnFrontend: true,
      sendEmail: false,
    },
  })

  const previewTitle = form.watch('title')
  const previewContent = form.watch('content')
  const displayOnFrontend = form.watch('displayOnFrontend')

  useEffect(() => {
    try {
      const parsed = JSON.parse(data || '[]')
      if (Array.isArray(parsed)) {
        setAnnouncements(
          parsed.map((item, idx) => ({
            ...item,
            id: item.id || idx + 1,
            title: item.title || '',
            audience: item.audience === 'main_site' ? 'main_site' : 'all',
          }))
        )
      }
    } catch {
      setAnnouncements([])
    }
  }, [data])

  useEffect(() => {
    setIsEnabled(enabled)
  }, [enabled])

  const handleToggleEnabled = async (checked: boolean) => {
    try {
      await updateOption.mutateAsync({
        key: 'console_setting.announcements_enabled',
        value: checked,
      })
      setIsEnabled(checked)
      toast.success(t('Setting saved'))
    } catch {
      toast.error(t('Failed to update setting'))
    }
  }

  const handleAdd = () => {
    setEditingAnnouncement(null)
    form.reset({
      title: '',
      content: '',
      publishDate: new Date().toISOString(),
      type: 'general',
      extra: '',
      audience: 'all',
      displayOnFrontend: true,
      sendEmail: false,
    })
    setShowDialog(true)
  }

  const handleEdit = (announcement: Announcement) => {
    setEditingAnnouncement(announcement)
    form.reset({
      title: announcement.title,
      content: announcement.content,
      publishDate: announcement.publishDate,
      type: announcement.type,
      extra: announcement.extra || '',
      audience: announcement.audience || 'all',
      displayOnFrontend: true,
      sendEmail: false,
    })
    setShowDialog(true)
  }

  const handleDelete = (announcement: Announcement) => {
    setEditingAnnouncement(announcement)
    setDeleteTarget('single')
    setShowDeleteDialog(true)
  }

  const handleBatchDelete = () => {
    if (selectedIds.length === 0) {
      toast.error(t('Please select items to delete'))
      return
    }
    setDeleteTarget('batch')
    setShowDeleteDialog(true)
  }

  const confirmDelete = () => {
    if (deleteTarget === 'single' && editingAnnouncement) {
      setAnnouncements((prev) =>
        prev.filter((item) => item.id !== editingAnnouncement.id)
      )
      setHasChanges(true)
      toast.success(t('Announcement deleted. Click "Save Settings" to apply.'))
    } else if (deleteTarget === 'batch') {
      setAnnouncements((prev) =>
        prev.filter((item) => !selectedIds.includes(item.id))
      )
      setSelectedIds([])
      setHasChanges(true)
      toast.success(
        t('{{count}} announcements deleted. Click "Save Settings" to apply.', {
          count: selectedIds.length,
        })
      )
    }
    setShowDeleteDialog(false)
    setEditingAnnouncement(null)
  }

  const saveAnnouncementList = async (nextAnnouncements: Announcement[]) => {
    await updateOption.mutateAsync({
      key: 'console_setting.announcements',
      value: JSON.stringify(nextAnnouncements),
    })
    setAnnouncements(nextAnnouncements)
    setHasChanges(false)
  }

  const handleSubmitForm = async (values: AnnouncementFormValues) => {
    const { displayOnFrontend, sendEmail, ...announcementValues } = values
    const publication = planAnnouncementPublication({
      announcements,
      draft: announcementValues,
      editingId: editingAnnouncement?.id,
      displayOnFrontend,
    })
    let phase: 'save' | 'email' = 'save'

    setIsSubmitting(true)
    try {
      if (publication.shouldSave) {
        await saveAnnouncementList(publication.announcements)
      }

      if (sendEmail) {
        phase = 'email'
        const result = await sendAnnouncementEmail({
          title: announcementValues.title,
          content: announcementValues.content,
          type: announcementValues.type,
          audience: announcementValues.audience,
        })

        if (!result.success) {
          toast.error(result.message || t('Failed to send announcement email'))
          return
        } else {
          toast.success(
            t('Announcement email sent to {{sent}} of {{total}} users', {
              sent: result.data?.sent ?? 0,
              total: result.data?.total ?? 0,
            })
          )
        }
      } else {
        toast.success(
          editingAnnouncement
            ? t('Announcement updated successfully')
            : t('Announcement added successfully')
        )
      }

      setShowDialog(false)
    } catch {
      toast.error(
        phase === 'email'
          ? t('Failed to send announcement email')
          : t('Failed to save announcements')
      )
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleSaveAll = async () => {
    try {
      await saveAnnouncementList(announcements)
      toast.success(t('Announcements saved successfully'))
    } catch {
      toast.error(t('Failed to save announcements'))
    }
  }

  const toggleSelectAll = (checked: boolean) => {
    setSelectedIds(checked ? announcements.map((item) => item.id) : [])
  }

  const toggleSelectOne = (id: number, checked: boolean) => {
    setSelectedIds((prev) =>
      checked ? [...prev, id] : prev.filter((item) => item !== id)
    )
  }

  const sortedAnnouncements = useMemo(() => {
    return [...announcements].sort((a, b) => {
      return (
        new Date(b.publishDate).getTime() - new Date(a.publishDate).getTime()
      )
    })
  }, [announcements])

  const getRelativeTime = (date: string) => {
    const now = new Date()
    const past = new Date(date)
    const diffMs = now.getTime() - past.getTime()
    const diffMins = Math.floor(diffMs / 60000)
    const diffHours = Math.floor(diffMins / 60)
    const diffDays = Math.floor(diffHours / 24)

    if (diffMins < 60) return `${diffMins}m ago`
    if (diffHours < 24) return `${diffHours}h ago`
    return `${diffDays}d ago`
  }

  return (
    <SettingsSection
      title={t('Announcements')}
      description={t('Broadcast short system notices on the dashboard')}
    >
      <div className='flex flex-col gap-4'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <div className='flex flex-wrap items-center gap-2'>
            <Button onClick={handleAdd} size='sm'>
              <HugeiconsIcon icon={Add01Icon} data-icon='inline-start' />
              {t('Add Announcement')}
            </Button>
            <Button
              onClick={handleBatchDelete}
              size='sm'
              variant='destructive'
              disabled={selectedIds.length === 0}
            >
              <HugeiconsIcon icon={Delete02Icon} data-icon='inline-start' />
              {t('Delete (')}
              {selectedIds.length})
            </Button>
            <Button
              onClick={handleSaveAll}
              size='sm'
              variant='secondary'
              disabled={!hasChanges || updateOption.isPending}
            >
              <HugeiconsIcon icon={FloppyDiskIcon} data-icon='inline-start' />
              {updateOption.isPending ? t('Saving...') : t('Save Settings')}
            </Button>
          </div>
          <div className='flex items-center gap-2'>
            <span className='text-muted-foreground text-sm'>
              {t('Enabled')}
            </span>
            <Switch checked={isEnabled} onCheckedChange={handleToggleEnabled} />
          </div>
        </div>

        <div className='rounded-md border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className='w-12'>
                  <Checkbox
                    checked={
                      selectedIds.length === announcements.length &&
                      announcements.length > 0
                    }
                    onCheckedChange={toggleSelectAll}
                  />
                </TableHead>
                <TableHead>{t('Title')}</TableHead>
                <TableHead>{t('Content')}</TableHead>
                <TableHead>{t('Publish Date')}</TableHead>
                <TableHead>{t('Type')}</TableHead>
                <TableHead>{t('Audience')}</TableHead>
                <TableHead>{t('Extra')}</TableHead>
                <TableHead className='w-32'>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sortedAnnouncements.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={8} className='h-24 text-center'>
                    {t(
                      'No announcements yet. Click "Add Announcement" to create one.'
                    )}
                  </TableCell>
                </TableRow>
              ) : (
                sortedAnnouncements.map((announcement) => (
                  <TableRow key={announcement.id}>
                    <TableCell>
                      <Checkbox
                        checked={selectedIds.includes(announcement.id)}
                        onCheckedChange={(checked) =>
                          toggleSelectOne(announcement.id, checked as boolean)
                        }
                      />
                    </TableCell>
                    <TableCell className='max-w-56'>
                      <div className='flex flex-col gap-1'>
                        <span
                          className='truncate text-sm font-medium'
                          title={announcement.title}
                        >
                          {announcement.title}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell
                      className='text-muted-foreground max-w-xs truncate'
                      title={announcement.content}
                    >
                      {announcement.content}
                    </TableCell>
                    <TableCell>
                      <div className='flex flex-col gap-1'>
                        <span className='text-sm font-medium'>
                          {getRelativeTime(announcement.publishDate)}
                        </span>
                        <span className='text-muted-foreground text-xs'>
                          {dayjs(announcement.publishDate).format(
                            'YYYY-MM-DD HH:mm:ss'
                          )}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell>
                      <StatusBadge
                        label={t(
                          typeOptions.find(
                            (opt) => opt.value === announcement.type
                          )?.label ?? 'General Announcement'
                        )}
                        variant={
                          typeOptions.find(
                            (opt) => opt.value === announcement.type
                          )?.badgeVariant ?? 'neutral'
                        }
                        copyable={false}
                      />
                    </TableCell>
                    <TableCell>
                      <StatusBadge
                        label={t(
                          announcement.audience === 'main_site'
                            ? 'Main site users only'
                            : 'All users'
                        )}
                        variant={
                          announcement.audience === 'main_site'
                            ? 'info'
                            : 'neutral'
                        }
                        copyable={false}
                      />
                    </TableCell>
                    <TableCell
                      className='text-muted-foreground max-w-xs truncate'
                      title={announcement.extra}
                    >
                      {announcement.extra || '-'}
                    </TableCell>
                    <TableCell>
                      <div className='flex gap-2'>
                        <Button
                          onClick={() => handleEdit(announcement)}
                          size='sm'
                          variant='ghost'
                        >
                          <HugeiconsIcon
                            icon={Edit02Icon}
                            data-icon='inline-start'
                          />
                        </Button>
                        <Button
                          onClick={() => handleDelete(announcement)}
                          size='sm'
                          variant='ghost'
                        >
                          <HugeiconsIcon
                            icon={Delete02Icon}
                            data-icon='inline-start'
                          />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
      </div>

      <Dialog open={showDialog} onOpenChange={setShowDialog}>
        <DialogContent className='flex h-dvh max-h-dvh w-screen max-w-none flex-col overflow-hidden rounded-none sm:h-auto sm:max-h-[calc(100dvh-2rem)] sm:max-w-[min(92vw,1200px)] sm:rounded-xl'>
          <DialogHeader className='shrink-0 pr-8'>
            <DialogTitle>
              {editingAnnouncement
                ? t('Edit Announcement')
                : t('Add Announcement')}
            </DialogTitle>
            <DialogDescription>
              {t(
                'Create a frontend announcement, send an email broadcast, or both.'
              )}
            </DialogDescription>
          </DialogHeader>
          <Form {...form}>
            <form
              onSubmit={form.handleSubmit(handleSubmitForm)}
              className='flex min-h-0 flex-1 flex-col gap-4'
            >
              <ScrollArea className='min-h-0 flex-1 pr-4'>
                <div className='flex flex-col gap-4 pb-1'>
                  <FormField
                    control={form.control}
                    name='title'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Title')}</FormLabel>
                        <FormControl>
                          <Input
                            placeholder={t('Enter announcement title')}
                            {...field}
                          />
                        </FormControl>
                        <FormDescription>
                          {t(
                            'Used as the announcement title and email subject line.'
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <div className='grid min-w-0 gap-5 lg:grid-cols-[minmax(0,2fr)_minmax(18rem,1fr)] lg:items-start'>
                    <FormField
                      control={form.control}
                      name='content'
                      render={({ field }) => (
                        <FormItem className='min-w-0'>
                          <FormLabel>{t('Content')}</FormLabel>
                          <Tabs defaultValue='write' className='gap-3'>
                            <TabsList>
                              <TabsTrigger value='write'>
                                {t('Write')}
                              </TabsTrigger>
                              <TabsTrigger value='preview'>
                                {t('Preview')}
                              </TabsTrigger>
                            </TabsList>
                            <div className='min-w-0'>
                              <TabsContent value='write'>
                                <FormControl>
                                  <Textarea
                                    placeholder={t(
                                      'Enter announcement content (supports Markdown/HTML)'
                                    )}
                                    rows={18}
                                    className='min-h-64 w-full resize-y font-mono text-sm leading-relaxed sm:min-h-96 lg:h-[420px] lg:resize-none'
                                    {...field}
                                  />
                                </FormControl>
                              </TabsContent>
                              <TabsContent value='preview'>
                                <ScrollArea className='bg-muted/20 h-64 rounded-md border p-4 sm:h-96 lg:h-[420px]'>
                                  {previewContent ? (
                                    <div className='flex flex-col gap-3'>
                                      {previewTitle && (
                                        <h3 className='text-base font-semibold'>
                                          {previewTitle}
                                        </h3>
                                      )}
                                      <Markdown>{previewContent}</Markdown>
                                    </div>
                                  ) : (
                                    <div className='text-muted-foreground flex h-56 items-center justify-center text-sm sm:h-[352px] lg:h-[388px]'>
                                      {t('Markdown preview')}
                                    </div>
                                  )}
                                </ScrollArea>
                              </TabsContent>
                            </div>
                          </Tabs>
                          <FormDescription>
                            {t(
                              'Maximum 10000 characters. Supports Markdown and HTML.'
                            )}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <div className='flex min-w-0 flex-col gap-4'>
                      <div className='flex flex-col gap-2'>
                        <div>
                          <p className='font-medium'>
                            {t('Delivery channels')}
                          </p>
                          <p className='text-muted-foreground text-sm'>
                            {t(
                              'Choose where this announcement will be delivered.'
                            )}
                          </p>
                        </div>
                        <div className='grid gap-3'>
                          <FormField
                            control={form.control}
                            name='displayOnFrontend'
                            render={({ field }) => (
                              <FormItem className='flex flex-row items-start gap-3 rounded-md border p-3'>
                                <FormControl>
                                  <Checkbox
                                    checked={field.value}
                                    onCheckedChange={(checked) =>
                                      field.onChange(checked === true)
                                    }
                                  />
                                </FormControl>
                                <div className='flex min-w-0 flex-1 gap-3'>
                                  <HugeiconsIcon
                                    icon={ViewIcon}
                                    strokeWidth={2}
                                    className='mt-0.5 size-5 shrink-0'
                                  />
                                  <div className='flex min-w-0 flex-col gap-1'>
                                    <FormLabel>
                                      {t('Show on frontend')}
                                    </FormLabel>
                                    <FormDescription>
                                      {t(
                                        'Save this announcement and display it to frontend users.'
                                      )}
                                    </FormDescription>
                                    <FormMessage />
                                  </div>
                                </div>
                              </FormItem>
                            )}
                          />
                          <FormField
                            control={form.control}
                            name='sendEmail'
                            render={({ field }) => (
                              <FormItem className='flex flex-row items-start gap-3 rounded-md border p-3'>
                                <FormControl>
                                  <Checkbox
                                    checked={field.value}
                                    onCheckedChange={(checked) =>
                                      field.onChange(checked === true)
                                    }
                                  />
                                </FormControl>
                                <div className='flex min-w-0 flex-1 gap-3'>
                                  <HugeiconsIcon
                                    icon={Mail01Icon}
                                    strokeWidth={2}
                                    className='mt-0.5 size-5 shrink-0'
                                  />
                                  <div className='flex min-w-0 flex-col gap-1'>
                                    <FormLabel>
                                      {t('Send announcement email')}
                                    </FormLabel>
                                    <FormDescription>
                                      {t(
                                        'Send to enabled users with an email address in the selected audience.'
                                      )}
                                    </FormDescription>
                                  </div>
                                </div>
                              </FormItem>
                            )}
                          />
                        </div>
                      </div>
                      <FormField
                        control={form.control}
                        name='audience'
                        render={({ field }) => (
                          <FormItem className='flex flex-row items-center justify-between gap-4 rounded-md border p-3'>
                            <div className='flex min-w-0 flex-col gap-1'>
                              <FormLabel htmlFor='announcement-main-site-only'>
                                {t('Main site users only')}
                              </FormLabel>
                              <FormDescription>
                                {t(
                                  'Agent-site users will not see this announcement or receive its email.'
                                )}
                              </FormDescription>
                            </div>
                            <FormControl>
                              <Switch
                                id='announcement-main-site-only'
                                checked={field.value === 'main_site'}
                                onCheckedChange={(checked) =>
                                  field.onChange(checked ? 'main_site' : 'all')
                                }
                              />
                            </FormControl>
                          </FormItem>
                        )}
                      />
                      {displayOnFrontend && (
                        <FormField
                          control={form.control}
                          name='publishDate'
                          render={({ field }) => (
                            <FormItem>
                              <FormLabel>{t('Publish Date')}</FormLabel>
                              <FormControl>
                                <DateTimePicker
                                  value={
                                    field.value
                                      ? new Date(field.value)
                                      : undefined
                                  }
                                  onChange={(date) =>
                                    field.onChange(
                                      date ? date.toISOString() : ''
                                    )
                                  }
                                  placeholder={t('Select publish date')}
                                />
                              </FormControl>
                              <FormDescription>
                                {t(
                                  'Date and time when this announcement should be displayed'
                                )}
                              </FormDescription>
                              <FormMessage />
                            </FormItem>
                          )}
                        />
                      )}
                      <FormField
                        control={form.control}
                        name='type'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Type')}</FormLabel>
                            <Select
                              items={[
                                ...typeOptions.map((option) => ({
                                  value: option.value,
                                  label: (
                                    <div className='flex items-center gap-2'>
                                      <div
                                        className={`size-3 rounded-full ${option.color}`}
                                      />
                                      {t(option.label)}
                                    </div>
                                  ),
                                })),
                              ]}
                              onValueChange={field.onChange}
                              value={field.value}
                            >
                              <FormControl>
                                <SelectTrigger>
                                  <SelectValue
                                    placeholder={t('Select announcement type')}
                                  />
                                </SelectTrigger>
                              </FormControl>
                              <SelectContent alignItemWithTrigger={false}>
                                <SelectGroup>
                                  {typeOptions.map((option) => (
                                    <SelectItem
                                      key={option.value}
                                      value={option.value}
                                    >
                                      <div className='flex items-center gap-2'>
                                        <div
                                          className={`size-3 rounded-full ${option.color}`}
                                        />
                                        {t(option.label)}
                                      </div>
                                    </SelectItem>
                                  ))}
                                </SelectGroup>
                              </SelectContent>
                            </Select>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      {displayOnFrontend && (
                        <FormField
                          control={form.control}
                          name='extra'
                          render={({ field }) => (
                            <FormItem>
                              <FormLabel>
                                {t('Extra Notes (Optional)')}
                              </FormLabel>
                              <FormControl>
                                <Input
                                  placeholder={t('Additional information')}
                                  {...field}
                                />
                              </FormControl>
                              <FormDescription>
                                {t(
                                  'Optional supplementary information (max 100 characters)'
                                )}
                              </FormDescription>
                              <FormMessage />
                            </FormItem>
                          )}
                        />
                      )}
                    </div>
                  </div>
                </div>
              </ScrollArea>
              <DialogFooter className='shrink-0'>
                <Button
                  type='button'
                  variant='outline'
                  onClick={() => setShowDialog(false)}
                >
                  {t('Cancel')}
                </Button>
                <Button type='submit' disabled={isSubmitting}>
                  {isSubmitting
                    ? t('Processing...')
                    : !displayOnFrontend
                      ? t('Send Email')
                      : editingAnnouncement
                        ? t('Update')
                        : t('Add')}
                </Button>
              </DialogFooter>
            </form>
          </Form>
        </DialogContent>
      </Dialog>

      <AlertDialog open={showDeleteDialog} onOpenChange={setShowDeleteDialog}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Are you sure?')}</AlertDialogTitle>
            <AlertDialogDescription>
              {deleteTarget === 'single'
                ? 'This announcement will be removed from the list.'
                : `${selectedIds.length} announcements will be removed from the list.`}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={confirmDelete}>
              {t('Delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </SettingsSection>
  )
}
