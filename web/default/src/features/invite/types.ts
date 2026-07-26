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
export type AffiliateRoleConfig = {
  id: string
  name: string
  topup_reward_ratio?: number | null
  topup_reward_limit?: number | null
  consume_reward_ratio?: number | null
  inviter_reward_quota?: number | null
  invitee_reward_quota?: number | null
}

export type AffiliateRewardPolicy = {
  role_id: string
  role_name: string
  uses_default_role: boolean
  topup_reward_ratio: number
  topup_reward_limit: number
  consume_reward_ratio: number
  inviter_reward_quota: number
  invitee_reward_quota: number
}
