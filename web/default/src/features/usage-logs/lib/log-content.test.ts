import type { TFunction } from 'i18next'
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import zh from '../../../i18n/locales/zh.json'
import type { UsageLog } from '../data/schema'
import { getLocalizedLogContent } from './log-content'

function interpolate(value: string, options?: Record<string, unknown>): string {
  return Object.entries(options ?? {}).reduce(
    (result, [name, replacement]) =>
      result.replaceAll(`{{${name}}}`, String(replacement)),
    value
  )
}

const t = ((key: string, options?: Record<string, unknown>) =>
  interpolate(key, options)) as TFunction

const zhTranslations = zh.translation as Record<string, string>
const zhT = ((key: string, options?: Record<string, unknown>) =>
  interpolate(zhTranslations[key] ?? key, options)) as TFunction

function usageLog(overrides: Partial<UsageLog>): UsageLog {
  return {
    id: 1,
    user_id: 1,
    created_at: 1779955200,
    type: 4,
    content: '',
    username: '',
    token_name: '',
    model_name: '',
    quota: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    use_time: 0,
    is_stream: false,
    channel: 0,
    channel_name: '',
    token_id: 0,
    group: '',
    ip: '',
    other: '',
    request_id: '',
    upstream_request_id: '',
    ...overrides,
  }
}

describe('getLocalizedLogContent', () => {
  test('localizes structured password and OAuth login methods', () => {
    assert.equal(
      getLocalizedLogContent(
        usageLog({
          type: 7,
          content: 'Logged in successfully via password',
          other: JSON.stringify({
            op: { action: 'login', params: { method: 'password' } },
          }),
        }),
        t
      ),
      'Logged in successfully via Password'
    )
    assert.equal(
      getLocalizedLogContent(
        usageLog({
          type: 7,
          content: 'Logged in successfully via password',
          other: JSON.stringify({
            op: { action: 'login', params: { method: 'password' } },
          }),
        }),
        zhT
      ),
      '已通过密码成功登录'
    )
    assert.equal(
      getLocalizedLogContent(
        usageLog({
          type: 7,
          other: JSON.stringify({
            op: { action: 'login', params: { method: 'oauth:github' } },
          }),
        }),
        t
      ),
      'Logged in successfully via OAuth (github)'
    )
  })

  test('localizes structured management actions and their values', () => {
    assert.equal(
      getLocalizedLogContent(
        usageLog({
          type: 3,
          other: JSON.stringify({
            op: {
              action: 'user.manage',
              params: { action: 'enable', username: 'alice', id: 42 },
            },
          }),
        }),
        t
      ),
      'Enabled user alice (ID: 42)'
    )
    assert.equal(
      getLocalizedLogContent(
        usageLog({
          type: 3,
          other: JSON.stringify({
            op: {
              action: 'channel.status_update',
              params: { id: 9, status: 1 },
            },
          }),
        }),
        t
      ),
      'Updated channel 9 status to Enabled'
    )
  })

  test('localizes historical English and Chinese log content', () => {
    assert.equal(
      getLocalizedLogContent(
        usageLog({
          type: 7,
          content: 'Logged in successfully via passkey',
        }),
        t
      ),
      'Logged in successfully via Passkey'
    )
    assert.equal(
      getLocalizedLogContent(
        usageLog({
          content: '用户签到，获得额度 $1.25',
        }),
        t
      ),
      'Checked in and received $1.25 quota'
    )
    assert.equal(
      getLocalizedLogContent(
        usageLog({
          type: 3,
          content: 'created workspace account alice (42)',
        }),
        t
      ),
      'Created workspace account alice (ID: 42)'
    )
  })

  test('keeps unknown and provider error content unchanged', () => {
    assert.equal(
      getLocalizedLogContent(usageLog({ content: 'future system event' }), t),
      'future system event'
    )
    assert.equal(
      getLocalizedLogContent(
        usageLog({ type: 5, content: 'upstream timeout' }),
        t
      ),
      'upstream timeout'
    )
  })
})
