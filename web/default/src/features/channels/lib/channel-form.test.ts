import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { Channel } from '../types'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  CONVERSION_OPTIONS,
  CONVERSION_OPTION_OPENAI_IMAGE_GENERATIONS_TO_GEMINI,
  REQUEST_MODE_OPTIONS,
  divideChannelRatioBySupplierScale,
  transformChannelToFormDefaults,
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

  test('keeps status in create payloads but omits it from general updates', () => {
    const formData = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'status-separated-channel',
      key: 'sk-test',
      models: 'gpt-4o',
      status: 2,
    }

    const createPayload = transformFormDataToCreatePayload(formData)
    const updatePayload = transformFormDataToUpdatePayload(formData, 42)

    assert.equal(createPayload.channel.status, 2)
    assert.equal('status' in updatePayload, false)
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

  test('stores and reloads AWS Bedrock request conversion setting for AWS and Anthropic channels', () => {
    for (const type of [14, 33]) {
      const formData = {
        ...CHANNEL_FORM_DEFAULT_VALUES,
        name: 'aws-bedrock-channel',
        type,
        key: 'test-key',
        models: 'claude-opus-4-8',
        aws_bedrock_request_conversion_enabled: true,
      }

      const createPayload = transformFormDataToCreatePayload(formData)
      const setting = JSON.parse(createPayload.channel.setting || '{}')
      assert.equal(setting.aws_bedrock_request_conversion_enabled, true)

      const channel = {
        ...createPayload.channel,
        id: 42,
        type,
        status: 1,
        name: 'aws-bedrock-channel',
        channel_ratio: 1,
        created_time: 0,
        test_time: 0,
        response_time: 0,
        other: '',
        balance: 0,
        balance_updated_time: 0,
        used_quota: 0,
        other_info: '',
        remark: '',
        max_input_tokens: 0,
        channel_info: {
          is_multi_key: false,
          multi_key_size: 0,
          multi_key_polling_index: 0,
          multi_key_mode: 'random',
        },
        settings: '{}',
      } as Channel

      const defaults = transformChannelToFormDefaults(channel)
      assert.equal(defaults.aws_bedrock_request_conversion_enabled, true)
    }
  })

  test('exposes concrete image endpoints and both Gemini image conversions', () => {
    const imageEndpoint = REQUEST_MODE_OPTIONS.find(
      (option) => option.value === 'openai.image.generations'
    )
    const geminiEndpoint = REQUEST_MODE_OPTIONS.find(
      (option) => option.value === 'gemini.generate_content'
    )

    assert.equal(imageEndpoint?.endpoint, 'POST /v1/images/generations')
    assert.equal(
      geminiEndpoint?.endpoint,
      'POST /v1beta/models/{model}:generateContent'
    )
    assert.deepEqual(geminiEndpoint?.aliases, [
      'POST /v1/models/{model}:generateContent',
    ])
    assert.ok(
      CONVERSION_OPTIONS.some(
        (option) =>
          option.value === CONVERSION_OPTION_OPENAI_IMAGE_GENERATIONS_TO_GEMINI
      )
    )
  })

  test('stores explicit image endpoints and conversions in channel setting', () => {
    const formData = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'explicit-image-protocol-channel',
      key: 'sk-test',
      models: 'custom-provider-photo-v9',
      protocol_native_modes: [
        'openai.image.generations',
        'gemini.generate_content',
      ],
      protocol_enabled_conversions: [
        'openai.image.generations_to_gemini.generate_content',
        'gemini.generate_content_to_openai.image.generations',
      ],
    }

    const createSettings = JSON.parse(
      transformFormDataToCreatePayload(formData).channel.setting || '{}'
    )
    const updateSettings = JSON.parse(
      transformFormDataToUpdatePayload(formData, 42).setting || '{}'
    )

    assert.deepEqual(createSettings.protocol, updateSettings.protocol)
    assert.deepEqual(createSettings.protocol.native_modes, [
      'openai.image.generations',
      'gemini.generate_content',
    ])
    assert.deepEqual(createSettings.protocol.enabled_conversions, [
      'openai.image.generations_to_gemini.generate_content',
      'gemini.generate_content_to_openai.image.generations',
    ])
  })

  test('loads explicit image endpoints and conversions from channel setting', () => {
    const channel = {
      id: 42,
      type: 24,
      key: 'sk-test',
      status: 1,
      name: 'gemini-image-channel',
      channel_ratio: 1,
      created_time: 0,
      test_time: 0,
      response_time: 0,
      other: '',
      balance: 0,
      balance_updated_time: 0,
      models: 'gemini-3.1-flash-image-preview',
      group: 'default',
      used_quota: 0,
      other_info: '',
      remark: '',
      max_input_tokens: 0,
      channel_info: {
        is_multi_key: false,
        multi_key_size: 0,
        multi_key_polling_index: 0,
        multi_key_mode: 'random',
      },
      settings: '{}',
      setting: JSON.stringify({
        protocol: {
          native_modes: ['gemini.generate_content'],
          enabled_conversions: [
            'openai.image.generations_to_gemini.generate_content',
            'future.image.conversion',
          ],
        },
      }),
    } as Channel

    const defaults = transformChannelToFormDefaults(channel)

    assert.deepEqual(defaults.protocol_native_modes, [
      'gemini.generate_content',
    ])
    assert.deepEqual(defaults.protocol_enabled_conversions, [
      'openai.image.generations_to_gemini.generate_content',
      'future.image.conversion',
    ])
  })
})

