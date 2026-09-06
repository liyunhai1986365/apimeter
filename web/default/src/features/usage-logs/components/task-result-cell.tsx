import { useCallback, useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
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
  useTaskImageStatus,
  type TaskImageMetadata,
} from '../hooks/use-task-image-status'
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
  onMetadata: (metadata: TaskImageMetadata) => void
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
  const imageStatus = useTaskImageStatus(detail ?? props.log)
  const onMetadata = props.onMetadata
  useEffect(() => {
    if (detail)
      onMetadata({
        image_status: detail.image_status,
        image_expires_at: detail.image_expires_at,
        image_has_url: detail.image_has_url,
      })
  }, [detail, onMetadata])
  const onOpenChange = (open: boolean) => {
    if (!open) props.onClose()
  }
  if (detail && imageStatus !== 'expired') {
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
        {imageStatus === 'expired' && <p>{t('Image expired')}</p>}
        {imageStatus === 'partially_expired' && (
          <p>{t('Some images expired')}</p>
        )}
        {detail && imageStatus !== 'expired' && (
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
  const [opened, setOpened] = useState<{
    taskId: string
    status: TaskLog['image_status']
  } | null>(null)
  const [metadata, setMetadata] = useState<{
    taskId: string
    image: TaskImageMetadata
  }>()
  const imageStatus = useTaskImageStatus(
    metadata?.taskId === props.log.task_id ? metadata.image : props.log
  )
  const onMetadata = useCallback(
    (value: TaskImageMetadata) =>
      setMetadata({ taskId: props.log.task_id, image: value }),
    [props.log.task_id]
  )
  // A deadline changes the status and unmounts the detail query immediately.
  // An initially unknown status may become available after fetching metadata.
  const open =
    opened?.taskId === props.log.task_id &&
    (opened.status === imageStatus ||
      (!opened.status && imageStatus === 'available'))
  let label: string
  if (imageStatus === 'expired')
    return <Badge variant='secondary'>{t('Image expired')}</Badge>
  if (imageStatus === 'partially_expired') label = t('Some images expired')
  else if (getTaskLogImageModelName(props.log) || props.log.image_status)
    label = t('Click to preview image')
  else if (props.log.platform === 'suno') label = t('Click to preview audio')
  else label = t('Click to preview video')
  return (
    <>
      <Button
        variant='link'
        size='sm'
        onClick={() =>
          setOpened({ taskId: props.log.task_id, status: imageStatus })
        }
      >
        {label}
      </Button>
      {open && (
        <TaskResultDialog
          log={props.log}
          isAdmin={props.isAdmin}
          onClose={() => setOpened(null)}
          onMetadata={onMetadata}
        />
      )}
    </>
  )
}
