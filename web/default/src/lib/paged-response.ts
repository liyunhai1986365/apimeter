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

export type PagedResponseShape<T> = {
  success?: boolean
  data?: {
    items?: T[] | null
    total?: number | null
    page?: number | null
    page_size?: number | null
    [key: string]: unknown
  } | null
}

export type NormalizedPagedData<T> = {
  items: T[]
  total: number
  page?: number
  page_size?: number
  [key: string]: unknown
}

export function normalizePagedData<T>(
  response: PagedResponseShape<T> | null | undefined
): NormalizedPagedData<T> {
  const data = response?.success === false ? undefined : response?.data
  const {
    items: _items,
    total: _total,
    page: _page,
    page_size: _pageSize,
    ...rest
  } = data ?? {}
  const normalized: NormalizedPagedData<T> = {
    ...rest,
    items: Array.isArray(data?.items) ? data.items : [],
    total: typeof data?.total === 'number' ? data.total : 0,
  }

  if (typeof data?.page === 'number') {
    normalized.page = data.page
  }
  if (typeof data?.page_size === 'number') {
    normalized.page_size = data.page_size
  }

  return normalized
}
