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
import { useTranslation } from 'react-i18next'
import { getLegalConsentAvailability } from '@/features/auth/lib/legal-consent-storage'
import type { SystemStatus } from '../types'

type LegalLink = {
  label: string
  href: string
}

export function useLegalLinks(status?: SystemStatus | null) {
  const { t } = useTranslation()
  const {
    userAgreementEnabled: hasUserAgreement,
    privacyPolicyEnabled: hasPrivacyPolicy,
  } = getLegalConsentAvailability(status)

  const links = [
    hasUserAgreement
      ? {
          label: t('User Agreement'),
          href: '/user-agreement',
        }
      : null,
    hasPrivacyPolicy
      ? {
          label: t('Privacy Policy'),
          href: '/privacy-policy',
        }
      : null,
  ].filter(Boolean) as LegalLink[]

  return {
    hasLegalLinks: links.length > 0,
    hasUserAgreement,
    hasPrivacyPolicy,
    links,
  }
}

export function LegalInlineLinks({ links }: { links: LegalLink[] }) {
  const { t } = useTranslation()

  return (
    <>
      {links.map((link, index) => (
        <span key={link.href}>
          {index > 0 ? ` ${t('and')} ` : null}
          <a
            href={link.href}
            target='_blank'
            rel='noopener noreferrer'
            className='text-primary font-medium underline underline-offset-4 hover:opacity-80'
          >
            {link.label}
          </a>
        </span>
      ))}
    </>
  )
}
