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
import { formatNumber, formatQuota } from '@/lib/format'
import type { SystemStatus } from '@/features/auth/types'

export type InviteRewardConfig = {
  topupRewardRatio: number
  topupRewardLimit: number
  inviterRegisterQuota: number
}

export function getInviteRewardConfig(
  status: SystemStatus | Record<string, unknown> | null | undefined
): InviteRewardConfig {
  return {
    topupRewardRatio: toNumber(status?.affiliate_topup_reward_ratio),
    topupRewardLimit: toNumber(status?.affiliate_topup_reward_limit),
    inviterRegisterQuota: toNumber(status?.quota_for_inviter),
  }
}

export function hasInviteRewards(config: InviteRewardConfig): boolean {
  return config.topupRewardRatio > 0 || config.inviterRegisterQuota > 0
}

export function formatInviteRewardRatio(value: number): string {
  return `${formatNumber(value)}%`
}

export function formatInviteRegisterReward(value: number): string {
  return formatQuota(value)
}

function toNumber(value: unknown): number {
  const numberValue = Number(value ?? 0)
  return Number.isFinite(numberValue) ? numberValue : 0
}
