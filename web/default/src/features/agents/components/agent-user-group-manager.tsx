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
import { Edit3, Save, Users } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { TableEmpty } from '@/components/data-table'
import type { AgentGroupRatio, AgentUserGroupConfig } from '../types'

type AgentUserGroupManagerProps = {
  userGroups: AgentUserGroupConfig[]
  groupRatios: AgentGroupRatio[]
  groupName: string
  visibleGroups: string[]
  canSave: boolean
  isPending: boolean
  onGroupNameChange: (value: string) => void
  onVisibleGroupsChange: (value: string[]) => void
  onSave: () => void
  onEdit: (rule: AgentUserGroupConfig) => void
}

export function AgentUserGroupManager(props: AgentUserGroupManagerProps) {
  const { t } = useTranslation()
  const availableSelectionGroups = useMemo(
    () =>
      props.groupRatios.filter(
        (item) => item.configured && item.available && item.group_name
      ),
    [props.groupRatios]
  )

  const toggleVisibleGroup = (groupName: string) => {
    if (props.visibleGroups.includes(groupName)) {
      props.onVisibleGroupsChange(
        props.visibleGroups.filter((item) => item !== groupName)
      )
      return
    }
    props.onVisibleGroupsChange([...props.visibleGroups, groupName].sort())
  }

  return (
    <section className='rounded-lg border p-3'>
      <div className='mb-4 flex flex-col gap-2 md:flex-row md:items-start md:justify-between'>
        <div>
          <h3 className='text-sm font-semibold'>{t('User Groups')}</h3>
          <p className='text-muted-foreground mt-1 max-w-2xl text-xs'>
            {t(
              'User groups are only used when assigning agent users. Appended groups expand the selectable model groups for members.'
            )}
          </p>
        </div>
        <div className='rounded-md border px-3 py-2 text-right text-xs'>
          <div className='text-muted-foreground whitespace-nowrap text-[11px]'>
            {t('Configured')}
          </div>
          <div className='text-sm font-semibold'>{props.userGroups.length}</div>
        </div>
      </div>

      <div className='grid gap-4 xl:grid-cols-[320px_minmax(0,1fr)]'>
        <div className='flex flex-col gap-3 xl:border-r xl:pr-4'>
          <div className='grid gap-1.5'>
            <span className='text-muted-foreground text-xs'>
              {t('User group name')}
            </span>
            <Input
              value={props.groupName}
              onChange={(event) => props.onGroupNameChange(event.target.value)}
              placeholder={t('User group name')}
            />
          </div>

          <div className='grid gap-2 rounded-md bg-muted/40 px-3 py-2'>
            <div>
              <div className='text-xs font-medium'>
                {t('Appended selectable groups')}
              </div>
              <div className='text-muted-foreground text-[11px]'>
                {t(
                  'These groups are appended to the selectable model groups for users in this user group.'
                )}
              </div>
            </div>
            <div className='flex max-h-36 flex-wrap gap-2 overflow-y-auto'>
              {availableSelectionGroups.length === 0 ? (
                <span className='text-muted-foreground text-xs'>
                  {t('No selectable groups configured')}
                </span>
              ) : (
                availableSelectionGroups.map((item) => (
                  <button
                    key={item.group_name}
                    type='button'
                    className='data-[selected=true]:bg-primary data-[selected=true]:text-primary-foreground rounded-md border px-2 py-1 text-xs'
                    data-selected={props.visibleGroups.includes(
                      item.group_name
                    )}
                    onClick={() => toggleVisibleGroup(item.group_name)}
                  >
                    {item.group_name}
                  </button>
                ))
              )}
            </div>
          </div>

          <Button
            className='w-full'
            disabled={!props.canSave || props.isPending}
            onClick={props.onSave}
          >
            <Save />
            {t('Save User Group')}
          </Button>
        </div>

        <div className='min-w-0'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('User Group')}</TableHead>
                <TableHead>{t('Appended groups')}</TableHead>
                <TableHead>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {props.userGroups.length === 0 ? (
                <TableEmpty
                  colSpan={3}
                  title={t('No User Groups')}
                  description={t(
                    'Create user groups before assigning agent users.'
                  )}
                  icon={<Users className='size-6' />}
                />
              ) : (
                props.userGroups.map((rule) => (
                  <TableRow key={rule.group_name}>
                    <TableCell className='font-mono text-xs'>
                      {rule.group_name}
                    </TableCell>
                    <TableCell>
                      <div className='text-muted-foreground max-w-[260px] truncate text-xs'>
                        {rule.visible_groups && rule.visible_groups.length > 0
                          ? rule.visible_groups.join(', ')
                          : t('No appended groups')}
                      </div>
                    </TableCell>
                    <TableCell>
                      <Button
                        variant='ghost'
                        size='sm'
                        onClick={() => props.onEdit(rule)}
                      >
                        <Edit3 className='size-4' />
                        {t('Edit')}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
      </div>
    </section>
  )
}
