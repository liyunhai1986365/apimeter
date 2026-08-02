import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { PricingModel, PricingVendor } from '@/features/pricing/types'
import {
  buildProviderDirectory,
  filterProviderDirectory,
  getProviderSlugBase,
} from './provider-directory'

function model(
  name: string,
  vendorId: number,
  category: PricingModel['category'] = 'text'
): PricingModel {
  return {
    id: vendorId,
    model_name: name,
    vendor_id: vendorId,
    category,
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: ['default'],
    supported_endpoint_types: ['openai'],
  }
}

const vendors: PricingVendor[] = [
  { id: 2, name: 'Anthropic', sort_order: 20 },
  { id: 1, name: 'OpenAI', sort_order: 10 },
  { id: 3, name: 'Unused', sort_order: 5 },
]

describe('provider directory', () => {
  test('builds stable slugs and only includes vendors with visible models', () => {
    const providers = buildProviderDirectory(
      [model('claude-sonnet', 2), model('gpt-image-1', 1, 'image')],
      vendors
    )

    assert.deepEqual(
      providers.map((provider) => ({
        name: provider.vendor.name,
        slug: provider.slug,
        models: provider.models.map((item) => item.model_name),
      })),
      [
        { name: 'OpenAI', slug: 'openai', models: ['gpt-image-1'] },
        {
          name: 'Anthropic',
          slug: 'anthropic',
          models: ['claude-sonnet'],
        },
      ]
    )
  })

  test('falls back for non-latin names and disambiguates duplicate slugs', () => {
    assert.equal(getProviderSlugBase('通义千问', 8), 'provider-8')
    const providers = buildProviderDirectory(
      [model('a', 1), model('b', 2)],
      [
        { id: 1, name: 'Vendor AI' },
        { id: 2, name: 'Vendor.AI' },
      ]
    )

    assert.deepEqual(
      providers.map((provider) => provider.slug),
      ['vendor-ai', 'vendor-ai-2']
    )
  })

  test('filters by provider name, description, and model ID', () => {
    const providers = buildProviderDirectory(
      [model('claude-sonnet', 2), model('gpt-5', 1)],
      [
        { id: 1, name: 'OpenAI', description: 'Reasoning APIs' },
        { id: 2, name: 'Anthropic' },
      ]
    )

    assert.deepEqual(
      filterProviderDirectory(providers, 'claude').map(
        (provider) => provider.vendor.name
      ),
      ['Anthropic']
    )
    assert.deepEqual(
      filterProviderDirectory(providers, 'reasoning').map(
        (provider) => provider.vendor.name
      ),
      ['OpenAI']
    )
  })
})
