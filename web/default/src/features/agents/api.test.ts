import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  buildAgentGroupRuleRows,
  buildAgentGroupRatioPayload,
  buildAgentUserGroupPayload,
  buildAgentUserListParams,
  buildAgentUserGroupOptions,
  getAgentGroupRatioEditValue,
  getAgentGroupRatioFormDraft,
  getAgentGroupRatioInputFloor,
  getAgentSystemGroupRatioFloor,
  getAgentGroupRatioTableValues,
  getAgentSystemGroupDefaultRatio,
  getAgentUserGroupFormDraft,
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
        visible_groups: ['vip'],
        group_ratios: { vip: 1.7 },
      }),
      {
        group_name: 'member',
        visible_groups: ['vip'],
        group_ratios: { vip: 1.7 },
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

  test('builds an empty user group draft when creating from the list', () => {
    assert.deepEqual(getAgentUserGroupFormDraft(), {
      groupName: '',
      visibleGroups: [],
      groupRatios: {},
    })
  })

  test('copies user group rules into an edit draft without sharing arrays', () => {
    const rule = {
      group_name: 'vip-users',
      visible_groups: ['default', 'vip'],
      group_ratios: { vip: 1.4 },
    }
    const draft = getAgentUserGroupFormDraft(rule)

    rule.visible_groups.push('hidden')
    rule.group_ratios.vip = 2

    assert.deepEqual(draft, {
      groupName: 'vip-users',
      visibleGroups: ['default', 'vip'],
      groupRatios: { vip: 1.4 },
    })
  })
})

describe('agent group ratio payload helpers', () => {
  test('builds editable rule rows for every system group', () => {
    assert.deepEqual(
      buildAgentGroupRuleRows([
        {
          group_name: 'vip',
          system_group_name: 'vip',
          agent_ratio: 1.2,
          system_ratio: 1.4,
          configured_ratio: 1.6,
          effective_ratio: 1.6,
          configured: true,
          visible: false,
          available: true,
        },
        {
          group_name: 'default',
          system_group_name: 'default',
          agent_ratio: 1.1,
          system_ratio: 1,
          configured_ratio: 0,
          effective_ratio: 1.1,
          configured: false,
          visible: true,
          available: true,
        },
      ]),
      [
        {
          systemGroupName: 'vip',
          status: 'configured',
          agentDiscount: '1.2',
          effectiveDiscount: '1.6',
          baseDiscount: '1.4',
          visible: false,
          available: true,
        },
        {
          systemGroupName: 'default',
          status: 'system_default',
          agentDiscount: '1.1',
          effectiveDiscount: '1.1',
          baseDiscount: '1',
          visible: true,
          available: true,
        },
      ]
    )
  })

  test('sends system group as the rule key with custom description', () => {
    const payload = buildAgentGroupRatioPayload({
      group_name: ' agent-pro ',
      system_group_name: ' vip ',
      description: ' Premium rule ',
      ratio: 1.8,
      visible: false,
    })

    assert.deepEqual(payload, {
      group_name: 'vip',
      system_group_name: 'vip',
      description: 'Premium rule',
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
        group_name: 'vip',
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

  test('builds an agent group edit draft from the selected system group rule', () => {
    assert.deepEqual(
      getAgentGroupRatioFormDraft({
        group_name: 'vip',
        system_group_name: 'vip',
        description: 'VIP visible rule',
        system_ratio: 1.4,
        configured_ratio: 1.6,
        effective_ratio: 1.6,
        configured: true,
        visible: false,
        available: true,
      }),
      {
        systemGroupName: 'vip',
        description: 'VIP visible rule',
        ratio: '1.6',
        visible: false,
      }
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
            group_name: 'vip',
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
        group_name: 'OpenAI',
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

  test('shows agent user-group discount when no sales rule is configured', () => {
    assert.deepEqual(
      getAgentGroupRatioTableValues({
        group_name: 'vip',
        system_group_name: 'vip',
        agent_ratio: 1.4,
        system_ratio: 1.25,
        configured_ratio: 0,
        effective_ratio: 1.4,
        configured: false,
        visible: true,
        available: true,
      }),
      {
        agentDiscount: '1.4',
        effectiveDiscount: '1.4',
      }
    )
  })

  test('uses agent user-group discount as floor when no sales rule is configured', () => {
    const groupRatios = [
      {
        group_name: 'vip',
        system_group_name: 'vip',
        agent_ratio: 1.4,
        system_ratio: 1.25,
        configured_ratio: 0,
        effective_ratio: 1.4,
        configured: false,
        visible: true,
        available: true,
      },
    ]

    assert.equal(getAgentGroupRatioInputFloor(groupRatios, 'vip'), 1.4)
    assert.equal(getAgentSystemGroupRatioFloor(groupRatios, 'vip'), 1.4)
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

    assert.equal(getAgentGroupRatioInputFloor(groupRatios, 'svip'), 1)
  })

  test('uses agent initial ratio as floor for the system group rule', () => {
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

    assert.equal(getAgentGroupRatioInputFloor(groupRatios, ' svip '), 1)
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

    assert.equal(getAgentGroupRatioInputFloor(groupRatios, 'svip'), 1.2)
  })
})
