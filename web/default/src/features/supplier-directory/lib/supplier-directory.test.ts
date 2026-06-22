import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { PricingData, PricingModel } from '@/features/pricing/types'
import {
  ALL_SUPPLIER_CATEGORY_VALUE,
  ALL_SUPPLIER_VENDOR_VALUE,
  buildSupplierDirectoryData,
  filterSupplierProviders,
  filterSupplierDirectoryItems,
} from './supplier-directory'

function model(
  name: string,
  vendorId: number,
  enableGroups: string[]
): PricingModel {
  return {
    id: vendorId * 100,
    model_name: name,
    vendor_id: vendorId,
    vendor_name: vendorId === 1 ? 'OpenAI' : 'Anthropic',
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: enableGroups,
  }
}

const pricing: PricingData = {
  success: true,
  data: [
    model('gpt-5', 1, ['official']),
    model('claude-sonnet', 2, ['premium']),
    model('shared-model', 1, ['all']),
  ],
  vendors: [
    { id: 2, name: 'Anthropic', sort_order: 20 },
    { id: 1, name: 'OpenAI', sort_order: 10 },
  ],
  group_ratio: {
    official: 0.8,
    premium: 1,
  },
  group_perf: {
    official: {
      group: 'official',
      avg_ttft_ms: 120,
      avg_latency_ms: 450,
      success_rate: 99.5,
      avg_tps: 23.4,
      request_count: 42,
    },
  },
  usable_group: {
    official: { desc: 'Official stable route', ratio: 0.75 },
    premium: 'Premium route',
    auto: 'Automatic route',
  },
  group_display: {
    categories: [
      { id: 'stable', name: 'Stable', order: 1 },
      { id: 'premium-cat', name: 'Premium', order: 2 },
    ],
    groups: [
      { group: 'premium', category_id: 'premium-cat', order: 2 },
      { group: 'official', category_id: 'stable', order: 1 },
    ],
  },
  supported_endpoint: {},
  auto_groups: [],
}

describe('supplier directory', () => {
  test('builds user-facing supplier entries from pricing groups', () => {
    const directory = buildSupplierDirectoryData(pricing)

    assert.deepEqual(
      directory.items.map((item) => item.group),
      ['official', 'premium']
    )
    assert.equal(directory.items[0].description, 'Official stable route')
    assert.equal(directory.items[0].ratio, 0.75)
    assert.equal(directory.items[0].performance?.avg_latency_ms, 450)
    assert.deepEqual(
      directory.items[0].models.map((item) => item.model_name),
      ['gpt-5', 'shared-model']
    )
    assert.deepEqual(
      directory.items[0].vendors.map((item) => item.name),
      ['OpenAI']
    )
  })

  test('builds category tabs and vendor filters with counts', () => {
    const directory = buildSupplierDirectoryData(pricing)

    assert.deepEqual(directory.categories, [
      { value: ALL_SUPPLIER_CATEGORY_VALUE, label: 'All categories', count: 2 },
      { value: 'stable', label: 'Stable', count: 1 },
      { value: 'premium-cat', label: 'Premium', count: 1 },
    ])
    assert.deepEqual(directory.vendors, [
      { value: ALL_SUPPLIER_VENDOR_VALUE, label: 'All vendors', count: 2 },
      { value: '1', label: 'OpenAI', count: 2 },
      { value: '2', label: 'Anthropic', count: 1 },
    ])
  })

  test('groups supplier information by vendor for provider list view', () => {
    const directory = buildSupplierDirectoryData(pricing)

    assert.deepEqual(
      directory.providers.map((provider) => provider.vendor.name),
      ['OpenAI', 'Anthropic']
    )
    assert.deepEqual(
      directory.providers[0].groups.map((group) => group.group),
      ['official', 'premium']
    )
    assert.deepEqual(
      directory.providers[0].models.map((model) => model.model_name),
      ['gpt-5', 'shared-model']
    )
    assert.deepEqual(
      directory.providers[0].discountedGroups.map((group) => group.group),
      ['official']
    )
  })

  test('filters provider rows by category and search', () => {
    const directory = buildSupplierDirectoryData(pricing)

    assert.deepEqual(
      filterSupplierProviders(directory.providers, {
        category: 'premium-cat',
        search: '',
      }).map((provider) => ({
        vendor: provider.vendor.name,
        groups: provider.groups.map((group) => group.group),
      })),
      [
        { vendor: 'OpenAI', groups: ['premium'] },
        { vendor: 'Anthropic', groups: ['premium'] },
      ]
    )

    assert.deepEqual(
      filterSupplierProviders(directory.providers, {
        category: ALL_SUPPLIER_CATEGORY_VALUE,
        search: 'claude',
      }).map((provider) => provider.vendor.name),
      ['Anthropic']
    )
  })

  test('filters suppliers by category, vendor, and search', () => {
    const directory = buildSupplierDirectoryData(pricing)

    assert.deepEqual(
      filterSupplierDirectoryItems(directory.items, {
        category: 'stable',
        vendor: ALL_SUPPLIER_VENDOR_VALUE,
        search: '',
      }).map((item) => item.group),
      ['official']
    )
    assert.deepEqual(
      filterSupplierDirectoryItems(directory.items, {
        category: ALL_SUPPLIER_CATEGORY_VALUE,
        vendor: '2',
        search: '',
      }).map((item) => item.group),
      ['premium']
    )
    assert.deepEqual(
      filterSupplierDirectoryItems(directory.items, {
        category: ALL_SUPPLIER_CATEGORY_VALUE,
        vendor: ALL_SUPPLIER_VENDOR_VALUE,
        search: 'claude',
      }).map((item) => item.group),
      ['premium']
    )
  })
})
