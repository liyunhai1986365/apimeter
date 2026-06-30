import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { API_KEY_ONBOARDING_OVERLAY_CLASS } from './api-key-onboarding-styles'

describe('api key onboarding dialog styles', () => {
  test('uses a strong backdrop mask for the post-create onboarding dialog', () => {
    assert.match(API_KEY_ONBOARDING_OVERLAY_CLASS, /bg-black\/45/)
    assert.match(API_KEY_ONBOARDING_OVERLAY_CLASS, /backdrop-blur-sm/)
  })
})
