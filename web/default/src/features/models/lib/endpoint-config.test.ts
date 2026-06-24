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

  test('preserves structured endpoint config when normalizing and serializing', () => {
    const raw = JSON.stringify({
      'seedance2-native-video': {
        path: '/api/v3/contents/generations/tasks',
        method: 'post',
        label: 'Seedance2 Native Video',
        config: {
          protocol_profile_id: 'seedance2-service-inference',
          mode: 'native_video',
          submit_path: '/api/v3/contents/generations/tasks',
          fetch_path: '/api/v3/contents/generations/tasks/{task_id}',
          path_params: ['task_id'],
          parameters: {
            resolution: { type: 'string', enum: ['480p', '720p'] },
            duration: { type: 'integer', default: 4 },
          },
        },
      },
    })

    const rows = normalizeEndpointConfig(raw)

    assert.equal(rows[0].method, 'POST')
    assert.deepEqual(rows[0].config?.path_params, ['task_id'])

    const serialized = JSON.parse(serializeEndpointConfig(rows))
    assert.deepEqual(
      serialized['seedance2-native-video'].config,
      rows[0].config
    )
  })
})
