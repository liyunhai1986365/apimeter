import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  getApiKeyFormDefaultValues,
  addGroupToChain,
  addGroupsToChain,
  getApiKeyGroupDisplayItems,
  getApiKeyRoutingStrategyLabel,
  removeGroupFromChain,
  transformFormDataToPayload,
  transformApiKeyToFormDefaults,
} from './api-key-form'
import {
  AUTO_GROUP_VALUE,
  buildApiKeyGroupOptions,
  shouldFallbackApiKeyGroup,
} from './api-key-groups'

describe('api key group options', () => {
  test('creates new API keys with smart routing by default', () => {
    const defaults = getApiKeyFormDefaultValues()
    const payload = transformFormDataToPayload({
      ...defaults,
      name: 'test-key',
    })

    assert.equal(defaults.routing_mode, 'smart')
    assert.equal(defaults.routing_strategy, 'smart_auto')
    assert.deepEqual(defaults.group_chain, [AUTO_GROUP_VALUE])
    assert.equal(defaults.cross_group_retry, true)
    assert.equal(payload.group, AUTO_GROUP_VALUE)
    assert.equal(
      payload.group_policy,
      '{"type":"routing_strategy","strategy":"smart_auto"}'
    )
    assert.equal(payload.cross_group_retry, true)
  })

  test('serializes excluded suppliers for smart routing keys', () => {
    const defaults = getApiKeyFormDefaultValues()
    const payload = transformFormDataToPayload({
      ...defaults,
      name: 'smart-key',
      routing_strategy: 'price_first',
      routing_excluded_groups: ['vip', 'backup', 'vip'],
    })

    assert.equal(payload.group, AUTO_GROUP_VALUE)
    assert.equal(
      payload.group_policy,
      '{"type":"routing_strategy","strategy":"price_first","excluded_groups":["vip","backup"]}'
    )
  })

  test('serializes ordered group chain in payload', () => {
    const defaults = getApiKeyFormDefaultValues()
    const payload = transformFormDataToPayload({
      ...defaults,
      name: 'ordered-key',
      routing_mode: 'manual',
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

  test('batch adding groups replaces auto, preserves order, and skips duplicates', () => {
    assert.deepEqual(
      addGroupsToChain([AUTO_GROUP_VALUE], ['vip', 'backup', 'vip']),
      ['vip', 'backup']
    )
    assert.deepEqual(addGroupsToChain(['vip'], ['backup', 'vip', 'economy']), [
      'vip',
      'backup',
      'economy',
    ])
  })

  test('previews only the primary group and hides extra groups in API key list', () => {
    const display = getApiKeyGroupDisplayItems({
      group: 'vip',
      group_policy: '{"type":"ordered","groups":["vip","backup","economy"]}',
    })

    assert.deepEqual(display.visibleGroups, ['vip'])
    assert.deepEqual(display.hiddenGroups, ['backup', 'economy'])
    assert.equal(display.hiddenCount, 2)
  })

  test('shows smart routing strategy name in API key list display', () => {
    const display = getApiKeyGroupDisplayItems({
      group: AUTO_GROUP_VALUE,
      group_policy: '{"type":"routing_strategy","strategy":"price_first"}',
    })

    assert.equal(display.routingStrategy, 'price_first')
    assert.equal(
      getApiKeyRoutingStrategyLabel(display.routingStrategy),
      'Price first'
    )
    assert.deepEqual(display.visibleGroups, [])
    assert.equal(display.hiddenCount, 0)
  })

  test('shows excluded suppliers in smart routing list display', () => {
    const display = getApiKeyGroupDisplayItems({
      group: AUTO_GROUP_VALUE,
      group_policy:
        '{"type":"routing_strategy","strategy":"smart_auto","excluded_groups":["vip","backup","vip","auto"]}',
    })

    assert.equal(display.routingStrategy, 'smart_auto')
    assert.deepEqual(display.excludedGroups, ['vip', 'backup'])
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

  test('does not include auto in manual supplier options', () => {
    const options = buildApiKeyGroupOptions(
      {
        default: { desc: 'Default group', ratio: 1 },
      },
      true
    )

    assert.equal(
      options.some((option) => option.value === AUTO_GROUP_VALUE),
      false
    )
    assert.equal(options[0]?.value, 'default')
  })

  test('filters backend auto group metadata from manual supplier options', () => {
    const options = buildApiKeyGroupOptions(
      {
        auto: { desc: 'Backend auto', ratio: '' },
        default: { desc: 'Default group', ratio: 1 },
      },
      true
    )

    assert.equal(
      options.filter((option) => option.value === AUTO_GROUP_VALUE).length,
      0
    )
    assert.equal(options[0]?.value, 'default')
  })

  test('falls back from auto in manual supplier options', () => {
    const options = buildApiKeyGroupOptions(
      {
        default: { desc: 'Default group', ratio: 1 },
      },
      true
    )

    assert.equal(shouldFallbackApiKeyGroup(AUTO_GROUP_VALUE, options), true)
    assert.equal(shouldFallbackApiKeyGroup('missing', options), true)
  })

  test('does not add auto option when editing an existing auto key', () => {
    const options = buildApiKeyGroupOptions(
      {
        default: { desc: 'Default group', ratio: 1 },
      },
      false,
      AUTO_GROUP_VALUE
    )

    assert.equal(options[0]?.value, 'default')
    assert.equal(shouldFallbackApiKeyGroup(AUTO_GROUP_VALUE, options), true)
  })

  test('converts legacy auto keys to smart automatic routing when editing', () => {
    const defaults = transformApiKeyToFormDefaults({
      id: 1,
      workspace_id: 0,
      workspace_name: '',
      name: 'legacy-auto',
      key: 'sk-test',
      status: 1,
      group: AUTO_GROUP_VALUE,
      group_policy: '',
      remain_quota: 0,
      unlimited_quota: true,
      expired_time: -1,
      model_limits_enabled: false,
      model_limits: '',
      allow_ips: '',
      used_quota: 0,
      today_used_quota: 0,
      created_time: 0,
      accessed_time: 0,
      cross_group_retry: true,
      image_settings: { format: 'follow_request', store: 'default' },
    })

    assert.equal(defaults.routing_mode, 'smart')
    assert.equal(defaults.routing_strategy, 'smart_auto')
    assert.deepEqual(defaults.group_chain, [AUTO_GROUP_VALUE])
  })

  test('parses excluded suppliers for smart routing keys when editing', () => {
    const defaults = transformApiKeyToFormDefaults({
      id: 2,
      workspace_id: 0,
      workspace_name: '',
      name: 'smart-exclude',
      key: 'sk-test',
      status: 1,
      group: AUTO_GROUP_VALUE,
      group_policy:
        '{"type":"routing_strategy","strategy":"success_first","excluded_groups":["vip","backup"]}',
      remain_quota: 0,
      unlimited_quota: true,
      expired_time: -1,
      model_limits_enabled: false,
      model_limits: '',
      allow_ips: '',
      used_quota: 0,
      today_used_quota: 0,
      created_time: 0,
      accessed_time: 0,
      cross_group_retry: true,
      image_settings: { format: 'follow_request', store: 'default' },
    })

    assert.equal(defaults.routing_mode, 'smart')
    assert.equal(defaults.routing_strategy, 'success_first')
    assert.deepEqual(defaults.routing_excluded_groups, ['vip', 'backup'])
  })
})
