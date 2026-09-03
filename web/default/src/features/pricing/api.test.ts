import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { hydratePricingModels } from './api'
import type { PricingData } from './types'

describe('hydratePricingModels', () => {
  test('merges current-user model group ratios over group fallbacks', () => {
    const data = {
      success: true,
      data: [
        {
          id: 1,
          model_name: 'glm-5.2',
          quota_type: 0,
          model_ratio: 1,
          completion_ratio: 1,
          enable_groups: ['alibaba'],
        },
        {
          id: 2,
          model_name: 'qwen3.7',
          quota_type: 0,
          model_ratio: 1,
          completion_ratio: 1,
          enable_groups: ['alibaba'],
        },
      ],
      vendors: [],
      user_group: 'vip',
      group_ratio: { alibaba: 1, backup: 0.8 },
      group_model_ratio: {
        alibaba: { 'glm-5.2': 0.62, 'qwen3.7': 0.5 },
      },
      usable_group: { alibaba: 'Alibaba' },
      supported_endpoint: {},
      auto_groups: [],
    } satisfies PricingData

    const models = hydratePricingModels(data)

    assert.deepEqual(models[0].group_ratio, {
      alibaba: 0.62,
      backup: 0.8,
    })
    assert.deepEqual(models[1].group_ratio, {
      alibaba: 0.5,
      backup: 0.8,
    })
    assert.deepEqual(data.group_ratio, { alibaba: 1, backup: 0.8 })
  })
})
