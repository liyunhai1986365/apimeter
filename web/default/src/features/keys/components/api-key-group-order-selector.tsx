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
import { useMemo, useState } from 'react'
import {
  ArrowDown,
  ArrowUp,
  Check,
  ChevronsUpDown,
  Plus,
  Trash2,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { addGroupToChain, removeGroupFromChain } from '../lib/api-key-form'
import { AUTO_GROUP_VALUE } from '../lib/api-key-groups'
import {
  GroupDescription,
  GroupRatioBadge,
  type ApiKeyGroupOption,
} from './api-key-group-combobox'

type ApiKeyGroupOrderSelectorProps = {
  options: ApiKeyGroupOption[]
  value: string[]
  onChange: (value: string[]) => void
  disabled?: boolean
}

function normalizeChain(value: string[]) {
  const seen = new Set<string>()
  const result: string[] = []
  for (const item of value) {
    const group = item.trim()
    if (!group || seen.has(group)) continue
    seen.add(group)
    result.push(group)
  }
  return result.length > 0 ? result : [AUTO_GROUP_VALUE]
}

export function ApiKeyGroupOrderSelector({
  options,
  value,
  onChange,
  disabled,
}: ApiKeyGroupOrderSelectorProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [searchValue, setSearchValue] = useState('')
  const selected = normalizeChain(value)
  const optionMap = useMemo(
    () => new Map(options.map((option) => [option.value, option])),
    [options]
  )
  const selectedSet = useMemo(() => new Set(selected), [selected])
  const availableOptions = useMemo(
    () => options.filter((option) => !selectedSet.has(option.value)),
    [options, selectedSet]
  )
  const filteredOptions = useMemo(() => {
    const search = searchValue.trim().toLowerCase()
    if (!search) return availableOptions

    return availableOptions.filter((option) => {
      const ratioText = String(option.ratio ?? '').toLowerCase()
      return (
        option.value.toLowerCase().includes(search) ||
        option.label.toLowerCase().includes(search) ||
        option.desc?.toLowerCase().includes(search) ||
        ratioText.includes(search)
      )
    })
  }, [availableOptions, searchValue])

  const emit = (next: string[]) => onChange(normalizeChain(next))

  const move = (index: number, direction: -1 | 1) => {
    const targetIndex = index + direction
    if (targetIndex < 0 || targetIndex >= selected.length) return
    const next = [...selected]
    const [item] = next.splice(index, 1)
    next.splice(targetIndex, 0, item)
    emit(next)
  }

  const remove = (index: number) => {
    emit(removeGroupFromChain(selected, index))
  }

  const add = (group: string) => {
    emit(addGroupToChain(selected, group))
    setOpen(false)
    setSearchValue('')
  }

  return (
    <div className='space-y-2'>
      <div className='space-y-2'>
        {selected.map((group, index) => {
          const option = optionMap.get(group) ?? {
            value: group,
            label: group,
            desc: group,
          }
          return (
            <div
              key={`${group}-${index}`}
              className='bg-muted/35 flex min-h-14 items-center gap-2 rounded-lg border px-2.5 py-2 sm:min-h-16 sm:px-3'
            >
              <Badge
                variant='secondary'
                className='bg-background text-muted-foreground h-6 min-w-7 justify-center rounded-md px-1.5 tabular-nums'
              >
                {index + 1}
              </Badge>
              <div className='min-w-0 flex-1'>
                <div className='flex min-w-0 items-center gap-2'>
                  <span className='truncate text-sm font-medium'>
                    {option.label}
                  </span>
                  <span className='hidden sm:block'>
                    <GroupRatioBadge ratio={option.ratio} />
                  </span>
                </div>
                <GroupDescription desc={option.desc} />
              </div>
              <div className='flex shrink-0 items-center gap-1'>
                <Button
                  type='button'
                  variant='ghost'
                  size='icon'
                  className='size-8'
                  disabled={disabled || index === 0}
                  onClick={() => move(index, -1)}
                  title={t('Move up')}
                >
                  <ArrowUp className='size-4' />
                </Button>
                <Button
                  type='button'
                  variant='ghost'
                  size='icon'
                  className='size-8'
                  disabled={disabled || index === selected.length - 1}
                  onClick={() => move(index, 1)}
                  title={t('Move down')}
                >
                  <ArrowDown className='size-4' />
                </Button>
                <Button
                  type='button'
                  variant='ghost'
                  size='icon'
                  className={cn(
                    'size-8',
                    selected.length === 1 &&
                      group === AUTO_GROUP_VALUE &&
                      'text-muted-foreground'
                  )}
                  disabled={
                    disabled ||
                    (selected.length === 1 && group === AUTO_GROUP_VALUE)
                  }
                  onClick={() => remove(index)}
                  title={t('Remove')}
                >
                  <Trash2 className='size-4' />
                </Button>
              </div>
            </div>
          )
        })}
      </div>

      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger
          render={
            <Button
              type='button'
              variant='outline'
              className='h-9 w-full justify-between rounded-lg border-dashed'
              disabled={disabled || availableOptions.length === 0}
            />
          }
        >
          <span className='flex items-center gap-2'>
            <Plus className='size-4' />
            {t('Add group')}
          </span>
          <ChevronsUpDown className='size-4 opacity-50' />
        </PopoverTrigger>
        <PopoverContent
          className='w-[var(--anchor-width)] overflow-hidden rounded-xl p-0 shadow-lg'
          onWheel={(event) => event.stopPropagation()}
          onTouchMove={(event) => event.stopPropagation()}
          onPointerDown={(event) => event.stopPropagation()}
        >
          <Command shouldFilter={false}>
            <CommandInput
              placeholder={t('Search...')}
              value={searchValue}
              onValueChange={setSearchValue}
            />
            <CommandList className='max-h-[320px]'>
              <CommandEmpty>{t('No group found.')}</CommandEmpty>
              <CommandGroup>
                {filteredOptions.map((option) => (
                  <CommandItem
                    key={option.value}
                    value={option.value}
                    onSelect={() => add(option.value)}
                    className='data-[selected=true]:bg-muted items-start gap-3 rounded-lg px-3 py-3 transition-colors'
                  >
                    <Check className='mt-0.5 size-4 opacity-0' />
                    <span className='min-w-0 flex-1'>
                      <span className='block truncate font-medium'>
                        {option.label}
                      </span>
                      <GroupDescription desc={option.desc} />
                    </span>
                    <GroupRatioBadge ratio={option.ratio} />
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </div>
  )
}
