import { type TFunction } from 'i18next'
import type { PricingData } from '../types'

export type EndpointInfo = PricingData['supported_endpoint'][string]

export function getEndpointFallbackLabel(type: string, t: TFunction): string {
  const labels: Record<string, string> = {
    openai: t('OpenAI Chat'),
    'openai-response': t('OpenAI Responses'),
    'openai-response-compact': t('OpenAI Responses Compact'),
    anthropic: 'Anthropic',
    gemini: 'Gemini',
    'jina-rerank': t('Jina Rerank'),
    'image-generation': t('OpenAI Image Generation'),
    'openai-image-edit': t('OpenAI Image Edit'),
    'gemini-image-generation': t('Gemini Native Image Generation'),
    embeddings: t('Embeddings'),
    'openai-video': t('OpenAI Video'),
    'seedance2-native-video': t('Seedance2 Native Video'),
  }

  return labels[type] ?? type
}

export function getEndpointLabel(
  type: string,
  endpointMap: PricingData['supported_endpoint'] | undefined,
  t: TFunction
): string {
  return endpointMap?.[type]?.label || getEndpointFallbackLabel(type, t)
}
