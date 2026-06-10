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
import { useEffect, useState } from 'react'
import { FileText, ShieldCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import {
  isLegalConsentPromptDismissed,
  markLegalConsentAccepted,
  markLegalConsentPromptDismissed,
  shouldShowLegalConsentPrompt,
} from '@/features/auth/lib/legal-consent-storage'
import type { SystemStatus } from '../types'
import { LegalInlineLinks, useLegalLinks } from './legal-links'

type LegalFirstUseDialogProps = {
  status?: SystemStatus | null
}

export function LegalFirstUseDialog({ status }: LegalFirstUseDialogProps) {
  const { t } = useTranslation()
  const { links, hasUserAgreement, hasPrivacyPolicy } = useLegalLinks(status)
  const [open, setOpen] = useState(false)
  const [agreed, setAgreed] = useState(false)

  useEffect(() => {
    setOpen(
      shouldShowLegalConsentPrompt({
        userAgreementEnabled: hasUserAgreement,
        privacyPolicyEnabled: hasPrivacyPolicy,
        promptDismissed: isLegalConsentPromptDismissed(),
      })
    )
  }, [hasPrivacyPolicy, hasUserAgreement])

  const handleOpenChange = (nextOpen: boolean) => {
    setOpen(nextOpen)
    if (!nextOpen) markLegalConsentPromptDismissed()
  }

  const handleAgreeAndContinue = () => {
    markLegalConsentAccepted()
    setOpen(false)
  }

  if (links.length === 0) return null

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className='p-0 sm:max-w-2xl' showCloseButton>
        <div className='space-y-5 p-5 sm:p-6'>
          <div className='flex size-12 items-center justify-center rounded-lg bg-blue-600 text-white shadow-sm shadow-blue-600/25'>
            <ShieldCheck className='size-6' aria-hidden='true' />
          </div>

          <DialogHeader className='text-left'>
            <DialogTitle className='text-xl leading-7 font-semibold'>
              {t('Please review the service and privacy terms')}
            </DialogTitle>
            <DialogDescription className='leading-6'>
              {t(
                'Before continuing, please review the following policies. We do not store your model conversation content, images, videos, or other multimodal content; only token usage statistics needed for billing are retained. Our services are available only in supported countries and regions. Please comply with applicable local laws and read and agree to the policies below.'
              )}
            </DialogDescription>
          </DialogHeader>

          <div className='grid gap-2'>
            {links.map((link) => (
              <a
                key={link.href}
                href={link.href}
                target='_blank'
                rel='noopener noreferrer'
                className='border-border/70 bg-muted/40 hover:border-primary/40 hover:bg-muted flex min-h-12 items-center gap-3 rounded-lg border px-3 py-2 text-sm font-medium transition-colors'
              >
                <FileText
                  className='text-primary size-4 shrink-0'
                  aria-hidden='true'
                />
                {link.label}
              </a>
            ))}
          </div>

          <p className='text-muted-foreground text-xs leading-5'>
            {t('You can revisit the')} <LegalInlineLinks links={links} />{' '}
            {t('from the registration page at any time.')}
          </p>

          <div className='border-border/70 bg-muted/40 flex items-start gap-3 rounded-lg border p-3'>
            <Checkbox
              id='first-use-legal-consent'
              checked={agreed}
              onCheckedChange={(value) => setAgreed(value === true)}
              className='mt-0.5'
            />
            <Label
              htmlFor='first-use-legal-consent'
              className='text-muted-foreground text-xs leading-5 font-normal'
            >
              {t('I have read and agree to these terms')}
            </Label>
          </div>
        </div>

        <DialogFooter className='mx-0 mb-0'>
          <Button
            type='button'
            className='h-10 w-full rounded-lg sm:w-auto'
            disabled={!agreed}
            onClick={handleAgreeAndContinue}
          >
            {t('Agree and continue')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
