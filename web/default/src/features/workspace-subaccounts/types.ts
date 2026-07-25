import type { Workspace } from '@/features/keys/types'

export interface WorkspaceSubaccountSummary {
  id: number
  username: string
  display_name: string
  email: string
  status: number
  must_change_password: boolean
  created_at: number
  last_login_at: number
  workspace_count: number
  token_count: number
  last_used_at: number
}

export interface WorkspaceSubaccountDetails {
  user: WorkspaceSubaccountSummary
  workspaces: Workspace[]
}

export interface WorkspaceSubaccountPayload {
  username: string
  display_name: string
  email: string
  password: string
  workspace_ids: number[]
}

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}
