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
import { z } from 'zod'
import type { TFunction } from 'i18next'
import { parseQuotaFromDollars, quotaUnitsToDollars } from '@/lib/format'
import { USER_FACING_GROUP_TERMS } from '@/lib/user-facing-group-terms'
import { DEFAULT_GROUP } from '../constants'
import { type ApiKeyFormData, type ApiKey } from '../types'
import { AUTO_GROUP_VALUE } from './api-key-groups'

const ORDERED_GROUP_POLICY_TYPE = 'ordered'
const ROUTING_STRATEGY_POLICY_TYPE = 'routing_strategy'

export type ApiKeyRoutingMode = 'smart' | 'manual'
export type ApiKeyRoutingStrategy =
  | 'smart_auto'
  | 'price_first'
  | 'speed_first'
  | 'success_first'

type OrderedGroupPolicy = {
  type?: string
  groups?: unknown
  strategy?: unknown
  excluded_groups?: unknown
}

// ============================================================================
// Form Schema
// ============================================================================

export function getApiKeyFormSchema(t: TFunction) {
  return z
    .object({
      name: z.string().min(1, t('Please enter a name')),
      remain_quota_dollars: z.number().optional(),
      expired_time: z.date().optional(),
      unlimited_quota: z.boolean(),
      model_limits: z.array(z.string()),
      allow_ips: z.string().optional(),
      group_chain: z
        .array(z.string())
        .min(1, t(USER_FACING_GROUP_TERMS.selectAtLeastOne)),
      routing_mode: z.enum(['smart', 'manual']).optional(),
      routing_strategy: z
        .enum(['smart_auto', 'price_first', 'speed_first', 'success_first'])
        .optional(),
      routing_excluded_groups: z.array(z.string()).optional(),
      cross_group_retry: z.boolean().optional(),
      image_response_format: z
        .enum(['follow_request', 'url', 'b64_json'])
        .optional(),
      image_store_strategy: z
        .enum(['default', 'only_store_base64', 'force_store_url_and_base64'])
        .optional(),
      tokenCount: z.number().min(1).optional(),
    })
    .superRefine((data, ctx) => {
      if (data.unlimited_quota) {
        return
      }

      if (
        data.remain_quota_dollars === undefined ||
        data.remain_quota_dollars < 0
      ) {
        ctx.addIssue({
          code: 'custom',
          path: ['remain_quota_dollars'],
          message: t('Quota must be zero or greater'),
        })
      }
    })
}

export type ApiKeyFormValues = z.infer<ReturnType<typeof getApiKeyFormSchema>>

// ============================================================================
// Form Defaults
// ============================================================================

export const API_KEY_FORM_DEFAULT_VALUES: ApiKeyFormValues = {
  name: '',
  remain_quota_dollars: 10,
  expired_time: undefined,
  unlimited_quota: true,
  model_limits: [],
  allow_ips: '',
  routing_mode: 'smart',
  routing_strategy: 'smart_auto',
  routing_excluded_groups: [],
  group_chain: [AUTO_GROUP_VALUE],
  cross_group_retry: true,
  image_response_format: 'follow_request',
  image_store_strategy: 'default',
  tokenCount: 1,
}

export function getApiKeyFormDefaultValues(): ApiKeyFormValues {
  return {
    ...API_KEY_FORM_DEFAULT_VALUES,
    routing_mode: 'smart',
    routing_strategy: 'smart_auto',
    routing_excluded_groups: [],
    group_chain: [AUTO_GROUP_VALUE],
    cross_group_retry: true,
  }
}

// ============================================================================
// Form Data Transformation
// ============================================================================

/**
 * Transform form data to API payload
 */
