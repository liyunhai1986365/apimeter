import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { type Workspace } from '../types'
import {
  canDeleteWorkspace,
  getWorkspaceAfterDelete,
  normalizeWorkspaceSettingsForm,
} from './workspace-settings'

function workspace(id: number, name: string, isDefault = false): Workspace {
  return {
    id,
    user_id: 1,
    name,
    description: '',
    is_default: isDefault,
    status: 1,
    access_users: [],
    created_time: 0,
    updated_time: 0,
    token_count: 0,
  }
}

describe('workspace settings helpers', () => {
  test('normalizes editable workspace name and description', () => {
    assert.deepEqual(
      normalizeWorkspaceSettingsForm({
        name: '  Research  ',
        description: '  Model experiments  ',
      }),
      {
        name: 'Research',
        description: 'Model experiments',
      }
    )
  })

  test('rejects blank workspace names', () => {
    assert.throws(
      () => normalizeWorkspaceSettingsForm({ name: '   ', description: '' }),
      /workspace name/
    )
  })

  test('does not allow deleting the default workspace', () => {
    assert.equal(canDeleteWorkspace(workspace(1, 'Default', true)), false)
    assert.equal(canDeleteWorkspace(workspace(2, 'Project')), true)
  })

  test('selects the default workspace after deleting the current workspace', () => {
    const workspaces = [
      workspace(1, 'Default', true),
      workspace(2, 'Project A'),
      workspace(3, 'Project B'),
    ]

    assert.equal(getWorkspaceAfterDelete(workspaces, 2), 1)
  })
})
