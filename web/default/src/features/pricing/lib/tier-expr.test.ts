import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
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
