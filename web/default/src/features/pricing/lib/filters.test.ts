import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  FILTER_ALL,
  MODALITY_TYPES,
  MODEL_CATEGORIES,
  SORT_OPTIONS,
} from '../constants'
import type { PricingModel, ModelCategory, Modality } from '../types'
import { filterAndSortModels } from './filters'

const baseFilters = {
  search: '',
  vendor: FILTER_ALL,
  group: FILTER_ALL,
  quotaType: FILTER_ALL,
  endpointType: FILTER_ALL,
  category: MODEL_CATEGORIES.ALL,
  tag: FILTER_ALL,
  inputModality: MODALITY_TYPES.ALL,
  outputModality: MODALITY_TYPES.ALL,
  sortBy: SORT_OPTIONS.NAME,
}

function model(
  name: string,
  category?: ModelCategory,
  modalities?: {
    input?: Modality[]
    output?: Modality[]
  },
  overrides: Partial<PricingModel> = {}
): PricingModel {
  return {
    id: 0,
    model_name: name,
    category,
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: [],
    input_modalities: modalities?.input,
    output_modalities: modalities?.output,
    ...overrides,
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

  test('sorts models with substantive metadata before bare configured models', () => {
    const bare = model('configured-but-bare', 'text')
    bare.sort_order = 0
    const curated = model('curated-model', 'text')
    curated.sort_order = 100
    curated.has_metadata = true

    const result = filterAndSortModels([bare, curated], baseFilters)

    assert.deepEqual(
      result.map((item) => item.model_name),
      ['curated-model', 'configured-but-bare']
    )
  })

  test('sorts equal model order by newest update time first', () => {
    const older = model('a-older-model', 'text')
    older.sort_order = 100
    older.updated_time = 100
    const newer = model('z-newer-model', 'text')
    newer.sort_order = 100
    newer.updated_time = 200

    const result = filterAndSortModels([older, newer], baseFilters)

    assert.deepEqual(
      result.map((item) => item.model_name),
      ['z-newer-model', 'a-older-model']
    )
  })

  test('filters model square results by input and output modality', () => {
    const result = filterAndSortModels(
      [
        model('vision-chat', 'text', {
          input: ['text', 'image'],
          output: ['text'],
        }),
        model('image-generator', 'image', {
          input: ['text'],
          output: ['image'],
        }),
        model('audio-transcriber', 'audio', {
          input: ['audio'],
          output: ['text'],
        }),
      ],
      {
        ...baseFilters,
        inputModality: MODALITY_TYPES.TEXT,
        outputModality: MODALITY_TYPES.IMAGE,
      }
    )

    assert.deepEqual(
      result.map((item) => item.model_name),
      ['image-generator']
    )
  })

  test('filters model square results by vendor and group from URL state', () => {
    const result = filterAndSortModels(
      [
        model('vendor-a-default', 'text', undefined, {
          vendor_name: 'Vendor A',
          enable_groups: ['default'],
        }),
        model('vendor-a-vip', 'text', undefined, {
          vendor_name: 'Vendor A',
          enable_groups: ['vip'],
        }),
        model('vendor-b-vip', 'text', undefined, {
          vendor_name: 'Vendor B',
          enable_groups: ['vip'],
        }),
      ],
      {
        ...baseFilters,
        vendor: 'Vendor A',
        group: 'vip',
      }
    )

    assert.deepEqual(
      result.map((item) => item.model_name),
      ['vendor-a-vip']
    )
  })
})
