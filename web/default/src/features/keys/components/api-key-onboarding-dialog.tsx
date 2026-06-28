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
import { useMemo, useState, type ReactNode } from 'react'
import {
  Eye,
  EyeOff,
  ExternalLink,
  Link2,
  LockKeyhole,
  Rocket,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { BundledLanguage } from 'shiki/bundle/web'
import { getLobeIcon } from '@/lib/lobe-icon'
import { getPublicServerAddress } from '@/lib/server-address'
import { getAgentToolsURL } from '@/lib/site-branding'
import { cn } from '@/lib/utils'
import { useStatus } from '@/hooks/use-status'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'
import { CopyButton } from '@/components/copy-button'

type Provider = 'openai' | 'anthropic' | 'gemini'
type Lang = 'curl' | 'python' | 'typescript' | 'javascript'

type ApiKeyOnboardingDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  apiKey: string
  apiKeyName?: string
}

const PROVIDER_LABELS: Record<Provider, string> = {
  openai: 'OpenAI',
  anthropic: 'Anthropic',
  gemini: 'Gemini',
}

const PROVIDER_STYLES: Record<
  Provider,
  {
    icon: string
    tone: string
    activeTone: string
  }
> = {
  openai: {
    icon: 'OpenAI.Color',
    tone: 'border-emerald-200 bg-emerald-50/60 hover:bg-emerald-50 dark:border-emerald-900/60 dark:bg-emerald-950/20',
    activeTone:
      'border-emerald-500 bg-emerald-50 ring-2 ring-emerald-500/20 dark:bg-emerald-950/40',
  },
  anthropic: {
    icon: 'Claude.Color',
    tone: 'border-amber-200 bg-amber-50/60 hover:bg-amber-50 dark:border-amber-900/60 dark:bg-amber-950/20',
    activeTone:
      'border-amber-500 bg-amber-50 ring-2 ring-amber-500/20 dark:bg-amber-950/40',
  },
  gemini: {
    icon: 'Gemini.Color',
    tone: 'border-sky-200 bg-sky-50/60 hover:bg-sky-50 dark:border-sky-900/60 dark:bg-sky-950/20',
    activeTone:
      'border-sky-500 bg-sky-50 ring-2 ring-sky-500/20 dark:bg-sky-950/40',
  },
}

const LANG_LABELS: Record<Lang, string> = {
  curl: 'curl',
  python: 'Python',
  typescript: 'TypeScript',
  javascript: 'JavaScript',
}

const LANG_HIGHLIGHT: Record<Lang, BundledLanguage> = {
  curl: 'bash',
  python: 'python',
  typescript: 'typescript',
  javascript: 'javascript',
}

function normalizeApiKey(apiKey: string): string {
  const trimmed = apiKey.trim()
  if (!trimmed) return ''
  return trimmed.startsWith('sk-') ? trimmed : `sk-${trimmed}`
}

function maskApiKey(apiKey: string): string {
  if (!apiKey) return ''
  if (apiKey.length <= 12) return `${apiKey.slice(0, 4)}********`
  return `${apiKey.slice(0, 7)}${'*'.repeat(18)}${apiKey.slice(-6)}`
}

