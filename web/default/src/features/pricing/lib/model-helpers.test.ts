import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { PricingModel, PricingGroupDisplayConfig } from '../types'
import { getAvailableGroups } from './model-helpers'

function model(enableGroups: string[]): PricingModel {
  return {
    id: 1,
    model_name: 'test-model',
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: enableGroups,
  }
}

describe('pricing model helpers', () => {
  test('sorts available groups by category order and group order', () => {
    const groupDisplay: PricingGroupDisplayConfig = {
      categories: [
        { id: 'official', name: 'Official', order: 20 },
        { id: 'partner', name: 'Partner', order: 10 },
      ],
      groups: [
        { group: 'default', category_id: 'official', order: 10 },
        { group: 'vip', category_id: 'partner', order: 20 },
        { group: 'backup', category_id: 'partner', order: 10 },
      ],
    }

    const result = getAvailableGroups(
      model(['default', 'vip', 'backup', 'other']),
      {
        default: 'Default group',
        vip: 'VIP group',
        backup: 'Backup group',
        other: 'Other group',
      },
      groupDisplay
    )

    assert.deepEqual(result, ['backup', 'vip', 'default', 'other'])
  })
})
