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
import { Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

type AgentUsersListControlsProps = {
  keyword: string
  page: number
  pageSize: number
  total: number
  itemCount: number
  isLoading?: boolean
  onKeywordChange: (value: string) => void
  onSearch: () => void
  onClear: () => void
  onPageChange: (page: number) => void
  onPageSizeChange: (pageSize: number) => void
}

type AgentUsersSearchControlsProps = Pick<
  AgentUsersListControlsProps,
  'keyword' | 'isLoading' | 'onKeywordChange' | 'onSearch' | 'onClear'
>

type AgentUsersPaginationControlsProps = Pick<
  AgentUsersListControlsProps,
  | 'page'
  | 'pageSize'
  | 'total'
  | 'itemCount'
  | 'isLoading'
  | 'onPageChange'
  | 'onPageSizeChange'
>

export function AgentUsersSearchControls(
  props: AgentUsersSearchControlsProps
) {
  const { t } = useTranslation()

  return (
    <div className='mb-3 grid gap-2 rounded-lg bg-muted/30 p-2 sm:grid-cols-[minmax(0,280px)_auto_auto]'>
      <Input
        value={props.keyword}
        onChange={(event) => props.onKeywordChange(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === 'Enter') {
            props.onSearch()
          }
        }}
        placeholder={t('Search by user ID, username, email, or display name')}
      />
      <Button
        variant='outline'
        disabled={props.isLoading}
        onClick={props.onSearch}
      >
        <Search />
        {t('Search')}
      </Button>
      <Button
        variant='ghost'
        disabled={props.isLoading || props.keyword.trim() === ''}
        onClick={props.onClear}
      >
        {t('Clear')}
      </Button>
    </div>
  )
}

export function AgentUsersPaginationControls(
  props: AgentUsersPaginationControlsProps
) {
  const { t } = useTranslation()
  const currentPage = Math.max(1, props.page)
  const totalPages = Math.max(1, Math.ceil(props.total / props.pageSize))
  const start = props.total === 0 ? 0 : (currentPage - 1) * props.pageSize + 1
  const end = props.total === 0 ? 0 : start + Math.max(0, props.itemCount - 1)
  const canGoPrevious = currentPage > 1 && !props.isLoading
  const canGoNext =
    currentPage < totalPages && props.itemCount > 0 && !props.isLoading

  return (
    <div className='mt-3 flex flex-wrap items-center justify-end gap-2 border-t pt-3 text-xs'>
      <span className='text-muted-foreground mr-auto'>
        {t('Showing {{start}}-{{end}} of {{total}}', {
          start,
          end,
          total: props.total,
        })}
      </span>
      <label className='text-muted-foreground flex items-center gap-2'>
        {t('Rows per page')}
        <select
          className='border-input bg-background h-8 rounded-md border px-2 text-xs'
          value={props.pageSize}
          disabled={props.isLoading}
          onChange={(event) => props.onPageSizeChange(Number(event.target.value))}
        >
          {PAGE_SIZE_OPTIONS.map((size) => (
            <option key={size} value={size}>
              {size}
            </option>
          ))}
        </select>
      </label>
      <span className='text-muted-foreground'>
        {t('Page {{current}} of {{total}}', {
          current: currentPage,
          total: totalPages,
        })}
      </span>
      <Button
        variant='outline'
        size='sm'
        disabled={!canGoPrevious}
        onClick={() => props.onPageChange(currentPage - 1)}
      >
        {t('Previous')}
      </Button>
      <Button
        variant='outline'
        size='sm'
        disabled={!canGoNext}
        onClick={() => props.onPageChange(currentPage + 1)}
      >
        {t('Next')}
      </Button>
    </div>
  )
}
