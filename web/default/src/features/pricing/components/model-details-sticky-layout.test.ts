import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  MODEL_DETAILS_STICKY_BAR_CLASS,
  MODEL_DETAILS_STICKY_INFO_CLASS,
} from './model-details-sticky-layout'

describe('model details sticky layout', () => {
  test('uses an 84px mobile bar and a 48px desktop bar', () => {
    assert.match(MODEL_DETAILS_STICKY_BAR_CLASS, /\bh-\[5\.25rem\](?:\s|$)/)
    assert.match(MODEL_DETAILS_STICKY_BAR_CLASS, /\bsm:h-12\b/)
  })

  test('keeps the model information row compact', () => {
    assert.match(MODEL_DETAILS_STICKY_INFO_CLASS, /\bh-8\b/)
    assert.doesNotMatch(MODEL_DETAILS_STICKY_INFO_CLASS, /\bpy-[1-9]/)
  })
})
