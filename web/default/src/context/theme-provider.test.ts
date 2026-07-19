import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { fileURLToPath } from 'node:url'
import { DEFAULT_THEME } from './theme-provider'

test('uses dark mode as the application default', () => {
  assert.equal(DEFAULT_THEME, 'dark')
})

test('applies the dark fallback before React mounts', () => {
  const indexHtml = readFileSync(
    fileURLToPath(new URL('../../index.html', import.meta.url)),
    'utf8'
  )

  assert.match(indexHtml, /savedTheme === 'system'/)
  assert.match(indexHtml, /: 'dark'\s*\n\s*document\.documentElement/)
})
