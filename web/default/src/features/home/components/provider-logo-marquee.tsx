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
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'
import type { LogoRailProvider } from '../landing-data'

interface ProviderLogoMarqueeProps {
  providers: LogoRailProvider[]
}

function LogoRailItem(props: { provider: LogoRailProvider }) {
  return (
    <div className='flex min-w-max items-center gap-3 px-5 py-2'>
      <span className='flex size-10 shrink-0 items-center justify-center'>
        {getLobeIcon(props.provider.iconName, 36)}
      </span>
      <span className='text-foreground text-lg font-semibold tracking-tight'>
        {props.provider.name}
      </span>
    </div>
  )
}

function LogoRail(props: {
  providers: LogoRailProvider[]
  reverse?: boolean
  className?: string
}) {
  const railItems = [...props.providers, ...props.providers]

  return (
    <div
      className={cn(
        'home-logo-marquee overflow-hidden',
        '[-webkit-mask-image:linear-gradient(to_right,transparent,black_12%,black_88%,transparent)] [mask-image:linear-gradient(to_right,transparent,black_12%,black_88%,transparent)]',
        props.className
      )}
    >
      <div
        className={cn(
          'flex w-max items-center gap-8',
          props.reverse
            ? 'animate-logo-marquee-reverse'
            : 'animate-logo-marquee'
        )}
      >
        {railItems.map((provider, index) => (
          <LogoRailItem
            key={`${provider.id}-${props.reverse ? 'reverse' : 'forward'}-${index}`}
            provider={provider}
          />
        ))}
      </div>
    </div>
  )
}

export function ProviderLogoMarquee(props: ProviderLogoMarqueeProps) {
  const firstRow = props.providers.filter((_, index) => index % 2 === 0)
  const secondRow = props.providers.filter((_, index) => index % 2 === 1)

  return (
    <div className='mx-auto w-full max-w-5xl space-y-5' aria-hidden='true'>
      <LogoRail providers={firstRow} />
      <LogoRail
        providers={secondRow}
        reverse
        className='mx-auto max-w-[92%]'
      />
    </div>
  )
}
