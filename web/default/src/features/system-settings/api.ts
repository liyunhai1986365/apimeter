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
import { api } from '@/lib/api'
import type {
  ConfirmPaymentComplianceResponse,
  CryptoPaymentConfig,
  CryptoPaymentConfigResponse,
  AnnouncementEmailResponse,
  AnnouncementAudience,
  DeleteLogsResponse,
  FetchUpstreamRatiosRequest,
  SystemOptionsResponse,
  UpdateOptionRequest,
  UpdateOptionResponse,
  GlobalWebhookTestResponse,
  RoutingStrategyRefreshResponse,
  RoutingStrategySnapshotResponse,
  UpstreamChannelsResponse,
  UpstreamRatiosResponse,
} from './types'

export async function getSystemOptions() {
  const res = await api.get<SystemOptionsResponse>('/api/option/')
  return res.data
}

export async function updateSystemOption(request: UpdateOptionRequest) {
  const res = await api.put<UpdateOptionResponse>('/api/option/', request)
  return res.data
}

export async function sendAnnouncementEmail(request: {
  title: string
  content: string
  type: string
  audience: AnnouncementAudience
}) {
  const res = await api.post<AnnouncementEmailResponse>(
    '/api/option/announcements/email',
    request
  )
  return res.data
}

export async function sendGlobalWebhookTest() {
  const res = await api.post<GlobalWebhookTestResponse>(
    '/api/option/global-webhook/test'
  )
  return res.data
}

export async function confirmPaymentCompliance() {
  const res = await api.post<ConfirmPaymentComplianceResponse>(
    '/api/option/payment_compliance',
    { confirmed: true }
  )
  return res.data
}

export async function getCryptoPaymentConfig() {
  const res = await api.get<CryptoPaymentConfigResponse>(
    '/api/option/crypto-payment'
  )
  return res.data
}

export async function saveCryptoPaymentConfig(request: CryptoPaymentConfig) {
  const res = await api.post<CryptoPaymentConfigResponse>(
    '/api/option/crypto-payment',
    request
  )
  return res.data
}

export async function deleteLogsBefore(targetTimestamp: number) {
  const res = await api.delete<DeleteLogsResponse>('/api/log/', {
    params: { target_timestamp: targetTimestamp },
  })
  return res.data
}

export async function resetModelRatios() {
  const res = await api.post<UpdateOptionResponse>(
    '/api/option/rest_model_ratio'
  )
  return res.data
}

export async function getRoutingStrategySnapshots() {
  const res = await api.get<RoutingStrategySnapshotResponse>(
    '/api/option/routing-strategies'
  )
  return res.data
}

export async function refreshRoutingStrategySnapshots() {
  const res = await api.post<RoutingStrategyRefreshResponse>(
    '/api/option/routing-strategies/refresh'
  )
  return res.data
}

export async function saveRoutingStrategyOverride(request: {
  strategy: string
  user_group: string
  groups: string[]
  manual_override: boolean
}) {
  const res = await api.put<UpdateOptionResponse>(
    '/api/option/routing-strategies/override',
    request
  )
  return res.data
}

export async function getUpstreamChannels() {
  const res = await api.get<UpstreamChannelsResponse>(
    '/api/ratio_sync/channels'
  )
  return res.data
}

export async function fetchUpstreamRatios(request: FetchUpstreamRatiosRequest) {
  const res = await api.post<UpstreamRatiosResponse>(
    '/api/ratio_sync/fetch',
    request
  )
  return res.data
}
