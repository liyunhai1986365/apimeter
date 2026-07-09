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
export type AnnouncementType =
  | 'product_update'
  | 'system_maintenance'
  | 'model_release'
  | 'pricing_update'
  | 'incident'
  | 'general'

export type AnnouncementCategory =
  | 'all'
  | 'product_update'
  | 'system_maintenance'
  | 'model_release'

export interface AnnouncementLike {
  type?: unknown
}

export const ANNOUNCEMENT_TYPE_OPTIONS = [
  {
    value: 'product_update',
    label: 'Product Update',
    color: 'bg-info',
    badgeVariant: 'info',
  },
  {
    value: 'system_maintenance',
    label: 'System Maintenance',
    color: 'bg-warning',
    badgeVariant: 'warning',
  },
  {
    value: 'model_release',
    label: 'Model Release',
    color: 'bg-chart-4',
    badgeVariant: 'purple',
  },
  {
    value: 'pricing_update',
    label: 'Pricing Update',
    color: 'bg-success',
    badgeVariant: 'success',
  },
  {
    value: 'incident',
    label: 'Incident Notice',
    color: 'bg-destructive',
    badgeVariant: 'danger',
  },
  {
    value: 'general',
    label: 'General Announcement',
    color: 'bg-neutral',
    badgeVariant: 'neutral',
  },
] as const

export const ANNOUNCEMENT_CATEGORY_OPTIONS = [
  { value: 'all', label: 'All' },
  { value: 'product_update', label: 'Product Updates' },
  { value: 'system_maintenance', label: 'Maintenance' },
  { value: 'model_release', label: 'Model Releases' },
] as const

export function getAnnouncementTypeOption(type?: string) {
  return (
    ANNOUNCEMENT_TYPE_OPTIONS.find((option) => option.value === type) ??
    ANNOUNCEMENT_TYPE_OPTIONS[5]
  )
}

export function getAnnouncementCategoryLabel(type?: string): string {
  const option = getAnnouncementTypeOption(type)
  return option.label
}

export function filterAnnouncementsByCategory<T extends AnnouncementLike>(
  announcements: T[],
  category: AnnouncementCategory
): T[] {
  if (category === 'all') return announcements

  return announcements.filter((item) => item.type === category)
}
