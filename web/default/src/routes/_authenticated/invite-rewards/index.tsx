import { createFileRoute, notFound } from '@tanstack/react-router'
import { isCurrentAgentSite } from '@/lib/agent-site-access'
import { InviteRewards } from '@/features/invite-rewards'

export const Route = createFileRoute('/_authenticated/invite-rewards/')({
  beforeLoad: async () => {
    if (await isCurrentAgentSite()) throw notFound()
  },
  component: InviteRewards,
})
