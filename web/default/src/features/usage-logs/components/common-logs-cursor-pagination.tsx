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
import { ArrowLeft01Icon, ArrowRight01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

const PAGE_SIZE_OPTIONS = [10, 20, 30, 40, 50, 100]

interface CommonLogsCursorPaginationProps {
  pageIndex: number
  pageSize: number
  hasMore: boolean
  isFetching: boolean
  onPrevious: () => void
  onNext: () => void
  onPageSizeChange: (pageSize: number) => void
}

export function CommonLogsCursorPagination({
  pageIndex,
  pageSize,
  hasMore,
  isFetching,
  onPrevious,
  onNext,
  onPageSizeChange,
}: CommonLogsCursorPaginationProps) {
  const { t } = useTranslation()

  return (
    <div className='flex items-center justify-between gap-3'>
      <div className='flex items-center gap-2'>
        <Select
          items={PAGE_SIZE_OPTIONS.map((value) => ({
            value: String(value),
            label: value,
          }))}
          value={String(pageSize)}
          onValueChange={(value) => onPageSizeChange(Number(value))}
        >
          <SelectTrigger className='h-8 w-[64px] sm:w-[70px]'>
            <SelectValue placeholder={pageSize} />
          </SelectTrigger>
          <SelectContent side='top' alignItemWithTrigger={false}>
            <SelectGroup>
              {PAGE_SIZE_OPTIONS.map((value) => (
                <SelectItem key={value} value={String(value)}>
                  {value}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <span className='text-sm font-medium'>{t('Rows per page')}</span>
      </div>

      <div className='flex items-center gap-2'>
        <span className='text-sm font-medium whitespace-nowrap'>
          {t('Page {{current}}', { current: pageIndex + 1 })}
        </span>
        <Button
          variant='outline'
          size='icon-sm'
          disabled={pageIndex === 0 || isFetching}
          onClick={onPrevious}
          aria-label={t('Previous')}
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} data-icon='inline-start' />
        </Button>
        <Button
          variant='outline'
          size='icon-sm'
          disabled={!hasMore || isFetching}
          onClick={onNext}
          aria-label={t('Next')}
        >
          <HugeiconsIcon icon={ArrowRight01Icon} data-icon='inline-end' />
        </Button>
      </div>
    </div>
  )
}
