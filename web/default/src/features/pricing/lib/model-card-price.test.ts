import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { PricingModel } from '../types'
import { buildModelCardPriceDisplay } from './model-card-price'

const zhDiscountLabels = {
  originalPrice: '原价',
  fold: '{{value}}折',
  percentDiscount: '{{value}}%折扣',
  percentPrice: '{{value}}%价格',
  startingFrom: '{{value}}起',
}

function tokenModel(overrides: Partial<PricingModel> = {}): PricingModel {
  return {
    id: 1,
    model_name: 'chat-model',
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 3,
    enable_groups: ['default', 'vip'],
    group_ratio: {
      default: 1,
      vip: 0.5,
    },
    ...overrides,
  }
}

describe('buildModelCardPriceDisplay', () => {
  test('builds original and lowest prices for token-based model cards', () => {
    const display = buildModelCardPriceDisplay(tokenModel(), {
      tokenUnit: 'M',
      discountLabels: zhDiscountLabels,
    })

    assert.equal(display.kind, 'token')
    assert.equal(display.billingLabelKey, 'Token-based')
    assert.equal(display.unitLabel, '1M')
    assert.equal(display.discountLabel, '5折起')
    assert.deepEqual(
      display.entries.map((entry) => ({
        label: entry.labelKey,
        original: entry.original,
        current: entry.current,
        unit: entry.unitLabel,
      })),
      [
        {
          label: 'Input',
          original: '$2',
          current: '$1',
          unit: '1M',
        },
        {
          label: 'Output',
          original: '$6',
          current: '$3',
          unit: '1M',
        },
      ]
    )
  })

  test('includes cache write price for token-based models with create cache ratio', () => {
    const display = buildModelCardPriceDisplay(
      tokenModel({
        create_cache_ratio: 1.25,
      }),
      {
        tokenUnit: 'M',
        discountLabels: zhDiscountLabels,
      }
    )

    assert.deepEqual(
      display.entries.map((entry) => ({
        key: entry.key,
        label: entry.labelKey,
        original: entry.original,
        current: entry.current,
        unit: entry.unitLabel,
      })),
      [
        {
          key: 'input',
          label: 'Input',
          original: '$2',
          current: '$1',
          unit: '1M',
        },
        {
          key: 'output',
          label: 'Output',
          original: '$6',
          current: '$3',
          unit: '1M',
        },
        {
          key: 'create_cache',
          label: 'Cache Write',
          original: '$2.5',
          current: '$1.25',
          unit: '1M',
        },
      ]
    )
  })

  test('builds original and lowest prices for per-request model cards', () => {
    const display = buildModelCardPriceDisplay(
      tokenModel({
        quota_type: 1,
        model_price: 0.02,
      }),
      {
        tokenUnit: 'M',
        discountLabels: zhDiscountLabels,
      }
    )

    assert.equal(display.kind, 'request')
    assert.equal(display.billingLabelKey, 'Per Request')
    assert.equal(display.discountLabel, '5折起')
    assert.deepEqual(display.entries, [
      {
        key: 'request',
        labelKey: 'Request',
        original: '$0.02',
        current: '$0.01',
        unitLabel: 'request',
        featured: true,
      },
    ])
  })

  test('keeps dynamic pricing entries usage-based and group-discount aware', () => {
    const display = buildModelCardPriceDisplay(
      tokenModel({
        billing_mode: 'tiered_expr',
        billing_expr: 'tier("base", p * 1 + c * 2)',
      }),
      {
        tokenUnit: 'M',
        discountLabels: zhDiscountLabels,
      }
    )

    assert.equal(display.kind, 'dynamic')
    assert.equal(display.billingLabelKey, 'Dynamic Pricing')
    assert.equal(display.discountLabel, '5折起')
    assert.deepEqual(
      display.entries.map((entry) => ({
        label: entry.labelKey,
        original: entry.original,
        current: entry.current,
      })),
      [
        {
          label: 'Input',
          original: '$1',
          current: '$0.5',
        },
        {
          label: 'Output',
          original: '$2',
          current: '$1',
        },
      ]
    )
  })

  test('can include every dynamic pricing tier as price specs', () => {
    const display = buildModelCardPriceDisplay(
      tokenModel({
        billing_mode: 'tiered_expr',
        billing_expr:
          'len <= 200000 ? tier("standard", p * 3 + c * 15) : tier("long_context", p * 6 + c * 22.5)',
      }),
      {
        tokenUnit: 'M',
        discountLabels: zhDiscountLabels,
        includeAllDynamicTiers: true,
      }
    )

    assert.equal(display.kind, 'dynamic')
    assert.deepEqual(
      display.entries.map((entry) => ({
        key: entry.key,
        spec: entry.specLabel,
        original: entry.original,
        current: entry.current,
      })),
      [
        {
          key: 'p',
          spec: 'standard',
          original: '$3',
          current: '$1.5',
        },
        {
          key: 'c',
          spec: 'standard',
          original: '$15',
          current: '$7.5',
        },
        {
          key: 'p',
          spec: 'long_context',
          original: '$6',
          current: '$3',
        },
        {
          key: 'c',
          spec: 'long_context',
          original: '$22.5',
          current: '$11.25',
        },
      ]
    )
  })

  test('omits discount badges and original strikethrough when no group discount exists', () => {
    const display = buildModelCardPriceDisplay(
      tokenModel({
        enable_groups: ['default'],
        group_ratio: {
          default: 1,
        },
      }),
      {
        tokenUnit: 'M',
        discountLabels: zhDiscountLabels,
      }
    )

    assert.equal(display.discountLabel, undefined)
    assert.equal(display.hasDiscount, false)
    assert.equal(display.entries[0].original, '$2')
    assert.equal(display.entries[0].current, '$2')
  })
})
