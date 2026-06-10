/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getApiKeys, getWorkspaces } from '../api'

export const ALL_WORKSPACE_FILTER_VALUE = '__all_workspaces__'
export const ALL_TOKEN_FILTER_VALUE = '__all_tokens__'

const TOKEN_OPTIONS_PAGE_SIZE = 1000

export interface WorkspaceFilterOption {
  value: string
  label: string
  id?: number
  isDefault?: boolean
  tokenCount?: number
}

export interface TokenFilterOption {
  value: string
  label: string
}

interface UseWorkspaceTokenOptionsParams {
  workspaceName?: string
  tokenName?: string
  enabled?: boolean
}

function appendCurrentOption<T extends { value: string; label: string }>(
  options: T[],
  currentValue?: string
): T[] {
  const value = currentValue?.trim()
  if (!value || options.some((option) => option.value === value)) return options
  return [{ value, label: value } as T, ...options]
}

export function useWorkspaceTokenOptions(
  params: UseWorkspaceTokenOptionsParams = {}
) {
  const enabled = params.enabled ?? true
  const workspaceQuery = useQuery({
    queryKey: ['workspace-token-filter-options', 'workspaces'],
    queryFn: async () => {
      const result = await getWorkspaces()
      if (!result.success) throw new Error(result.message || 'Load failed')
      return result.data || []
    },
    enabled,
    staleTime: 60_000,
  })

  const workspaces = workspaceQuery.data || []
  const selectedWorkspace = workspaces.find(
    (workspace) => workspace.name === params.workspaceName
  )
  const selectedWorkspaceId = selectedWorkspace?.id
  const canLoadTokens =
    enabled && (!params.workspaceName || !!selectedWorkspaceId)

  const tokenQuery = useQuery({
    queryKey: [
      'workspace-token-filter-options',
      'tokens',
      selectedWorkspaceId || 'all',
      params.workspaceName || '',
    ],
    queryFn: async () => {
      const result = await getApiKeys({
        p: 1,
        size: TOKEN_OPTIONS_PAGE_SIZE,
        workspace_id: selectedWorkspaceId,
      })
      if (!result.success) throw new Error(result.message || 'Load failed')
      return result.data?.items || []
    },
    enabled: canLoadTokens,
    staleTime: 60_000,
  })

  const workspaceOptions = useMemo<WorkspaceFilterOption[]>(() => {
    const options = workspaces.map((workspace) => ({
      value: workspace.name,
      label: workspace.name,
      id: workspace.id,
      isDefault: workspace.is_default,
      tokenCount: workspace.token_count,
    }))
    return appendCurrentOption(options, params.workspaceName)
  }, [params.workspaceName, workspaces])

  const tokenOptions = useMemo<TokenFilterOption[]>(() => {
    const seen = new Set<string>()
    const options: TokenFilterOption[] = []

    for (const token of tokenQuery.data || []) {
      const name = token.name?.trim()
      if (!name || seen.has(name)) continue
      seen.add(name)
      options.push({ value: name, label: name })
    }

    return appendCurrentOption(options, params.tokenName)
  }, [params.tokenName, tokenQuery.data])

  return {
    workspaceOptions,
    tokenOptions,
    isLoadingWorkspaces: workspaceQuery.isLoading,
    isLoadingTokens: tokenQuery.isLoading,
  }
}
