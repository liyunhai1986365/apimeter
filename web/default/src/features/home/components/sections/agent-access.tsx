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
import { useState } from 'react'
import {
  ArrowRight,
  Check,
  CircleCheck,
  Copy,
  Monitor,
  Sparkles,
  SquareTerminal,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  formatSystemTemplate,
  getAgentToolsURL,
  getCliDisplayName,
  getCliDocsURL,
  getCliInstallCommands,
  getCliScreenshotURL,
} from '@/lib/site-branding'
import { cn } from '@/lib/utils'
import { useSystemConfig } from '@/hooks/use-system-config'
import { Button } from '@/components/ui/button'
import { AnimateInView } from '@/components/animate-in-view'

const SUPPORTED_AGENT_TOOLS = [
  'Claude Code',
  'Codex CLI',
  'Gemini CLI',
  'OpenClaw',
]

export function AgentAccess() {
  const { t } = useTranslation()
  const { systemName, serverAddress } = useSystemConfig()
  const [installTarget, setInstallTarget] = useState<'unix' | 'windows'>('unix')
  const [copied, setCopied] = useState(false)
  const installCommands = getCliInstallCommands(serverAddress)
  const installCommand = installCommands[installTarget]
  const cliName = getCliDisplayName(systemName)

  const copyCommand = async () => {
    await navigator.clipboard?.writeText(installCommand)
    setCopied(true)
    window.setTimeout(() => setCopied(false), 1600)
  }

  return (
    <section className='relative z-10 px-6 py-16 md:py-24'>
      <div className='mx-auto grid max-w-6xl gap-10 lg:grid-cols-[minmax(0,0.95fr)_minmax(380px,1.05fr)] lg:items-center'>
        <AnimateInView className='max-w-3xl'>
          <div className='border-border/70 bg-background/80 text-muted-foreground inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-medium shadow-sm'>
            <SquareTerminal className='text-primary size-4' />
            {cliName}
          </div>

          <h2 className='mt-6 text-3xl leading-tight font-bold tracking-tight md:text-5xl'>
            {t('Configure mainstream Agent platforms in one command')}
          </h2>
          <p className='text-muted-foreground mt-5 max-w-2xl text-sm leading-relaxed md:text-base'>
            {formatSystemTemplate(
              t(
                'ModelSell CLI automatically writes API keys, API URLs, and default models into Claude Code, Codex CLI, Gemini CLI, and OpenClaw, saving repetitive configuration file edits.'
              ),
              systemName
            )}
          </p>

          <div className='border-border/70 bg-background mt-7 overflow-hidden rounded-2xl border shadow-sm'>
            <div className='bg-muted/40 flex flex-col gap-2 border-b p-2 sm:flex-row sm:items-center sm:justify-between'>
              <div
                className='flex gap-1'
                role='tablist'
                aria-label={t('Installation system')}
              >
                <button
                  type='button'
                  role='tab'
                  aria-selected={installTarget === 'unix'}
                  onClick={() => setInstallTarget('unix')}
                  className={cn(
                    'inline-flex items-center gap-2 rounded-md px-3 py-1.5 text-xs font-medium transition',
                    installTarget === 'unix'
                      ? 'bg-background text-foreground shadow-sm'
                      : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
                  )}
                >
                  <SquareTerminal className='size-4' />
                  macOS / Linux
                </button>
                <button
                  type='button'
                  role='tab'
                  aria-selected={installTarget === 'windows'}
                  onClick={() => setInstallTarget('windows')}
                  className={cn(
                    'inline-flex items-center gap-2 rounded-md px-3 py-1.5 text-xs font-medium transition',
                    installTarget === 'windows'
                      ? 'bg-background text-foreground shadow-sm'
                      : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
                  )}
                >
                  <Monitor className='size-4' />
                  Windows
                </button>
              </div>

              <Button
                type='button'
                variant='ghost'
                size='sm'
                onClick={copyCommand}
                aria-label={t('Copy command')}
              >
                {copied ? (
                  <Check className='size-4' />
                ) : (
                  <Copy className='size-4' />
                )}
                {copied ? t('Copied') : t('Copy')}
              </Button>
            </div>

            <pre className='overflow-x-auto bg-slate-950 p-4 text-xs leading-6 text-slate-100 md:text-sm'>
              <code>{installCommand}</code>
            </pre>
          </div>

          <div className='mt-8 flex flex-wrap gap-3'>
            <Button
              type='button'
              className='group'
              render={<a href={getCliDocsURL(serverAddress)} />}
            >
              {t('View CLI docs')}
              <ArrowRight className='size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
            </Button>
            <Button
              type='button'
              variant='outline'
              render={<a href={getAgentToolsURL(serverAddress)} />}
            >
              <Sparkles className='size-4' />
              {t('Browse Agent tools')}
            </Button>
          </div>

          <div className='mt-7 flex flex-wrap gap-2'>
            {SUPPORTED_AGENT_TOOLS.map((tool) => (
              <span
                key={tool}
                className='border-border/70 bg-background text-muted-foreground inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-sm'
              >
                <CircleCheck className='text-primary size-3.5' />
                {tool}
              </span>
            ))}
          </div>
        </AnimateInView>

        <AnimateInView animation='scale-in' className='relative'>
          <div
            aria-hidden='true'
            className='bg-primary/5 border-border/70 absolute -inset-4 rounded-3xl border'
          />
          <img
            src={getCliScreenshotURL(serverAddress)}
            alt={t('ModelSell CLI interactive configuration screen', {
              defaultValue: `${cliName} interactive configuration screen`,
            })}
            className='border-border/70 bg-background relative w-full rounded-2xl border shadow-2xl'
            loading='lazy'
          />
        </AnimateInView>
      </div>
    </section>
  )
}
