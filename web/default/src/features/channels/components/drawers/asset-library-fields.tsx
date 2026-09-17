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
import type { ChannelFormValues } from '../../lib/channel-form'

const backends = [
  ['inherit', 'Follow video protocol'],
  ['volcengine-assets', 'Volcengine official assets (12 operations)'],
  ['tgxmaas', 'TgxMaas assets (12 operations)'],
  ['task', 'Unified task assets (upload and query)'],
  ['modelsell', 'Modelsell assets (upload and query)'],
  ['api-assets', 'API assets REST (create, list, get, delete)'],
  ['material', 'Material assets REST (create, list, get, delete)'],
  ['service-inference', 'Service Inference assets (assets and groups)'],
  ['max-service-inference', 'Max Service Inference assets (create and query)'],
  ['disabled', 'Disable asset library'],
] as const

export function AssetLibraryFields(props: {
  form: UseFormReturn<ChannelFormValues>
}) {
  const { t } = useTranslation()
  const backend = props.form.watch('asset_backend') || 'inherit'
  const auth = props.form.watch('asset_auth_mode') || 'channel_key'
  const profile = props.form.watch('protocol_profile_id')
  const explicit = !['inherit', 'disabled'].includes(backend)
  const official = backend === 'volcengine-assets'
  const project =
    official ||
    backend === 'tgxmaas' ||
    (backend === 'inherit' && profile === 'seedance-tgxmaas')
  const inputs: {
    name:
      | 'asset_base_url'
      | 'asset_region'
      | 'asset_api_key'
      | 'asset_access_key_id'
      | 'asset_secret_access_key'
      | 'protocol_project_name'
    label: string
    secret?: boolean
    placeholder?: string
    required?: boolean
  }[] = []
  if (explicit)
    inputs.push({
      name: 'asset_base_url',
      label: 'Asset library Base URL',
      placeholder: official
        ? 'https://ark.cn-beijing.volcengineapi.com'
        : t('Empty means use channel Base URL'),
    })
  if (project)
    inputs.push({
      name: 'protocol_project_name',
      label: 'Project name (ProjectName)',
      placeholder: 'nmyk',
      required: true,
    })
  if (official)
    inputs.push(
      {
        name: 'asset_region',
        label: 'Signing region',
        placeholder: 'cn-beijing',
      },
      {
        name: 'asset_access_key_id',
        label: 'Asset AccessKey ID',
        secret: true,
      },
      {
        name: 'asset_secret_access_key',
        label: 'Asset Secret Access Key',
        secret: true,
      }
    )
  if (explicit && !official && auth === 'api_key')
    inputs.push({ name: 'asset_api_key', label: 'Asset API key', secret: true })
  return (
    <FieldGroup className='px-4 py-3'>
      <Controller
        control={props.form.control}
        name='asset_backend'
        render={({ field }) => (
          <Field>
            <FieldLabel htmlFor='asset-backend'>
              {t('Asset library type')}
            </FieldLabel>
            <Select
              value={field.value || 'inherit'}
              onValueChange={(value) => {
                field.onChange(value)
                props.form.setValue('asset_api_key', '')
                props.form.setValue('asset_access_key_id', '')
                props.form.setValue('asset_secret_access_key', '')
                props.form.setValue('asset_base_url', '')
                props.form.setValue(
                  'asset_auth_mode',
                  value === 'volcengine-assets' ? 'aksk' : 'channel_key'
                )
              }}
            >
              <SelectTrigger id='asset-backend'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {backends.map(([value, label]) => (
                    <SelectItem key={value} value={value}>
                      {t(label)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <FieldDescription>
              {t(
                'Asset library settings are independent of video generation. Available operations depend on the selected provider.'
              )}
            </FieldDescription>
          </Field>
        )}
      />
      {explicit && !official && (
        <Controller
          control={props.form.control}
          name='asset_auth_mode'
          render={({ field }) => (
            <Field>
              <FieldLabel htmlFor='asset-auth'>
                {t('Asset authentication')}
              </FieldLabel>
              <Select
                value={field.value || 'channel_key'}
                onValueChange={field.onChange}
              >
                <SelectTrigger id='asset-auth'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value='channel_key'>
                      {t('Use channel key')}
                    </SelectItem>
                    <SelectItem value='api_key'>
                      {t('Use separate API key')}
                    </SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
          )}
        />
      )}
      {inputs.map((input) => (
        <Controller
          key={input.name}
          control={props.form.control}
          name={input.name}
          render={({ field, fieldState }) => (
            <Field data-invalid={fieldState.invalid}>
              <FieldLabel htmlFor={input.name}>{t(input.label)}</FieldLabel>
              <Input
                {...field}
                id={input.name}
                value={field.value || ''}
                type={input.secret ? 'password' : 'text'}
                autoComplete={input.secret ? 'new-password' : 'off'}
                placeholder={input.placeholder}
                required={input.required}
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
      ))}
      {explicit && (
        <FieldDescription>
          {t(
            'Leave credentials empty to keep saved credentials when editing. Enter both AK and SK to replace official credentials.'
          )}
        </FieldDescription>
      )}
    </FieldGroup>
  )
}
