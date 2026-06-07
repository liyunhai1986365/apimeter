import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  appendRetryPolicyRuleToSetting,
  buildRetryPolicyRuleFromLog,
  RETRY_POLICY_TEMPLATES,
} from './retry-policy-templates'

describe('retry policy templates', () => {
  test('appends a retry rule without replacing existing channel setting', () => {
    const setting = JSON.stringify({
      proxy: 'socks5://127.0.0.1:1080',
      retry_policy_rules: [
        {
          name: 'existing',
          action: 'skip_retry',
          status_codes: '400',
        },
      ],
    })

    const next = appendRetryPolicyRuleToSetting(setting, {
      name: 'private ip',
      action: 'retry',
      models: ['gpt-image-2'],
      message_contains: ['private ip'],
    })
    const parsed = JSON.parse(next)

    assert.equal(parsed.proxy, 'socks5://127.0.0.1:1080')
    assert.equal(parsed.retry_policy_rules.length, 2)
    assert.deepEqual(parsed.retry_policy_rules[1], {
      name: 'private ip',
      action: 'retry',
      models: ['gpt-image-2'],
      message_contains: ['private ip'],
    })
  })

  test('builds a channel retry rule from an error log', () => {
    const rule = buildRetryPolicyRuleFromLog({
      id: 1,
      type: 5,
      model_name: 'gpt-image-2',
      channel: 29,
      content: 'status_code=400, download image failed: private ip rejected',
      other: JSON.stringify({
        admin_info: {
          error_code: 'bad_response_status_code',
          status_code: 400,
        },
      }),
    })

    assert.deepEqual(rule, {
      name: 'log#1 gpt-image-2 status 400',
      action: 'retry',
      models: ['gpt-image-2'],
      status_codes: '400',
      error_codes: ['bad_response_status_code'],
      message_contains: ['private ip rejected'],
    })
  })

  test('provides common insertable retry templates', () => {
    assert.ok(RETRY_POLICY_TEMPLATES.length >= 3)
    assert.equal(RETRY_POLICY_TEMPLATES[0].rule.action, 'retry')
  })
})
