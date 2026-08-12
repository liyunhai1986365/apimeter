import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { PricingModel } from '../types'
import {
  buildQuotationFilename,
  buildQuotationRows,
  normalizeQuotationLocale,
} from './quotation-pdf'

function model(overrides: Partial<PricingModel> = {}): PricingModel {
  return {
    id: 1,
    model_name: 'gpt-test',
    vendor_name: 'OpenAI',
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 2,
    enable_groups: ['default'],
    group_ratio: { default: 0.5 },
    ...overrides,
  }
}

describe('quotation PDF data', () => {
  test('uses the shared pricing display for token-based quote rows', () => {
    const [row] = buildQuotationRows([model()], {
      tokenUnit: 'M',
      usableGroup: { default: 'Default supplier' },
    })

    assert.equal(row.modelName, 'gpt-test')
    assert.equal(row.vendorName, 'OpenAI')
    assert.equal(row.billingLabelKey, 'Token-based')
    assert.notEqual(row.primaryPrice, '-')
    assert.notEqual(row.outputPrice, '-')
    assert.equal(row.primaryUnitLabel, '1M')
    assert.equal(row.outputUnitLabel, '1M')
    assert.equal(row.cacheUnitLabel, '1M')
    assert.deepEqual(row.inputModalities, ['text'])
    assert.deepEqual(row.outputModalities, ['text'])
    assert.deepEqual(row.supplierDiscounts, [
      {
        group: 'default',
        description: 'Default supplier',
        ratio: 0.5,
        label: '50% off',
      },
    ])
  })

  test('includes the current input-to-output modality flow', () => {
    const [row] = buildQuotationRows([
      model({
        input_modalities: ['text', 'image', 'file'],
        output_modalities: ['text', 'image'],
      }),
    ])

    assert.deepEqual(row.inputModalities, ['text', 'image', 'file'])
    assert.deepEqual(row.outputModalities, ['text', 'image'])
  })

  test('keeps official standard prices independent from supplier discounts', () => {
    const discounted = buildQuotationRows(
      [model({ group_ratio: { default: 0.5 } })],
      { tokenUnit: 'M', usableGroup: { default: 'Discount supplier' } }
    )[0]
    const standard = buildQuotationRows(
      [model({ group_ratio: { default: 1 } })],
      { tokenUnit: 'M', usableGroup: { default: 'Official supplier' } }
    )[0]

    assert.equal(discounted.primaryPrice, standard.primaryPrice)
    assert.equal(discounted.outputPrice, standard.outputPrice)
    assert.equal(discounted.supplierDiscounts[0]?.ratio, 0.5)
  })

  test('groups by category then sorts by manufacturer and model', () => {
    const rows = buildQuotationRows([
      model({
        id: 4,
        model_name: 'z-video',
        category: 'video',
        vendor_name: 'Vendor A',
      }),
      model({
        id: 3,
        model_name: 'z-text',
        category: 'text',
        vendor_name: 'Vendor B',
      }),
      model({
        id: 2,
        model_name: 'b-text',
        category: 'text',
        vendor_name: 'Vendor A',
      }),
      model({
        id: 1,
        model_name: 'a-text',
        category: 'text',
        vendor_name: 'Vendor A',
      }),
    ])

    assert.deepEqual(
      rows.map((row) => row.modelName),
      ['a-text', 'b-text', 'z-text', 'z-video']
    )
  })

  test('expands all supplier groups and preserves configured group order', () => {
    const [row] = buildQuotationRows(
      [
        model({
          enable_groups: ['all'],
          group_ratio: { supplierA: 0.8, supplierB: 0.6 },
        }),
      ],
      {
        usableGroup: { supplierA: 'A', supplierB: 'B' },
        groupDisplay: {
          categories: [{ id: 'preferred', name: 'Preferred', order: 1 }],
          groups: [
            { group: 'supplierB', category_id: 'preferred', order: 1 },
            { group: 'supplierA', category_id: 'preferred', order: 2 },
          ],
        },
      }
    )

    assert.deepEqual(
      row.supplierDiscounts.map((item) => item.group),
      ['supplierB', 'supplierA']
    )
  })

  test('marks identical supplier prices for silent merging only within the same category and manufacturer', () => {
    const rows = buildQuotationRows(
      [
        model({ id: 1, model_name: 'gpt-a' }),
        model({ id: 2, model_name: 'gpt-b' }),
        model({
          id: 3,
          model_name: 'gpt-c',
          group_ratio: { default: 0.6 },
        }),
        model({
          id: 4,
          model_name: 'gemini-a',
          vendor_name: 'Google',
        }),
      ],
      { usableGroup: { default: 'Default supplier' } }
    )

    assert.deepEqual(
      Object.fromEntries(
        rows.map((row) => [row.modelName, row.supplierPricingMode])
      ),
      {
        'gemini-a': 'full',
        'gpt-a': 'full',
        'gpt-b': 'same-segment',
        'gpt-c': 'full',
      }
    )
  })

  test('marks unsupported expressions for online detail lookup', () => {
    const [row] = buildQuotationRows([
      model({ billing_mode: 'tiered_expr', billing_expr: 'unsupported(x)' }),
    ])

    assert.equal(row.requiresOnlineDetails, true)
    assert.equal(row.primaryPrice, '-')
  })

  test('keeps per-second prices in mixed dynamic tiers', () => {
    const rows = buildQuotationRows([
      model({
        billing_mode: 'tiered_expr',
        billing_expr:
          'param("parameters.resolution") == "1080P" ? tier("mixed", 0.1 * 1000000 + p * 2 + c * 4 + param("parameters.duration") * 0.5 * 1000000) : tier("base", p * 1 + c * 2)',
      }),
    ])

    assert.ok(rows.some((row) => row.scenario.includes('Per second')))
    assert.ok(rows.some((row) => row.primaryUnitLabel === 'second'))
  })

  test('creates stable, filesystem-safe quotation filenames', () => {
    assert.equal(
      buildQuotationFilename('Modelsell / Enterprise', new Date(2026, 7, 12)),
      'Modelsell-Enterprise-pricing-quotation-2026-08-12.pdf'
    )
  })

  test('normalizes project locale aliases for Intl APIs', () => {
    assert.equal(normalizeQuotationLocale('zhCN'), 'zh-CN')
    assert.equal(normalizeQuotationLocale('pt_BR'), 'pt-BR')
    assert.equal(normalizeQuotationLocale(''), 'en')
  })
})
