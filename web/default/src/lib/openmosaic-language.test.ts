import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { buildOpenMosaicEmbedUrl } from './openmosaic-language'

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

      assert.equal(url.searchParams.get('lang'), language)
      assert.equal(url.searchParams.get('code'), 'one-time-code')
      assert.equal(
        url.searchParams.get('site_origin'),
        'https://new-api.example.com'
      )
      assert.equal(url.searchParams.get('redirect'), '/image')
    }
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
