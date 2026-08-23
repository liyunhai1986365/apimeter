import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  MAINLAND_CODE_SNIPPETS,
  MAINLAND_LOGO_RAIL_PROVIDERS,
} from './landing-data'

describe('mainland China home presentation', () => {
  test('uses domestic provider brands and examples', () => {
    const visibleText = [
      ...MAINLAND_LOGO_RAIL_PROVIDERS.map((provider) => provider.name),
      ...MAINLAND_CODE_SNIPPETS.map((snippet) => snippet.code),
    ].join('\n')

    assert.match(visibleText, /DeepSeek/)
    assert.match(visibleText, /Qwen|qwen/)
    assert.doesNotMatch(visibleText, /GPT|Claude|Gemini|Google|Anthropic/)
  })
})
