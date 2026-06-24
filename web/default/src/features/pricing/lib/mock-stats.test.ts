import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  buildSupportedParameters,
  buildTaskQueryParameters,
} from './mock-stats'
import type { PricingModel } from '../types'

function modelWithEndpoints(endpoints: string[]): PricingModel {
  return {
    id: 1,
    model_name: 'doubao-seedance-2-0-fast',
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: ['default'],
    supported_endpoint_types: endpoints,
  }
}

describe('buildSupportedParameters', () => {
  test('uses Seedance2 native video endpoint parameters', () => {
    const params = buildSupportedParameters(
      modelWithEndpoints(['seedance2-native-video'])
    )
    const names = params.map((p) => p.name)

    assert.deepEqual(names.slice(0, 4), [
      'content',
      'resolution',
      'duration',
      'ratio',
    ])
    assert.ok(names.includes('generate_audio'))
    assert.ok(!names.includes('temperature'))
  })

  test('uses generic OpenAI video endpoint parameters', () => {
    const params = buildSupportedParameters(modelWithEndpoints(['openai-video']))
    const names = params.map((p) => p.name)

    assert.deepEqual(names.slice(0, 3), ['prompt', 'size', 'seconds'])
    assert.ok(names.includes('metadata'))
    assert.ok(!names.includes('temperature'))
  })
})

describe('buildTaskQueryParameters', () => {
  test('returns task_id for video task query endpoints', () => {
    const params = buildTaskQueryParameters(
      modelWithEndpoints(['seedance2-native-video'])
    )

    assert.deepEqual(params.map((p) => p.name), ['task_id'])
    assert.equal(params[0].required, true)
    assert.equal(params[0].type, 'string')
    assert.equal(params[0].location, 'path')
  })

  test('returns no task query parameters for non-task endpoints', () => {
    assert.deepEqual(buildTaskQueryParameters(modelWithEndpoints(['openai'])), [])
  })
})
