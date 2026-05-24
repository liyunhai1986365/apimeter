import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { extractScriptSrc } from './customer-service-script'

describe('extractScriptSrc', () => {
  test('extracts src from pasted script tags', () => {
    assert.equal(
      extractScriptSrc(
        '<script src="//code.tidio.co/lgdxmqumd5zeatipsguynjy3uijop1t3.js" async></script>'
      ),
      '//code.tidio.co/lgdxmqumd5zeatipsguynjy3uijop1t3.js'
    )
  })

  test('accepts bare script URLs', () => {
    assert.equal(
      extractScriptSrc('https://code.tidio.co/example.js'),
      'https://code.tidio.co/example.js'
    )
  })

  test('ignores inline script content', () => {
    assert.equal(extractScriptSrc('<script>alert(1)</script>'), '')
  })
})