export function transformFormDataToPayload(
  data: ApiKeyFormValues
): ApiKeyFormData {
  const groupChain = normalizeGroupChain(data.group_chain)
  const routingMode = data.routing_mode || 'smart'
  const routingStrategy = data.routing_strategy || 'smart_auto'
  const excludedGroups = normalizeConcreteGroupList(
    data.routing_excluded_groups
  )
  const groupPolicy =
    routingMode === 'smart'
      ? JSON.stringify(
          excludedGroups.length > 0
            ? {
                type: ROUTING_STRATEGY_POLICY_TYPE,
                strategy: routingStrategy,
                excluded_groups: excludedGroups,
              }
            : {
                type: ROUTING_STRATEGY_POLICY_TYPE,
                strategy: routingStrategy,
              }
        )
      : groupChain.length === 1 && groupChain[0] === AUTO_GROUP_VALUE
        ? ''
        : JSON.stringify({
            type: ORDERED_GROUP_POLICY_TYPE,
            groups: groupChain,
          })

  return {
    name: data.name,
    remain_quota: data.unlimited_quota
      ? 0
      : parseQuotaFromDollars(data.remain_quota_dollars || 0),
    expired_time: data.expired_time
      ? Math.floor(data.expired_time.getTime() / 1000)
      : -1,
    unlimited_quota: data.unlimited_quota,
    model_limits_enabled: data.model_limits.length > 0,
    model_limits: data.model_limits.join(','),
    allow_ips: data.allow_ips || '',
    group:
      routingMode === 'smart'
        ? AUTO_GROUP_VALUE
        : groupChain[0] || AUTO_GROUP_VALUE,
    group_policy: groupPolicy,
    cross_group_retry:
      routingMode === 'manual' && groupChain.length > 1
        ? true
        : !!data.cross_group_retry,
    image_settings: {
      format: data.image_response_format || 'follow_request',
      store:
        data.image_response_format === 'url' &&
        data.image_store_strategy !== 'default'
          ? data.image_store_strategy || 'default'
          : 'default',
    },
  }
}

/**
 * Transform API key data to form defaults
 */
export function transformApiKeyToFormDefaults(
  apiKey: ApiKey
): ApiKeyFormValues {
  const groupChain = parseApiKeyGroupChain(apiKey)
  const routingPolicy = parseApiKeyRoutingPolicy(apiKey)

  return {
    name: apiKey.name,
    remain_quota_dollars: apiKey.unlimited_quota
      ? 0
      : quotaUnitsToDollars(apiKey.remain_quota),
    expired_time:
      apiKey.expired_time > 0
        ? new Date(apiKey.expired_time * 1000)
        : undefined,
    unlimited_quota: apiKey.unlimited_quota,
    model_limits: apiKey.model_limits
      ? apiKey.model_limits.split(',').filter(Boolean)
      : [],
    allow_ips: apiKey.allow_ips || '',
    routing_mode: routingPolicy.mode,
    routing_strategy: routingPolicy.strategy,
    routing_excluded_groups: routingPolicy.excludedGroups,
    group_chain: groupChain,
    cross_group_retry: !!apiKey.cross_group_retry,
    image_response_format: apiKey.image_settings?.format || 'follow_request',
    image_store_strategy:
      apiKey.image_settings?.store === 'only_store_base64' ||
      apiKey.image_settings?.store === 'force_store_url_and_base64'
        ? apiKey.image_settings.store
        : 'default',
    tokenCount: 1,
  }
}

export function parseApiKeyRoutingPolicy(
  apiKey: Pick<ApiKey, 'group' | 'group_policy'>
): {
  mode: ApiKeyRoutingMode
  strategy: ApiKeyRoutingStrategy
  excludedGroups: string[]
} {
  const policyText = apiKey.group_policy?.trim()
  if (policyText) {
    try {
      const policy = JSON.parse(policyText) as OrderedGroupPolicy
      if (
        policy?.type === ROUTING_STRATEGY_POLICY_TYPE &&
        typeof policy.strategy === 'string' &&
        isApiKeyRoutingStrategy(policy.strategy)
      ) {
        return {
          mode: 'smart',
          strategy: policy.strategy,
          excludedGroups: Array.isArray(policy.excluded_groups)
            ? normalizeConcreteGroupList(
                policy.excluded_groups.filter(
                  (group): group is string => typeof group === 'string'
                )
              )
            : [],
        }
      }
    } catch (_error) {
      // Fall through to manual mode.
    }
  }
  if (parseApiKeyGroupChain(apiKey)[0] === AUTO_GROUP_VALUE) {
    return {
      mode: 'smart',
      strategy: 'smart_auto',
      excludedGroups: [],
    }
  }
  return {
    mode: 'manual',
    strategy: 'smart_auto',
    excludedGroups: [],
  }
}

