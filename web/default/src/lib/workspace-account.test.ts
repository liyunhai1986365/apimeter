import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { AuthUser } from '@/stores/auth-store'
import {
  canManageTeamSettings,
  getWorkspaceAccountRedirect,
} from './workspace-account'

const workspaceAccount: AuthUser = {
  id: 2,
  username: 'workspace-manager',
  role: 1,
  workspace_subaccount: true,
}

describe('getWorkspaceAccountRedirect', () => {
  test('does not restrict main accounts', () => {
    assert.equal(
      getWorkspaceAccountRedirect(
        { ...workspaceAccount, workspace_subaccount: false },
        '/wallet'
      ),
      null
    )
  })

  test('keeps workspace account routes inside the limited console', () => {
    for (const pathname of [
      '/keys',
      '/dashboard/overview',
      '/usage-logs/common',
      '/profile',
    ]) {
      assert.equal(
        getWorkspaceAccountRedirect(workspaceAccount, pathname),
        null
      )
    }
    assert.equal(
      getWorkspaceAccountRedirect(workspaceAccount, '/wallet'),
      '/keys'
    )
  })

  test('forces temporary-password accounts to change their password first', () => {
    const temporaryAccount = {
      ...workspaceAccount,
      must_change_password: true,
    }

    assert.equal(
      getWorkspaceAccountRedirect(temporaryAccount, '/keys'),
      '/change-password'
    )
    assert.equal(
      getWorkspaceAccountRedirect(temporaryAccount, '/change-password'),
      null
    )
  })
})

describe('canManageTeamSettings', () => {
  test('shows team settings only to main accounts', () => {
    assert.equal(canManageTeamSettings(null), false)
    assert.equal(canManageTeamSettings(workspaceAccount), false)
    assert.equal(
      canManageTeamSettings({
        ...workspaceAccount,
        workspace_subaccount: false,
      }),
      true
    )
  })
})
