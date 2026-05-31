import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { appendEndpointTemplate } from './endpoint-template'

const openAI = { path: '/v1/chat/completions', method: 'POST' }
const gemini = {
  path: '/v1beta/models/{model}:generateContent',
  method: 'POST',
}

describe('appendEndpointTemplate', () => {
  test('creates endpoint JSON when no endpoints exist', () => {
    assert.equal(
      appendEndpointTemplate('', 'openai', openAI),
      JSON.stringify({ openai: openAI }, null, 2)
    )
  })

  test('keeps existing endpoints and appends the selected template', () => {
    const current = JSON.stringify(
      {
        openai: openAI,
        anthropic: { path: '/v1/messages', method: 'POST' },
      },
      null,
      2
    )

    const next = appendEndpointTemplate(current, 'gemini', gemini)

    assert.deepEqual(JSON.parse(next).openai, openAI)
    assert.deepEqual(JSON.parse(next).anthropic, {
      path: '/v1/messages',
      method: 'POST',
    })
    assert.deepEqual(JSON.parse(next).gemini, gemini)
  })

  test('returns the current text unchanged when endpoints JSON is invalid', () => {
    assert.equal(
      appendEndpointTemplate('{ invalid json', 'openai', openAI),
      '{ invalid json'
    )
  })
})
