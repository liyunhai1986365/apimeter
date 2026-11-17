import assert from 'node:assert/strict'
import { test } from 'node:test'
import { DEFAULT_PRICES, parseInitialPrices } from './tool-price-config'

test('legacy Google overrides survive loading, saving and reopening without adding new charges', () => {
  for (const saved of [
    { google_search: 0 },
    { google_search: 10 },
    { 'google_search:gemini-2.5*': 0 },
    { 'google_search:gemini-3*': 2, file_search: 0 },
  ]) {
    const loaded = parseInitialPrices(JSON.stringify(saved))
    const reopened = parseInitialPrices(JSON.stringify(loaded))
    assert.deepEqual(reopened, loaded)
    for (const [key, value] of Object.entries(saved)) {
      assert.equal(reopened[key], value)
    }
    assert.equal(
      Object.keys(reopened).some((key) =>
        key.startsWith('google_search_grounded_prompt')
      ),
      false
    )
    assert.equal(
      Object.keys(reopened).filter((key) => key.startsWith('google_search'))
        .length,
      1
    )
    assert.equal(reopened.web_search, DEFAULT_PRICES.web_search)
  }
})

test('explicit Google pricing rules retain exact scope across save and reload', () => {
  const saved = {
    google_search: 2,
    'google_search_grounded_prompt:gemini-2.5*': 0,
  }
  const loaded = parseInitialPrices(JSON.stringify(saved))
  assert.deepEqual(parseInitialPrices(JSON.stringify(loaded)), loaded)
  assert.equal(loaded.google_search, 2)
  assert.equal(loaded['google_search_grounded_prompt:gemini-2.5*'], 0)
  assert.equal('google_search_grounded_prompt' in loaded, false)
  assert.equal('google_search:gemini-2.5*' in loaded, false)
})

test('new settings keep model-specific Google defaults and unrelated saved prices', () => {
  for (const raw of [undefined, '', '{}', 'invalid', 'null', '[]']) {
    assert.deepEqual(parseInitialPrices(raw), DEFAULT_PRICES)
  }
  const loaded = parseInitialPrices('{"file_search":0}')
  assert.equal(loaded.file_search, 0)
  assert.equal(loaded['google_search:gemini-2.5*'], 0)
  assert.equal(loaded['google_search_grounded_prompt:gemini-2.5*'], 35)
})
