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
  ApiResponse,
  ConfigureItem,
  ConfiguredChannel,
  NewAPISupplierBalanceResult,
  NewAPISupplier,
  NewAPISupplierCheckResult,
  PagedResponse,
  TestModelRequest,
  TestModelResult,
} from './types'

export async function listNewAPISuppliers(params?: {
  p?: number
  page_size?: number
}): Promise<PagedResponse<NewAPISupplier>> {
  const res = await api.get('/api/newapi_suppliers/', { params })
  return res.data
}

export async function createNewAPISupplier(
  data: Partial<NewAPISupplier>
): Promise<ApiResponse<NewAPISupplier>> {
  const res = await api.post('/api/newapi_suppliers/', data)
  return res.data
}

export async function updateNewAPISupplier(
  data: Partial<NewAPISupplier> & { id: number }
): Promise<ApiResponse<NewAPISupplier>> {
  const res = await api.put('/api/newapi_suppliers/', data)
  return res.data
}

export async function deleteNewAPISupplier(
  id: number
): Promise<ApiResponse<null>> {
  const res = await api.delete(`/api/newapi_suppliers/${id}`)
  return res.data
}

export async function checkNewAPISupplier(
  id: number
): Promise<ApiResponse<NewAPISupplierCheckResult>> {
  const res = await api.post(`/api/newapi_suppliers/${id}/check`)
  return res.data
}

export async function queryNewAPISupplierBalance(
  id: number
): Promise<ApiResponse<NewAPISupplierBalanceResult>> {
  const res = await api.post(`/api/newapi_suppliers/${id}/balance`)
  return res.data
}

export async function configureNewAPISupplierChannels(
  id: number,
  items: ConfigureItem[]
): Promise<ApiResponse<ConfiguredChannel[]>> {
  const res = await api.post(`/api/newapi_suppliers/${id}/configure_channels`, {
    items,
  })
  return res.data
}

export async function testNewAPISupplierModel(
  id: number,
  data: TestModelRequest
): Promise<TestModelResult> {
  const res = await api.post(`/api/newapi_suppliers/${id}/test_model`, data)
  return res.data
}

export async function getLocalGroups(): Promise<ApiResponse<string[]>> {
  const res = await api.get('/api/group/')
  return res.data
}
