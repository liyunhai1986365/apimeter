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
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Check, Copy, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import { getErrorRequestLog } from '../../api'
import type { ErrorRequestLog } from '../../types'

interface ErrorRequestLogDialogProps {
  logId: number | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

function formatJsonText(value: string): string {
  if (!value) return ''

  try {
    return JSON.stringify(JSON.parse(value), null, 2)
  } catch {
    return value
  }
}

function EvidenceField({
  label,
  value,
}: {
  label: string
  value: string | number | boolean | null | undefined
}) {
  return (
    <div className='flex min-w-0 flex-col gap-1'>
      <Label className='text-muted-foreground text-xs'>{label}</Label>
      <div className='text-foreground bg-muted/30 min-h-7 truncate rounded-md border px-2 py-1.5 font-mono text-xs'>
        {value === undefined || value === null || value === ''
          ? '-'
          : String(value)}
      </div>
    </div>
  )
}

function EvidenceBlock({
  label,
  value,
  copiedText,
  onCopy,
}: {
  label: string
  value: string
  copiedText: string | null
  onCopy: (value: string) => void
}) {
  const displayValue = value || '-'

  return (
    <div className='flex flex-col gap-2'>
      <div className='flex items-center justify-between gap-2'>
        <Label className='text-sm font-semibold'>{label}</Label>
        <Button
          variant='ghost'
          size='icon-sm'
          onClick={() => onCopy(displayValue)}
          title={label}
          disabled={!value}
        >
          {copiedText === displayValue ? <Check /> : <Copy />}
        </Button>
      </div>
      <pre className='bg-muted/50 text-foreground max-h-64 overflow-auto rounded-md border p-3 font-mono text-xs leading-relaxed break-all whitespace-pre-wrap'>
        {displayValue}
      </pre>
    </div>
  )
}

export function ErrorRequestLogDialog({
  logId,
  open,
  onOpenChange,
}: ErrorRequestLogDialogProps) {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })
  const [requestLog, setRequestLog] = useState<ErrorRequestLog | null>(null)
  const [isLoading, setIsLoading] = useState(false)

  const fetchRequestLog = useCallback(
    async (id: number) => {
      setIsLoading(true)
      try {
        const result = await getErrorRequestLog(id)
        if (result.success) {
          setRequestLog(result.data || null)
        } else {
          toast.error(result.message || t('Failed to fetch error request log'))
        }
      } catch (error) {
        // eslint-disable-next-line no-console
        console.error('Failed to fetch error request log:', error)
        toast.error(t('Failed to fetch error request log'))
      } finally {
        setIsLoading(false)
      }
    },
    [t]
  )

  useEffect(() => {
    if (open && logId) {
      fetchRequestLog(logId)
    }
    if (!open) {
      setRequestLog(null)
    }
  }, [fetchRequestLog, logId, open])

  const requestHeaders = useMemo(
    () => formatJsonText(requestLog?.request_headers || ''),
    [requestLog?.request_headers]
  )
  const requestBody = useMemo(
    () => formatJsonText(requestLog?.request_body || ''),
    [requestLog?.request_body]
  )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-4xl'>
        <DialogHeader>
          <DialogTitle>{t('Error Request Evidence')}</DialogTitle>
          <DialogDescription>
            {t(
              'Review the independently stored failed request snapshot for retry routing analysis.'
            )}
          </DialogDescription>
        </DialogHeader>

        {isLoading ? (
          <div className='flex items-center justify-center py-10'>
            <Loader2 className='text-muted-foreground size-6 animate-spin' />
          </div>
        ) : requestLog ? (
          <ScrollArea className='max-h-[70vh] pr-4'>
            <div className='flex flex-col gap-5 py-2'>
              <div className='grid grid-cols-1 gap-3 md:grid-cols-3'>
                <EvidenceField label={t('Log ID')} value={requestLog.log_id} />
                <EvidenceField
                  label={t('Request ID')}
                  value={requestLog.request_id}
                />
                <EvidenceField
                  label={t('Upstream Request ID')}
                  value={requestLog.upstream_request_id}
                />
                <EvidenceField
                  label={t('Method')}
                  value={requestLog.request_method}
                />
                <EvidenceField
                  label={t('Request Path')}
                  value={requestLog.request_path}
                />
                <EvidenceField
                  label={t('Status Code')}
                  value={requestLog.status_code}
                />
                <EvidenceField
                  label={t('Model')}
                  value={requestLog.model_name}
                />
                <EvidenceField label={t('Group')} value={requestLog.group} />
                <EvidenceField
                  label={t('Stream')}
                  value={requestLog.is_stream ? t('Yes') : t('No')}
                />
                <EvidenceField
                  label={t('Username')}
                  value={requestLog.username}
                />
                <EvidenceField
                  label={t('Token Name')}
                  value={requestLog.token_name}
                />
                <EvidenceField
                  label={t('Content Length')}
                  value={requestLog.content_length}
                />
                <EvidenceField
                  label={t('Error Type')}
                  value={requestLog.error_type}
                />
                <EvidenceField
                  label={t('Error Code')}
                  value={requestLog.error_code}
                />
                <EvidenceField
                  label={t('Request Truncated')}
                  value={requestLog.request_truncated ? t('Yes') : t('No')}
                />
              </div>

              <EvidenceField
                label={t('Request URL')}
                value={requestLog.request_url}
              />
              <EvidenceField
                label={t('Request Hash')}
                value={requestLog.request_hash}
              />

              <EvidenceBlock
                label={t('Request Headers')}
                value={requestHeaders}
                copiedText={copiedText}
                onCopy={copyToClipboard}
              />
              <EvidenceBlock
                label={t('Request Body')}
                value={requestBody}
                copiedText={copiedText}
                onCopy={copyToClipboard}
              />
            </div>
          </ScrollArea>
        ) : (
          <div className='text-muted-foreground py-8 text-center text-sm'>
            {t('No error request log found')}
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
