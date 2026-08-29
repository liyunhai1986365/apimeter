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
import type { TFunction } from 'i18next'
import type { UsageLog } from '../data/schema'
import type { LogOtherData } from '../types'
import { parseLogOther } from './format'

const AUDIT_ACTION_KEYS: Record<string, string> = {
  'user.create': 'Created user {{username}} (role {{role}})',
  'user.update': 'Updated user {{username}} (ID: {{id}})',
  'user.delete': 'Deleted user {{username}} (ID: {{id}})',
  'user.quota_add': 'Increased user quota by {{quota}}',
  'user.quota_subtract': 'Decreased user quota by {{quota}}',
  'user.quota_override': 'Overrode user quota from {{from}} to {{to}}',
  'user.binding_clear': 'Cleared {{bindingType}} binding for user {{username}}',
  'user.2fa_disable': 'Force-disabled two-factor authentication for the user',
  'user.passkey_register': 'Registered a passkey',
  'user.passkey_delete': 'Deleted a passkey',
  'user.reset_passkey': 'Reset the user passkey',
  'user.topup_complete': 'Completed a user top-up',
  'user.oauth_unbind': 'Unbound a user OAuth account',
  'option.update': 'Updated system setting {{key}}',
  'option.payment_compliance': 'Updated payment compliance settings',
  'option.reset_ratio': 'Reset model ratios',
  'option.clear_affinity_cache': 'Cleared channel affinity cache',
  'channel.create': 'Created channel {{name}} (type {{type}}, count {{count}})',
  'channel.update': 'Updated channel {{name}} (ID: {{id}})',
  'channel.delete': 'Deleted channel {{name}} (ID: {{id}})',
  'channel.delete_batch': 'Batch deleted {{count}} channels',
  'channel.delete_disabled': 'Deleted all disabled channels ({{count}})',
  'channel.key_view': 'Viewed channel key {{name}} (ID: {{id}})',
  'channel.tag_disable': 'Disabled channels with tag {{tag}}',
  'channel.tag_enable': 'Enabled channels with tag {{tag}}',
  'channel.tag_edit': 'Edited channels with tag {{tag}}',
  'channel.tag_batch_set': 'Batch set tag for {{count}} channels',
  'channel.copy':
    'Copied channel (source ID: {{sourceId}}) to {{name}} (new ID: {{id}})',
  'channel.multi_key_manage':
    'Multi-key management {{action}} on channel (ID: {{id}})',
  'channel.upstream_apply':
    'Applied upstream model changes to channel (ID: {{id}})',
  'channel.upstream_apply_all':
    'Applied upstream model changes to {{count}} channels',
  'redemption.create':
    'Created {{count}} redemption codes named {{name}} ({{quota}} each)',
  'redemption.update': 'Updated a redemption code',
  'redemption.delete': 'Deleted a redemption code',
  'redemption.delete_invalid': 'Deleted invalid redemption codes',
  'custom_oauth.create': 'Created a custom OAuth provider',
  'custom_oauth.update': 'Updated a custom OAuth provider',
  'custom_oauth.delete': 'Deleted a custom OAuth provider',
  'performance.clear_disk_cache': 'Cleared disk cache',
  'performance.gc': 'Ran garbage collection',
  'performance.clear_logs': 'Cleared system logs',
  'prefill_group.create': 'Created a prefill group',
  'prefill_group.update': 'Updated a prefill group',
  'prefill_group.delete': 'Deleted a prefill group',
  'vendor.create': 'Created a vendor',
  'vendor.update': 'Updated a vendor',
  'vendor.delete': 'Deleted a vendor',
  'model.create': 'Created model metadata',
  'model.update': 'Updated model metadata',
  'model.delete': 'Deleted model metadata',
  'model.sync_upstream': 'Synchronized upstream models',
  'deployment.create': 'Created a model deployment',
  'deployment.update': 'Updated a model deployment',
  'deployment.delete': 'Deleted a model deployment',
  'subscription.plan_create': 'Created a subscription plan',
  'subscription.plan_update': 'Updated a subscription plan',
  'subscription.bind': 'Bound a subscription plan to a user',
  'subscription.plan_reset': 'Reset active subscriptions for plan {{plan_id}}',
  'subscription.user_plan_reset':
    'Reset active plan {{plan_id}} subscriptions for user {{target_user_id}}',
  'log.cleanup_start': 'Started usage log cleanup',
}

const MANAGE_USER_ACTION_KEYS: Record<string, string> = {
  disable: 'Disabled user {{username}} (ID: {{id}})',
  enable: 'Enabled user {{username}} (ID: {{id}})',
  delete: 'Deleted user {{username}} (ID: {{id}})',
  promote: 'Promoted user {{username}} (ID: {{id}}) to administrator',
  demote: 'Demoted user {{username}} (ID: {{id}}) to regular user',
}

