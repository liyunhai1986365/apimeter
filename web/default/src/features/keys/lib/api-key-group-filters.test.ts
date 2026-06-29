import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { PerfGroupSummary } from '@/features/performance-metrics/types'
import type {
  PricingGroupDisplayConfig,
  PricingModel,
  PricingVendor,
} from '@/features/pricing/types'
import type { ApiKeyGroupOption } from '../components/api-key-group-combobox'
import {
  ALL_CATEGORY_VALUE,
  ALL_VENDOR_VALUE,
  buildPricingGroupUrl,
  buildApiKeyGroupFilterMetadata,
  filterApiKeyGroupOptions,
  sortApiKeyGroupOptions,
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

const sortableOptions: ApiKeyGroupOption[] = [
  { value: 'standard', label: 'Standard', desc: 'Standard', ratio: 1 },
  { value: 'economy', label: 'Economy', desc: 'Economy', ratio: 0.6 },
  { value: 'premium', label: 'Premium', desc: 'Premium', ratio: 1.4 },
  { value: 'unknown', label: 'Unknown', desc: 'Unknown' },
]

const groupPerformance: Record<string, PerfGroupSummary> = {
  standard: {
    group: 'standard',
    avg_ttft_ms: 80,
    avg_latency_ms: 500,
    success_rate: 99,
    avg_tps: 20,
    request_count: 10,
  },
  economy: {
    group: 'economy',
    avg_ttft_ms: 100,
    avg_latency_ms: 700,
    success_rate: 98,
    avg_tps: 18,
    request_count: 10,
  },
  premium: {
    group: 'premium',
    avg_ttft_ms: 60,
    avg_latency_ms: 300,
    success_rate: 99,
    avg_tps: 22,
    request_count: 10,
  },
}

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

  test('sorts supplier options by discount, latency, and name', () => {
    assert.deepEqual(
      sortApiKeyGroupOptions(sortableOptions, {
        sort: 'discount_desc',
        groupPerformance,
      }).map((option) => option.value),
      ['economy', 'standard', 'premium', 'unknown']
    )

    assert.deepEqual(
      sortApiKeyGroupOptions(sortableOptions, {
        sort: 'discount_asc',
        groupPerformance,
      }).map((option) => option.value),
      ['premium', 'standard', 'economy', 'unknown']
    )

    assert.deepEqual(
      sortApiKeyGroupOptions(sortableOptions, {
        sort: 'latency_asc',
        groupPerformance,
      }).map((option) => option.value),
      ['premium', 'standard', 'economy', 'unknown']
    )

    assert.deepEqual(
      sortApiKeyGroupOptions(sortableOptions, {
        sort: 'latency_desc',
        groupPerformance,
      }).map((option) => option.value),
      ['economy', 'standard', 'premium', 'unknown']
    )

    assert.deepEqual(
      sortApiKeyGroupOptions(
        [sortableOptions[1], sortableOptions[2], sortableOptions[0]],
        {
          sort: 'name_asc',
          groupPerformance,
        }
      ).map((option) => option.value),
      ['economy', 'premium', 'standard']
    )

    assert.deepEqual(
      sortApiKeyGroupOptions(
        [sortableOptions[1], sortableOptions[2], sortableOptions[0]],
        {
          sort: 'name_desc',
          groupPerformance,
        }
      ).map((option) => option.value),
      ['standard', 'premium', 'economy']
    )
  })

  test('builds model square urls for supplier groups', () => {
    assert.equal(buildPricingGroupUrl('default'), '/pricing?group=default')
    assert.equal(
      buildPricingGroupUrl('provider group'),
      '/pricing?group=provider+group'
    )
    assert.equal(
      buildPricingGroupUrl('user_owned:1:7'),
      '/pricing?group=user_owned%3A1%3A7'
    )
  })
})
