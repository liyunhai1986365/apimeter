import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  buildUserOwnedProviderPayload,
  providerToFormState,
} from './user-owned-provider-form'

describe('user owned provider form helpers', () => {
  test('builds create payload with normalized models and required key', () => {
    const payload = buildUserOwnedProviderPayload({
      name: 'My OpenAI',
      type: 1,
      key: 'sk-test',
      baseUrl: 'https://api.example.com',
      models: 'gpt-4o\ngpt-4.1, gpt-4o',
    })

    assert.deepEqual(payload, {
      mode: 'single',
      channel: {
        type: 1,
        key: 'sk-test',
        name: 'My OpenAI',
        models: 'gpt-4o,gpt-4.1',
        base_url: 'https://api.example.com',
        openai_organization: undefined,
        other: '',
        settings:
          '{"allow_service_tier":false,"disable_store":false,"allow_safety_identifier":false,"allow_include_obfuscation":false,"allow_inference_geo":false}',
        status: 1,
        channel_ratio: 1,
        priority: 0,
        weight: 0,
        auto_ban: 1,
        retry_enabled: true,
      },
    })
  })

  test('builds edit payload that allows blank key to preserve existing key', () => {
    const payload = buildUserOwnedProviderPayload(
      {
        name: 'Updated',
        type: 20,
        key: '',
        baseUrl: '',
        models: 'openrouter/auto',
      },
      { allowBlankKey: true }
    )

    assert.equal(payload.channel.key, '')
    assert.equal(payload.channel.base_url, undefined)
  })

  test('maps provider rows to edit form without exposing key', () => {
    assert.deepEqual(
      providerToFormState({
        id: 7,
        name: 'Provider',
        type: 14,
        group: 'user_owned:1:7',
        models: 'claude-sonnet-4,claude-opus-4',
        base_url: 'https://api.anthropic.com',
        status: 1,
      }),
      {
        name: 'Provider',
        type: 14,
        key: '',
        baseUrl: 'https://api.anthropic.com',
        models: 'claude-sonnet-4, claude-opus-4',
        other: '',
        openaiOrganization: '',
        azureResponsesVersion: '',
        vertexKeyType: 'json',
        awsKeyType: 'ak_sk',
        openrouterEnterprise: false,
      }
    )
  })

  test('builds type-specific settings for AWS API key mode', () => {
    const payload = buildUserOwnedProviderPayload({
      name: 'AWS',
      type: 33,
      key: 'api-key|us-east-1',
      baseUrl: '',
      models: 'claude-sonnet',
      awsKeyType: 'api_key',
    })

    assert.equal(payload.channel.settings, '{"aws_key_type":"api_key"}')
  })

  test('builds type-specific settings for Vertex API key mode and region', () => {
    const payload = buildUserOwnedProviderPayload({
      name: 'Vertex',
      type: 41,
      key: 'vertex-key',
      baseUrl: '',
      models: 'gemini-2.5-pro',
      vertexKeyType: 'api_key',
      other: '{"default":"us-central1"}',
    })

    assert.equal(payload.channel.settings, '{"vertex_key_type":"api_key"}')
    assert.equal(payload.channel.other, '{"default":"us-central1"}')
  })

  test('builds Azure endpoint and API version fields', () => {
    const payload = buildUserOwnedProviderPayload({
      name: 'Azure',
      type: 3,
      key: 'azure-key',
      baseUrl: 'https://example.openai.azure.com',
      models: 'gpt-4o',
      other: '2025-04-01-preview',
      azureResponsesVersion: 'preview',
    })

    assert.equal(payload.channel.base_url, 'https://example.openai.azure.com')
    assert.equal(payload.channel.other, '2025-04-01-preview')
    assert.equal(
      payload.channel.settings,
      '{"azure_responses_version":"preview"}'
    )
  })
})
