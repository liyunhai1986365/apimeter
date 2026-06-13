import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  buildAgentUserListParams,
  parseAgentBranding,
  stringifyAgentBranding,
} from './api'

describe('agent branding helpers', () => {
  test('preserves custom home page content in agent branding', () => {
    const branding = parseAgentBranding(
      JSON.stringify({
        site_name: 'Agent Site',
        logo: 'https://agent.example.com/logo.png',
        home_page_content: '# Agent Home',
        header_nav_modules: '{"home":false}',
      })
    )

    assert.deepEqual(branding, {
      site_name: 'Agent Site',
      logo: 'https://agent.example.com/logo.png',
      home_page_content: '# Agent Home',
      header_nav_modules: '{"home":false}',
    })
    assert.equal(
      stringifyAgentBranding(branding),
      JSON.stringify({
        site_name: 'Agent Site',
        logo: 'https://agent.example.com/logo.png',
        home_page_content: '# Agent Home',
        header_nav_modules: '{"home":false}',
      })
    )
  })
})

describe('agent user list params', () => {
  test('includes pagination and trimmed keyword when searching users', () => {
    assert.deepEqual(buildAgentUserListParams(3, 50, '  alice@example.com  '), {
      p: 3,
      page_size: 50,
      keyword: 'alice@example.com',
    })
  })

  test('omits empty keyword when listing users', () => {
    assert.deepEqual(buildAgentUserListParams(1, 20, '   '), {
      p: 1,
      page_size: 20,
    })
  })
})
