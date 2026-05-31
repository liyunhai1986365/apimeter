import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { FILTER_ALL, MODEL_CATEGORIES, SORT_OPTIONS } from '../constants'
import { filterAndSortModels } from './filters'
import type { PricingModel, ModelCategory } from '../types'

const baseFilters = {
  search: '',
  vendor: FILTER_ALL,
  group: FILTER_ALL,
  quotaType: FILTER_ALL,
  endpointType: FILTER_ALL,
  category: MODEL_CATEGORIES.ALL,
  tag: FILTER_ALL,
  sortBy: SORT_OPTIONS.NAME,
}

function model(name: string, category?: ModelCategory): PricingModel {
  return {
    id: 0,
    model_name: name,
    category,
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: [],
  }
}

describe('pricing model filters', () => {
  test('filters model square results by model category', () => {
    const result = filterAndSortModels(
      [model('embedding-model', 'vector'), model('chat-model', 'text')],
      {
        ...baseFilters,
        category: MODEL_CATEGORIES.VECTOR,
      }
    )

    assert.deepEqual(
      result.map((item) => item.model_name),
      ['embedding-model']
    )
  })

  test('searches model square results by alias models', () => {
    const main = model('main-model', 'text')
    main.alias_models = ['preview-model', 'legacy-model']

    const result = filterAndSortModels([main, model('other-model', 'text')], {
      ...baseFilters,
      search: 'preview',
    })

    assert.deepEqual(
      result.map((item) => item.model_name),
      ['main-model']
    )
  })

  test('sorts model square name view by configured model order first', () => {
    const early = model('z-model', 'text')
    early.sort_order = 10
    const late = model('a-model', 'text')
    late.sort_order = 20

    const result = filterAndSortModels([late, early], baseFilters)

    assert.deepEqual(
      result.map((item) => item.model_name),
      ['z-model', 'a-model']
    )
  })
})
