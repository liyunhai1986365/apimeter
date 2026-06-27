import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  buildAgentGroupRatioPayload,
  buildAgentUserListParams,
  buildAgentUserGroupOptions,
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
        site_style:
          '{"preset":"ocean-breeze","radius":"lg","scale":"sm","contentLayout":"centered"}',
      })
    )

    assert.deepEqual(branding, {
      site_name: 'Agent Site',
      logo: 'https://agent.example.com/logo.png',
      home_page_content: '# Agent Home',
      header_nav_modules: '{"home":false}',
      site_style:
        '{"preset":"ocean-breeze","radius":"lg","scale":"sm","contentLayout":"centered"}',
    })
    assert.equal(
      stringifyAgentBranding(branding),
      JSON.stringify({
        site_name: 'Agent Site',
        logo: 'https://agent.example.com/logo.png',
        home_page_content: '# Agent Home',
        header_nav_modules: '{"home":false}',
        site_style:
          '{"preset":"ocean-breeze","radius":"lg","scale":"sm","contentLayout":"centered"}',
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

describe('agent user group helpers', () => {
  test('keeps user group options separate from pricing groups', () => {
    assert.deepEqual(
      buildAgentUserGroupOptions([
        { group_name: 'member', visible_groups: ['default'] },
        { group_name: 'vip-member', visible_groups: ['vip'] },
      ]),
      ['member', 'vip-member']
    )
  })
})

describe('agent group ratio payload helpers', () => {
  test('sends one mapped system group with custom description', () => {
    const payload = buildAgentGroupRatioPayload({
      group_name: ' agent-pro ',
      system_group_name: ' vip ',
      description: ' Premium proxy group ',
      ratio: 1.8,
      visible: false,
    })

    assert.deepEqual(payload, {
      group_name: 'agent-pro',
      system_group_name: 'vip',
      description: 'Premium proxy group',
      ratio: 1.8,
      visible: false,
    })
    assert.equal('system_group_names' in payload, false)
    assert.equal('visible_groups' in payload, false)
    assert.equal('remove_groups' in payload, false)
  })
})
