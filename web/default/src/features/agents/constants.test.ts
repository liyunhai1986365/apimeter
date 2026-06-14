import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { AGENT_HOME_PAGE_CONTENT_TEXTAREA_CLASS } from './constants'

describe('agent UI constants', () => {
  test('keeps home page content textarea fixed height and internally scrollable', () => {
    assert.match(AGENT_HOME_PAGE_CONTENT_TEXTAREA_CLASS, /\bh-40\b/)
    assert.match(AGENT_HOME_PAGE_CONTENT_TEXTAREA_CLASS, /\bmin-h-40\b/)
    assert.match(AGENT_HOME_PAGE_CONTENT_TEXTAREA_CLASS, /\bmax-h-40\b/)
    assert.match(AGENT_HOME_PAGE_CONTENT_TEXTAREA_CLASS, /\boverflow-y-auto\b/)
    assert.match(AGENT_HOME_PAGE_CONTENT_TEXTAREA_CLASS, /\bresize-none\b/)
    assert.match(
      AGENT_HOME_PAGE_CONTENT_TEXTAREA_CLASS,
      /\[field-sizing:fixed\]/
    )
  })
})
