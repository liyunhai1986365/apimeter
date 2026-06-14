import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  DEFAULT_THEME_CUSTOMIZATION,
  parseThemeCustomizationFromStatus,
  serializeThemeCustomization,
} from './theme-customization'

describe('theme customization helpers', () => {
  test('parses agent site style from status branding', () => {
    assert.deepEqual(
      parseThemeCustomizationFromStatus({
        agent_branding: {
          site_style: JSON.stringify({
            preset: 'ocean-breeze',
            radius: 'lg',
            scale: 'sm',
            contentLayout: 'centered',
          }),
        },
      }),
      {
        preset: 'ocean-breeze',
        radius: 'lg',
        scale: 'sm',
        contentLayout: 'centered',
      }
    )
  })

  test('falls back when agent site style has invalid values', () => {
    assert.deepEqual(
      parseThemeCustomizationFromStatus({
        agent_branding: {
          site_style: JSON.stringify({
            preset: 'not-a-theme',
            radius: 'huge',
            scale: 'tiny',
            contentLayout: 'wide',
          }),
        },
      }),
      DEFAULT_THEME_CUSTOMIZATION
    )
  })

  test('serializes theme customization in a stable compact format', () => {
    assert.equal(
      serializeThemeCustomization({
        preset: 'forest-whisper',
        radius: 'md',
        scale: 'lg',
        contentLayout: 'full',
      }),
      '{"preset":"forest-whisper","radius":"md","scale":"lg","contentLayout":"full"}'
    )
  })
})