function localizedLoginMethod(method: unknown, t: TFunction): string {
  const normalized = String(method ?? '').trim()
  if (normalized.startsWith('oauth:')) {
    const provider = normalized.slice('oauth:'.length).trim()
    return provider ? t('OAuth ({{provider}})', { provider }) : t('OAuth')
  }

  const keys: Record<string, string> = {
    password: 'Password',
    '2fa': 'Two-factor authentication',
    passkey: 'Passkey',
    wechat: 'WeChat',
    telegram: 'Telegram',
    oauth: 'OAuth',
    unknown: 'Unknown method',
  }
  return t(keys[normalized.toLowerCase()] ?? 'Unknown method')
}

function localizedChannelStatus(status: unknown, t: TFunction): string {
  if (Number(status) === 1) return t('Enabled')
  if (Number(status) === 2) return t('Disabled')
  return String(status ?? '')
}

function renderStructuredContent(
  other: LogOtherData | null,
  t: TFunction
): string | null {
  const action = other?.op?.action
  if (!action) return null

  const params = { ...(other?.op?.params ?? {}) }
  if (action === 'login') {
    return t('Logged in successfully via {{method}}', {
      method: localizedLoginMethod(
        params.method ?? other?.login_method ?? 'unknown',
        t
      ),
    })
  }
  if (action === 'generic') {
    const method = params.method ?? other?.audit_info?.method ?? ''
    const route = params.route ?? other?.audit_info?.route ?? ''
    return t('Performed {{method}} request on {{route}}', { method, route })
  }
  if (action === 'user.manage') {
    const actionKey = MANAGE_USER_ACTION_KEYS[String(params.action ?? '')]
    if (actionKey) return t(actionKey, params)
    return t('Performed {{action}} on user {{username}} (ID: {{id}})', params)
  }
  if (action === 'channel.status_update') {
    return t('Updated channel {{id}} status to {{status}}', {
      ...params,
      status: localizedChannelStatus(params.status, t),
    })
  }
  if (action === 'channel.status_update_batch') {
    return t('Updated {{count}} of {{total}} channels to {{status}}', {
      ...params,
      status: localizedChannelStatus(params.status, t),
    })
  }

  const key = AUDIT_ACTION_KEYS[action]
  return key ? t(key, params) : null
}

type LegacyMatch = {
  key: string
  values?: Record<string, string>
}

function legacyMatch(
  content: string,
  pattern: RegExp,
  key: string,
  names: string[]
): LegacyMatch | null {
  const match = content.match(pattern)
  if (!match) return null
  return {
    key,
    values: Object.fromEntries(
      names.map((name, index) => [name, match[index + 1]])
    ),
  }
}

