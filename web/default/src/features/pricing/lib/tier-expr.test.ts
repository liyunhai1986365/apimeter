import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  parseTiersFromExpr,
  splitBillingExprAndRequestRules,
} from './billing-expr'
import {
  buildDynamicTierPriceDisplayRows,
  getDynamicPriceEntries,
  getDynamicPriceUnitLabelKey,
  splitDynamicPriceEntriesForDisplay,
} from './dynamic-price'
import { evalExprLocally } from './tier-expr'

const emptyExtras = {
  cacheReadTokens: 0,
  cacheCreateTokens: 0,
  cacheCreate1hTokens: 0,
  imageTokens: 0,
  imageOutputTokens: 0,
  audioInputTokens: 0,
  audioOutputTokens: 0,
}

describe('evalExprLocally', () => {
  test('supports request helper functions used by server expressions', () => {
    const result = evalExprLocally(
      'param("resolution") == "1080p" ? tier("request", c * 4) : tier("base", c * 2)',
      0,
      100,
      emptyExtras
    )

    assert.equal(result.error, null)
    assert.equal(result.cost, 200)
    assert.equal(result.matchedTier, 'base')
  })

  test('supports expr nil literal in request-aware expressions', () => {
    const result = evalExprLocally(
      `param("resolution") == "1080p"
        ? (param("content.#(type==\\"video_url\\").video_url.url") != nil
            ? tier("1080p_video_input", c * 4.3055555556)
            : tier("1080p_no_video_input", c * 7.0833333333))
        : (param("content.#(type==\\"video_url\\").video_url.url") != nil
            ? tier("480_720p_video_input", c * 3.8888888889)
            : tier("480_720p_no_video_input", c * 6.3888888889))`,
      0,
      100,
      emptyExtras
    )

    assert.equal(result.error, null)
    assert.ok(Math.abs(result.cost - 638.88888889) < 0.00000001)
    assert.equal(result.matchedTier, '480_720p_no_video_input')
  })
})

