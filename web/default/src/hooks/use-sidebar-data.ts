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
import {
  LayoutDashboard,
  Activity,
  Key,
  FileText,
  Wallet,
  Box,
  Users,
  Ticket,
  User,
  Command,
  Radio,
  FlaskConical,
  MessageSquare,
  CreditCard,
  BadgeDollarSign,
  ReceiptText,
  ListTodo,
  Settings2,
  Settings,
  Store,
  Network,
  Handshake,
  Monitor,
  ChartNoAxesCombined,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { WORKSPACE_IDS } from '@/components/layout/lib/workspace-registry'
import { type SidebarData } from '@/components/layout/types'

type SidebarUser =
  | {
      has_agent?: boolean
      permissions?: {
        agent_console?: boolean
      }
    }
  | null
  | undefined

export function buildSidebarData(
  t: (key: string) => string,
  user: SidebarUser
): SidebarData {
  const canUseAgentConsole = Boolean(
    user &&
    (user.has_agent === true || user.permissions?.agent_console === true)
  )

  return {
    workspaces: [
      {
        id: WORKSPACE_IDS.DEFAULT,
        name: '', // Dynamically fetches system name
        logo: Command,
        plan: '', // Dynamically fetches system version
      },
    ],
    navGroups: [
      {
        id: 'chat',
        title: t('Chat'),
        items: [
          {
            title: t('Playground'),
            url: '/playground',
            icon: FlaskConical,
          },
          {
            title: t('Chat'),
            icon: MessageSquare,
            type: 'chat-presets',
          },
        ],
      },
      {
        id: 'general',
        title: t('General'),
        items: [
          {
            title: t('Overview'),
            url: '/dashboard/overview',
            icon: Activity,
          },
          {
            title: t('Dashboard'),
            url: '/dashboard/models',
            icon: LayoutDashboard,
          },
          {
            title: t('API Keys'),
            url: '/keys',
            icon: Key,
          },
          {
            title: t('Usage Logs'),
            url: '/usage-logs/common',
            icon: FileText,
          },
          {
            title: t('Task Logs'),
            url: '/usage-logs/task',
            activeUrls: ['/usage-logs/drawing'],
            configUrls: ['/usage-logs/drawing', '/usage-logs/task'],
            icon: ListTodo,
          },
        ],
      },
      {
        id: 'personal',
        title: t('Personal'),
        items: [
          {
            title: t('Wallet'),
            url: '/wallet',
            icon: Wallet,
          },
          {
            title: t('Billing'),
            url: '/billing/monthly',
            activeUrls: ['/billing'],
            icon: ReceiptText,
          },
          {
            title: t('Subscription'),
            url: '/user-subscription',
            icon: BadgeDollarSign,
          },
          {
            title: t('Suppliers'),
            url: '/provider',
            icon: Handshake,
          },
          {
            title: t('Profile'),
            url: '/profile',
            icon: User,
          },
          ...(canUseAgentConsole
            ? [
                {
                  title: t('Agent Console'),
                  url: '/agents',
                  icon: Store,
                },
              ]
            : []),
        ],
      },
      {
        id: 'admin',
        title: t('Admin'),
        items: [
          {
            title: t('Channels'),
            url: '/channels',
            icon: Radio,
          },
          {
            title: t('Supplier Management'),
            url: '/suppliers',
            icon: Network,
          },
          {
            title: t('Models'),
            url: '/models/metadata',
            icon: Box,
          },
          {
            title: t('Model Monitor'),
            url: '/model-monitor',
            icon: Monitor,
          },
          {
            title: t('Model Profit'),
            url: '/model-profit',
            icon: ChartNoAxesCombined,
          },
          {
            title: t('Operation Records'),
            url: '/usage-logs/channel-operations',
            icon: FileText,
          },
          {
            title: t('Users'),
            url: '/users',
            icon: Users,
          },
          {
            title: t('Agent Management'),
            url: '/agent-management',
            icon: Settings2,
          },
          {
            title: t('Redemption Codes'),
            url: '/redemption-codes',
            icon: Ticket,
          },
          {
            title: t('Subscription Management'),
            url: '/subscriptions',
            icon: CreditCard,
          },
          {
            title: t('System Settings'),
            url: '/system-settings/site',
            activeUrls: ['/system-settings'],
            icon: Settings,
          },
        ],
      },
    ],
  }
}

export function useSidebarData(): SidebarData {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)

  return buildSidebarData(t, user)
}