function matchLegacyContent(content: string): LegacyMatch | null {
  const exact: Record<string, string> = {
    开始设置两步验证: 'Started two-factor authentication setup',
    成功启用两步验证: 'Enabled two-factor authentication successfully',
    禁用两步验证: 'Disabled two-factor authentication',
    重新生成两步验证备用码:
      'Regenerated two-factor authentication backup codes',
    管理员强制禁用了用户的两步验证:
      'Force-disabled two-factor authentication for the user',
    '通用安全验证成功 (验证方式: 2FA)':
      'Security verification succeeded via two-factor authentication',
    代理调整用户额度: 'Agent adjusted user quota',
    代理使用结算余额给用户充值:
      'Agent topped up a user with settlement balance',
    管理员增加代理结算余额: 'Administrator increased agent settlement balance',
  }
  if (exact[content]) return { key: exact[content] }

  const patterns: Array<LegacyMatch | null> = [
    legacyMatch(
      content,
      /^Logged in successfully via (.+)$/,
      'Logged in successfully via {{method}}',
      ['method']
    ),
    legacyMatch(
      content,
      /^admin cleared (.+) binding for user (.+)$/,
      'Cleared {{bindingType}} binding for user {{username}}',
      ['bindingType', 'username']
    ),
    legacyMatch(
      content,
      /^created workspace account (.+) \((\d+)\)$/,
      'Created workspace account {{username}} (ID: {{id}})',
      ['username', 'id']
    ),
    legacyMatch(
      content,
      /^updated workspace account (.+) \((\d+)\)$/,
      'Updated workspace account {{username}} (ID: {{id}})',
      ['username', 'id']
    ),
    legacyMatch(
      content,
      /^changed workspace account (\d+) status to (\d+)$/,
      'Changed workspace account {{id}} status to {{status}}',
      ['id', 'status']
    ),
    legacyMatch(
      content,
      /^reset workspace account (\d+) password$/,
      'Reset workspace account {{id}} password',
      ['id']
    ),
    legacyMatch(
      content,
      /^deleted workspace account (\d+)$/,
      'Deleted workspace account {{id}}',
      ['id']
    ),
    legacyMatch(
      content,
      /^set workspace (\d+) members to (.+)$/,
      'Set workspace {{id}} members to {{members}}',
      ['id', 'members']
    ),
    legacyMatch(
      content,
      /^revoked workspace (\d+) access$/,
      'Revoked workspace {{id}} access',
      ['id']
    ),
    legacyMatch(
      content,
      /^set workspace account (\d+) workspaces to (.+)$/,
      'Set workspace account {{id}} workspaces to {{workspaces}}',
      ['id', 'workspaces']
    ),
    legacyMatch(
      content,
      /^查看渠道密钥信息 \(渠道ID: (\d+)\)$/,
      'Viewed channel key (channel ID: {{id}})',
      ['id']
    ),
    legacyMatch(
      content,
      /^用户签到，获得额度 (.+)$/,
      'Checked in and received {{quota}} quota',
      ['quota']
    ),
    legacyMatch(
      content,
      /^新用户注册赠送 (.+)$/,
      'Received {{quota}} quota for new user registration',
      ['quota']
    ),
    legacyMatch(
      content,
      /^使用邀请码赠送 (.+)$/,
      'Received {{quota}} quota for using an invitation code',
      ['quota']
    ),
    legacyMatch(
      content,
      /^邀请用户赠送 (.+)$/,
      'Received {{quota}} quota for inviting a user',
      ['quota']
    ),
    legacyMatch(
      content,
      /^邀请用户 (\d+) 充值奖励到账，充值额度: (.+)，奖励额度: (.+)$/,
      'Received a top-up reward from invited user {{userId}}: top-up {{topupQuota}}, reward {{rewardQuota}}',
      ['userId', 'topupQuota', 'rewardQuota']
    ),
    legacyMatch(
      content,
      /^邀请用户 (\d+) 的 (.+) 实际消耗奖励到账，消耗额度: (.+)，奖励额度: (.+)$/,
      'Received a usage reward from invited user {{userId}} for {{date}}: usage {{usageQuota}}, reward {{rewardQuota}}',
      ['userId', 'date', 'usageQuota', 'rewardQuota']
    ),
    legacyMatch(
      content,
      /^管理员向用户发送邮件: (.+)$/,
      'Administrator sent an email to the user: {{subject}}',
      ['subject']
    ),
    legacyMatch(
      content,
      /^管理员群发公告邮件: (.+)$/,
      'Administrator sent an announcement email: {{title}}',
      ['title']
    ),
    legacyMatch(
      content,
      /^管理员增加用户额度 (.+)$/,
      'Administrator increased user quota by {{quota}}',
      ['quota']
    ),
    legacyMatch(
      content,
      /^管理员减少用户额度 (.+)$/,
      'Administrator decreased user quota by {{quota}}',
      ['quota']
    ),
    legacyMatch(
      content,
      /^管理员覆盖用户额度从 (.+) 为 (.+)$/,
      'Administrator changed user quota from {{from}} to {{to}}',
      ['from', 'to']
    ),
    legacyMatch(
      content,
      /^管理员登记用户还款 (.+)，待还信控额度从 (.+) 减少至 (.+)$/,
      'Administrator recorded a repayment of {{quota}}; outstanding credit quota decreased from {{from}} to {{to}}',
      ['quota', 'from', 'to']
    ),
    legacyMatch(
      content,
      /^管理员发放信控额度 (.+)，用户余额从 (.+) 增加至 (.+)，待还信控额度为 (.+)$/,
      'Administrator granted {{quota}} credit quota; user balance increased from {{from}} to {{to}}, outstanding credit quota {{creditQuota}}',
      ['quota', 'from', 'to', 'creditQuota']
    ),
    legacyMatch(
      content,
      /^管理员重置订阅套餐 (.+)（ID: (\d+)）额度$/,
      'Administrator reset quota for subscription plan {{title}} (ID: {{id}})',
      ['title', 'id']
    ),
  ]
  return patterns.find((value) => value != null) ?? null
}

/**
 * Localize system, management, and login log details. Structured operation
 * metadata is preferred; exact legacy content patterns keep historical logs
 * translatable without changing provider/error messages.
 */
export function getLocalizedLogContent(log: UsageLog, t: TFunction): string {
  const content = log.content ?? ''
  if (![3, 4, 7].includes(log.type)) return content

  const structured = renderStructuredContent(parseLogOther(log.other), t)
  if (structured) return structured

  const legacy = matchLegacyContent(content)
  if (!legacy) return content
  if (legacy.key === 'Logged in successfully via {{method}}') {
    return t(legacy.key, {
      method: localizedLoginMethod(legacy.values?.method, t),
    })
  }
  if (legacy.key === 'Changed workspace account {{id}} status to {{status}}') {
    return t(legacy.key, {
      ...legacy.values,
      status: localizedChannelStatus(legacy.values?.status, t),
    })
  }
  return t(legacy.key, legacy.values)
}
