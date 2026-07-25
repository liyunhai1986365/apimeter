import { api } from '@/lib/api'
import type {
  ApiResponse,
  WorkspaceSubaccountDetails,
  WorkspaceSubaccountPayload,
  WorkspaceSubaccountSummary,
} from './types'

export async function getWorkspaceSubaccounts(): Promise<
  ApiResponse<WorkspaceSubaccountSummary[]>
> {
  const response = await api.get('/api/workspace-subaccounts')
  return response.data
}

export async function getWorkspaceSubaccount(
  id: number
): Promise<ApiResponse<WorkspaceSubaccountDetails>> {
  const response = await api.get(`/api/workspace-subaccounts/${id}`)
  return response.data
}

export async function createWorkspaceSubaccount(
  payload: WorkspaceSubaccountPayload
): Promise<ApiResponse<WorkspaceSubaccountSummary>> {
  const response = await api.post('/api/workspace-subaccounts', payload)
  return response.data
}

export async function updateWorkspaceSubaccount(
  id: number,
  payload: Pick<WorkspaceSubaccountPayload, 'display_name' | 'email'>
): Promise<ApiResponse<WorkspaceSubaccountSummary>> {
  const response = await api.put(`/api/workspace-subaccounts/${id}`, payload)
  return response.data
}

export async function updateWorkspaceSubaccountStatus(
  id: number,
  status: number
): Promise<ApiResponse> {
  const response = await api.put(`/api/workspace-subaccounts/${id}/status`, {
    status,
  })
  return response.data
}

export async function resetWorkspaceSubaccountPassword(
  id: number,
  password: string
): Promise<ApiResponse> {
  const response = await api.post(
    `/api/workspace-subaccounts/${id}/reset-password`,
    { password }
  )
  return response.data
}

export async function deleteWorkspaceSubaccount(
  id: number
): Promise<ApiResponse> {
  const response = await api.delete(`/api/workspace-subaccounts/${id}`)
  return response.data
}

export async function setWorkspaceAccess(
  workspaceId: number,
  accessUserIds: number[]
): Promise<ApiResponse> {
  const response = await api.put(`/api/workspaces/${workspaceId}/access`, {
    access_user_ids: accessUserIds,
  })
  return response.data
}

export async function setWorkspaceSubaccountWorkspaces(
  accountId: number,
  workspaceIds: number[]
): Promise<ApiResponse> {
  const response = await api.put(
    `/api/workspace-subaccounts/${accountId}/workspaces`,
    { workspace_ids: workspaceIds }
  )
  return response.data
}
