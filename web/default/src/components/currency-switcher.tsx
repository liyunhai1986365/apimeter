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
import { DollarSign } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  useSystemConfigStore,
  type UserCurrencyDisplayType,
} from '@/stores/system-config-store'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

const CURRENCY_OPTIONS: {
  value: UserCurrencyDisplayType
  label: string
  symbol: string
}[] = [
  { value: 'USD', label: 'USD', symbol: '$' },
  { value: 'CNY', label: 'CNY', symbol: '¥' },
]

type CurrencySwitcherProps = {
  className?: string
}

export function CurrencySwitcher({ className }: CurrencySwitcherProps) {
  const { t } = useTranslation()
  const displayCurrency = useSystemConfigStore((s) => s.displayCurrency)
  const setDisplayCurrency = useSystemConfigStore((s) => s.setDisplayCurrency)
  const mainlandChinaPresentationEnabled = useSystemConfigStore(
    (s) => s.config.mainlandChinaPresentationEnabled
  )

  if (mainlandChinaPresentationEnabled) return null

  const activeCurrency =
    CURRENCY_OPTIONS.find((option) => option.value === displayCurrency) ??
    CURRENCY_OPTIONS[0]

  const handleChangeCurrency = (currency: UserCurrencyDisplayType) => {
    if (currency === displayCurrency) return
    setDisplayCurrency(currency)
    window.location.reload()
  }

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger
        render={
          <Button
            variant='ghost'
            size='sm'
            className={cn('h-9 gap-1.5 px-2 text-xs font-medium', className)}
          />
        }
      >
        <DollarSign className='size-[1.05rem]' />
        <span className='hidden sm:inline'>{activeCurrency.label}</span>
        <span className='sr-only'>{t('Currency')}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' className='min-w-28'>
        {CURRENCY_OPTIONS.map((option) => (
          <DropdownMenuItem
            key={option.value}
            onClick={() => handleChangeCurrency(option.value)}
          >
            <span className='w-4 text-center'>{option.symbol}</span>
            <span>{option.label}</span>
            <span
              className={cn(
                'bg-primary ms-auto size-1.5 rounded-full',
                displayCurrency !== option.value && 'invisible'
              )}
            />
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
