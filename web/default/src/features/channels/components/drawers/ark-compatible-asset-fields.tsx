import { Controller, type UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import {
  parseAssetFixedQuery,
  validAssetEndpointPath,
} from '../../lib/ark-compatible-assets'
import type { ChannelFormValues } from '../../lib/channel-form'

export function ArkCompatibleAssetFields(props: {
  form: UseFormReturn<ChannelFormValues>
}) {
  const { t } = useTranslation()
  const base =
    props.form.watch('asset_base_url') || props.form.watch('base_url') || ''
  const path = props.form.watch('asset_endpoint_path')?.trim() || '/api'
  const version = props.form.watch('asset_api_version')?.trim() || '2024-01-01'
  const query = props.form.watch('asset_fixed_query') || '{}'
  let preview = ''
  try {
    const url = new URL(base.trim().replace(/\/+$/, '') + path)
    if (
      validAssetEndpointPath(path) &&
      ['http:', 'https:'].includes(url.protocol) &&
      !url.username &&
      !url.password
    ) {
      url.search = new URLSearchParams({
        ...parseAssetFixedQuery(query),
        Action: 'CreateAsset',
        Version: version,
      }).toString()
      preview = `POST ${url.toString()}`
    }
  } catch {
    // Show a preview only after the connection fields form a valid URL.
  }
  const paginationItems = [
    { value: 'page', label: t('Page numbers (PageNumber / PageSize)') },
    { value: 'cursor', label: t('Cursor (MaxResults / NextToken)') },
  ]
  return (
    <FieldGroup>
      <FieldDescription>
        {t(
          'Supports 10 asset and group operations with Bearer authentication. The upstream manages ProjectName; omit it from client requests. Liveness is not supported.'
        )}
      </FieldDescription>
      <FieldDescription>
        {t(
          'MoreRouter: Base URL https://morerouter.cn, path /api, fixed query {"ns":"asset_manager"} (international: asset_manager_intl). Confirm pagination with your provider.'
        )}
      </FieldDescription>
      <Controller
        control={props.form.control}
        name='asset_endpoint_path'
        render={({ field, fieldState }) => (
          <Field data-invalid={fieldState.invalid}>
            <FieldLabel htmlFor='asset-endpoint-path'>
              {t('Asset endpoint path')}
            </FieldLabel>
            <Input
              {...field}
              value={field.value || ''}
              id='asset-endpoint-path'
              placeholder='/api'
              aria-invalid={fieldState.invalid}
            />
            <FieldError>
              {fieldState.error?.message
                ? t(fieldState.error.message)
                : undefined}
            </FieldError>
          </Field>
        )}
      />
      <Controller
        control={props.form.control}
        name='asset_api_version'
        render={({ field, fieldState }) => (
          <Field data-invalid={fieldState.invalid}>
            <FieldLabel htmlFor='asset-api-version'>
              {t('Asset API version')}
            </FieldLabel>
            <Input
              {...field}
              value={field.value || ''}
              id='asset-api-version'
              placeholder='2024-01-01'
              aria-invalid={fieldState.invalid}
            />
            <FieldError>
              {fieldState.error?.message
                ? t(fieldState.error.message)
                : undefined}
            </FieldError>
          </Field>
        )}
      />
      <Controller
        control={props.form.control}
        name='asset_fixed_query'
        render={({ field, fieldState }) => (
          <Field data-invalid={fieldState.invalid}>
            <FieldLabel htmlFor='asset-fixed-query'>
              {t('Fixed query parameters (JSON)')}
            </FieldLabel>
            <Textarea
              {...field}
              value={field.value || ''}
              id='asset-fixed-query'
              placeholder='{"ns":"asset_manager"}'
              aria-invalid={fieldState.invalid}
              className='font-mono'
            />
            <FieldDescription>
              {t(
                'String values only. Action and Version are generated automatically. Put credentials in the key field.'
              )}
            </FieldDescription>
            <FieldError>
              {fieldState.error?.message
                ? t(fieldState.error.message)
                : undefined}
            </FieldError>
          </Field>
        )}
      />
      <Controller
        control={props.form.control}
        name='asset_pagination'
        render={({ field, fieldState }) => (
          <Field data-invalid={fieldState.invalid}>
            <FieldLabel htmlFor='asset-pagination'>
              {t('Upstream pagination')}
            </FieldLabel>
            <Select
              items={paginationItems}
              value={field.value || 'page'}
              onValueChange={field.onChange}
            >
              <SelectTrigger
                id='asset-pagination'
                aria-invalid={fieldState.invalid}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {paginationItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <FieldError>
              {fieldState.error?.message
                ? t(fieldState.error.message)
                : undefined}
            </FieldError>
          </Field>
        )}
      />
      {preview && (
        <FieldDescription className='font-mono break-all'>
          {preview}
        </FieldDescription>
      )}
    </FieldGroup>
  )
}
