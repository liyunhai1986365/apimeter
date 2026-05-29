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
import { Copy, ExternalLink, Video } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

interface VideoPreviewDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  videoUrl: string
}

export function VideoPreviewDialog(props: VideoPreviewDialogProps) {
  const { t } = useTranslation()
  const [hasError, setHasError] = useState(false)
  const [isPortrait, setIsPortrait] = useState(false)
  const videoUrl = props.videoUrl

  useEffect(() => {
    setHasError(false)
    setIsPortrait(false)
  }, [videoUrl])

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[92vh] w-[min(92vw,960px)] sm:max-w-[min(92vw,960px)]'>
        <DialogHeader>
          <DialogTitle className='flex items-center gap-2'>
            <Video className='h-5 w-5' />
            {t('Video Preview')}
          </DialogTitle>
        </DialogHeader>

        <div className='space-y-3'>
          {!hasError ? (
            <div className='bg-muted/30 flex max-h-[68vh] min-h-[220px] items-center justify-center overflow-hidden rounded-md border'>
              <video
                src={videoUrl}
                controls
                preload='metadata'
                onLoadedMetadata={(event) => {
                  const video = event.currentTarget
                  setIsPortrait(video.videoHeight > video.videoWidth)
                }}
                onError={() => setHasError(true)}
                className={cn(
                  'block max-h-[68vh] max-w-full object-contain',
                  isPortrait ? 'h-[68vh] w-auto' : 'h-auto w-full'
                )}
              />
            </div>
          ) : (
            <div className='bg-muted/30 flex max-h-[68vh] min-h-[220px] w-full items-center justify-center rounded-md border p-6 text-center'>
              <span className='text-destructive text-sm'>
                {t('Video playback failed')}
              </span>
            </div>
          )}

          <div className='flex min-w-0 flex-col gap-2 sm:flex-row sm:items-center'>
            <div className='border-border bg-muted/30 min-w-0 max-w-full flex-1 overflow-hidden rounded-md border px-3 py-2'>
              <p className='text-muted-foreground mb-1 text-[11px]'>
                {t('Preview URL')}
              </p>
              <p
                className='max-h-24 overflow-y-auto break-all whitespace-normal font-mono text-xs leading-relaxed'
                title={videoUrl}
              >
                {videoUrl}
              </p>
            </div>
            <div className='flex shrink-0 gap-2'>
              <Button
                variant='outline'
                size='sm'
                className='gap-1 text-xs'
                onClick={() => window.open(videoUrl, '_blank')}
              >
                <ExternalLink className='h-3 w-3' />
                {t('Open in new tab')}
              </Button>
              <Button
                variant='outline'
                size='sm'
                className='gap-1 text-xs'
                onClick={() => {
                  navigator.clipboard.writeText(videoUrl)
                  toast.success(t('Copied'))
                }}
              >
                <Copy className='h-3 w-3' />
                {t('Copy Link')}
              </Button>
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
