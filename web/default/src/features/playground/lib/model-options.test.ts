import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { PricingModel } from '@/features/pricing/types'
import { buildPlaygroundModelOptions } from './model-options'

function pricingModel(
  model_name: string,
  enable_groups: string[],
  alias_models?: string[]
): PricingModel {
  return {
    id: 0,
    model_name,
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups,
    alias_models,
  }
}

describe('playground model options', () => {
  test('shows model square model names for the selected group', () => {
    const options = buildPlaygroundModelOptions(
      [
        pricingModel('gpt-main', ['default'], ['gpt-main-preview']),
        pricingModel('claude-main', ['vip']),
        pricingModel('shared-main', ['all']),
      ],
      'default'
    )

    assert.deepEqual(
      options.map((option) => option.value),
      ['gpt-main', 'shared-main']
    )
  })

  test('does not expose alias model names as selectable models', () => {
    const options = buildPlaygroundModelOptions(
      [pricingModel('gpt-main', ['default'], ['gpt-main-preview'])],
      'default'
    )

    assert.deepEqual(options, [{ label: 'gpt-main', value: 'gpt-main' }])
  })
})
