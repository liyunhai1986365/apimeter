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
import type { ApiKey } from '../types'

export function getApiKeyQuotaDisplay(
  apiKey: Pick<ApiKey, 'used_quota' | 'remain_quota' | 'unlimited_quota'>
) {
  if (apiKey.unlimited_quota) {
    return {
      unlimited: true,
      leftQuota: apiKey.used_quota,
      usedQuota: apiKey.used_quota,
      remainingQuota: apiKey.remain_quota,
      totalQuota: null,
      percentage: 100,
    }
  }

  const usedQuota = apiKey.used_quota
  const remainingQuota = apiKey.remain_quota
  const totalQuota = usedQuota + remainingQuota
  const percentage =
    totalQuota > 0 ? (remainingQuota / totalQuota) * 100 : 0

  return {
    unlimited: false,
    leftQuota: remainingQuota,
    usedQuota,
    remainingQuota,
    totalQuota,
    percentage,
  }
}
