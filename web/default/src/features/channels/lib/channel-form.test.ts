import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  divideChannelRatioBySupplierScale,
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
      channel_ratio: 0.143,
    }

    const createPayload = transformFormDataToCreatePayload(formData)
    const updatePayload = transformFormDataToUpdatePayload(formData, 42)

    assert.equal(createPayload.channel.channel_ratio, 0.143)
    assert.equal(updatePayload.channel_ratio, 0.143)
  })

  test('divides channel cost ratio by supplier scale', () => {
    assert.equal(divideChannelRatioBySupplierScale(7), 1)
    assert.equal(divideChannelRatioBySupplierScale(1.25), 0.179)
    assert.equal(divideChannelRatioBySupplierScale(Number.NaN), 0)
  })

  test('stores OpenAI image response format override in channel setting', () => {
    const formData = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'image-format-channel',
      key: 'sk-test',
      models: 'gpt-image-2',
      openai_image_response_format: 'b64_json' as const,
    }

    const createPayload = transformFormDataToCreatePayload(formData)
    const updatePayload = transformFormDataToUpdatePayload(formData, 42)

    assert.equal(
      JSON.parse(createPayload.channel.setting || '{}')
        .openai_image_response_format,
      'b64_json'
    )
    assert.equal(
      JSON.parse(updatePayload.setting || '{}').openai_image_response_format,
      'b64_json'
    )
  })
})
