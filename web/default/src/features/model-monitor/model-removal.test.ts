import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { removeModelFromChannelModels } from './model-removal'

describe('removeModelFromChannelModels', () => {
  test('removes the failed model from a channel model list', () => {
    assert.equal(
      removeModelFromChannelModels(
        'gpt-4o, claude-sonnet, gpt-5',
        'claude-sonnet'
      ),
      'gpt-4o,gpt-5'
    )
  })

  test('keeps model ids with partial name overlap', () => {
    assert.equal(
      removeModelFromChannelModels('gpt-4,gpt-4o,gpt-4.1', 'gpt-4'),
      'gpt-4o,gpt-4.1'
    )
  })
})
