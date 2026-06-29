import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { PricingModel } from '../types'
import {
  buildModelCardGroups,
  sortModelsByCardGroupOrder,
} from './model-card-groups'

function model(input: Partial<PricingModel> & { model_name: string }): PricingModel {
  return {
    id: 0,
    model_name: input.model_name,
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: [],
    ...input,
  }
}

describe('model card groups', () => {
  test('groups models by configured vendor order and sorts models inside each group', () => {
    const groups = buildModelCardGroups([
      model({
        model_name: 'z-late',
        vendor_id: 20,
        vendor_name: 'Vendor B',
        vendor_sort_order: 10,
        sort_order: 20,
      }),
      model({
        model_name: 'a-early',
        vendor_id: 20,
        vendor_name: 'Vendor B',
        vendor_sort_order: 10,
        sort_order: 10,
      }),
      model({
        model_name: 'vendor-a-model',
        vendor_id: 10,
        vendor_name: 'Vendor A',
        vendor_sort_order: 20,
      }),
    ])

    assert.deepEqual(
      groups.map((group) => group.name),
      ['Vendor B', 'Vendor A']
    )
    assert.deepEqual(
      groups[0].models.map((item) => item.model_name),
      ['a-early', 'z-late']
    )
  })

  test('flattens models using the same vendor and model order as card groups', () => {
    const sorted = sortModelsByCardGroupOrder([
      model({
        model_name: 'z-late',
        vendor_id: 20,
        vendor_name: 'Vendor B',
        vendor_sort_order: 10,
        sort_order: 20,
      }),
      model({
        model_name: 'vendor-a-model',
        vendor_id: 10,
        vendor_name: 'Vendor A',
        vendor_sort_order: 20,
      }),
      model({
        model_name: 'a-early',
        vendor_id: 20,
        vendor_name: 'Vendor B',
        vendor_sort_order: 10,
        sort_order: 10,
      }),
    ])

    assert.deepEqual(
      sorted.map((item) => item.model_name),
      ['a-early', 'z-late', 'vendor-a-model']
    )
  })

  test('prioritizes models that have metadata inside the same vendor group', () => {
    const groups = buildModelCardGroups([
      model({
        model_name: 'configured-but-no-metadata',
        vendor_id: 20,
        vendor_name: 'Vendor B',
        sort_order: 10,
      }),
      model({
        model_name: 'metadata-created-high-order',
        vendor_id: 20,
        vendor_name: 'Vendor B',
        sort_order: 30,
        ...({ has_metadata: true } as Partial<PricingModel>),
      }),
      model({
        model_name: 'metadata-created-low-order',
        vendor_id: 20,
        vendor_name: 'Vendor B',
        sort_order: 20,
        ...({ has_metadata: true } as Partial<PricingModel>),
      }),
    ])

    assert.deepEqual(
      groups[0].models.map((item) => item.model_name),
      [
        'metadata-created-low-order',
        'metadata-created-high-order',
        'configured-but-no-metadata',
      ]
    )
  })
})
