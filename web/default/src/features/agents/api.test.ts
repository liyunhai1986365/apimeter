import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  buildAgentGroupRatioPayload,
  buildAgentUserGroupPayload,
  buildAgentUserListParams,
  buildAgentUserGroupOptions,
  getAgentGroupRatioEditValue,
  getAgentGroupRatioInputFloor,
  getAgentSystemGroupRatioFloor,
  getAgentGroupRatioTableValues,
  getAgentSystemGroupDefaultRatio,
  getAgentUserGroupRatioFloor,
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

  test('sends user group ratio overrides with appended groups', () => {
    assert.deepEqual(
      buildAgentUserGroupPayload({
        group_name: ' member ',
        visible_groups: ['agent-vip'],
        group_ratios: { 'agent-vip': 1.7 },
      }),
      {
        group_name: 'member',
        visible_groups: ['agent-vip'],
        group_ratios: { 'agent-vip': 1.7 },
      }
    )
  })

  test('uses agent initial ratio as user group override floor', () => {
    assert.equal(
      getAgentUserGroupRatioFloor({
        group_name: 'svip',
        system_group_name: 'svip',
        agent_ratio: 1,
        system_ratio: 1,
        configured_ratio: 1,
        effective_ratio: 1.2,
        configured: true,
        visible: true,
        available: true,
      }),
      1
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

  test('uses configured ratio for editing instead of effective ratio', () => {
    assert.equal(
      getAgentGroupRatioEditValue({
        group_name: 'agent-vip',
        system_group_name: 'vip',
        system_ratio: 1.4,
        configured_ratio: 1.6,
        effective_ratio: 1.6,
        configured: true,
        visible: true,
        available: true,
      }),
      '1.6'
    )
  })

  test('uses agent discount when starting from an existing system group option', () => {
    assert.equal(
      getAgentSystemGroupDefaultRatio(
        [
          {
            group_name: 'vip',
            system_group_name: 'vip',
            system_ratio: 1.4,
            configured_ratio: 0,
            effective_ratio: 1.4,
            configured: false,
            visible: true,
            available: true,
          },
          {
            group_name: 'agent-vip',
            system_group_name: 'vip',
            agent_ratio: 1.2,
            system_ratio: 1.4,
            configured_ratio: 1.4,
            effective_ratio: 1.4,
            configured: true,
            visible: true,
            available: true,
          },
        ],
        'vip'
      ),
      '1.2'
    )
  })

  test('shows agent initial ratio separately from effective ratio in table', () => {
    assert.deepEqual(
      getAgentGroupRatioTableValues({
        group_name: 'default22',
        system_group_name: 'OpenAI',
        agent_ratio: 0.2,
        system_ratio: 0.24,
        configured_ratio: 0.24,
        effective_ratio: 0.24,
        configured: true,
        visible: true,
        available: true,
      }),
      {
        agentDiscount: '0.2',
        effectiveDiscount: '0.24',
      }
    )
  })

  test('uses agent initial ratio as floor when editing an existing agent group', () => {
    const groupRatios = [
      {
        group_name: 'svip',
        system_group_name: 'svip',
        agent_ratio: 1,
        system_ratio: 1.2,
        configured_ratio: 1.2,
        effective_ratio: 1.2,
        configured: true,
        visible: true,
        available: true,
      },
    ]

    assert.equal(getAgentGroupRatioInputFloor(groupRatios, 'svip', 'svip'), 1)
  })

  test('uses agent initial ratio as floor when creating a new agent group for the same system group', () => {
    const groupRatios = [
      {
        group_name: 'svip',
        system_group_name: 'svip',
        agent_ratio: 1,
        system_ratio: 1.2,
        configured_ratio: 1.2,
        effective_ratio: 1.2,
        configured: true,
        visible: true,
        available: true,
      },
    ]

    assert.equal(
      getAgentGroupRatioInputFloor(groupRatios, 'new-svip', 'svip'),
      1
    )
    assert.equal(getAgentSystemGroupRatioFloor(groupRatios, 'svip'), 1)
  })

  test('uses current system ratio as floor when the system group has no agent discount yet', () => {
    const groupRatios = [
      {
        group_name: 'svip',
        system_group_name: 'svip',
        system_ratio: 1.2,
        configured_ratio: 0,
        effective_ratio: 1.2,
        configured: false,
        visible: true,
        available: true,
      },
    ]

    assert.equal(
      getAgentGroupRatioInputFloor(groupRatios, 'new-svip', 'svip'),
      1.2
    )
  })
})
