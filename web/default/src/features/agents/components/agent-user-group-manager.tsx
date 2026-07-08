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
import { useMemo, useState } from 'react'
import { Edit3, Plus, RotateCcw, Save, Users } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
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
import { getAgentUserGroupRatioFloor } from '../api'
import type { AgentGroupRatio, AgentUserGroupConfig } from '../types'

type AgentUserGroupManagerProps = {
  userGroups: AgentUserGroupConfig[]
  groupRatios: AgentGroupRatio[]
  groupName: string
  visibleGroups: string[]
  groupRatioOverrides: Record<string, number>
  canSave: boolean
  isPending: boolean
  onGroupNameChange: (value: string) => void
  onVisibleGroupsChange: (value: string[]) => void
  onGroupRatioOverridesChange: (value: Record<string, number>) => void
  onSave: () => void
  onEdit: (rule: AgentUserGroupConfig) => void
  onResetForm: () => void
}

export function AgentUserGroupManager(props: AgentUserGroupManagerProps) {
  const { t } = useTranslation()
  const [editorOpen, setEditorOpen] = useState(false)
  const availableSelectionGroups = useMemo(
    () => props.groupRatios.filter((item) => item.available && item.group_name),
    [props.groupRatios]
  )
  const groupRatioDefaults = useMemo(() => {
    const defaults = new Map<string, number>()
    for (const item of availableSelectionGroups) {
      defaults.set(item.group_name, getAgentUserGroupRatioFloor(item))
    }
    return defaults
  }, [availableSelectionGroups])

  const toggleVisibleGroup = (groupName: string) => {
    if (props.visibleGroups.includes(groupName)) {
      props.onVisibleGroupsChange(
        props.visibleGroups.filter((item) => item !== groupName)
      )
      const nextRatios = { ...props.groupRatioOverrides }
      delete nextRatios[groupName]
      props.onGroupRatioOverridesChange(nextRatios)
      return
    }
    props.onVisibleGroupsChange([...props.visibleGroups, groupName].sort())
    props.onGroupRatioOverridesChange({
      ...props.groupRatioOverrides,
      [groupName]: groupRatioDefaults.get(groupName) ?? 1,
    })
  }

  const updateGroupRatio = (groupName: string, value: string) => {
    const nextRatios = { ...props.groupRatioOverrides }
    const ratio = Number(value)
    if (!Number.isFinite(ratio) || ratio < 0) {
      delete nextRatios[groupName]
    } else {
      nextRatios[groupName] = Math.max(
        ratio,
        groupRatioDefaults.get(groupName) ?? 0
      )
    }
    props.onGroupRatioOverridesChange(nextRatios)
  }

  const openCreateDialog = () => {
    props.onResetForm()
    setEditorOpen(true)
  }

  const openEditDialog = (rule: AgentUserGroupConfig) => {
    props.onEdit(rule)
    setEditorOpen(true)
  }

  const editingExistingGroup = props.userGroups.some(
    (rule) => rule.group_name === props.groupName
  )

  return (
    <Card size='sm'>
      <CardHeader>
        <CardTitle>{t('User Groups')}</CardTitle>
        <CardDescription>
          <span className='max-w-2xl'>
            {t(
              'Configure user groups to override system groups, show different groups to different users, and set flexible discounts.'
            )}
          </span>
        </CardDescription>
        <CardAction>
          <div className='flex items-center gap-2'>
            <div className='rounded-md border px-3 py-2 text-right text-xs'>
              <div className='text-muted-foreground text-[11px] whitespace-nowrap'>
                {t('Configured')}
              </div>
              <div className='text-sm font-semibold'>
                {props.userGroups.length}
              </div>
            </div>
            <Button type='button' onClick={openCreateDialog}>
              <Plus />
              {t('Create User Group')}
            </Button>
          </div>
        </CardAction>
      </CardHeader>

      <CardContent>
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
                    <div className='text-muted-foreground max-w-[360px] truncate text-xs'>
                      {rule.visible_groups && rule.visible_groups.length > 0
                        ? rule.visible_groups.join(', ')
                        : t('No appended groups')}
                    </div>
                    {rule.group_ratios &&
                    Object.keys(rule.group_ratios).length > 0 ? (
                      <div className='text-muted-foreground mt-1 max-w-[360px] truncate text-[11px]'>
                        {Object.entries(rule.group_ratios)
                          .map(([group, ratio]) => `${group}: ${ratio}`)
                          .join(', ')}
                      </div>
                    ) : null}
                  </TableCell>
                  <TableCell>
                    <Button
                      variant='ghost'
                      size='sm'
                      onClick={() => openEditDialog(rule)}
                    >
                      <Edit3 />
                      {t('Edit')}
                    </Button>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </CardContent>

      <Dialog open={editorOpen} onOpenChange={setEditorOpen}>
        <DialogContent className='sm:max-w-lg'>
          <DialogHeader>
            <DialogTitle>
              {editingExistingGroup
                ? t('Edit User Group')
                : t('Create User Group')}
            </DialogTitle>
            <DialogDescription>
              {t(
                'Configure which agent system groups this user group can select.'
              )}
            </DialogDescription>
          </DialogHeader>

          <FieldGroup>
            <Field>
              <FieldLabel>{t('User group name')}</FieldLabel>
              <Input
                value={props.groupName}
                onChange={(event) =>
                  props.onGroupNameChange(event.target.value)
                }
                placeholder={t('User group name')}
              />
            </Field>

            <Field>
              <FieldLabel>{t('Appended selectable groups')}</FieldLabel>
              <FieldDescription>
                {t(
                  'Select system groups this user group can access or override.'
                )}
              </FieldDescription>
              <div className='bg-muted/40 flex max-h-36 flex-wrap gap-2 overflow-y-auto rounded-md px-3 py-2'>
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
            </Field>

            {props.visibleGroups.length > 0 ? (
              <Field>
                <FieldLabel>{t('User group ratios')}</FieldLabel>
                <FieldDescription>
                  {t(
                    'Optional ratios for appended groups. Empty values use the agent group default.'
                  )}
                </FieldDescription>
                <div className='grid max-h-44 gap-2 overflow-y-auto rounded-md border px-3 py-2'>
                  {props.visibleGroups.map((groupName) => (
                    <label
                      key={groupName}
                      className='grid grid-cols-[minmax(0,1fr)_90px] items-center gap-2 text-xs'
                    >
                      <span className='truncate font-mono'>{groupName}</span>
                      <Input
                        value={
                          props.groupRatioOverrides[groupName]?.toString() ?? ''
                        }
                        onChange={(event) =>
                          updateGroupRatio(groupName, event.target.value)
                        }
                        type='number'
                        step='0.01'
                        min={groupRatioDefaults.get(groupName) ?? 0}
                        placeholder={t('Default')}
                      />
                    </label>
                  ))}
                </div>
              </Field>
            ) : null}
          </FieldGroup>

          <DialogFooter>
            <Button
              variant='outline'
              disabled={props.isPending}
              onClick={props.onResetForm}
            >
              <RotateCcw />
              {t('Reset')}
            </Button>
            <Button
              disabled={!props.canSave || props.isPending}
              onClick={props.onSave}
            >
              <Save />
              {editingExistingGroup
                ? t('Update User Group')
                : t('Save User Group')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}
