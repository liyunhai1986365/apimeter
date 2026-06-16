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
import { Code2, Plus, Table2, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { ENDPOINT_TEMPLATES } from '../constants'
import {
  normalizeEndpointConfig,
  serializeEndpointConfig,
  type EndpointConfigRow,
} from '../lib/endpoint-config'

type EndpointConfigEditorProps = {
  value: string
  onChange: (value: string) => void
  disabled?: boolean
}

type EndpointOption = {
  value: string
  label: string
}

const CUSTOM_TYPE = '__custom__'
const DEFAULT_CUSTOM_ENDPOINT: EndpointConfigRow = {
  type: '',
  path: '',
  method: 'POST',
  label: '',
  docs_url: '',
}

function getEndpointOptionLabel(type: string, t: (key: string) => string) {
  const labels: Record<string, string> = {
    openai: t('OpenAI Chat'),
    'openai-response': t('OpenAI Responses'),
    'openai-response-compact': t('OpenAI Responses Compact'),
    anthropic: 'Anthropic',
    gemini: 'Gemini',
    'jina-rerank': t('Jina Rerank'),
    'image-generation': t('OpenAI Image Generation'),
    'openai-image-edit': t('OpenAI Image Edit'),
    'gemini-image-generation': t('Gemini Native Image Generation'),
    embeddings: t('Embeddings'),
    'openai-video': t('OpenAI Video'),
  }
  return labels[type] ?? type
}

function makeRowFromTemplate(type: string): EndpointConfigRow {
  const template = ENDPOINT_TEMPLATES[type]
  if (!template) return { ...DEFAULT_CUSTOM_ENDPOINT, type }
  return {
    type,
    path: template.path,
    method: template.method,
    label: template.label ?? '',
    docs_url: template.docs_url ?? '',
  }
}

function toDraftRows(value: string): EndpointConfigRow[] {
  return normalizeEndpointConfig(value)
}

export function EndpointConfigEditor({
  value,
  onChange,
  disabled = false,
}: EndpointConfigEditorProps) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<'visual' | 'json'>('visual')
  const [rows, setRows] = useState<EndpointConfigRow[]>(() =>
    toDraftRows(value)
  )
  const [jsonValue, setJsonValue] = useState(value)

  const options = useMemo<EndpointOption[]>(
    () =>
      Object.keys(ENDPOINT_TEMPLATES).map((type) => ({
        value: type,
        label: getEndpointOptionLabel(type, t),
      })),
    [t]
  )

  useEffect(() => {
    if (value === jsonValue) return
    setJsonValue(value)
    setRows(toDraftRows(value))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value])

  const emitRows = (nextRows: EndpointConfigRow[]) => {
    const nextJson = serializeEndpointConfig(nextRows)
    setRows(nextRows)
    setJsonValue(nextJson)
    onChange(nextJson)
  }

  const handleAddEndpoint = (type: string) => {
    if (!type) return
    const nextRow =
      type === CUSTOM_TYPE
        ? { ...DEFAULT_CUSTOM_ENDPOINT }
        : makeRowFromTemplate(type)
    emitRows([...rows.filter((row) => row.type !== nextRow.type), nextRow])
  }

  const handleRowChange = (
    index: number,
    field: keyof EndpointConfigRow,
    fieldValue: string
  ) => {
    const nextRows = rows.map((row, rowIndex) =>
      rowIndex === index ? { ...row, [field]: fieldValue } : row
    )
    emitRows(nextRows)
  }

  const handleDeleteRow = (index: number) => {
    emitRows(rows.filter((_, rowIndex) => rowIndex !== index))
  }

  const handleJsonChange = (nextValue: string) => {
    setJsonValue(nextValue)
    setRows(toDraftRows(nextValue))
    onChange(nextValue)
  }

  const toggleMode = () => {
    if (mode === 'visual') {
      setMode('json')
      return
    }
    setRows(toDraftRows(jsonValue))
    setMode('visual')
  }

  return (
    <div className='space-y-3'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={toggleMode}
          disabled={disabled}
        >
          {mode === 'visual' ? (
            <>
              <Code2 className='mr-2 size-4' />
              {t('JSON Mode')}
            </>
          ) : (
            <>
              <Table2 className='mr-2 size-4' />
              {t('Visual Mode')}
            </>
          )}
        </Button>

        {mode === 'visual' && (
          <Select<string>
            items={[
              ...options,
              { value: CUSTOM_TYPE, label: t('Custom endpoint') },
            ]}
            onValueChange={(nextValue) => {
              if (nextValue !== null) handleAddEndpoint(nextValue)
            }}
          >
            <SelectTrigger size='sm' className='w-[230px]'>
              <SelectValue placeholder={t('Add supported API...')} />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                {options.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
                <SelectItem value={CUSTOM_TYPE}>
                  {t('Custom endpoint')}
                </SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        )}
      </div>

      {mode === 'json' ? (
        <Textarea
          value={jsonValue}
          onChange={(event) => handleJsonChange(event.target.value)}
          placeholder={JSON.stringify(
            {
              'openai-image-edit': ENDPOINT_TEMPLATES['openai-image-edit'],
            },
            null,
            2
          )}
          disabled={disabled}
          rows={10}
          className='font-mono text-sm'
        />
      ) : rows.length > 0 ? (
        <div className='space-y-2'>
          <div className='text-muted-foreground hidden grid-cols-[1.2fr_1.3fr_88px_1fr_1.2fr_40px] gap-2 text-xs font-medium lg:grid'>
            <span>{t('API type')}</span>
            <span>{t('Path')}</span>
            <span>{t('Method')}</span>
            <span>{t('Display name')}</span>
            <span>{t('Docs URL')}</span>
            <span />
          </div>
          {rows.map((row, index) => (
            <div
              key={`${row.type}-${index}`}
              className='grid gap-2 rounded-lg border p-2 lg:grid-cols-[1.2fr_1.3fr_88px_1fr_1.2fr_40px] lg:items-center'
            >
              <Input
                value={row.type}
                onChange={(event) =>
                  handleRowChange(index, 'type', event.target.value)
                }
                placeholder={t('custom-api-type')}
                disabled={disabled}
              />
              <Input
                value={row.path}
                onChange={(event) =>
                  handleRowChange(index, 'path', event.target.value)
                }
                placeholder='/v1/...'
                disabled={disabled}
              />
              <Input
                value={row.method}
                onChange={(event) =>
                  handleRowChange(index, 'method', event.target.value)
                }
                placeholder='POST'
                disabled={disabled}
              />
              <Input
                value={row.label}
                onChange={(event) =>
                  handleRowChange(index, 'label', event.target.value)
                }
                placeholder={getEndpointOptionLabel(row.type, t)}
                disabled={disabled}
              />
              <Input
                value={row.docs_url}
                onChange={(event) =>
                  handleRowChange(index, 'docs_url', event.target.value)
                }
                placeholder='https://docs.example.com/...'
                disabled={disabled}
              />
              <Button
                type='button'
                variant='ghost'
                size='icon'
                aria-label={t('Delete endpoint')}
                onClick={() => handleDeleteRow(index)}
                disabled={disabled}
                className='h-10 w-10 justify-self-end'
              >
                <Trash2 className='size-4' aria-hidden='true' />
              </Button>
            </div>
          ))}
        </div>
      ) : (
        <div className='text-muted-foreground flex h-24 items-center justify-center rounded-md border border-dashed text-sm'>
          {t('No endpoints configured. Add supported APIs to define endpoints.')}
        </div>
      )}

      {mode === 'visual' && (
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={() => handleAddEndpoint(CUSTOM_TYPE)}
          disabled={disabled}
          className='w-full'
        >
          <Plus className='mr-2 size-4' />
          {t('Add custom endpoint')}
        </Button>
      )}
    </div>
  )
}
