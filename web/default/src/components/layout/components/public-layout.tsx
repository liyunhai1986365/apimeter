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
import { useNotifications } from '@/hooks/use-notifications'
import { SystemNoticeBanner } from '@/components/system-notice-banner'
import {
  InvitePromoBanner,
  useInvitePromoBanner,
} from '@/features/invite/components/invite-promo-banner'
import type { TopNavLink } from '../types'
import { PublicHeader, type PublicHeaderProps } from './public-header'

type PublicLayoutProps = {
  children: React.ReactNode
  showMainContainer?: boolean
  navContent?: React.ReactNode
  headerProps?: Omit<PublicHeaderProps, 'navContent' | 'notifications'>
  navLinks?: TopNavLink[]
  showThemeSwitch?: boolean
  showAuthButtons?: boolean
  showNotifications?: boolean
  logo?: React.ReactNode
  siteName?: string
}

export function PublicLayout(props: PublicLayoutProps) {
  const invitePromoBanner = useInvitePromoBanner()
  const notifications = useNotifications()
  const showSystemNoticeBanner =
    props.showNotifications !== false &&
    !notifications.isNoticeClosed &&
    notifications.notice.trim() !== ''
  const showInvitePromoBanner =
    invitePromoBanner.visible && !showSystemNoticeBanner

  return (
    <div
      className='bg-background text-foreground relative min-h-svh overflow-x-clip'
      style={
        {
          '--app-header-height': '4rem',
          '--invite-promo-banner-height': showInvitePromoBanner
            ? '2.5rem'
            : '0px',
          '--system-notice-banner-height': showSystemNoticeBanner
            ? '2.5rem'
            : '0px',
        } as React.CSSProperties
      }
    >
      <InvitePromoBanner
        onDismiss={invitePromoBanner.dismiss}
        visible={showInvitePromoBanner}
      />
      <SystemNoticeBanner
        notice={notifications.notice}
        hidden={!showSystemNoticeBanner}
        fixed
        topClassName='top-[var(--invite-promo-banner-height,0px)]'
        onCloseToday={notifications.closeToday}
      />
      <PublicHeader
        navContent={props.navContent}
        navLinks={props.navLinks}
        showThemeSwitch={props.showThemeSwitch}
        showAuthButtons={props.showAuthButtons}
        showNotifications={props.showNotifications}
        logo={props.logo}
        siteName={props.siteName}
        notifications={notifications}
        {...props.headerProps}
      />

      {props.showMainContainer !== false ? (
        <main className='container px-4 py-6 pt-[calc(var(--app-header-height)+var(--invite-promo-banner-height)+var(--system-notice-banner-height)+1rem)] md:px-4'>
          {props.children}
        </main>
      ) : (
        props.children
      )}
    </div>
  )
}
