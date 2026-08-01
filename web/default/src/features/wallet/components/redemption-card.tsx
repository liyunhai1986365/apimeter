/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { GiftIcon, Link01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from '@/components/ui/input-group'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { TitledCard } from '@/components/ui/titled-card'

interface RedemptionCardProps {
  code: string
  onCodeChange: (code: string) => void
  onRedeem: () => void
  redeeming: boolean
  enabled: boolean
  topupLink?: string
  loading?: boolean
}

export function RedemptionCard({
  code,
  onCodeChange,
  onRedeem,
  redeeming,
  enabled,
  topupLink,
  loading,
}: RedemptionCardProps) {
  const { t } = useTranslation()

  return (
    <TitledCard
      title={t('Redemption Code')}
      description={t('Enter your redemption code')}
      icon={<HugeiconsIcon icon={GiftIcon} className='size-4' />}
      titleClassName='text-base sm:text-lg'
      contentClassName='flex flex-col gap-3'
    >
      {loading ? (
        <div className='flex flex-col gap-3'>
          <Skeleton className='h-10 w-full' />
          <Skeleton className='h-4 w-2/3' />
        </div>
      ) : enabled ? (
        <form
          onSubmit={(event) => {
            event.preventDefault()
            onRedeem()
          }}
        >
          <FieldGroup className='gap-3'>
            <Field data-disabled={redeeming || undefined}>
              <FieldLabel htmlFor='wallet-redemption-code' className='sr-only'>
                {t('Redemption Code')}
              </FieldLabel>
              <InputGroup className='h-10'>
                <InputGroupInput
                  id='wallet-redemption-code'
                  value={code}
                  onChange={(event) => onCodeChange(event.target.value)}
                  placeholder={t('Enter your redemption code')}
                  autoComplete='off'
                  disabled={redeeming}
                />
                <InputGroupAddon align='inline-end'>
                  <InputGroupButton
                    type='submit'
                    variant='default'
                    size='sm'
                    disabled={redeeming || code.trim() === ''}
                  >
                    {redeeming ? <Spinner data-icon='inline-start' /> : null}
                    {t('Redeem')}
                  </InputGroupButton>
                </InputGroupAddon>
              </InputGroup>
              {topupLink ? (
                <FieldDescription>
                  {t('Need a redemption code?')}{' '}
                  <a
                    href={topupLink}
                    target='_blank'
                    rel='noopener noreferrer'
                    className='inline-flex items-center gap-1'
                  >
                    {t('Get one here')}
                    <HugeiconsIcon icon={Link01Icon} className='size-3.5' />
                  </a>
                </FieldDescription>
              ) : null}
            </Field>
          </FieldGroup>
        </form>
      ) : (
        <Alert>
          <AlertDescription>
            {t(
              'Redemption codes are disabled until the administrator confirms compliance terms.'
            )}
          </AlertDescription>
        </Alert>
      )}
    </TitledCard>
  )
}
