import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  buildRequestRuleExpr,
  combineBillingExpr,
  tryParseRequestRuleExpr,
  parseTiersFromExpr,
  splitBillingExprAndRequestRules,
} from './billing-expr'
import {
  buildDynamicTierPriceDisplayRows,
  getDynamicPriceEntries,
  getDynamicPriceUnitLabelKey,
  splitDynamicPriceEntriesForDisplay,
} from './dynamic-price'
import {
  createPricingEditorState,
  switchPricingEditorMode,
  editRawPricing,
  editVisualPricing,
  editPricingRules,
} from './pricing-editor-state'
import {
  CACHE_MODE_GENERIC,
  CACHE_MODE_TIMED,
  createDefaultVisualConfig,
  evalExprLocally,
  generateExprFromVisualConfig,
  getTierCacheMode,
  isVisualPriceEnabled,
  setVisualTierCacheMode,
  setVisualTierPrice,
  setVisualTierPriceEnabled,
  tryParseVisualConfig,
} from './tier-expr'

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

  test('supports pixels helper used by image-size pricing expressions', () => {
    const result = evalExprLocally(
      `pixels("2048x2048") > 2360000
        ? tier("gt_236mp", 0.60 * 1000000)
        : tier("lte_236mp", 0.30 * 1000000)`,
      0,
      0,
      emptyExtras
    )

    assert.equal(result.error, null)
    assert.equal(result.cost, 600000)
    assert.equal(result.matchedTier, 'gt_236mp')
  })

  test('supports named image resolution tiers in pixels helper', () => {
    const result = evalExprLocally(
      `pixels("2K") > 2360000
        ? tier("gt_236mp", 0.60 * 1000000)
        : tier("lte_236mp", 0.30 * 1000000)`,
      0,
      0,
      emptyExtras
    )

    assert.equal(result.error, null)
    assert.equal(result.cost, 600000)
    assert.equal(result.matchedTier, 'gt_236mp')
  })
})

