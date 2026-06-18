import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  getApiKeyFormDefaultValues,
  addGroupToChain,
  getApiKeyGroupDisplayItems,
  removeGroupFromChain,
  transformFormDataToPayload,
} from './api-key-form'
import {
  AUTO_GROUP_VALUE,
  buildApiKeyGroupOptions,
  shouldFallbackApiKeyGroup,
} from './api-key-groups'

describe('api key group options', () => {
  test('creates new API keys with auto group by default', () => {
    const defaults = getApiKeyFormDefaultValues()
    const payload = transformFormDataToPayload({
      ...defaults,
      name: 'test-key',
    })

    assert.deepEqual(defaults.group_chain, [AUTO_GROUP_VALUE])
    assert.equal(defaults.cross_group_retry, true)
    assert.equal(payload.group, AUTO_GROUP_VALUE)
    assert.equal(payload.group_policy, '')
    assert.equal(payload.cross_group_retry, true)
  })

  test('serializes ordered group chain in payload', () => {
    const defaults = getApiKeyFormDefaultValues()
    const payload = transformFormDataToPayload({
      ...defaults,
      name: 'ordered-key',
      group_chain: ['vip', 'backup'],
    })

    assert.equal(payload.group, 'vip')
    assert.equal(
      payload.group_policy,
      '{"type":"ordered","groups":["vip","backup"]}'
    )
    assert.equal(payload.cross_group_retry, true)
  })

  test('replaces auto when adding the first concrete group and restores auto when empty', () => {
    assert.deepEqual(addGroupToChain([AUTO_GROUP_VALUE], 'vip'), ['vip'])
    assert.deepEqual(addGroupToChain(['vip'], 'backup'), ['vip', 'backup'])
    assert.deepEqual(removeGroupFromChain(['vip'], 0), [AUTO_GROUP_VALUE])
  })

  test('selecting auto clears all concrete groups', () => {
    assert.deepEqual(addGroupToChain(['vip', 'backup'], AUTO_GROUP_VALUE), [
      AUTO_GROUP_VALUE,
    ])
  })

  test('previews only the primary group and hides extra groups in API key list', () => {
    const display = getApiKeyGroupDisplayItems(
      {
        group: 'vip',
        group_policy: '{"type":"ordered","groups":["vip","backup","economy"]}',
      }
    )

    assert.deepEqual(display.visibleGroups, ['vip'])
    assert.deepEqual(display.hiddenGroups, ['backup', 'economy'])
    assert.equal(display.hiddenCount, 2)
  })

  test('resets image storage strategy unless URL response format is selected', () => {
    const defaults = getApiKeyFormDefaultValues()
    const payload = transformFormDataToPayload({
      ...defaults,
      name: 'image-key',
      image_response_format: 'b64_json',
      image_store_strategy: 'force_store_url_and_base64',
    })

    assert.deepEqual(payload.image_settings, {
      format: 'b64_json',
      store: 'default',
    })
  })

  test('stores API key image storage strategy when URL response format is selected', () => {
    const defaults = getApiKeyFormDefaultValues()
    const payload = transformFormDataToPayload({
      ...defaults,
      name: 'image-key',
      image_response_format: 'url',
      image_store_strategy: 'force_store_url_and_base64',
    })

    assert.deepEqual(payload.image_settings, {
      format: 'url',
      store: 'force_store_url_and_base64',
    })
  })

  test('adds auto group option when default auto group is enabled', () => {
    const options = buildApiKeyGroupOptions(
      {
        default: { desc: 'Default group', ratio: 1 },
      },
      true
    )

    assert.equal(options[0]?.value, AUTO_GROUP_VALUE)
    assert.equal(options[0]?.label, 'auto 自动供应商')
    assert.equal(
      options[0]?.desc,
      'Automatically selects the best available supplier with circuit breaker mechanism'
    )
    assert.equal(options[1]?.value, 'default')
  })

  test('keeps backend auto group metadata when it is already returned', () => {
    const options = buildApiKeyGroupOptions(
      {
        auto: { desc: 'Backend auto', ratio: '' },
        default: { desc: 'Default group', ratio: 1 },
      },
      true
    )

    assert.equal(
      options.filter((option) => option.value === AUTO_GROUP_VALUE).length,
      1
    )
    assert.equal(options[0]?.label, 'auto 自动供应商')
    assert.equal(options[0]?.desc, 'Backend auto')
  })

  test('does not fallback from auto when auto default is enabled', () => {
    const options = buildApiKeyGroupOptions(
      {
        default: { desc: 'Default group', ratio: 1 },
      },
      true
    )

    assert.equal(shouldFallbackApiKeyGroup(AUTO_GROUP_VALUE, options), false)
    assert.equal(shouldFallbackApiKeyGroup('missing', options), true)
  })

  test('adds auto option when editing an existing auto key', () => {
    const options = buildApiKeyGroupOptions(
      {
        default: { desc: 'Default group', ratio: 1 },
      },
      false,
      AUTO_GROUP_VALUE
    )

    assert.equal(options[0]?.value, AUTO_GROUP_VALUE)
    assert.equal(shouldFallbackApiKeyGroup(AUTO_GROUP_VALUE, options), false)
  })
})
