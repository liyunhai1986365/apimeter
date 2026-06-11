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
import type { ComboboxInputOption } from '@/components/ui/combobox-input'
import { getUserGroups } from '@/lib/api'
import { getChannelFilterOptions } from '@/features/channels/api'

interface UseLogFilterOptionsParams {
  group?: string
  channel?: string
  includeGroups?: boolean
  includeChannels?: boolean
}

function appendCurrentOption(
  options: ComboboxInputOption[],
  currentValue?: string,
  currentLabel?: string
): ComboboxInputOption[] {
  const value = currentValue?.trim()
  if (!value || options.some((option) => option.value === value)) return options
  return [{ value, label: currentLabel || value }, ...options]
}

export function useLogFilterOptions(params: UseLogFilterOptionsParams = {}) {
  const includeGroups = params.includeGroups ?? true

  const groupsQuery = useQuery({
    queryKey: ['log-filter-options', 'groups'],
    queryFn: async () => {
      const result = await getUserGroups()
      if (!result.success) throw new Error(result.message || 'Load failed')
      return Object.keys(result.data || {})
    },
    enabled: includeGroups,
    staleTime: 60_000,
  })

  const channelsQuery = useQuery({
    queryKey: ['log-filter-options', 'channels'],
    queryFn: async () => {
      const result = await getChannelFilterOptions()
      if (!result.success) throw new Error(result.message || 'Load failed')
      return result.data || []
    },
    enabled: !!params.includeChannels,
    staleTime: 60_000,
  })

  const groupOptions = useMemo<ComboboxInputOption[]>(() => {
    const options = (groupsQuery.data || [])
      .filter(Boolean)
      .map((group) => ({ value: group, label: group }))
    return appendCurrentOption(options, params.group)
  }, [groupsQuery.data, params.group])

  const channelOptions = useMemo<ComboboxInputOption[]>(() => {
    const options = (channelsQuery.data || []).map((channel) => {
      const value = String(channel.id)
      return {
        value,
        label: channel.name ? `${value} · ${channel.name}` : `#${value}`,
      }
    })
    return appendCurrentOption(
      options,
      params.channel,
      params.channel ? `#${params.channel}` : undefined
    )
  }, [channelsQuery.data, params.channel])

  return {
    groupOptions,
    channelOptions,
  }
}
