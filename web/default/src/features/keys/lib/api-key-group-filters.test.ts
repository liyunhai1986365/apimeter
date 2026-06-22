import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type {
  PricingGroupDisplayConfig,
  PricingModel,
  PricingVendor,
} from '@/features/pricing/types'
import type { ApiKeyGroupOption } from '../components/api-key-group-combobox'
import {
  ALL_CATEGORY_VALUE,
  ALL_VENDOR_VALUE,
  buildApiKeyGroupFilterMetadata,
  filterApiKeyGroupOptions,
} from './api-key-group-filters'

const options: ApiKeyGroupOption[] = [
  { value: 'default', label: 'default', desc: 'Default' },
  { value: 'fast', label: 'fast', desc: 'Fast' },
  { value: 'backup', label: 'backup', desc: 'Backup' },
  {
    value: 'user_owned:1:7',
    label: 'My OpenAI',
    desc: 'Use your own upstream keys',
  },
]

const groupDisplay: PricingGroupDisplayConfig = {
  categories: [
    { id: 'official', name: 'Official', order: 10 },
    { id: 'partner', name: 'Partner', order: 20 },
  ],
  groups: [
    { group: 'default', category_id: 'official', order: 10 },
    { group: 'fast', category_id: 'partner', order: 10 },
  ],
}

const vendors: PricingVendor[] = [
  { id: 1, name: 'OpenAI', sort_order: 20 },
  { id: 2, name: 'Anthropic', sort_order: 10 },
]

const models: PricingModel[] = [
  {
    id: 1,
    model_name: 'gpt-model',
    vendor_id: 1,
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: ['default'],
  },
  {
    id: 2,
    model_name: 'claude-model',
    vendor_id: 2,
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: ['all'],
  },
]

describe('api key group filters', () => {
  test('builds category tabs and vendor filters from pricing metadata', () => {
    const metadata = buildApiKeyGroupFilterMetadata({
      options,
      groupDisplay,
      vendors,
      models,
    })

    assert.deepEqual(
      metadata.categories.map((category) => category.value),
      [
        ALL_CATEGORY_VALUE,
        'user_owned',
        'official',
        'partner',
        '__uncategorized__',
      ]
    )
    assert.deepEqual(
      metadata.vendors.map((vendor) => vendor.value),
      [ALL_VENDOR_VALUE, '2', '1']
    )
    assert.deepEqual(
      metadata.vendors.map((vendor) => vendor.label),
      ['All', 'Anthropic', 'OpenAI']
    )
    assert.deepEqual(
      options.map((option) => metadata.groupCategoryLabels.get(option.value)),
      ['Official', 'Partner', 'Uncategorized', 'User-owned suppliers']
    )

    assert.deepEqual(
      filterApiKeyGroupOptions(options, metadata, {
        category: 'official',
        vendor: '1',
        search: '',
      }).map((option) => option.value),
      ['default']
    )
    assert.deepEqual(
      filterApiKeyGroupOptions(options, metadata, {
        category: 'official',
        vendor: '2',
        search: '',
      }).map((option) => option.value),
      ['default']
    )
    assert.deepEqual(
      filterApiKeyGroupOptions(options, metadata, {
        category: 'partner',
        vendor: '1',
        search: '',
      }).map((option) => option.value),
      []
    )
    assert.deepEqual(
      filterApiKeyGroupOptions(options, metadata, {
        category: 'user_owned',
        vendor: ALL_VENDOR_VALUE,
        search: '',
      }).map((option) => option.value),
      ['user_owned:1:7']
    )
  })
})
