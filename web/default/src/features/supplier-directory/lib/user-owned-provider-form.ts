import type { Channel } from '@/features/channels/types'
import type { UserOwnedProviderPayload } from '../api'

export type ProviderFormState = {
  name: string
  type: number
  key: string
  baseUrl: string
  models: string
  other: string
  openaiOrganization: string
  azureResponsesVersion: string
  vertexKeyType: 'json' | 'api_key'
  awsKeyType: 'ak_sk' | 'api_key'
  openrouterEnterprise: boolean
}

export const defaultProviderForm: ProviderFormState = {
  name: '',
  type: 1,
  key: '',
  baseUrl: '',
  models: '',
  other: '',
  openaiOrganization: '',
  azureResponsesVersion: '',
  vertexKeyType: 'json',
  awsKeyType: 'ak_sk',
  openrouterEnterprise: false,
}

export type UserOwnedProviderRow = Pick<
  Channel,
  'id' | 'name' | 'type' | 'group' | 'models' | 'status'
> & {
  base_url?: string | null
  openai_organization?: string | null
  other?: string
  settings?: string
  stats?: {
    channel_id: number
    request_count: number
    prompt_tokens: number
    completion_tokens: number
    quota: number
  }
}

type ProviderFormInput = Pick<
  ProviderFormState,
  'name' | 'type' | 'key' | 'baseUrl' | 'models'
> &
  Partial<
    Pick<
      ProviderFormState,
      | 'other'
      | 'openaiOrganization'
      | 'azureResponsesVersion'
      | 'vertexKeyType'
      | 'awsKeyType'
      | 'openrouterEnterprise'
    >
  >

export function normalizeProviderModels(value: string) {
  const seen = new Set<string>()
  const models: string[] = []
  for (const item of value.split(/[\n,]/)) {
    const model = item.trim()
    if (!model || seen.has(model)) continue
    seen.add(model)
    models.push(model)
  }
  return models.join(',')
}

export function providerToFormState(
  provider: UserOwnedProviderRow
): ProviderFormState {
  const settings = parseProviderSettings(provider.settings)
  return {
    name: provider.name,
    type: provider.type,
    key: '',
    baseUrl: provider.base_url || '',
    models: normalizeProviderModels(provider.models).split(',').join(', '),
    other: provider.other || '',
    openaiOrganization: provider.openai_organization || '',
    azureResponsesVersion:
      typeof settings.azure_responses_version === 'string'
        ? settings.azure_responses_version
        : '',
    vertexKeyType:
      settings.vertex_key_type === 'api_key' ? 'api_key' : 'json',
    awsKeyType: settings.aws_key_type === 'api_key' ? 'api_key' : 'ak_sk',
    openrouterEnterprise: settings.openrouter_enterprise === true,
  }
}

function parseProviderSettings(
  value: string | undefined
): Record<string, unknown> {
  if (!value) return {}
  try {
    const parsed = JSON.parse(value)
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
      ? parsed
      : {}
  } catch {
    return {}
  }
}

function buildProviderSettings(form: ProviderFormInput) {
  const settings: Record<string, unknown> = {}

  if (form.type === 1) {
    settings.allow_service_tier = false
    settings.disable_store = false
    settings.allow_safety_identifier = false
    settings.allow_include_obfuscation = false
    settings.allow_inference_geo = false
  }
  if (form.type === 3 && form.azureResponsesVersion?.trim()) {
    settings.azure_responses_version = form.azureResponsesVersion.trim()
  }
  if (form.type === 20) {
    settings.openrouter_enterprise = form.openrouterEnterprise === true
  }
  if (form.type === 33) {
    settings.aws_key_type = form.awsKeyType || 'ak_sk'
  }
  if (form.type === 41) {
    settings.vertex_key_type = form.vertexKeyType || 'json'
  }

  return JSON.stringify(settings)
}

export function buildUserOwnedProviderPayload(
  form: ProviderFormInput,
  options: { allowBlankKey?: boolean } = {}
): UserOwnedProviderPayload {
  const key = form.key.trim()
  return {
    mode: 'single',
    channel: {
      type: form.type,
      key: options.allowBlankKey ? key : key,
      name: form.name.trim(),
      models: normalizeProviderModels(form.models),
      base_url: form.baseUrl.trim() || undefined,
      openai_organization:
        form.type === 1
          ? form.openaiOrganization?.trim() || undefined
          : undefined,
      other: form.other?.trim() || '',
      settings: buildProviderSettings(form),
      status: 1,
      channel_ratio: 1,
      priority: 0,
      weight: 0,
      auto_ban: 1,
      retry_enabled: true,
    },
  }
}
