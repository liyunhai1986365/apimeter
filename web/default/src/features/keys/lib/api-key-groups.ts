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
import { USER_FACING_GROUP_TERMS } from '@/lib/user-facing-group-terms'
import type { ApiKeyGroupOption } from '../components/api-key-group-combobox'

export const AUTO_GROUP_VALUE = 'auto'
export const AUTO_GROUP_LABEL = 'auto 自动供应商'
export const AUTO_GROUP_DESCRIPTION =
  USER_FACING_GROUP_TERMS.autoDescription

type UserGroupInfo = {
  desc: string
  ratio: number | string
}

export function buildApiKeyGroupOptions(
  groupsRaw: Record<string, UserGroupInfo>,
  includeAutoGroup: boolean,
  selectedGroup?: string
): ApiKeyGroupOption[] {
  const options = Object.entries(groupsRaw).map(([key, info]) => ({
    value: key,
    label: key === AUTO_GROUP_VALUE ? AUTO_GROUP_LABEL : key,
    desc: info.desc || key,
    ratio: info.ratio,
  }))

  if (
    (includeAutoGroup || selectedGroup === AUTO_GROUP_VALUE) &&
    !options.some((option) => option.value === AUTO_GROUP_VALUE)
  ) {
    return [
      {
        value: AUTO_GROUP_VALUE,
        label: AUTO_GROUP_LABEL,
        desc: AUTO_GROUP_DESCRIPTION,
      },
      ...options,
    ]
  }

  return options
}

export function shouldFallbackApiKeyGroup(
  group: string | undefined,
  options: ApiKeyGroupOption[]
): boolean {
  if (!group) return false
  return !options.some((option) => option.value === group)
}
