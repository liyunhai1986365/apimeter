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
import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import type { WebhookFormat } from '@/lib/webhook-format'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

type WebhookFormatFieldProps = {
  value: WebhookFormat
  onValueChange: (value: WebhookFormat) => void
  disabled?: boolean
  invalid?: boolean
}

export function WebhookFormatField(props: WebhookFormatFieldProps) {
  const { t } = useTranslation()
  const id = useId()

  return (
    <Field data-disabled={props.disabled} data-invalid={props.invalid}>
      <FieldLabel id={`${id}-label`}>{t('Webhook format')}</FieldLabel>
      <ToggleGroup
        variant='outline'
        value={[props.value]}
        disabled={props.disabled}
        aria-invalid={props.invalid}
        aria-labelledby={`${id}-label`}
        aria-describedby={`${id}-description`}
        onValueChange={(values) => {
          const value = values[0]
          if (value === 'generic' || value === 'wecom') {
            props.onValueChange(value)
          }
        }}
      >
        <ToggleGroupItem value='generic'>{t('Generic JSON')}</ToggleGroupItem>
        <ToggleGroupItem value='wecom'>
          {t('WeCom group robot')}
        </ToggleGroupItem>
      </ToggleGroup>
      <FieldDescription id={`${id}-description`}>
        {props.value === 'wecom'
          ? t(
              'Send Markdown to a WeCom group robot. No webhook secret is required.'
            )
          : t('Send the standard webhook JSON payload.')}
      </FieldDescription>
    </Field>
  )
}
