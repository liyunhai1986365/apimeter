import { type Workspace } from '../types'

export type WorkspaceSettingsForm = {
  name: string
  description: string
}

export function normalizeWorkspaceSettingsForm(
  form: WorkspaceSettingsForm
): WorkspaceSettingsForm {
  const name = form.name.trim()
  if (!name) {
    throw new Error('Please enter a workspace name')
  }
  return {
    name,
    description: form.description.trim(),
  }
}

export function canDeleteWorkspace(workspace: Workspace | null | undefined) {
  return !!workspace && !workspace.is_default
}

export function getWorkspaceAfterDelete(
  workspaces: Workspace[],
  deletedWorkspaceId: number
) {
  const remaining = workspaces.filter(
    (workspace) => workspace.id !== deletedWorkspaceId
  )
  return (
    remaining.find((workspace) => workspace.is_default)?.id ??
    remaining[0]?.id ??
    null
  )
}
