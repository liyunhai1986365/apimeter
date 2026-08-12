import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  buildOpenMosaicCallbackUrl,
  buildOpenMosaicEmbedUrl,
  buildOpenMosaicStartUrl,
} from './openmosaic-language'

describe('OpenMosaic language handoff', () => {
  test('passes every supported new-api language to the embedded route', () => {
    for (const language of ['zhCN', 'en', 'fr', 'ru', 'ja', 'vi', 'zhTW']) {
      const url = new URL(
        buildOpenMosaicEmbedUrl(
          'https://creation.example.com',
          { code: 'one-time-code', site_origin: 'https://new-api.example.com' },
          language
        )
      )

      assert.equal(url.pathname, '/auth/apimeter/embed')
      assert.equal(url.searchParams.get('lang'), language)
      assert.equal(url.searchParams.get('code'), 'one-time-code')
      assert.equal(
        url.searchParams.get('site_origin'),
        'https://new-api.example.com'
      )
      assert.equal(url.searchParams.get('redirect'), '/image')
    }
  })

  test('builds the APIMeter start and callback contract paths', () => {
    const start = new URL(
      buildOpenMosaicStartUrl('https://creation.example.com/base/')
    )
    const callback = new URL(
      buildOpenMosaicCallbackUrl('https://creation.example.com/base/')
    )

    assert.equal(start.origin, 'https://creation.example.com')
    assert.equal(start.pathname, '/api/auth/apimeter/start')
    assert.equal(start.searchParams.get('redirect'), '/home')
    assert.equal(callback.origin, 'https://creation.example.com')
    assert.equal(callback.pathname, '/auth/apimeter/callback')
    assert.equal(callback.search, '')
  })

  test('normalizes browser-style Chinese language tags', () => {
    const url = new URL(
      buildOpenMosaicEmbedUrl(
        'https://creation.example.com',
        { code: 'code', site_origin: 'https://new-api.example.com' },
        'zh-TW'
      )
    )

    assert.equal(url.searchParams.get('lang'), 'zhTW')
  })
})