describe('visual billing expression config', () => {
  const seedanceExpr =
    'param("resolution") == "1080p" ? (param("content.#(type==\\"video_url\\").video_url.url") != nil ? tier("1080p_video_input", c * 4.3055555556) : tier("1080p_no_video_input", c * 7.0833333333)) : (param("content.#(type==\\"video_url\\").video_url.url") != nil ? tier("480_720p_video_input", c * 3.8888888889) : tier("480_720p_no_video_input", c * 6.3888888889))'

  test('parses nested request-param tiers into an editable prioritized list', () => {
    const config = tryParseVisualConfig(seedanceExpr)

    assert.ok(config)
    assert.equal(config.tiers.length, 4)
    assert.deepEqual(
      config.tiers.map((tier) => ({
        label: tier.label,
        output: tier.output_unit_cost,
        conditions: tier.conditions,
      })),
      [
        {
          label: '1080p_video_input',
          output: 4.3055555556,
          conditions: [
            {
              source: 'param',
              path: 'resolution',
              mode: 'eq',
              value: '1080p',
            },
            {
              source: 'param',
              path: 'content.#(type=="video_url").video_url.url',
              mode: 'exists',
              value: '',
            },
          ],
        },
        {
          label: '1080p_no_video_input',
          output: 7.0833333333,
          conditions: [
            {
              source: 'param',
              path: 'resolution',
              mode: 'eq',
              value: '1080p',
            },
          ],
        },
        {
          label: '480_720p_video_input',
          output: 3.8888888889,
          conditions: [
            {
              source: 'param',
              path: 'content.#(type=="video_url").video_url.url',
              mode: 'exists',
              value: '',
            },
          ],
        },
        {
          label: '480_720p_no_video_input',
          output: 6.3888888889,
          conditions: [],
        },
      ]
    )

    const generated = generateExprFromVisualConfig(config)
    const reparsed = tryParseVisualConfig(generated)
    assert.ok(reparsed)
    assert.deepEqual(reparsed, config)
  })

  test('keeps existing token, cache, and media tiers visual-editable', () => {
    const expression =
      'len <= 200000 ? tier("standard", p * 3 + c * 15 + cr * 0.3 + cc * 3.75 + cc1h * 6 + img * 2 + img_o * 20 + ai * 4 + ao * 40) : tier("long", p * 6 + c * 22.5 + cr * 0.6)'

    const config = tryParseVisualConfig(expression)
    assert.ok(config)
    assert.equal(config.tiers.length, 2)
    assert.deepEqual(config.tiers[0].conditions, [
      { source: 'token', var: 'len', op: '<=', value: 200000 },
    ])
    assert.equal(config.tiers[0].cache_create_1h_unit_cost, 6)
    assert.equal(config.tiers[0].image_output_unit_cost, 20)
    assert.equal(config.tiers[0].audio_output_unit_cost, 40)
    assert.deepEqual(
      tryParseVisualConfig(generateExprFromVisualConfig(config)),
      config
    )
  })

  test('round-trips mixed token, header, and time tier conditions', () => {
    const expression =
      'len >= 1000 && header("X-Priority") == "high" && hour("UTC") >= 8 ? tier("priority", p * 2 + c * 8) : tier("base", p * 1 + c * 4)'

    const config = tryParseVisualConfig(expression)
    assert.ok(config)
    assert.equal(config.tiers[0].conditions.length, 3)
    assert.deepEqual(
      tryParseVisualConfig(generateExprFromVisualConfig(config)),
      config
    )
  })

  test('round-trips token, request, per-second, and per-request visual prices', () => {
    const expression =
      'param("parameters.resolution") == "1080P" ? tier("1080p", 0.1 * 1000000 + p * 2 + c * 4 + param("parameters.duration") * 0.5 * 1000000 + cr * 0.2 + img * 3) : tier("base", p * 1 + c * 2)'

    const config = tryParseVisualConfig(expression)
    assert.ok(config)
    assert.equal(config.tiers[0].per_request_unit_cost, 0.1)
    assert.equal(config.tiers[0].per_second_unit_cost, 0.5)
    assert.equal(config.tiers[0].cache_read_unit_cost, 0.2)
    assert.equal(config.tiers[0].image_unit_cost, 3)

    const generated = generateExprFromVisualConfig(config)
    assert.ok(tryParseVisualConfig(generated))
    assert.equal(parseTiersFromExpr(generated)[0].perRequestPrice, 0.1)
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

  test('preserves tiers and conditions with null coalescing and optional chaining', () => {
    const fast = 'param("fast") ?? false'
    const priority = 'param("meta")?.priority == "high"'
    const tiers = parseTiersFromExpr(
      `${fast} ? ${priority} ? tier("priority", p * 3) : tier("fast", p * 2) : tier("normal", p * 1)`
    )
    const condition = (expression: string) => ({
      source: 'expression',
      expression,
    })
    assert.deepEqual(
      tiers.map(({ label, inputPrice, conditions }) => [
        label,
        inputPrice,
        conditions,
      ]),
      [
        ['priority', 3, [condition(fast), condition(priority)]],
        ['fast', 2, [condition(fast), condition(`!(${priority})`)]],
        ['normal', 1, [condition(`!(${fast})`)]],
      ]
    )
  })

  test('keeps conditional add-ons in the raw expression instead of showing only the base price', () => {
    const tiers = parseTiersFromExpr(`param("resolution") == "0.5k"
      ? tier("0.5k", 45000 + (param("enable_web_search") == true ? 14000 : 0) + (param("enable_image_search") == true ? 14000 : 0))
      : param("resolution") == "2k"
        ? tier("2k", 105000 + (param("enable_web_search") == true ? 14000 : 0) + (param("enable_image_search") == true ? 14000 : 0))
        : param("resolution") == "4k"
          ? tier("4k", 140000 + (param("enable_web_search") == true ? 14000 : 0) + (param("enable_image_search") == true ? 14000 : 0))
          : tier("1k", 70000 + (param("enable_web_search") == true ? 14000 : 0) + (param("enable_image_search") == true ? 14000 : 0))`)

    assert.deepEqual(tiers, [])
  })

  test('exposes pixel-threshold image request prices', () => {
    const tiers = parseTiersFromExpr(
      'pixels(param("size")) <= 2360000 ? tier("lte_236mp", 0.30 * 1000000) : tier("gt_236mp", 0.60 * 1000000)'
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
        { label: 'lte_236mp', prices: ['$0.3'] },
        { label: 'gt_236mp', prices: ['$0.6'] },
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

describe('complete and explicit pricing display', () => {
  test('falls back to the entire formula for unsupported price terms', () => {
    for (const expr of [
      'tier("base", response("cost") * 1000000)',
      'tier("base", max(p * 2, 1000) + c * 4)',
      'tier("base", p * 2 / 100 + c * 4)',
      'tier("base", p * 2 + p * 3 + c * 4)',
      'tier("base", p * 2 - cr * 0.1)',
      'tier("base", p * 1e999)',
      'tier("base", param("p * 999"))',
      'len < 100 ? tier("simple", p * 2) : tier("complex", max(p * 2, 1000))',
      'len < 100 ? 0 : tier("paid", p * 2)',
      'len < 100 ? tier("paid", p * 2) : p * 0 + c * 0',
    ]) {
      assert.deepEqual(parseTiersFromExpr(expr), [], expr)
    }
  })

  test('keeps every explicit zero category while leaving unconfigured prices absent', () => {
    const tiers = parseTiersFromExpr(
      'tier("free_extras", p * 2 + c * 3 + cr * 0 + cc * 0 + cc1h * 0 + img * 0 + img_o * 0 + ai * 0 + ao * 0 + param("parameters.duration") * 0 * 1000000 + 0 * 1000000)'
    )
    const entries = getDynamicPriceEntries(tiers[0], { tokenUnit: 'M' })
    assert.equal(entries.length, 11)
    assert.equal(entries.filter((entry) => entry.value === 0).length, 9)
    assert.ok(
      entries
        .filter((entry) => entry.value === 0)
        .every((entry) => entry.formatted === '$0')
    )

    const simple = parseTiersFromExpr('tier("base", p * 2 + c * 0)')[0]
    assert.equal(simple.cacheReadPrice, undefined)
    assert.deepEqual(
      getDynamicPriceEntries(simple, { tokenUnit: 'M' }).map((entry) => [
        entry.key,
        entry.value,
      ]),
      [
        ['p', 2],
        ['c', 0],
      ]
    )
  })

  test('distinguishes zero and absent prices across tiers and group discounts', () => {
    const tiers = parseTiersFromExpr(
      'len < 100 ? tier("free_cache", p * 2 + cr * 0) : tier("regular", p * 4 + cc * 5)'
    )
    const rows = buildDynamicTierPriceDisplayRows(tiers, {
      tokenUnit: 'M',
      groupRatioMultiplier: 0.5,
    })
    assert.deepEqual(
      rows[0].regularEntries.map((entry) => [entry.key, entry.formatted]),
      [
        ['p', '$1'],
        ['cr', '$0'],
      ]
    )
    assert.deepEqual(
      rows[1].regularEntries.map((entry) => entry.key),
      ['p']
    )
    assert.deepEqual(rows[0].cacheWriteEntries, [])
    assert.deepEqual(
      rows[1].cacheWriteEntries.map((entry) => [entry.key, entry.formatted]),
      [['cc', '$2.5']]
    )
  })

  test('shows explicitly free tiers and preserves scientific notation prices', () => {
    const free = parseTiersFromExpr('tier("free", 0)')[0]
    assert.deepEqual(
      getDynamicPriceEntries(free, { tokenUnit: 'M' }).map((entry) => [
        entry.key,
        entry.formatted,
      ]),
      [['request', '$0']]
    )
    const scientific = parseTiersFromExpr(
      ' v1:tier("base", (p * 2e+0) + c * 1e-2 + cr * 0) '
    )[0]
    assert.deepEqual(
      getDynamicPriceEntries(scientific, { tokenUnit: 'M' }).map(
        (entry) => entry.value
      ),
      [2, 0.01, 0]
    )
  })
})

const peakCondition = `((hour("Asia/Shanghai") >= 9 && hour("Asia/Shanghai") < 12) ||
  (hour("Asia/Shanghai") >= 14 && hour("Asia/Shanghai") < 18))`
const offPeakDiscount = `(weekday("Asia/Shanghai") == 0 || weekday("Asia/Shanghai") == 6 ||
  hour("Asia/Shanghai") < 9 || (hour("Asia/Shanghai") >= 12 && hour("Asia/Shanghai") < 14) ||
  hour("Asia/Shanghai") >= 18) ? 0.5 : 1`
const flashExpr = `${peakCondition}
  ? tier("peak", p * 0.43795620437956206 + c * 1.3138686131386863 + cr * 0.014598540145985403)
  : tier("off_peak", p * 0.21897810218978103 + c * 0.65693430656934315 + cr * 0.0072992700729927014)`

describe('saved expression compatibility and preservation', () => {
  test('opens each previously unsupported production pricing shape', () => {
    const cases = [
      flashExpr,
      `${peakCondition} ? tier("peak", p * 1.3138686131386863 + c * 3.9416058394160585 + cr * 0.043795620437956206) : tier("off_peak", p * 0.65693430656934315 + c * 1.9708029197080292 + cr * 0.021897810218978103)`,
      `tier("base", p * 0.03 + c * 1.2 + cr * 0.006) * (\n${offPeakDiscount}\n)`,
      'img <= 2360000\n?\ntier("image_normal",43000)\n:\ntier("image_hd",86000)',
    ]
    for (const expr of cases) {
      const split = splitBillingExprAndRequestRules(expr)
      const state = createPricingEditorState(
        'model',
        split.billingExpr,
        split.requestRuleExpr
      )
      assert.equal(state.mode, 'visual', expr)
      const config = state.visualConfig!
      assert.ok(tryParseVisualConfig(generateExprFromVisualConfig(config)))
      assert.ok(parseTiersFromExpr(expr).length)
    }
  })

  test('retains source bytes across opening, mode switches, and saving without edits', () => {
    const expr = `v1:${flashExpr}`
    const state = createPricingEditorState('model', expr, '')
    const raw = switchPricingEditorMode(state, 'raw')
    const visual = switchPricingEditorMode(raw, 'visual')
    assert.equal(visual.billingExpr, expr)
    assert.equal(visual.rawExpr, expr)
    const reopened = createPricingEditorState(
      'model',
      visual.billingExpr,
      visual.requestRuleExpr
    )
    assert.equal(reopened.billingExpr, expr)
  })

  test('keeps unsupported formulas in raw mode without substituting default prices', () => {
    for (const expr of [
      'tier("base", p * 2 + p * 3 + c * 4)',
      'len < 100 ? 0 : tier("base", p * 2 + c * 4)',
      'len < 100 ? tier("base", p * 2 + c * 4) : cr * 0',
      'len < 100 ? (c < 50 ? tier("a", p * 2) : 0) : tier("b", p * 4)',
      'v2:tier("base", p * 2)',
      'tier("base", max(p * 2, 1000) + c * 4)',
    ]) {
      const state = createPricingEditorState('model', expr, '')
      assert.equal(state.mode, 'raw', expr)
      assert.equal(switchPricingEditorMode(state, 'visual').billingExpr, expr)
      assert.equal(switchPricingEditorMode(state, 'visual').mode, 'raw')
    }
  })

  test('preserves untouched long decimals and explicitly zero-priced token categories', () => {
    const expr =
      'tier("base", p * 0.43795620437956206 + c * 0.65693430656934315 + cr * 0)'
    const state = createPricingEditorState('model', expr, '')
    const config = state.visualConfig!
    config.tiers[0].input_unit_cost = 0.1234
    const changed = editVisualPricing(state, config)
    assert.match(changed.billingExpr, /p \* 0\.1234/)
    assert.match(changed.billingExpr, /c \* 0\.65693430656934315/)
    assert.match(changed.billingExpr, /cr \* 0/)
    const reopened = createPricingEditorState('model', changed.billingExpr, '')
    assert.equal(reopened.visualConfig!.tiers[0].input_unit_cost, 0.1234)
  })

  test('keeps a compound discount as one multiplier and shows its condition', () => {
    const groups = tryParseRequestRuleExpr(`(${offPeakDiscount})`)!
    assert.equal(groups.length, 1)
    assert.equal(groups[0].multiplier, '0.5')
    const rebuilt = buildRequestRuleExpr(groups)
    assert.deepEqual(tryParseRequestRuleExpr(rebuilt), groups)
    const display = parseTiersFromExpr(flashExpr)
    assert.ok(display[0].conditions.length)
    assert.match(JSON.stringify(display[0].conditions), /hour/)
    const state = createPricingEditorState(
      'model',
      flashExpr,
      `(${offPeakDiscount})`
    )
    groups[0].multiplier = '0.1234'
    assert.equal(editPricingRules(state, groups).billingExpr, flashExpr)
  })

  test('round-trips generated guarded parameter rules and preserves quoted numbers', () => {
    for (const condition of [
      'param("n") != nil && param("n") >= 2',
      'param("tags") != nil && has(param("tags"), "vip")',
      'param("code") == "123"',
      'header("Flag") == "true"',
      'param("code") == " vip "',
      'param("a( && ) * b") != nil',
    ]) {
      const rule = `(${condition} ? 0.5 : 1)`
      const groups = tryParseRequestRuleExpr(rule)
      assert.ok(groups, rule)
      assert.equal(buildRequestRuleExpr(groups), rule)
      const full = combineBillingExpr('tier("base", c * 3.3333)', rule)
      assert.equal(splitBillingExprAndRequestRules(full).requestRuleExpr, rule)
    }
    const expr = 'param("code") == "123" ? tier("a", c * 1) : tier("b", c * 2)'
    assert.match(
      generateExprFromVisualConfig(tryParseVisualConfig(expr)),
      /== "123"/
    )
  })

  test('keeps array and object comparisons in the raw billing expression', () => {
    for (const literal of ['["express"]', '{"speed":"express"}']) {
      const condition = `param("options") == ${literal}`
      const tiered = `${condition} ? tier("express", p * 2) : tier("normal", p * 1)`
      const rule = `(${condition} ? 2 : 1)`
      const multiplied = combineBillingExpr('tier("base", p * 1)', rule)
      for (const expr of [tiered, multiplied]) {
        const state = createPricingEditorState('model', expr, '')
        assert.equal(state.mode, 'raw')
        assert.equal(switchPricingEditorMode(state, 'visual').rawExpr, expr)
        assert.equal(switchPricingEditorMode(state, 'visual').mode, 'raw')
      }
      assert.equal(tryParseRequestRuleExpr(rule), null)
    }
  })

  test('shows image and nested request conditions in the pricing breakdown', () => {
    for (const expr of [
      'img <= 2360000 ? tier("normal",43000) : tier("hd",86000)',
      'param("resolution") == "1080p" ? (param("video") != nil ? tier("video", c * 3.3) : tier("plain", c * 5.6)) : tier("base", c * 2)',
    ]) {
      const tiers = parseTiersFromExpr(expr)
      assert.ok(tiers[0].conditions.length)
      assert.match(JSON.stringify(tiers[0].conditions), /img|resolution/)
    }
  })

  test('preserves raw edits and hydrates another model without carrying prior prices', () => {
    const state = createPricingEditorState('one', flashExpr, '')
    const expr = 'tier("custom", response("cost") * 1000000)'
    const edited = editRawPricing(state, expr)
    assert.equal(edited.billingExpr, expr)
    assert.equal(switchPricingEditorMode(edited, 'visual').mode, 'raw')
    const other = createPricingEditorState(
      'two',
      'tier("base", c * 0.1234)',
      ''
    )
    assert.equal(other.visualConfig!.tiers[0].output_unit_cost, 0.1234)
  })
})

test('does not promote a multiplier from one ternary branch to the whole price', () => {
  const expr =
    'param("vip") == true ? tier("vip", c * 1) : tier("base", c * 2) * (hour("UTC") < 9 ? 0.5 : 1)'
  assert.deepEqual(splitBillingExprAndRequestRules(expr), {
    billingExpr: expr,
    requestRuleExpr: '',
  })
  assert.equal(createPricingEditorState('model', expr, '').mode, 'raw')
  assert.deepEqual(parseTiersFromExpr(expr), [])
})

describe('billing formula preservation regressions', () => {
  test('retains exact substring boundaries when rebuilding request rules', () => {
    for (const condition of [
      'has(header("X-Tags"), " pro ")',
      'param("tags") != nil && has(param("tags"), " pro ")',
    ]) {
      const rule = `(${condition} ? 2 : 1)`
      const groups = tryParseRequestRuleExpr(rule)
      assert.ok(groups)
      assert.equal(buildRequestRuleExpr(groups), rule)
    }
  })

  test('preserves valid Expr-only string escapes in raw mode', () => {
    for (const condition of [
      String.raw`header("X\x2dTags") == "pro"`,
      String.raw`has(header("X-Tags"), "\x20pro\x20")`,
    ]) {
      const rule = `(${condition} ? 2 : 1)`
      const expr = `(tier("base", p * 1)) * ${rule}`
      assert.equal(tryParseRequestRuleExpr(rule), null)
      const { billingExpr, requestRuleExpr } =
        splitBillingExprAndRequestRules(expr)
      const state = createPricingEditorState(
        'test',
        billingExpr,
        requestRuleExpr
      )
      assert.equal(state.mode, 'raw')
      assert.deepEqual(switchPricingEditorMode(state, 'visual'), state)
      assert.equal(state.rawExpr, expr)
    }
  })

  const discount = '(hour("UTC") >= 0 ? 0.5 : 1)'

  test('keeps multipliers on their original summand, with or without whitespace', () => {
    for (const join of ['*', ' * ']) {
      const expr = `tier("base",p*2)+tier("extra",c*3)${join}${discount}`
      const parts = splitBillingExprAndRequestRules(expr)
      assert.deepEqual(parts, { billingExpr: expr, requestRuleExpr: '' })
      const state = createPricingEditorState(
        'model',
        parts.billingExpr,
        parts.requestRuleExpr
      )
      assert.equal(state.mode, 'raw')
      assert.equal(state.rawExpr, expr)
      assert.equal(
        evalExprLocally(state.rawExpr, 100, 100, emptyExtras).cost,
        350
      )
    }
  })

  test('leaves non-product arithmetic and boolean expressions opaque', () => {
    for (const expr of [
      `tier("base",p*2) - tier("extra",c*3) * ${discount}`,
      `tier("base",p*2) / 2 * ${discount}`,
      `tier("base",p*2) * ${discount} + 10`,
      `tier("base",p*2) * ${discount} == 100 ? 10 : 20`,
      `tier("base",p*2) ** 2 * ${discount}`,
    ]) {
      assert.deepEqual(splitBillingExprAndRequestRules(expr), {
        billingExpr: expr,
        requestRuleExpr: '',
      })
    }
  })

  test('extracts an overall multiplier when a sum is explicitly grouped', () => {
    const base = 'tier("base",p*2)+tier("extra",c*3)'
    const expr = `(${base}) * ${discount}`
    const parts = splitBillingExprAndRequestRules(expr)
    assert.deepEqual(parts, { billingExpr: base, requestRuleExpr: discount })
    assert.equal(
      combineBillingExpr(parts.billingExpr, parts.requestRuleExpr),
      expr
    )
    assert.equal(evalExprLocally(expr, 100, 100, emptyExtras).cost, 250)
  })

  test('ignores operators inside quoted strings and scientific coefficients', () => {
    const base = 'tier("a*+\\"b", p * 2e+0 + c * 3e-1)'
    const parts = splitBillingExprAndRequestRules(`${base} * ${discount}`)
    assert.equal(parts.billingExpr, base)
    assert.equal(parts.requestRuleExpr, discount)
    assert.ok(tryParseVisualConfig(parts.billingExpr))
    for (const quote of ["'", '`']) {
      const opaqueBase = `tier(${quote}a*+(b)${quote}, p * 2)`
      assert.equal(
        splitBillingExprAndRequestRules(`${opaqueBase} * ${discount}`)
          .billingExpr,
        opaqueBase
      )
    }
  })

  test('keeps the version prefix outside parentheses through rule and price edits', () => {
    const base = 'v1:tier("base", p * 3 + c * 15)'
    for (const rules of [
      discount,
      `${discount} * (hour("UTC") < 24 ? 2 : 1)`,
    ]) {
      const original = createPricingEditorState('model', base, '')
      const state = editPricingRules(original, tryParseRequestRuleExpr(rules)!)
      assert.match(state.rawExpr, /^v1:\(/)
      assert.doesNotMatch(state.rawExpr, /\(v1:/)
      const parts = splitBillingExprAndRequestRules(state.rawExpr)
      assert.equal(parts.billingExpr, base)
      assert.equal(
        tryParseRequestRuleExpr(parts.requestRuleExpr)!.length,
        tryParseRequestRuleExpr(rules)!.length
      )
      const reopened = createPricingEditorState(
        'model',
        parts.billingExpr,
        parts.requestRuleExpr
      )
      assert.equal(reopened.mode, 'visual')
      const edited = editVisualPricing(reopened, {
        tiers: [
          setVisualTierPrice(
            reopened.visualConfig!.tiers[0],
            'output_unit_cost',
            16
          ),
        ],
      })
      assert.match(edited.billingExpr, /^v1:/)
      const preview = evalExprLocally(edited.rawExpr, 100, 100, emptyExtras)
      assert.equal(preview.error, null)
      assert.equal(preview.cost, rules === discount ? 950 : 1900)
    }
  })

  test('does not reinterpret unsupported expression versions', () => {
    const expr = `v2:tier("base",p*2) * ${discount}`
    assert.deepEqual(splitBillingExprAndRequestRules(expr), {
      billingExpr: expr,
      requestRuleExpr: '',
    })
    assert.equal(createPricingEditorState('model', expr, '').mode, 'raw')
    assert.match(
      evalExprLocally(expr, 100, 100, emptyExtras).error!,
      /Unsupported expression version/
    )
  })

  test('makes an edited zero price independent of saving and reopening', () => {
    const base = 'tier("base",p*2+c*3)'
    for (const [field, variable] of [
      ['cache_read_unit_cost', 'cr'],
      ['cache_create_unit_cost', 'cc'],
      ['cache_create_1h_unit_cost', 'cc1h'],
      ['image_unit_cost', 'img'],
      ['image_output_unit_cost', 'img_o'],
      ['audio_input_unit_cost', 'ai'],
      ['audio_output_unit_cost', 'ao'],
      ['per_second_unit_cost', 'param("parameters.duration")'],
      ['per_request_unit_cost', ''],
    ]) {
      const initial = tryParseVisualConfig(base)!.tiers[0]
      assert.equal(isVisualPriceEnabled(initial, field), false)
      const priced = setVisualTierPrice(initial, field, 0.1)
      const saved = generateExprFromVisualConfig({ tiers: [priced] })
      const reopened = tryParseVisualConfig(saved)!.tiers[0]
      const directZero = generateExprFromVisualConfig({
        tiers: [setVisualTierPrice(priced, field, 0)],
      })
      const reopenedZero = generateExprFromVisualConfig({
        tiers: [setVisualTierPrice(reopened, field, 0)],
      })
      assert.equal(directZero, reopenedZero, field)
      assert.ok(
        directZero.includes(variable ? `${variable} * 0` : '0 * 1000000'),
        field
      )
      assert.equal(
        isVisualPriceEnabled(tryParseVisualConfig(directZero)!.tiers[0], field),
        true
      )
      const disabled = setVisualTierPriceEnabled(reopened, field, false)
      assert.equal(
        generateExprFromVisualConfig({ tiers: [disabled] }),
        generateExprFromVisualConfig({ tiers: [initial] })
      )
      assert.equal(
        isVisualPriceEnabled(
          tryParseVisualConfig(
            generateExprFromVisualConfig({ tiers: [disabled] })
          )!.tiers[0],
          field
        ),
        false
      )
    }
  })

  test('distinguishes enabling a free cache price from leaving it unset', () => {
    const config = createDefaultVisualConfig()
    assert.doesNotMatch(generateExprFromVisualConfig(config), /cr/)
    config.tiers[0] = setVisualTierPriceEnabled(
      config.tiers[0],
      'cache_read_unit_cost',
      true
    )
    assert.match(generateExprFromVisualConfig(config), /cr \* 0/)
    config.tiers[0] = setVisualTierPriceEnabled(
      config.tiers[0],
      'cache_read_unit_cost',
      false
    )
    assert.doesNotMatch(generateExprFromVisualConfig(config), /cr/)
  })

  test('shows a free one-hour cache price and removes it when switching to generic cache', () => {
    const config = tryParseVisualConfig(
      'tier("base", p * 2 + c * 3 + cc * 0 + cc1h * 0)'
    )!
    assert.equal(getTierCacheMode(config.tiers[0]), CACHE_MODE_TIMED)
    const generic = setVisualTierCacheMode(config.tiers[0], CACHE_MODE_GENERIC)
    const expr = generateExprFromVisualConfig({ tiers: [generic] })
    assert.doesNotMatch(expr, /cc1h/)
    assert.match(expr, /cc \* 0/)
    assert.equal(
      getTierCacheMode(tryParseVisualConfig(expr)!.tiers[0]),
      CACHE_MODE_GENERIC
    )
  })

  test('retains untouched high precision when toggling optional pricing', () => {
    const config = tryParseVisualConfig(
      'tier("base", c * 0.1234567890123456789)'
    )!
    config.tiers[0] = setVisualTierPriceEnabled(
      config.tiers[0],
      'cache_read_unit_cost',
      true
    )
    const generated = generateExprFromVisualConfig(config)
    assert.match(generated, /c \* 0\.1234567890123456789/)
    assert.match(generated, /cr \* 0/)
  })
})
