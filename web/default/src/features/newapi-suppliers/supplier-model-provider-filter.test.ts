import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  buildSupplierModelProviderFilters,
  selectSupplierProviderModels,
} from './supplier-model-provider-filter'

describe('supplier model provider filters', () => {
  test('builds provider filters from model plaza provider metadata', () => {
    const filters = buildSupplierModelProviderFilters(
      ['gpt-4o', 'claude-sonnet', 'gemini-pro'],
      {
        'gpt-4o': ['OpenAI'],
        'claude-sonnet': ['Anthropic'],
        'gemini-pro': ['Google', 'OpenAI'],
      }
    )

    assert.deepEqual(filters, [
      { provider: 'Anthropic', models: ['claude-sonnet'] },
      { provider: 'Google', models: ['gemini-pro'] },
      { provider: 'OpenAI', models: ['gemini-pro', 'gpt-4o'] },
    ])
  })

  test('selects only models belonging to the clicked provider', () => {
    const selected = selectSupplierProviderModels(
      ['gpt-4o', 'claude-sonnet', 'gemini-pro'],
      ['gemini-pro', 'gpt-4o']
    )

    assert.deepEqual(selected, {
      'gpt-4o': true,
      'claude-sonnet': false,
      'gemini-pro': true,
    })
  })
})
