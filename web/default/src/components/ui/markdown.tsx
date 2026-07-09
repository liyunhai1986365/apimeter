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
import ReactMarkdown from 'react-markdown'
import rehypeRaw from 'rehype-raw'
import remarkGfm from 'remark-gfm'
import { cn } from '@/lib/utils'

interface MarkdownProps {
  children: string
  className?: string
}

export function Markdown({ children, className }: MarkdownProps) {
  return (
    <div
      className={cn(
        'max-w-none text-sm [overflow-wrap:anywhere] break-words',
        '[&>*:first-child]:mt-0 [&>*:last-child]:mb-0',
        className
      )}
    >
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[rehypeRaw]}
        components={{
          h1: ({ node, className, ...props }) => (
            <h1
              className={cn(
                'mt-5 mb-3 text-2xl leading-tight font-semibold tracking-tight',
                className
              )}
              {...props}
            />
          ),
          h2: ({ node, className, ...props }) => (
            <h2
              className={cn(
                'mt-5 mb-2.5 text-xl leading-tight font-semibold tracking-tight',
                className
              )}
              {...props}
            />
          ),
          h3: ({ node, className, ...props }) => (
            <h3
              className={cn(
                'mt-4 mb-2 text-lg leading-snug font-semibold tracking-tight',
                className
              )}
              {...props}
            />
          ),
          h4: ({ node, className, ...props }) => (
            <h4
              className={cn(
                'mt-4 mb-2 text-base leading-snug font-semibold',
                className
              )}
              {...props}
            />
          ),
          p: ({ node, className, ...props }) => (
            <p
              className={cn('text-foreground my-2 leading-relaxed', className)}
              {...props}
            />
          ),
          a: ({ node, className, ...props }) => (
            <a
              {...props}
              className={cn(
                'text-primary underline-offset-4 hover:underline',
                className
              )}
              target='_blank'
              rel='noopener noreferrer'
            />
          ),
          ul: ({ node, className, ...props }) => (
            <ul
              className={cn('my-2 list-disc space-y-1 pl-5', className)}
              {...props}
            />
          ),
          ol: ({ node, className, ...props }) => (
            <ol
              className={cn('my-2 list-decimal space-y-1 pl-5', className)}
              {...props}
            />
          ),
          li: ({ node, className, ...props }) => (
            <li className={cn('leading-relaxed', className)} {...props} />
          ),
          blockquote: ({ node, className, ...props }) => (
            <blockquote
              className={cn(
                'border-l-primary bg-muted/50 text-muted-foreground my-3 border-l-2 py-1 pr-3 pl-4',
                className
              )}
              {...props}
            />
          ),
          code: ({ node, className, ...props }) => (
            <code
              className={cn(
                'bg-muted rounded px-1 py-0.5 text-[0.875em]',
                className
              )}
              {...props}
            />
          ),
          pre: ({ node, className, ...props }) => (
            <pre
              className={cn(
                'bg-muted my-3 overflow-x-auto rounded-md border p-3 text-sm',
                className
              )}
              {...props}
            />
          ),
          table: ({ node, className, ...props }) => (
            <div className='my-3 overflow-x-auto'>
              <table
                className={cn(
                  'w-full border-collapse border text-sm',
                  className
                )}
                {...props}
              />
            </div>
          ),
          th: ({ node, className, ...props }) => (
            <th
              className={cn(
                'bg-muted border px-3 py-2 text-left font-semibold',
                className
              )}
              {...props}
            />
          ),
          td: ({ node, className, ...props }) => (
            <td className={cn('border px-3 py-2', className)} {...props} />
          ),
          img: ({ node, className, ...props }) => (
            <img
              className={cn('my-3 rounded-lg shadow-sm', className)}
              {...props}
            />
          ),
        }}
      >
        {children}
      </ReactMarkdown>
    </div>
  )
}
