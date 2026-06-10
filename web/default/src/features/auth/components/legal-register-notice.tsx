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
import { FileText } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import type { SystemStatus } from '../types'
import { LegalInlineLinks, useLegalLinks } from './legal-links'

type LegalRegisterNoticeProps = {
  status?: SystemStatus | null
}

export function LegalRegisterNotice({ status }: LegalRegisterNoticeProps) {
  const { t } = useTranslation()
  const { links } = useLegalLinks(status)

  if (links.length === 0) return null

  return (
    <Alert className='border-blue-500/25 bg-blue-500/10 px-3 py-3 text-blue-950 dark:text-blue-100'>
      <FileText className='text-blue-600 dark:text-blue-300' />
      <AlertTitle className='text-sm font-semibold'>
        {t('Please read the service and privacy terms before registering')}
      </AlertTitle>
      <AlertDescription className='text-blue-950/75 dark:text-blue-100/75'>
        {t('Registration means you have read and agree to the')}{' '}
        <LegalInlineLinks links={links} />.
      </AlertDescription>
    </Alert>
  )
}
