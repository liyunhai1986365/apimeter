import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  buildConfiguredModelNameOptions,
  parseGroupModelRatioRules,
  parseUserGroupModelRatioRules,
  parseUserGroupRatioRules,
  serializeGroupModelRatioRules,
  serializeUserGroupModelRatioRules,
  serializeUserGroupRatioRules,
} from './model-specific-ratio-utils'

describe('model-specific ratio visual editor helpers', () => {
  test('parses and serializes group-model ratios including zero', () => {
    const rules = parseGroupModelRatioRules(
      JSON.stringify({
        alibaba: { 'qwen3.7': 0.5, 'glm-5.2': 0.7 },
        free: { 'free-model': 0 },
      })
    )

    assert.deepEqual(rules, [
      { group: 'alibaba', model: 'glm-5.2', ratio: 0.7 },
      { group: 'alibaba', model: 'qwen3.7', ratio: 0.5 },
      { group: 'free', model: 'free-model', ratio: 0 },
    ])
    assert.deepEqual(JSON.parse(serializeGroupModelRatioRules(rules)), {
      alibaba: { 'glm-5.2': 0.7, 'qwen3.7': 0.5 },
      free: { 'free-model': 0 },
    })
  })

  test('parses and serializes user-group model overrides', () => {
    const rules = parseUserGroupModelRatioRules(
      JSON.stringify({
        vip: {
          alibaba: { 'glm-5.2': 0.62 },
        },
        svip: {
          alibaba: { 'qwen3.7': 0.4 },
        },
      })
    )

    assert.deepEqual(rules, [
      {
        userGroup: 'svip',
        group: 'alibaba',
        model: 'qwen3.7',
        ratio: 0.4,
      },
      {
        userGroup: 'vip',
        group: 'alibaba',
        model: 'glm-5.2',
        ratio: 0.62,
      },
    ])
    assert.deepEqual(JSON.parse(serializeUserGroupModelRatioRules(rules)), {
      svip: { alibaba: { 'qwen3.7': 0.4 } },
      vip: { alibaba: { 'glm-5.2': 0.62 } },
    })
  })

  test('parses and serializes group-wide user-group overrides including zero', () => {
    const rules = parseUserGroupRatioRules(
      JSON.stringify({
        vip: { alibaba: 0.8, free: 0 },
        svip: { alibaba: 0.6 },
      })
    )

    assert.deepEqual(rules, [
      { userGroup: 'svip', group: 'alibaba', ratio: 0.6 },
      { userGroup: 'vip', group: 'alibaba', ratio: 0.8 },
      { userGroup: 'vip', group: 'free', ratio: 0 },
    ])
    assert.deepEqual(JSON.parse(serializeUserGroupRatioRules(rules)), {
      svip: { alibaba: 0.6 },
      vip: { alibaba: 0.8, free: 0 },
    })
  })

  test('collects unique model suggestions from pricing maps', () => {
    assert.deepEqual(
      buildConfiguredModelNameOptions(
        '{"glm-5.2":1,"qwen3.7":2}',
        '{"qwen3.7":0.5,"deepseek-v3":1}',
        'invalid'
      ),
      ['deepseek-v3', 'glm-5.2', 'qwen3.7']
    )
  })
})