function isApiKeyRoutingStrategy(
  value: string
): value is ApiKeyRoutingStrategy {
  return (
    value === 'smart_auto' ||
    value === 'price_first' ||
    value === 'speed_first' ||
    value === 'success_first'
  )
}

export function getApiKeyRoutingStrategyLabel(
  strategy: ApiKeyRoutingStrategy | undefined
) {
  switch (strategy) {
    case 'price_first':
      return 'Price first'
    case 'speed_first':
      return 'Speed first'
    case 'success_first':
      return 'Success rate first'
    case 'smart_auto':
    default:
      return 'Smart automatic'
  }
}

export function normalizeGroupChain(groups: string[] | undefined): string[] {
  const cleanGroups = (groups || [])
    .map((group) => group.trim())
    .filter(Boolean)
  const seen = new Set<string>()
  const result: string[] = []
  for (const group of cleanGroups) {
    if (seen.has(group)) continue
    seen.add(group)
    result.push(group)
  }
  if (result.includes(AUTO_GROUP_VALUE)) {
    return [AUTO_GROUP_VALUE]
  }
  return result.length > 0 ? result : [AUTO_GROUP_VALUE]
}

export function normalizeConcreteGroupList(
  groups: string[] | undefined
): string[] {
  const seen = new Set<string>()
  const result: string[] = []
  for (const group of groups || []) {
    const value = group.trim()
    if (!value || value === AUTO_GROUP_VALUE || seen.has(value)) continue
    seen.add(value)
    result.push(value)
  }
  return result
}

export function addGroupToChain(groups: string[], group: string): string[] {
  const targetGroup = group.trim()
  if (!targetGroup) return normalizeGroupChain(groups)
  if (targetGroup === AUTO_GROUP_VALUE) return [AUTO_GROUP_VALUE]
  const current = normalizeGroupChain(groups)
  if (current.includes(targetGroup)) return current
  if (current.length === 1 && current[0] === AUTO_GROUP_VALUE) {
    return [targetGroup]
  }
  return normalizeGroupChain([...current, targetGroup])
}

export function addGroupsToChain(
  groups: string[],
  nextGroups: string[]
): string[] {
  return nextGroups.reduce(
    (chain, group) => addGroupToChain(chain, group),
    normalizeGroupChain(groups)
  )
}

export function removeGroupFromChain(
  groups: string[],
  index: number
): string[] {
  const current = normalizeGroupChain(groups)
  return current.filter((_, i) => i !== index)
}

export function parseApiKeyGroupChain(
  apiKey: Pick<ApiKey, 'group' | 'group_policy'>
): string[] {
  const policyText = apiKey.group_policy?.trim()
  if (policyText) {
    try {
      const policy = JSON.parse(policyText) as OrderedGroupPolicy
      if (
        policy?.type === ORDERED_GROUP_POLICY_TYPE &&
        Array.isArray(policy.groups)
      ) {
        const groups = policy.groups.filter(
          (group): group is string => typeof group === 'string'
        )
        return normalizeGroupChain(groups)
      }
    } catch (_error) {
      // Fall through to legacy group.
    }
  }
  return normalizeGroupChain([apiKey.group || DEFAULT_GROUP])
}

export function getApiKeyGroupDisplayItems(
  apiKey: Pick<ApiKey, 'group' | 'group_policy'>,
  visibleLimit = 1
): {
  allGroups: string[]
  visibleGroups: string[]
  hiddenGroups: string[]
  hiddenCount: number
  routingStrategy?: ApiKeyRoutingStrategy
  excludedGroups: string[]
} {
  const routingPolicy = parseApiKeyRoutingPolicy(apiKey)
  if (routingPolicy.mode === 'smart') {
    return {
      allGroups: [],
      visibleGroups: [],
      hiddenGroups: [],
      hiddenCount: 0,
      routingStrategy: routingPolicy.strategy,
      excludedGroups: routingPolicy.excludedGroups,
    }
  }

  const allGroups = parseApiKeyGroupChain(apiKey)
  const limit = Math.max(1, visibleLimit)
  const visibleGroups = allGroups.slice(0, limit)
  const hiddenGroups = allGroups.slice(limit)

  return {
    allGroups,
    visibleGroups,
    hiddenGroups,
    hiddenCount: hiddenGroups.length,
    excludedGroups: [],
  }
}
