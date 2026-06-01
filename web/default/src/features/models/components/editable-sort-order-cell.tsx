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
import { useEffect, useRef, useState } from 'react'
import { Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useQueryClient } from '@tanstack/react-query'
import { cn } from '@/lib/utils'
import { Input } from '@/components/ui/input'
import { StatusBadge } from '@/components/status-badge'
import { updateModelSortOrder } from '../api'
import { modelsQueryKeys } from '../lib'
import { normalizeSortOrderInput } from '../lib/sort-order-editor'

interface EditableSortOrderCellProps {
  modelId: number
  value?: number
}

export function EditableSortOrderCell(props: EditableSortOrderCellProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const inputRef = useRef<HTMLInputElement | null>(null)
  const currentValue = props.value ?? 0
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(String(currentValue))
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!editing) {
      setDraft(String(currentValue))
    }
  }, [currentValue, editing])

  useEffect(() => {
    if (!editing) return
    inputRef.current?.focus()
    inputRef.current?.select()
  }, [editing])

  const cancelEdit = () => {
    setDraft(String(currentValue))
    setEditing(false)
  }

  const commitEdit = async () => {
    const normalizedValue = normalizeSortOrderInput(draft)
    if (normalizedValue == null) {
      toast.error(t('Sort order must be a non-negative number'))
      cancelEdit()
      return
    }

    if (normalizedValue === currentValue) {
      setEditing(false)
      setDraft(String(normalizedValue))
      return
    }

    setSaving(true)
    try {
      const response = await updateModelSortOrder([
        { id: props.modelId, sort_order: normalizedValue },
      ])
      if (response.success) {
        toast.success(t('Model sort order saved'))
        setDraft(String(normalizedValue))
        setEditing(false)
        queryClient.invalidateQueries({ queryKey: modelsQueryKeys.lists() })
        queryClient.invalidateQueries({ queryKey: ['pricing'] })
      } else {
        toast.error(response.message || t('Failed to save model sort order'))
        cancelEdit()
      }
    } catch (error) {
      toast.error(
        (error as Error)?.message || t('Failed to save model sort order')
      )
      cancelEdit()
    } finally {
      setSaving(false)
    }
  }

  if (editing) {
    return (
      <div className='flex h-7 w-20 items-center'>
        <Input
          ref={inputRef}
          type='number'
          min={0}
          step={1}
          inputMode='numeric'
          value={draft}
          disabled={saving}
          aria-label={t('Model sort order')}
          onChange={(event) => setDraft(event.target.value)}
          onBlur={commitEdit}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault()
              event.currentTarget.blur()
            } else if (event.key === 'Escape') {
              event.preventDefault()
              cancelEdit()
            }
          }}
          className='h-7 w-20 px-2 font-mono text-xs'
        />
        <Loader2
          className={cn(
            'text-muted-foreground ml-1 size-3 animate-spin',
            !saving && 'invisible'
          )}
        />
      </div>
    )
  }

  return (
    <button
      type='button'
      onDoubleClick={() => setEditing(true)}
      title={t('Double-click to edit')}
      className='cursor-text rounded-md focus-visible:ring-ring/50 focus-visible:ring-2 focus-visible:outline-none'
    >
      <StatusBadge
        label={String(currentValue)}
        variant='neutral'
        size='sm'
        copyable={false}
        className='font-mono'
      />
    </button>
  )
}
