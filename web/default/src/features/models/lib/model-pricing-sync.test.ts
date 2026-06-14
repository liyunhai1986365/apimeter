import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  convertRatioValuesToPriceValues,
  normalizeModelPricingValuesForInputMode,
  syncModelPricingMaps,
  type ModelPricingMaps,
} from './model-pricing-sync'

function createMaps(
  overrides: Partial<ModelPricingMaps> = {}
): ModelPricingMaps {
  return {
    priceMap: {},
    ratioMap: {},
    cacheMap: {},
    createCacheMap: {},
    completionMap: {},
    imageMap: {},
    audioMap: {},
    audioCompletionMap: {},
    billingModeMap: {},
    billingExprMap: {},
    ...overrides,
  }
}

describe('syncModelPricingMaps', () => {
  test('converts metadata price-mode lane prices to stored ratios', () => {
    const result = syncModelPricingMaps({
      maps: createMaps(),
      values: {
        ratio: '1.5',
        cacheRatio: '0.1',
        createCacheRatio: '3.75',
        imageRatio: '6',
        audioRatio: '9',
        audioCompletionRatio: '18',
      },
      pricingMode: 'per-token',
      pricingInputMode: 'price',
      finalModelName: 'gpt-test',
      isEditing: true,
    })

    assert.equal(result.ratioMap['gpt-test'], 1.5)
    assert.ok(Math.abs(result.cacheMap['gpt-test'] - 0.1 / 3) < 1e-12)
    assert.equal(result.createCacheMap['gpt-test'], 1.25)
    assert.equal(result.imageMap['gpt-test'], 2)
    assert.equal(result.audioMap['gpt-test'], 3)
    assert.equal(result.audioCompletionMap['gpt-test'], 2)
  })

  test('converts stored ratios back to metadata price-mode display values', () => {
    const result = convertRatioValuesToPriceValues({
      ratio: '1.5',
      cacheRatio: '0.03333333333333333',
      createCacheRatio: '1.25',
      imageRatio: '2',
      audioRatio: '3',
      audioCompletionRatio: '2',
    })

    assert.equal(result.cacheRatio, '0.1')
    assert.equal(result.createCacheRatio, '3.75')
    assert.equal(result.imageRatio, '6')
    assert.equal(result.audioRatio, '9')
    assert.equal(result.audioCompletionRatio, '18')
  })

  test('normalizes price-mode values without changing completion ratio', () => {
    const result = normalizeModelPricingValuesForInputMode(
      {
        ratio: '1.5',
        completionRatio: '2',
        cacheRatio: '0.1',
      },
      'price'
    )

    assert.equal(result.completionRatio, '2')
    assert.ok(Math.abs(Number(result.cacheRatio) - 0.1 / 3) < 1e-12)
  })

  test('keeps existing pricing when submitted pricing fields are empty', () => {
    const result = syncModelPricingMaps({
      maps: createMaps({
        ratioMap: { 'gpt-test': 1.25 },
        completionMap: { 'gpt-test': 2 },
      }),
      values: {
        ratio: '',
        completionRatio: '',
        price: '',
      },
      pricingMode: 'per-token',
      finalModelName: 'gpt-test',
      oldModelName: 'gpt-test',
      isEditing: true,
    })

    assert.equal(result.ratioMap['gpt-test'], 1.25)
    assert.equal(result.completionMap['gpt-test'], 2)
  })

  test('updates explicit per-token pricing and clears fixed per-request pricing', () => {
    const result = syncModelPricingMaps({
      maps: createMaps({
        priceMap: { 'gpt-test': 0.03 },
        ratioMap: { 'gpt-test': 1 },
        createCacheMap: { 'gpt-test': 1.2 },
        completionMap: { 'gpt-test': 2 },
      }),
      values: {
        ratio: '1.5',
        createCacheRatio: '0.25',
        completionRatio: '3',
      },
      pricingMode: 'per-token',
      finalModelName: 'gpt-test',
      oldModelName: 'gpt-test',
      isEditing: true,
    })

    assert.equal(result.priceMap['gpt-test'], undefined)
    assert.equal(result.ratioMap['gpt-test'], 1.5)
    assert.equal(result.createCacheMap['gpt-test'], 0.25)
    assert.equal(result.completionMap['gpt-test'], 3)
  })

  test('renames existing pricing when model name changes without new pricing input', () => {
    const result = syncModelPricingMaps({
      maps: createMaps({
        priceMap: { 'old-model': 0.05 },
        createCacheMap: { 'old-model': 1.8 },
        imageMap: { 'old-model': 4 },
        billingModeMap: { 'old-model': 'tiered_expr' },
        billingExprMap: { 'old-model': 'tier("base", p * 0.1)' },
      }),
      values: {
        price: '',
        ratio: '',
      },
      pricingMode: 'per-request',
      finalModelName: 'new-model',
      oldModelName: 'old-model',
      isEditing: true,
    })

    assert.equal(result.priceMap['old-model'], undefined)
    assert.equal(result.createCacheMap['old-model'], undefined)
    assert.equal(result.imageMap['old-model'], undefined)
    assert.equal(result.billingModeMap['old-model'], undefined)
    assert.equal(result.billingExprMap['old-model'], undefined)
    assert.equal(result.priceMap['new-model'], 0.05)
    assert.equal(result.createCacheMap['new-model'], 1.8)
    assert.equal(result.imageMap['new-model'], 4)
    assert.equal(result.billingModeMap['new-model'], 'tiered_expr')
    assert.equal(result.billingExprMap['new-model'], 'tier("base", p * 0.1)')
  })

  test('stores expression billing fields with fallback pricing', () => {
    const result = syncModelPricingMaps({
      maps: createMaps({
        priceMap: { 'gpt-test': 0.03 },
      }),
      values: {
        price: '',
        ratio: '2',
        cacheRatio: '0.2',
        createCacheRatio: '0.6',
        completionRatio: '3',
        billingExpr: 'tier("base", p * 0.1)',
      },
      pricingMode: 'tiered_expr',
      finalModelName: 'gpt-test',
      oldModelName: 'gpt-test',
      isEditing: true,
    })

    assert.equal(result.billingModeMap['gpt-test'], 'tiered_expr')
    assert.equal(result.billingExprMap['gpt-test'], 'tier("base", p * 0.1)')
    assert.equal(result.priceMap['gpt-test'], undefined)
    assert.equal(result.ratioMap['gpt-test'], 2)
    assert.equal(result.cacheMap['gpt-test'], 0.2)
    assert.equal(result.createCacheMap['gpt-test'], 0.6)
    assert.equal(result.completionMap['gpt-test'], 3)
  })
})
