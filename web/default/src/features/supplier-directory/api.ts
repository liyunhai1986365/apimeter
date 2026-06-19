import { api } from '@/lib/api'
import type { Channel } from '@/features/channels/types'

export type UserOwnedProviderPayload = {
  mode: 'single'
  channel: Partial<Channel> & {
    type: number
    key: string
    name: string
    models: string
  }
}

export async function getUserOwnedProviders(): Promise<{
  success: boolean
  message?: string
  data?: Channel[]
}> {
  const res = await api.get('/api/user/self/providers')
  return res.data
}

export async function createUserOwnedProvider(
  payload: UserOwnedProviderPayload
): Promise<{ success: boolean; message?: string; data?: Channel }> {
  const res = await api.post('/api/user/self/providers', payload)
  return res.data
}

export async function updateUserOwnedProvider(
  id: number,
  payload: UserOwnedProviderPayload
): Promise<{ success: boolean; message?: string; data?: Channel }> {
  const res = await api.put(`/api/user/self/providers/${id}`, payload)
  return res.data
}

export async function deleteUserOwnedProvider(
  id: number
): Promise<{ success: boolean; message?: string }> {
  const res = await api.delete(`/api/user/self/providers/${id}`)
  return res.data
}
