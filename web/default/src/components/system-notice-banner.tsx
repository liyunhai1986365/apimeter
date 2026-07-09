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
import { Megaphone, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Markdown } from '@/components/ui/markdown'

interface SystemNoticeBannerProps {
  notice: string
  hidden?: boolean
  fixed?: boolean
  topClassName?: string
  onCloseToday: () => void
}

export function SystemNoticeBanner({
  notice,
  hidden = false,
  fixed = false,
  topClassName,
  onCloseToday,
}: SystemNoticeBannerProps) {
  const { t } = useTranslation()
  const content = notice.trim()

  if (!content || hidden) return null

  return (
    <div
      className={cn(
        'h-[var(--system-notice-banner-height,2.5rem)] border-b border-white/45 bg-[linear-gradient(112deg,lab(96.4676%_-4.00505_-14.1008/.92)_0%,lab(93.1283%_19.7665_-20.9078/.82)_38%,lab(95.8075%_2.12398_49.9707/.78)_74%,lab(94.176%_-22.1378_29.2661/.62)_100%)] text-slate-900 shadow-[0_12px_32px_-24px_rgba(15,23,42,0.32)] dark:bg-[linear-gradient(112deg,lab(96.4676%_-4.00505_-14.1008/.92)_0%,lab(93.1283%_19.7665_-20.9078/.82)_38%,lab(95.8075%_2.12398_49.9707/.78)_74%,lab(94.176%_-22.1378_29.2661/.62)_100%)] dark:text-slate-900',
        fixed && 'fixed inset-x-0 top-0 z-[55]',
        fixed && topClassName
      )}
    >
      <div className='mx-auto flex h-full max-w-7xl items-center justify-center px-4 md:px-6'>
        <div className='flex max-w-full min-w-0 items-center justify-center gap-2'>
          <div className='flex size-7 shrink-0 items-center justify-center rounded-full bg-white/45 ring-1 ring-slate-900/8'>
            <Megaphone className='size-4 text-slate-800' />
          </div>
          <div className='min-w-0 overflow-hidden'>
            <Markdown className='line-clamp-1 text-center text-xs font-medium text-slate-900 sm:text-sm [&_*]:border-slate-900/20 [&_a]:text-slate-950 [&_a]:underline-offset-4 [&_blockquote]:my-0 [&_blockquote]:inline [&_blockquote]:border-slate-900/30 [&_blockquote]:bg-transparent [&_blockquote]:py-0 [&_blockquote]:pl-2 [&_blockquote]:text-slate-800 [&_code]:bg-white/55 [&_code]:text-slate-950 [&_h1]:my-0 [&_h1]:inline [&_h1]:text-sm [&_h1]:leading-none [&_h1]:font-semibold [&_h1]:text-slate-950 [&_h2]:my-0 [&_h2]:inline [&_h2]:text-sm [&_h2]:leading-none [&_h2]:font-semibold [&_h2]:text-slate-950 [&_h3]:my-0 [&_h3]:inline [&_h3]:text-sm [&_h3]:leading-none [&_h3]:font-semibold [&_h3]:text-slate-950 [&_h4]:my-0 [&_h4]:inline [&_h4]:text-sm [&_h4]:leading-none [&_h4]:font-semibold [&_h4]:text-slate-950 [&_li]:inline [&_li]:leading-none [&_ol]:my-0 [&_ol]:inline [&_ol]:pl-0 [&_p]:my-0 [&_p]:inline [&_p]:leading-none [&_p]:text-slate-900 [&_pre]:my-0 [&_pre]:inline [&_pre]:border-slate-900/20 [&_pre]:bg-white/55 [&_ul]:my-0 [&_ul]:inline [&_ul]:pl-0'>
              {content}
            </Markdown>
          </div>
          <button
            type='button'
            className='inline-flex size-7 shrink-0 items-center justify-center rounded-full text-slate-800 transition-colors hover:bg-white/55 hover:text-slate-950'
            onClick={onCloseToday}
            aria-label={t('Close')}
          >
            <X className='size-4' />
          </button>
        </div>
      </div>
    </div>
  )
}
