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
import type { ApiKeyGroupOption } from '../components/api-key-group-combobox'

export const AUTO_GROUP_VALUE = 'auto'

type UserGroupInfo = {
  desc: string
  ratio: number | string
  hide_discount?: boolean
}

export function buildApiKeyGroupOptions(
  groupsRaw: Record<string, UserGroupInfo>,
  _includeAutoGroup: boolean,
  _selectedGroup?: string
): ApiKeyGroupOption[] {
  return Object.entries(groupsRaw)
    .filter(([key]) => key !== AUTO_GROUP_VALUE)
    .map(([key, info]) => ({
      value: key,
      label: key,
      desc: info.desc || key,
      ratio: info.ratio,
      hideDiscount: info.hide_discount === true,
    }))
}

export function shouldFallbackApiKeyGroup(
  group: string | undefined,
  options: ApiKeyGroupOption[]
): boolean {
  if (!group) return false
  return !options.some((option) => option.value === group)
}