function escapeDoubleQuoted(value: string): string {
  return value.replace(/\\/g, '\\\\').replace(/"/g, '\\"')
}

function singleQuoted(value: string): string {
  return `'${value.replace(/'/g, "\\'")}'`
}

function getProviderBaseUrl(provider: Provider, serverAddress: string): string {
  switch (provider) {
    case 'anthropic':
      return serverAddress
    case 'openai':
    case 'gemini':
    default:
      return `${serverAddress}/v1`
  }
}

function buildOpenAISample(lang: Lang, serverAddress: string, apiKey: string) {
  const baseUrl = getProviderBaseUrl('openai', serverAddress)
  switch (lang) {
    case 'curl':
      return [
        `curl ${singleQuoted(`${baseUrl}/chat/completions`)} \\`,
        `  -H ${singleQuoted(`Authorization: Bearer ${apiKey}`)} \\`,
        `  -H 'Content-Type: application/json' \\`,
        `  -d '${JSON.stringify(
          {
            model: 'gpt-5.5',
            messages: [{ role: 'user', content: 'Say hello in one sentence.' }],
          },
          null,
          2
        )}'`,
      ].join('\n')
    case 'python':
      return [
        'from openai import OpenAI',
        '',
        'client = OpenAI(',
        `    api_key="${escapeDoubleQuoted(apiKey)}",`,
        `    base_url="${escapeDoubleQuoted(baseUrl)}",`,
        ')',
        '',
        'response = client.chat.completions.create(',
        '    model="gpt-5.5",',
        '    messages=[{"role": "user", "content": "Say hello in one sentence."}],',
        ')',
        'print(response.choices[0].message.content)',
      ].join('\n')
    case 'typescript':
      return [
        "import OpenAI from 'openai'",
        '',
        'const client = new OpenAI({',
        `  apiKey: ${singleQuoted(apiKey)},`,
        `  baseURL: ${singleQuoted(baseUrl)},`,
        '})',
        '',
        'const response = await client.chat.completions.create({',
        "  model: 'gpt-5.5',",
        "  messages: [{ role: 'user', content: 'Say hello in one sentence.' }],",
        '})',
        '',
        'console.log(response.choices[0]?.message?.content)',
      ].join('\n')
    case 'javascript':
      return [
        "import OpenAI from 'openai'",
        '',
        'const client = new OpenAI({',
        `  apiKey: ${singleQuoted(apiKey)},`,
        `  baseURL: ${singleQuoted(baseUrl)},`,
        '})',
        '',
        'const response = await client.chat.completions.create({',
        "  model: 'gpt-5.5',",
        "  messages: [{ role: 'user', content: 'Say hello in one sentence.' }],",
        '})',
        '',
        'console.log(response.choices[0]?.message?.content)',
      ].join('\n')
  }
}

function buildAnthropicSample(
  lang: Lang,
  serverAddress: string,
  apiKey: string
) {
  const baseUrl = getProviderBaseUrl('anthropic', serverAddress)
  switch (lang) {
    case 'curl':
      return [
        `curl ${singleQuoted(`${baseUrl}/v1/messages`)} \\`,
        `  -H ${singleQuoted(`x-api-key: ${apiKey}`)} \\`,
        `  -H 'anthropic-version: 2023-06-01' \\`,
        `  -H 'Content-Type: application/json' \\`,
        `  -d '${JSON.stringify(
          {
            model: 'claude-sonnet-4-6',
            max_tokens: 256,
            messages: [{ role: 'user', content: 'Say hello in one sentence.' }],
          },
          null,
          2
        )}'`,
      ].join('\n')
    case 'python':
      return [
        'import requests',
        '',
        `response = requests.post(`,
        `    "${escapeDoubleQuoted(baseUrl)}/v1/messages",`,
        '    headers={',
        `        "x-api-key": "${escapeDoubleQuoted(apiKey)}",`,
        '        "anthropic-version": "2023-06-01",',
        '        "content-type": "application/json",',
        '    },',
        '    json={',
        '        "model": "claude-sonnet-4-6",',
        '        "max_tokens": 256,',
        '        "messages": [{"role": "user", "content": "Say hello in one sentence."}],',
        '    },',
        ')',
        'print(response.json())',
      ].join('\n')
    case 'typescript':
      return [
        "import Anthropic from '@anthropic-ai/sdk'",
        '',
        'const client = new Anthropic({',
        `  apiKey: ${singleQuoted(apiKey)},`,
        `  baseURL: ${singleQuoted(baseUrl)},`,
        '})',
        '',
        'const message = await client.messages.create({',
        "  model: 'claude-sonnet-4-6',",
        '  max_tokens: 256,',
        "  messages: [{ role: 'user', content: 'Say hello in one sentence.' }],",
        '})',
        '',
        'console.log(message.content)',
      ].join('\n')
    case 'javascript':
      return [
        "import Anthropic from '@anthropic-ai/sdk'",
        '',
        'const client = new Anthropic({',
        `  apiKey: ${singleQuoted(apiKey)},`,
        `  baseURL: ${singleQuoted(baseUrl)},`,
        '})',
        '',
        'const message = await client.messages.create({',
        "  model: 'claude-sonnet-4-6',",
        '  max_tokens: 256,',
        "  messages: [{ role: 'user', content: 'Say hello in one sentence.' }],",
        '})',
        '',
        'console.log(message.content)',
      ].join('\n')
  }
}

function buildGeminiSample(lang: Lang, serverAddress: string, apiKey: string) {
  const baseUrl = getProviderBaseUrl('gemini', serverAddress)
  const endpoint = `${baseUrl}/models/gemini-2.5-flash:generateContent`
  switch (lang) {
    case 'curl':
      return [
        `curl ${singleQuoted(endpoint)} \\`,
        `  -H ${singleQuoted(`x-goog-api-key: ${apiKey}`)} \\`,
        `  -H 'Content-Type: application/json' \\`,
        `  -d '${JSON.stringify(
          {
            contents: [
              {
                role: 'user',
                parts: [{ text: 'Say hello in one sentence.' }],
              },
            ],
          },
          null,
          2
        )}'`,
      ].join('\n')
    case 'python':
      return [
        'import requests',
        '',
        `response = requests.post(`,
        `    "${escapeDoubleQuoted(endpoint)}",`,
        '    headers={',
        `        "x-goog-api-key": "${escapeDoubleQuoted(apiKey)}",`,
        '        "content-type": "application/json",',
        '    },',
        '    json={',
        '        "contents": [',
        '            {',
        '                "role": "user",',
        '                "parts": [{"text": "Say hello in one sentence."}],',
        '            }',
        '        ]',
        '    },',
        ')',
        'print(response.json())',
      ].join('\n')
    case 'typescript':
      return [
        `const response = await fetch(${singleQuoted(endpoint)}, {`,
        "  method: 'POST',",
        '  headers: {',
        `    'x-goog-api-key': ${singleQuoted(apiKey)},`,
        "    'content-type': 'application/json',",
        '  },',
        '  body: JSON.stringify({',
        '    contents: [',
        "      { role: 'user', parts: [{ text: 'Say hello in one sentence.' }] },",
        '    ],',
        '  }),',
        '})',
        '',
        'const data: unknown = await response.json()',
        'console.log(data)',
      ].join('\n')
    case 'javascript':
      return [
        `const response = await fetch(${singleQuoted(endpoint)}, {`,
        "  method: 'POST',",
        '  headers: {',
        `    'x-goog-api-key': ${singleQuoted(apiKey)},`,
        "    'content-type': 'application/json',",
        '  },',
        '  body: JSON.stringify({',
        '    contents: [',
        "      { role: 'user', parts: [{ text: 'Say hello in one sentence.' }] },",
        '    ],',
        '  }),',
        '})',
        '',
        'const data = await response.json()',
        'console.log(data)',
      ].join('\n')
  }
}

function buildSample(
  provider: Provider,
  lang: Lang,
  serverAddress: string,
  apiKey: string
): string {
  switch (provider) {
    case 'anthropic':
      return buildAnthropicSample(lang, serverAddress, apiKey)
    case 'gemini':
      return buildGeminiSample(lang, serverAddress, apiKey)
    case 'openai':
    default:
      return buildOpenAISample(lang, serverAddress, apiKey)
  }
}

function CredentialRow(props: {
  icon: typeof Link2
  label: string
  value: string
  displayValue?: string
  secret?: boolean
  action?: ReactNode
}) {
  const { t } = useTranslation()
  const Icon = props.icon

  return (
    <div className='border-border/70 bg-background/80 grid gap-2 rounded-lg border p-3 sm:grid-cols-[8rem_minmax(0,1fr)_auto] sm:items-center'>
      <div className='text-muted-foreground flex items-center gap-2 text-xs font-medium'>
        <Icon className='size-4' />
        {props.label}
      </div>
      <code
        className={cn(
          'bg-muted min-w-0 rounded-md px-2 py-1.5 font-mono text-xs break-all',
          props.secret && 'text-primary'
        )}
      >
        {props.displayValue ?? props.value}
      </code>
      <div className='flex flex-col gap-2 sm:flex-row'>
        {props.action}
        <CopyButton
          value={props.value}
          variant='outline'
          size='sm'
          tooltip={t('Copy {{label}}', { label: props.label })}
          className='w-full gap-1.5 sm:w-auto'
        >
          <span className='text-xs'>{t('Copy')}</span>
        </CopyButton>
      </div>
    </div>
  )
}

function ProviderSwitch(props: {
  value: Provider
  onChange: (provider: Provider) => void
  serverAddress: string
}) {
  return (
    <div className='grid gap-2 md:grid-cols-3'>
      {(Object.keys(PROVIDER_LABELS) as Provider[]).map((provider) => {
        const meta = PROVIDER_STYLES[provider]
        const selected = props.value === provider
        const baseUrl = getProviderBaseUrl(provider, props.serverAddress)

        return (
          <button
            key={provider}
            type='button'
            onClick={() => props.onChange(provider)}
            className={cn(
              'min-w-0 rounded-lg border p-3 text-left transition-colors',
              meta.tone,
              selected && meta.activeTone
            )}
          >
            <span className='flex items-center gap-2.5'>
              <span className='bg-background/80 flex size-9 shrink-0 items-center justify-center rounded-md border'>
                {getLobeIcon(meta.icon, 22)}
              </span>
              <span className='min-w-0 flex-1 space-y-1'>
                <span className='block text-sm font-medium'>
                  {PROVIDER_LABELS[provider]}
                </span>
                <span className='text-muted-foreground flex min-w-0 items-center gap-1.5 text-xs'>
                  <Link2 className='size-3.5 shrink-0' />
                  <code className='truncate font-mono'>{baseUrl}</code>
                </span>
              </span>
            </span>
          </button>
        )
      })}
    </div>
  )
}

export function ApiKeyOnboardingDialog(props: ApiKeyOnboardingDialogProps) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const [provider, setProvider] = useState<Provider>('openai')
  const [showApiKey, setShowApiKey] = useState(false)
  const [lang, setLang] = useState<Lang>('curl')
  const apiKey = normalizeApiKey(props.apiKey)
  const sampleApiKey = showApiKey ? apiKey : '<YOUR_API_KEY>'
  const serverAddress = useMemo(
    () => getPublicServerAddress(status, 'https://api.example.com'),
    [status]
  )
  const baseUrl = getProviderBaseUrl(provider, serverAddress)
  const docsUrl = getAgentToolsURL(serverAddress)
  const code = buildSample(provider, lang, serverAddress, sampleApiKey)

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[calc(100dvh-2rem)] gap-0 overflow-hidden p-0 sm:max-w-3xl'>
        <DialogHeader className='border-b px-4 py-4 sm:px-5'>
          <div className='flex items-start gap-3'>
            <div className='bg-primary/10 text-primary border-primary/20 flex size-10 shrink-0 items-center justify-center rounded-lg border'>
              <Rocket className='size-5' />
            </div>
            <div className='min-w-0 space-y-1'>
              <div className='flex flex-wrap items-center gap-2'>
                <DialogTitle>{t('API key created')}</DialogTitle>
                {props.apiKeyName && (
                  <Badge variant='secondary' className='max-w-full truncate'>
                    {props.apiKeyName}
                  </Badge>
                )}
              </div>
              <DialogDescription>
                {t(
                  'Your key is hidden by default. Reveal it only when you are ready to copy or test the examples below.'
                )}
              </DialogDescription>
            </div>
          </div>
        </DialogHeader>

        <ScrollArea className='max-h-[calc(100dvh-12rem)]'>
          <div className='space-y-4 px-4 py-4 sm:px-5'>
            <div className='space-y-2'>
              <CredentialRow
                icon={LockKeyhole}
                label={t('Created API key')}
                value={apiKey}
                displayValue={showApiKey ? apiKey : maskApiKey(apiKey)}
                secret
                action={
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    className='w-full gap-1.5 sm:w-auto'
                    onClick={() => setShowApiKey((value) => !value)}
                  >
                    {showApiKey ? (
                      <EyeOff className='size-3.5' />
                    ) : (
                      <Eye className='size-3.5' />
                    )}
                    <span className='text-xs'>
                      {showApiKey ? t('Hide full key') : t('Show full key')}
                    </span>
                  </Button>
                }
              />
            </div>

            <div className='space-y-3'>
              <ProviderSwitch
                value={provider}
                onChange={setProvider}
                serverAddress={serverAddress}
              />

              <section className='bg-card rounded-lg border'>
                <div className='flex flex-col gap-3 border-b p-3 sm:flex-row sm:items-center sm:justify-between sm:p-4'>
                  <div className='min-w-0 space-y-1'>
                    <div className='text-sm font-medium'>
                      {t('Compatible request sample for {{provider}}', {
                        provider: PROVIDER_LABELS[provider],
                      })}
                    </div>
                    <div className='text-muted-foreground flex min-w-0 items-center gap-1.5 text-xs'>
                      <Link2 className='size-3.5 shrink-0' />
                      <code className='truncate font-mono'>{baseUrl}</code>
                    </div>
                  </div>
                  <Tabs
                    value={lang}
                    onValueChange={(value) => setLang(value as Lang)}
                  >
                    <TabsList className='grid h-auto grid-cols-4 p-1'>
                      {(Object.keys(LANG_LABELS) as Lang[]).map((item) => (
                        <TabsTrigger
                          key={item}
                          value={item}
                          className='px-2 py-1 text-[11px] sm:text-xs'
                        >
                          {LANG_LABELS[item]}
                        </TabsTrigger>
                      ))}
                    </TabsList>
                  </Tabs>
                </div>
                <div className='p-3 sm:p-4'>
                  <CodeBlock
                    code={code}
                    language={LANG_HIGHLIGHT[lang]}
                    className='max-h-80'
                  >
                    <CodeBlockCopyButton
                      size='icon-sm'
                      className='bg-background/85 size-7 border shadow-xs backdrop-blur'
                    />
                  </CodeBlock>
                </div>
              </section>
            </div>

            <div className='bg-muted/40 rounded-lg border p-4'>
              <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
                <div className='space-y-1'>
                  <h3 className='text-sm font-medium'>
                    {t('Need another integration path?')}
                  </h3>
                  <p className='text-muted-foreground text-xs leading-5'>
                    {t(
                      'View client app guides, SDK configuration notes, and additional provider-compatible examples in the documentation.'
                    )}
                  </p>
                </div>
                <Button
                  variant='outline'
                  className='gap-2 sm:shrink-0'
                  render={
                    <a
                      href={docsUrl}
                      target='_blank'
                      rel='noopener noreferrer'
                    />
                  }
                >
                  {t('More integration methods')}
                  <ExternalLink className='size-4' />
                </Button>
              </div>
            </div>
          </div>
        </ScrollArea>

        <DialogFooter className='items-stretch sm:items-center'>
          <Button onClick={() => props.onOpenChange(false)}>{t('Done')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