describe('TgxMaas channel project', () => {
  const formData = {
    ...CHANNEL_FORM_DEFAULT_VALUES,
    name: 'project-channel',
    type: 999,
    key: 'test-key',
    models: 'seedance',
    protocol_profile_id: 'seedance-tgxmaas',
    protocol_project_name: ' nmyk ',
  }
  test('requires a project only for the TgxMaas profile', () => {
    assert.equal(
      channelFormSchema.safeParse({ ...formData, protocol_project_name: '' })
        .success,
      false
    )
    assert.equal(
      channelFormSchema.safeParse({ ...formData, protocol_project_name: '  ' })
        .success,
      false
    )
    assert.equal(channelFormSchema.safeParse(formData).success, true)
    assert.equal(
      channelFormSchema.safeParse({
        ...formData,
        protocol_profile_id: 'generic-video-json',
        protocol_project_name: '',
      }).success,
      true
    )
  })
  test('saves, reloads and updates the channel project', () => {
    const createPayload = transformFormDataToCreatePayload(formData)
    const setting = JSON.parse(createPayload.channel.setting || '{}')
    assert.equal(setting.protocol.project_name, 'nmyk')
    const channel = {
      ...createPayload.channel,
      id: 42,
      channel_info: { is_multi_key: false },
    } as Channel
    const defaults = transformChannelToFormDefaults(channel)
    assert.equal(defaults.protocol_project_name, 'nmyk')
    const update = transformFormDataToUpdatePayload(defaults, 42)
    assert.equal(
      JSON.parse(update.setting || '{}').protocol.project_name,
      'nmyk'
    )
    const switched = transformFormDataToCreatePayload({
      ...formData,
      protocol_profile_id: 'generic-video-json',
    })
    assert.equal(
      JSON.parse(switched.channel.setting || '{}').protocol.project_name,
      undefined
    )
  })
})

describe('independent asset library configuration', () => {
  test('sends official credentials separately from public channel settings', () => {
    const values = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: 999,
      name: 'official-assets',
      key: 'video-key',
      models: 'seedance',
      protocol_profile_id: 'seedance2-ark-task-assets',
      protocol_project_name: 'nmyk',
      asset_backend: 'volcengine-assets',
      asset_base_url: 'https://ark.cn-beijing.volcengineapi.com',
      asset_access_key_id: 'asset-ak',
      asset_secret_access_key: 'asset-sk',
    }
    const create = transformFormDataToCreatePayload(values)
    const update = transformFormDataToUpdatePayload(values, 42)
    const settings = JSON.parse(create.channel.setting || '{}')
    assert.equal(settings.protocol.profile_id, 'seedance2-ark-task-assets')
    assert.equal(settings.protocol.project_name, 'nmyk')
    assert.equal(settings.protocol.asset_library.auth_mode, 'aksk')
    assert.deepEqual(create.asset_credentials, {
      access_key_id: 'asset-ak',
      secret_access_key: 'asset-sk',
    })
    assert.deepEqual(update.asset_credentials, create.asset_credentials)
    assert.equal(create.channel.setting?.includes('asset-sk'), false)
    assert.equal(
      transformFormDataToUpdatePayload(
        { ...values, asset_access_key_id: '', asset_secret_access_key: '' },
        42
      ).asset_credentials,
      undefined
    )
  })
  test('requires projects only for backends that use them', () => {
    const values = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: 999,
      name: 'test',
      key: 'video-key',
      models: 'seedance',
      protocol_profile_id: 'seedance2-ark-task-assets',
      asset_backend: 'volcengine-assets',
    }
    assert.equal(channelFormSchema.safeParse(values).success, false)
    assert.equal(
      channelFormSchema.safeParse({ ...values, protocol_project_name: 'nmyk' })
        .success,
      true
    )
    assert.equal(
      channelFormSchema.safeParse({ ...values, asset_backend: 'task' }).success,
      true
    )
  })
  test('drops hidden credentials when switching to inherited or disabled libraries', () => {
    for (const backend of ['inherit', 'disabled']) {
      const values = {
        ...CHANNEL_FORM_DEFAULT_VALUES,
        type: 999,
        asset_backend: backend,
        asset_auth_mode: 'api_key',
        asset_api_key: 'hidden-secret',
      }
      assert.equal(
        transformFormDataToCreatePayload(values).asset_credentials,
        undefined
      )
    }
  })
})
