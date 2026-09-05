import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Skeleton } from '@/components/ui/skeleton'
import { getTaskLogDetail } from '../api'
import {
  getTaskLogImageModelName,
  getTaskLogImagePreviewUrl,
  getTaskLogVideoPreviewUrl,
} from '../lib/task-display'
import type { TaskLog } from '../types'
import {
  AudioPreviewDialog,
  type AudioClip,
} from './dialogs/audio-preview-dialog'
import { ImageDialog } from './dialogs/image-dialog'
import { VideoPreviewDialog } from './dialogs/video-preview-dialog'

function TaskResultDialog(props: {
  log: TaskLog
  isAdmin: boolean
  onClose: () => void
}) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['task-log-detail', props.isAdmin, props.log.task_id],
    queryFn: ({ signal }) =>
      getTaskLogDetail(props.log.task_id, props.isAdmin, signal),
    // Mount only after clicking, and release large results when closing.
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  const detail = query.data
  const onOpenChange = (open: boolean) => {
    if (!open) props.onClose()
  }
  if (detail) {
    const imageUrl = getTaskLogImagePreviewUrl(detail)
    if (imageUrl)
      return (
        <ImageDialog
          open
          onOpenChange={onOpenChange}
          imageUrl={imageUrl}
          taskId={detail.task_id}
        />
      )
    const videoUrl = getTaskLogVideoPreviewUrl(detail)
    if (videoUrl)
      return (
        <VideoPreviewDialog
          open
          onOpenChange={onOpenChange}
          videoUrl={videoUrl}
        />
      )
    if (
      Array.isArray(detail.data) &&
      detail.data.some((clip) => clip?.audio_url)
    ) {
      return (
        <AudioPreviewDialog
          open
          onOpenChange={onOpenChange}
          clips={detail.data as AudioClip[]}
        />
      )
    }
  }
  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('Details')}</DialogTitle>
        </DialogHeader>
        {query.isPending && <Skeleton className='h-40 w-full' />}
        {query.isError && <p role='alert'>{t('Failed to load logs')}</p>}
        {detail && (
          <pre className='max-h-96 overflow-auto break-all whitespace-pre-wrap'>
            {JSON.stringify(detail.data ?? {}, null, 2).slice(0, 16000)}
          </pre>
        )}
      </DialogContent>
    </Dialog>
  )
}

export function TaskResultCell(props: { log: TaskLog; isAdmin: boolean }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  let label: string
  if (getTaskLogImageModelName(props.log)) label = t('Click to preview image')
  else if (props.log.platform === 'suno') label = t('Click to preview audio')
  else label = t('Click to preview video')
  return (
    <>
      <Button variant='link' size='sm' onClick={() => setOpen(true)}>
        {label}
      </Button>
      {open && (
        <TaskResultDialog
          log={props.log}
          isAdmin={props.isAdmin}
          onClose={() => setOpen(false)}
        />
      )}
    </>
  )
}
