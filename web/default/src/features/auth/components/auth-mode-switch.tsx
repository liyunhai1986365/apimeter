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
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ButtonGroup } from '@/components/ui/button-group'

type AuthMode = 'sign-in' | 'sign-up'

type AuthModeSwitchProps = {
  activeMode: AuthMode
  showSignUp?: boolean
  redirectTo?: string
}

export function AuthModeSwitch({
  activeMode,
  showSignUp = true,
  redirectTo,
}: AuthModeSwitchProps) {
  const { t } = useTranslation()

  if (!showSignUp) return null

  return (
    <nav aria-label={`${t('Sign in')} / ${t('Sign up')}`}>
      <ButtonGroup className='bg-muted w-full rounded-lg p-1'>
        <Button
          variant={activeMode === 'sign-in' ? 'default' : 'ghost'}
          size='lg'
          className='h-10 flex-1'
          aria-current={activeMode === 'sign-in' ? 'page' : undefined}
          render={
            <Link
              to='/sign-in'
              search={redirectTo ? { redirect: redirectTo } : undefined}
            />
          }
        >
          {t('Sign in')}
        </Button>
        <Button
          variant={activeMode === 'sign-up' ? 'default' : 'ghost'}
          size='lg'
          className='h-10 flex-1'
          aria-current={activeMode === 'sign-up' ? 'page' : undefined}
          render={
            <Link
              to='/sign-up'
              search={redirectTo ? { redirect: redirectTo } : undefined}
            />
          }
        >
          {t('Sign up')}
        </Button>
      </ButtonGroup>
    </nav>
  )
}
