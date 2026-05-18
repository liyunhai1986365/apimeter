import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  getApiKeyFormDefaultValues,
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

    assert.equal(defaults.group, AUTO_GROUP_VALUE)
    assert.equal(defaults.cross_group_retry, true)
    assert.equal(payload.group, AUTO_GROUP_VALUE)
    assert.equal(payload.cross_group_retry, true)
  })

  test('adds auto group option when default auto group is enabled', () => {
    const options = buildApiKeyGroupOptions(
      {
        default: { desc: 'Default group', ratio: 1 },
      },
      true
    )

    assert.equal(options[0]?.value, AUTO_GROUP_VALUE)
    assert.equal(options[0]?.label, 'auto 自动分组')
    assert.equal(
      options[0]?.desc,
      'Automatically selects the best available group with circuit breaker mechanism'
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
    assert.equal(options[0]?.label, 'auto 自动分组')
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
