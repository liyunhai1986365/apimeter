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
import type { z } from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Textarea } from '@/components/ui/textarea'
import { createAgentDomainsSchema } from '../lib/domain-form'

export function AgentDomainForm(props: {
  isPending: boolean
  onSubmit: (domains: string[]) => Promise<unknown>
}) {
  const { t } = useTranslation()
  const id = useId()
  const schema = createAgentDomainsSchema(t)
  const form = useForm<
    z.input<typeof schema>,
    unknown,
    z.output<typeof schema>
  >({
    resolver: zodResolver(schema),
    defaultValues: { domains: '' },
  })
  const error = form.formState.errors.domains
  const pending = props.isPending || form.formState.isSubmitting

  return (
    <form
      className='mb-3'
      onSubmit={form.handleSubmit(async (values) => {
        try {
          await props.onSubmit(values.domains)
          form.reset()
        } catch (error) {
          form.setError('domains', {
            message:
              error instanceof Error ? error.message : t('Operation failed'),
          })
        }
      })}
    >
      <FieldGroup>
        <Field data-invalid={!!error} data-disabled={pending}>
          <FieldLabel htmlFor={id}>{t('Add Domains')}</FieldLabel>
          <Textarea
            id={id}
            {...form.register('domains')}
            placeholder={'agent.example.com\napi.example.com'}
            rows={3}
            disabled={pending}
            aria-invalid={!!error}
            aria-describedby={`${id}-help ${id}-error`}
            autoCapitalize='none'
            autoComplete='off'
            spellCheck={false}
          />
          <FieldDescription id={`${id}-help`}>
            {t(
              'Enter one domain per line or separate them with commas. Add up to 50 at a time.'
            )}
          </FieldDescription>
          <FieldError id={`${id}-error`} errors={error ? [error] : []} />
        </Field>
        <Field orientation='horizontal'>
          <Button type='submit' disabled={pending}>
            {t('Add Domains')}
          </Button>
        </Field>
      </FieldGroup>
    </form>
  )
}
