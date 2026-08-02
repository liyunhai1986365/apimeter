/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { getLobeIcon } from '@/lib/lobe-icon'

export function ProviderLogo(props: {
  name: string
  icon?: string
  size?: number
}) {
  const icon = props.icon ? getLobeIcon(props.icon, props.size ?? 28) : null

  return (
    <div className='bg-muted flex size-12 shrink-0 items-center justify-center rounded-xl'>
      {icon || (
        <span className='text-lg font-semibold'>
          {props.name.charAt(0).toUpperCase() || '?'}
        </span>
      )}
    </div>
  )
}
