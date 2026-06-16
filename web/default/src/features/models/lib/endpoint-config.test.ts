import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  normalizeEndpointConfig,
  serializeEndpointConfig,
} from './endpoint-config'

describe('endpoint config normalization', () => {
  test('normalizes legacy string and array endpoint JSON', () => {
    assert.deepEqual(
      normalizeEndpointConfig(
        JSON.stringify({
          openai: '/v1/chat/completions',
          gemini: true,
          ignored: false,
        })
      ),
      [
        {
          type: 'openai',
          path: '/v1/chat/completions',
          method: 'POST',
          label: '',
          docs_url: '',
        },
        {
          type: 'gemini',
          path: '',
          method: 'POST',
          label: '',
          docs_url: '',
        },
      ]
    )

    assert.deepEqual(normalizeEndpointConfig('["image-generation"]'), [
      {
        type: 'image-generation',
        path: '',
        method: 'POST',
        label: '',
        docs_url: '',
      },
    ])
  })

  test('preserves labels and docs urls when serializing endpoint config', () => {
    const serialized = serializeEndpointConfig([
      {
        type: 'openai-image-edit',
        path: '/v1/images/edits',
        method: 'POST',
        label: 'OpenAI Image Edit',
        docs_url: 'https://docs.example.com/openai-image-edit',
      },
    ])

    assert.deepEqual(JSON.parse(serialized), {
      'openai-image-edit': {
        path: '/v1/images/edits',
        method: 'POST',
        label: 'OpenAI Image Edit',
        docs_url: 'https://docs.example.com/openai-image-edit',
      },
    })
  })
})
