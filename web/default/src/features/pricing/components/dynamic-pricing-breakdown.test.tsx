import i18next from 'i18next'
import assert from 'node:assert/strict'
import { before, describe, test } from 'node:test'
import { renderToStaticMarkup } from 'react-dom/server'
import { initReactI18next } from 'react-i18next'
import { DynamicPricingBreakdown } from './dynamic-pricing-breakdown'

before(async () => {
  await i18next.use(initReactI18next).init({
    lng: 'en',
    resources: { en: { translation: {} } },
    interpolation: { escapeValue: false },
  })
})

describe('DynamicPricingBreakdown rendering', () => {
  test('renders the full custom formula instead of an empty or partial price table', () => {
    for (const expr of [
      'tier("base", response("cost") * 1000000)',
      'tier("base", max(p * 2, 1000) + c * 4)',
      'tier("base", 45000 + (param("search") == true ? 14000 : 0))',
    ]) {
      const html = renderToStaticMarkup(
        <DynamicPricingBreakdown billingExpr={expr} />
      )
      assert.ok(html.replaceAll('&quot;', '"').includes(expr))
      assert.match(html, /Raw expression/)
      assert.doesNotMatch(html, /Tiered price table/)
    }
  })

  test('renders a configured free cache column in both mobile and desktop layouts', () => {
    const html = renderToStaticMarkup(
      <DynamicPricingBreakdown billingExpr='tier("base", p * 2 + c * 3 + cr * 0)' />
    )
    assert.equal(html.match(/Cache Read/g)?.length, 2)
    assert.equal(html.match(/\$0\.0000/g)?.length, 2)
    assert.doesNotMatch(html, /Cache Write|Image In|Audio In|Raw expression/)
  })

  test('keeps an absent price distinct from zero in another tier', () => {
    const html = renderToStaticMarkup(
      <DynamicPricingBreakdown billingExpr='len < 100 ? tier("free_cache", p * 2 + cr * 0) : tier("regular", p * 4)' />
    )
    assert.equal(html.match(/\$0\.0000/g)?.length, 2)
    assert.match(html, />-</)
  })

  test('honors the caller option to hide cache columns', () => {
    const html = renderToStaticMarkup(
      <DynamicPricingBreakdown
        billingExpr='tier("base", p * 2 + cr * 0 + cc * 0 + cc1h * 0)'
        hideCacheColumns
      />
    )
    assert.doesNotMatch(html, /Cache Read|Cache Write/)
    assert.match(html, /\$2\.0000/)
  })
})
