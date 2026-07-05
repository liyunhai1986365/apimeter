import type { UsageLog } from '@/features/usage-logs/data/schema'
import { parseLogOther } from '@/features/usage-logs/lib/format'
import type { RetryPolicyRule } from '../types'

export type RetryPolicyTemplate = {
  labelKey: string
  descriptionKey: string
  rule: RetryPolicyRule
}

export const RETRY_POLICY_TEMPLATES: RetryPolicyTemplate[] = [
  {
    labelKey: 'Retry by model and message',
    descriptionKey:
      'Retry a specific model when the upstream error contains text',
    rule: {
      name: 'model message retry',
      action: 'retry',
      models: ['gpt-image-2'],
      message_contains: ['private ip'],
    },
  },
  {
    labelKey: 'Retry upstream rate limits',
    descriptionKey: 'Retry common upstream rate limit responses',
    rule: {
      name: 'upstream rate limit retry',
      action: 'retry',
      status_codes: '429',
      message_contains: ['rate limit', 'too many requests'],
    },
  },
  {
    labelKey: 'Skip invalid requests',
    descriptionKey: 'Do not retry client-side invalid request errors',
    rule: {
      name: 'invalid request skip retry',
      action: 'skip_retry',
      status_codes: '400',
      error_codes: ['invalid_request'],
    },
  },
  {
    labelKey: 'Retry provider temporary errors',
    descriptionKey:
      'Retry temporary upstream failures and overloaded responses',
    rule: {
      name: 'temporary upstream retry',
      action: 'retry',
      status_codes: '500-503',
      message_contains: ['temporary', 'overloaded', 'timeout'],
    },
  },
  {
    labelKey: 'Fail over to backup channels',
    descriptionKey:
      'Route matching upstream errors to configured backup groups, channels, or tags',
    rule: {
      name: 'temporary upstream failover',
      action: 'failover',
      priority: 100,
      status_codes: '429,500-504',
      message_contains: ['temporary', 'overloaded', 'timeout'],
      targets: {
        groups: ['backup'],
        channel_tags: ['stable'],
      },
      strategy: {
        max_retries: 2,
        exclude_failed_channel: true,
      },
    },
  },
  {
    labelKey: 'Recover Codex encrypted content',
    descriptionKey:
      'Retry Codex encrypted-content verification failures through configured compatible groups',
    rule: {
      name: 'codex encrypted content recovery',
      action: 'retry',
      models: ['gpt-5'],
      status_codes: '400',
      message_contains: [
        'encrypted content',
        'could not be verified',
        'could not be decrypted or parsed',
      ],
      retry_groups: ['codex-official-primary', 'codex-official-backup'],
      max_retries: 2,
    },
  },
]

export function formatRetryPolicyRule(rule: RetryPolicyRule): string {
  return JSON.stringify(rule, null, 2)
}

export function insertRetryPolicyTemplate(
  currentValue: string | undefined,
  template: RetryPolicyTemplate
): string {
  const rules = parseRetryPolicyRules(currentValue)
  rules.push(template.rule)
  return JSON.stringify(rules, null, 2)
}

export function appendRetryPolicyRuleToSetting(
  setting: string | null | undefined,
  rule: RetryPolicyRule
): string {
  const parsed = parseSettingRecord(setting)
  const rules = Array.isArray(parsed.retry_policy_rules)
    ? (parsed.retry_policy_rules as RetryPolicyRule[])
    : []
  parsed.retry_policy_rules = [...rules, rule]
  return JSON.stringify(parsed)
}

export function buildRetryPolicyRuleFromLog(
  log: Pick<
    UsageLog,
    'id' | 'type' | 'model_name' | 'channel' | 'content' | 'other'
  >
): RetryPolicyRule | null {
  if (log.type !== 5 || !log.channel) return null

  const other = parseLogOther(log.other)
  const adminInfo = other?.admin_info
  const statusCode =
    typeof adminInfo?.status_code === 'number'
      ? adminInfo.status_code
      : undefined
  const errorCode =
    typeof adminInfo?.error_code === 'string' ? adminInfo.error_code : undefined
  const messageNeedle = extractMessageNeedle(log.content)

  const rule: RetryPolicyRule = {
    name: `log#${log.id} ${log.model_name || 'model'}${statusCode ? ` status ${statusCode}` : ''}`,
    action: 'retry',
  }
  if (log.model_name) {
    rule.models = [log.model_name]
  }
  if (statusCode) {
    rule.status_codes = String(statusCode)
  }
  if (errorCode) {
    rule.error_codes = [errorCode]
  }
  if (messageNeedle) {
    rule.message_contains = [messageNeedle]
  }

  return rule
}

function parseRetryPolicyRules(value: string | undefined): RetryPolicyRule[] {
  const trimmed = value?.trim()
  if (!trimmed) return []
  try {
    const parsed = JSON.parse(trimmed)
    return Array.isArray(parsed) ? (parsed as RetryPolicyRule[]) : []
  } catch {
    return []
  }
}

function parseSettingRecord(
  setting: string | null | undefined
): Record<string, unknown> {
  const trimmed = setting?.trim()
  if (!trimmed) return {}
  try {
    const parsed = JSON.parse(trimmed)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>
    }
  } catch {
    return {}
  }
  return {}
}

function extractMessageNeedle(content: string | undefined): string {
  const message = content?.trim() ?? ''
  if (!message) return ''
  const privateIP = message.match(/private ip[^,.;\n]*/i)
  if (privateIP) return privateIP[0].trim()

  const afterStatus = message.replace(/^status_code=\d+\s*,\s*/i, '').trim()
  const segments = afterStatus
    .split(/[,;\n]/)
    .map((part) => part.trim())
    .filter(Boolean)
  return segments[segments.length - 1] || afterStatus.slice(0, 120)
}
