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
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { GroupBadge } from '@/components/group-badge'

export type PricingUsableGroupMap = Record<
  string,
  string | { desc?: string; ratio?: number }
>

function useTranslatedGroupDescription(desc?: string) {
  const { t } = useTranslation()
  return desc ? t(desc) : undefined
}

export function getUsableGroupDescription(
  usableGroup: PricingUsableGroupMap | undefined,
  group: string
) {
  const info = usableGroup?.[group]
  if (typeof info === 'string') return info
  return info?.desc
}

export function ModelDetailsGroupName(props: {
  group: string
  desc?: string
  usableGroup?: PricingUsableGroupMap
  className?: string
}) {
  const translatedDesc = useTranslatedGroupDescription(
    props.desc ?? getUsableGroupDescription(props.usableGroup, props.group)
  )
  const badge = (
    <span className={cn('inline-flex w-fit', props.className)}>
      <GroupBadge group={props.group} size='sm' />
    </span>
  )

  if (!translatedDesc) return badge

  return (
    <TooltipProvider delay={120}>
      <Tooltip>
        <TooltipTrigger render={badge} />
        <TooltipContent side='top' className='max-w-72 leading-relaxed'>
          {translatedDesc}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
