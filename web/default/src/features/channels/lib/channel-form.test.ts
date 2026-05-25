import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
} from './channel-form'

describe('channel form payload transforms', () => {
  test('preserves channel cost ratio when creating and updating channels', () => {
    const formData = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'cost-aware-channel',
      key: 'sk-test',
      models: 'gpt-4o',
      channel_ratio: 1.35,
    }

    const createPayload = transformFormDataToCreatePayload(formData)
    const updatePayload = transformFormDataToUpdatePayload(formData, 42)

    assert.equal(createPayload.channel.channel_ratio, 1.35)
    assert.equal(updatePayload.channel_ratio, 1.35)
  })
})