describe('parseTiersFromExpr', () => {
  test('extracts per-second duration prices from request-aware tier expressions', () => {
    const { billingExpr } =
      splitBillingExprAndRequestRules(`param("parameters.resolution") == "1080P"
      ? tier("1080p_per_second", param("parameters.duration") * 1 * 1000000)
      : tier("720p_per_second", param("parameters.duration") * 0.5 * 1000000)`)

    const tiers = parseTiersFromExpr(billingExpr)

    assert.equal(tiers.length, 2)
    assert.equal(tiers[0].label, '1080p_per_second')
    assert.equal(tiers[0].perSecondPrice, 1)
    assert.equal(tiers[1].label, '720p_per_second')
    assert.equal(tiers[1].perSecondPrice, 0.5)
  })

  test('exposes per-second prices to dynamic pricing entries', () => {
    const tiers = parseTiersFromExpr(
      'tier("720p_per_second", param("parameters.duration") * 0.5 * 1000000)'
    )

    const entries = getDynamicPriceEntries(tiers[0], {
      tokenUnit: 'M',
    })

    assert.equal(entries.length, 1)
    assert.equal(entries[0].field, 'perSecondPrice')
    assert.equal(entries[0].shortLabel, 'Per second')
    assert.equal(entries[0].formatted, '$0.5')
  })

  test('uses request as the unit label for per-request dynamic prices', () => {
    const tiers = parseTiersFromExpr('tier("1k", 70000)')
    const entries = getDynamicPriceEntries(tiers[0], {
      tokenUnit: 'M',
    })

    assert.equal(entries.length, 1)
    assert.equal(entries[0].field, 'perRequestPrice')
    assert.equal(entries[0].shortLabel, '')
    assert.equal(getDynamicPriceUnitLabelKey(entries[0], '1M'), 'request')
  })

  test('exposes constant request prices from param-based image tiers', () => {
    const tiers = parseTiersFromExpr(
      'param("resolution") == "0.5k" ? tier("0.5k", 0.045 * 1000000) : param("resolution") == "2k" ? tier("2k", 0.105 * 1000000) : tier("1k", 0.07 * 1000000)'
    )

    const rows = buildDynamicTierPriceDisplayRows(tiers, {
      tokenUnit: 'M',
    })

    assert.deepEqual(
      rows.map((row) => ({
        label: row.label,
        entries: row.regularEntries.map((entry) => ({
          field: entry.field,
          shortLabel: entry.shortLabel,
          formatted: entry.formatted,
        })),
      })),
      [
        {
          label: '0.5k',
          entries: [
            {
              field: 'perRequestPrice',
              shortLabel: '',
              formatted: '$0.045',
            },
          ],
        },
        {
          label: '2k',
          entries: [
            {
              field: 'perRequestPrice',
              shortLabel: '',
              formatted: '$0.105',
            },
          ],
        },
        {
          label: '1k',
          entries: [
            {
              field: 'perRequestPrice',
              shortLabel: '',
              formatted: '$0.07',
            },
          ],
        },
      ]
    )
  })

  test('exposes nano-banana resolution tier request prices', () => {
    const tiers = parseTiersFromExpr(
      'param("resolution") == "0.5k" ? tier("0.5k", 0.045 * 1000000) : param("resolution") == "2k" ? tier("2k", 0.105 * 1000000) : param("resolution") == "4k" ? tier("4k", 0.14 * 1000000) : tier("1k", 0.07 * 1000000)'
    )

    const rows = buildDynamicTierPriceDisplayRows(tiers, {
      tokenUnit: 'M',
    })

    assert.deepEqual(
      rows.map((row) => ({
        label: row.label,
        prices: row.regularEntries.map((entry) => entry.formatted),
      })),
      [
        { label: '0.5k', prices: ['$0.045'] },
        { label: '2k', prices: ['$0.105'] },
        { label: '4k', prices: ['$0.14'] },
        { label: '1k', prices: ['$0.07'] },
      ]
    )
  })

  test('exposes nano-banana request prices from quota-scaled constants with optional add-ons', () => {
    const tiers = parseTiersFromExpr(`param("resolution") == "0.5k"
      ? tier("0.5k", 45000 + (param("enable_web_search") == true ? 14000 : 0) + (param("enable_image_search") == true ? 14000 : 0))
      : param("resolution") == "2k"
        ? tier("2k", 105000 + (param("enable_web_search") == true ? 14000 : 0) + (param("enable_image_search") == true ? 14000 : 0))
        : param("resolution") == "4k"
          ? tier("4k", 140000 + (param("enable_web_search") == true ? 14000 : 0) + (param("enable_image_search") == true ? 14000 : 0))
          : tier("1k", 70000 + (param("enable_web_search") == true ? 14000 : 0) + (param("enable_image_search") == true ? 14000 : 0))`)

    const rows = buildDynamicTierPriceDisplayRows(tiers, {
      tokenUnit: 'M',
    })

    assert.deepEqual(
      rows.map((row) => ({
        label: row.label,
        prices: row.regularEntries.map((entry) => entry.formatted),
      })),
      [
        { label: '0.5k', prices: ['$0.045'] },
        { label: '2k', prices: ['$0.105'] },
        { label: '4k', prices: ['$0.14'] },
        { label: '1k', prices: ['$0.07'] },
      ]
    )
  })

  test('labels split cache write dynamic pricing entries by TTL', () => {
    const tiers = parseTiersFromExpr(
      'tier("base", p * 3 + c * 15 + cc * 3.75 + cc1h * 6)'
    )

    const entries = getDynamicPriceEntries(tiers[0], {
      tokenUnit: 'M',
    })

    assert.deepEqual(
      entries.map((entry) => ({
        field: entry.field,
        shortLabel: entry.shortLabel,
        formatted: entry.formatted,
      })),
      [
        {
          field: 'inputPrice',
          shortLabel: 'Input',
          formatted: '$3',
        },
        {
          field: 'outputPrice',
          shortLabel: 'Output',
          formatted: '$15',
        },
        {
          field: 'cacheCreatePrice',
          shortLabel: 'Cache Write (5m)',
          formatted: '$3.75',
        },
        {
          field: 'cacheCreate1hPrice',
          shortLabel: 'Cache Write (1h)',
          formatted: '$6',
        },
      ]
    )
  })

  test('splits cache write dynamic pricing entries for vertical display', () => {
    const tiers = parseTiersFromExpr(
      'tier("base", p * 3 + c * 15 + cr * 0.3 + cc * 3.75 + cc1h * 6)'
    )
    const entries = getDynamicPriceEntries(tiers[0], {
      tokenUnit: 'M',
    })

    const grouped = splitDynamicPriceEntriesForDisplay(entries)

    assert.deepEqual(
      grouped.regularEntries.map((entry) => entry.field),
      ['inputPrice', 'outputPrice', 'cacheReadPrice']
    )
    assert.deepEqual(
      grouped.cacheWriteEntries.map((entry) => entry.field),
      ['cacheCreatePrice', 'cacheCreate1hPrice']
    )
  })

  test('builds dynamic tier display rows with discounted values and originals', () => {
    const tiers = parseTiersFromExpr(
      'len <= 200000 ? tier("standard", p * 3 + c * 15 + cc * 3.75 + cc1h * 6) : tier("long_context", p * 6 + c * 22.5 + cc * 7.5 + cc1h * 12)'
    )

    const rows = buildDynamicTierPriceDisplayRows(
      tiers,
      {
        tokenUnit: 'M',
        groupRatioMultiplier: 0.5,
      },
      {
        tokenUnit: 'M',
        groupRatioMultiplier: 1,
      }
    )

    assert.equal(rows.length, 2)
    assert.equal(rows[0].label, 'standard')
    assert.deepEqual(
      rows[0].regularEntries.map((entry) => ({
        field: entry.field,
        formatted: entry.formatted,
      })),
      [
        { field: 'inputPrice', formatted: '$1.5' },
        { field: 'outputPrice', formatted: '$7.5' },
      ]
    )
    assert.deepEqual(
      rows[0].originalRegularEntries.map((entry) => entry.formatted),
      ['$3', '$15']
    )
    assert.deepEqual(
      rows[0].cacheWriteEntries.map((entry) => entry.formatted),
      ['$1.875', '$3']
    )
    assert.deepEqual(
      rows[0].originalCacheWriteEntries.map((entry) => entry.formatted),
      ['$3.75', '$6']
    )
  })
})
