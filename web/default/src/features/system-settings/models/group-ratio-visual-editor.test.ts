import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  buildConfiguredBillingGroupNameOptions,
  buildConfiguredGroupNameOptions,
  buildConfiguredUserGroupNameOptions,
} from './group-ratio-visual-editor'

describe('group ratio visual editor group options', () => {
  test('uses all configured pricing groups when display config is present', () => {
    const groups = buildConfiguredGroupNameOptions(
      JSON.stringify({
        default: 1,
        hidden: 1,
        vip: 1,
      }),
      JSON.stringify({
        default: '默认分组',
        hidden: '不可选但残留',
        vip: '用户分组残留',
      }),
      JSON.stringify({
        categories: [],
        groups: [
          { group: 'default', order: 10 },
          { group: 'vip', order: 20, user_group: true },
        ],
      })
    )

    assert.deepEqual(groups, ['default', 'vip', 'hidden'])
  })

  test('includes token-unselectable and user groups in add group dropdowns', () => {
    const groups = buildConfiguredGroupNameOptions(
      JSON.stringify({
        default: 1,
        hidden: 1,
        vip: 1,
      }),
      JSON.stringify({
        default: '默认分组',
      }),
      JSON.stringify({
        categories: [],
        groups: [
          { group: 'default', order: 10 },
          { group: 'hidden', order: 20 },
          { group: 'vip', order: 30, user_group: true },
        ],
      })
    )

    assert.deepEqual(groups, ['default', 'hidden', 'vip'])
  })

  test('includes every non-user billing group in model ratio dropdowns', () => {
    const groups = buildConfiguredBillingGroupNameOptions(
      JSON.stringify({
        default: 1,
        hidden: 1,
        vip: 1,
      }),
      JSON.stringify({
        default: '默认分组',
        vip: '用户分组',
      }),
      JSON.stringify({
        categories: [],
        groups: [
          { group: 'display-only', order: 10 },
          { group: 'default', order: 20 },
          { group: 'vip', order: 30, user_group: true },
          { group: 'auto', order: 40 },
        ],
      })
    )

    assert.deepEqual(groups, ['display-only', 'default', 'hidden'])
  })

  test('uses every displayed user group in user group dropdowns', () => {
    const groups = buildConfiguredUserGroupNameOptions(
      JSON.stringify({
        default: 1,
        hidden: 1,
        vip: 1,
      }),
      JSON.stringify({
        default: '默认分组',
        vip: '用户分组',
      }),
      JSON.stringify({
        categories: [],
        groups: [
          { group: 'default', order: 10 },
          { group: 'hidden', order: 20, user_group: false },
          { group: 'vip', order: 30, user_group: true },
          { group: 'orphan', order: 40, user_group: true },
        ],
      })
    )

    assert.deepEqual(groups, ['vip', 'orphan'])
  })
})
