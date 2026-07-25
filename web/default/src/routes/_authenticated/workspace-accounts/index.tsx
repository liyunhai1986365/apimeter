import { createFileRoute, redirect } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { WorkspaceSubaccounts } from '@/features/workspace-subaccounts'

export const Route = createFileRoute('/_authenticated/workspace-accounts/')({
  beforeLoad: () => {
    if (useAuthStore.getState().auth.user?.workspace_subaccount) {
      throw redirect({ to: '/keys' })
    }
  },
  component: WorkspaceSubaccounts,
})
