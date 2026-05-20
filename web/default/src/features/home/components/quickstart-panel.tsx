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
import { Check, Copy } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@/components/ui/tabs'
import { CODE_SNIPPETS } from '../landing-data'

export function QuickstartPanel() {
  const { t } = useTranslation()
  const [activeSnippet, setActiveSnippet] = useState(CODE_SNIPPETS[0].id)
  const [copied, setCopied] = useState(false)
  const selectedSnippet =
    CODE_SNIPPETS.find((snippet) => snippet.id === activeSnippet) ??
    CODE_SNIPPETS[0]

  const copyCode = async () => {
    await navigator.clipboard?.writeText(selectedSnippet.code)
    setCopied(true)
    window.setTimeout(() => setCopied(false), 1600)
  }

  return (
    <div className='border-border/70 bg-background rounded-2xl border p-5 shadow-sm'>
      <div className='mb-5 flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between'>
        <div>
          <p className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
            {t('Quickstart')}
          </p>
          <h3 className='mt-1 text-lg font-semibold'>
            {t('Drop in one base URL')}
          </h3>
          <p className='text-muted-foreground mt-2 max-w-xl text-sm leading-relaxed'>
            {t(
              'Keep your existing SDKs and point them at a single OpenAI-compatible endpoint.'
            )}
          </p>
        </div>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={copyCode}
          aria-label={t('Copy code')}
        >
          {copied ? <Check className='size-4' /> : <Copy className='size-4' />}
          {copied ? t('Copied') : t('Copy')}
        </Button>
      </div>

      <Tabs value={activeSnippet} onValueChange={setActiveSnippet}>
        <TabsList className='mb-3'>
          {CODE_SNIPPETS.map((snippet) => (
            <TabsTrigger key={snippet.id} value={snippet.id}>
              {snippet.label}
            </TabsTrigger>
          ))}
        </TabsList>
        {CODE_SNIPPETS.map((snippet) => (
          <TabsContent key={snippet.id} value={snippet.id}>
            <pre className='max-h-[360px] overflow-auto rounded-xl bg-slate-950 p-4 text-xs leading-relaxed text-slate-100 shadow-inner'>
              <code>{snippet.code}</code>
            </pre>
          </TabsContent>
        ))}
      </Tabs>
    </div>
  )
}
