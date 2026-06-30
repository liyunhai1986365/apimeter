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
import { useState } from 'react'
import { ApiKeyWorkspaceCreateDialog } from './api-key-workspace-create-dialog'
import { ApiKeyWorkspaceSettingsDialog } from './api-key-workspace-settings-dialog'
import { ApiKeyOnboardingDialog } from './api-key-onboarding-dialog'
import { ApiKeysDeleteDialog } from './api-keys-delete-dialog'
import { ApiKeysMutateDrawer } from './api-keys-mutate-drawer'
import { useApiKeys } from './api-keys-provider'
import { CCSwitchDialog } from './dialogs/cc-switch-dialog'

export function ApiKeysDialogs() {
  const { open, setOpen, currentRow, resolvedKey } = useApiKeys()
  const [createdApiKey, setCreatedApiKey] = useState<{
    key: string
    name: string
  } | null>(null)
  const mutateSide = open === 'create' ? 'left' : 'right'

  return (
    <>
      <ApiKeysMutateDrawer
        open={open === 'create' || open === 'update'}
        onOpenChange={(isOpen) => !isOpen && setOpen(null)}
        currentRow={open === 'update' ? currentRow || undefined : undefined}
        side={mutateSide}
        onApiKeyCreated={setCreatedApiKey}
      />
      <ApiKeyOnboardingDialog
        open={!!createdApiKey}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) setCreatedApiKey(null)
        }}
        apiKey={createdApiKey?.key || ''}
        apiKeyName={createdApiKey?.name}
      />
      <ApiKeysDeleteDialog />
      <ApiKeyWorkspaceCreateDialog
        open={open === 'workspace-create'}
        onOpenChange={(isOpen) => !isOpen && setOpen(null)}
      />
      <ApiKeyWorkspaceSettingsDialog
        open={open === 'workspace-settings'}
        onOpenChange={(isOpen) => !isOpen && setOpen(null)}
      />
      <CCSwitchDialog
        open={open === 'cc-switch'}
        onOpenChange={(isOpen) => !isOpen && setOpen(null)}
        tokenKey={resolvedKey}
      />
    </>
  )
}
