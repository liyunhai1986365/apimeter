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
import { useEffect, useState, type ReactNode } from 'react'
import { initializeInterfaceLanguage } from '@/i18n/config'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'

// These three messages live in a tiny, bundled namespace, so retries remain
// usable even when every dynamically loaded language file is unavailable.
export function LanguageReady(props: { children: ReactNode }) {
  const { t } = useTranslation('bootstrap')
  const [status, setStatus] = useState<'loading' | 'ready' | 'error'>('loading')
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    let active = true
    initializeInterfaceLanguage().then(
      () => {
        if (active) setStatus('ready')
      },
      () => {
        if (active) setStatus('error')
      }
    )
    return () => {
      active = false
    }
  }, [attempt])

  if (status === 'ready') return props.children

  return (
    <div className='flex min-h-svh items-center justify-center p-6'>
      {status === 'error' ? (
        <Alert className='max-w-md'>
          <AlertDescription>
            <div className='flex flex-col gap-4'>
              <p>{t('error')}</p>
              <Button
                variant='outline'
                onClick={() => {
                  setStatus('loading')
                  setAttempt((value) => value + 1)
                }}
              >
                {t('retry')}
              </Button>
            </div>
          </AlertDescription>
        </Alert>
      ) : (
        <div
          className='text-muted-foreground flex items-center gap-2'
          role='status'
        >
          <Spinner aria-hidden='true' />
          <span>{t('loading')}</span>
        </div>
      )}
    </div>
  )
}
