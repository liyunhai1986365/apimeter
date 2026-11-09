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
import { useCallback, useState } from 'react'
import { changeInterfaceLanguage } from '@/i18n/config'
import {
  INTERFACE_LANGUAGE_OPTIONS,
  normalizeInterfaceLanguage,
  replaceCurrentUrlLanguage,
} from '@/i18n/languages'
import { Languages, Check } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

export function LanguageSwitcher() {
  const { i18n, t } = useTranslation()
  const user = useAuthStore((s) => s.auth.user)
  const [loading, setLoading] = useState(false)
  const currentLanguage = normalizeInterfaceLanguage(i18n.language)

  const handleChangeLanguage = useCallback(
    async (code: string) => {
      if (loading || code === currentLanguage) return
      setLoading(true)
      try {
        if (!(await changeInterfaceLanguage(code))) return
        replaceCurrentUrlLanguage(code)
        if (user) {
          try {
            await api.put('/api/user/self', { language: code })
          } catch {
            // Best-effort persistence; don't block the UI on failure
          }
        }
      } catch {
        toast.error(t('error', { ns: 'bootstrap' }))
      } finally {
        setLoading(false)
      }
    },
    [currentLanguage, loading, t, user]
  )

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger
        disabled={loading}
        aria-busy={loading}
        render={<Button variant='ghost' size='icon' className='h-9 w-9' />}
      >
        <Languages className='size-[1.2rem]' />
        <span className='sr-only'>{t('Change language')}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end'>
        <DropdownMenuGroup>
          {INTERFACE_LANGUAGE_OPTIONS.map((lang) => (
            <DropdownMenuItem
              key={lang.code}
              disabled={loading}
              onClick={() => handleChangeLanguage(lang.code)}
            >
              {lang.label}
              <Check
                size={14}
                className={cn(
                  'ms-auto',
                  currentLanguage !== lang.code && 'hidden'
                )}
              />
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
