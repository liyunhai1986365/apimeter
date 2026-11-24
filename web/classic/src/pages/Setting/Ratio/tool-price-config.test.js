import assert from 'node:assert/strict';
import { test } from 'node:test';
import { DEFAULT_PRICES, parseInitialPrices } from './tool-price-config.js';

test('saving fresh or reset tool prices preserves both Google billing units', () => {
  for (const raw of [undefined, '{}', {}, JSON.stringify(DEFAULT_PRICES)]) {
    const prices = parseInitialPrices(raw);
    prices.file_search = 9;
    const saved = parseInitialPrices(JSON.stringify(prices));
    assert.equal(saved.google_search, 14);
    assert.equal(saved.google_search_grounded_prompt, 0);
    for (const version of ['2.5', '2.0', '1.5']) {
      assert.equal(saved[`google_search:gemini-${version}*`], 0);
      assert.equal(saved[`google_search_grounded_prompt:gemini-${version}*`], 35);
    }
  }
});

test('editing another tool preserves existing Google overrides and their scope', () => {
  for (const existing of [
    { google_search: 0 },
    { 'google_search:gemini-3*': 9 },
    { google_search: 2, 'google_search_grounded_prompt:gemini-2.5*': 0 },
    { file_search: 4 },
  ]) {
    const saved = { ...parseInitialPrices(JSON.stringify(existing)), web_search: 7 };
    assert.deepEqual(parseInitialPrices(JSON.stringify(saved)), {
      ...existing,
      web_search: 7,
    });
  }
});
