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
import { useState, useMemo, useEffect, useCallback, memo } from 'react'
import {
  ArrowDown,
  ArrowUp,
  ChevronDown,
  GripVertical,
  Pencil,
  Plus,
  Trash2,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { safeJsonParse } from '../utils/json-parser'

type GroupRatioVisualEditorProps = {
  groupRatio: string
  topupGroupRatio: string
  userUsableGroups: string
  groupGroupRatio: string
  autoGroups: string
  groupDisplayConfig: string
  onChange: (field: string, value: string) => void
}

type SimpleGroup = {
  name: string
  value: string
}

type GroupPricingRow = {
  _id: string
  name: string
  ratio: number
  selectable: boolean
  description: string
  categoryId: string
  order: number
}

type GroupDisplayCategory = {
  id: string
  name: string
  order: number
}

type GroupDisplayGroup = {
  group: string
  category_id: string
  order: number
}

type GroupDisplayConfig = {
  categories: GroupDisplayCategory[]
  groups: GroupDisplayGroup[]
}

type GroupOverride = {
  targetGroup: string
  ratio: number
}

const sectionCardClassName =
  'relative shadow-sm ring-0 before:pointer-events-none before:absolute before:inset-0 before:rounded-xl before:border before:border-border/90'
const sectionHeaderClassName = 'border-b bg-muted/20'

let groupPricingIdCounter = 0
function createGroupPricingId() {
  groupPricingIdCounter += 1
  return `gpr_${groupPricingIdCounter}`
}

function normalizeRatio(value: unknown): number {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : 1
}

function buildGroupPricingRows(
  groupRatio: string,
  userUsableGroups: string,
  groupDisplayConfig: string
): GroupPricingRow[] {
  const ratioMap = safeJsonParse<Record<string, number>>(groupRatio, {
    fallback: {},
    context: 'group ratios',
  })
  const usableMap = safeJsonParse<Record<string, string>>(userUsableGroups, {
    fallback: {},
    context: 'user usable groups',
  })
  const displayConfig = parseGroupDisplayConfig(groupDisplayConfig)
  const displayGroupMap = new Map(
    displayConfig.groups.map((item) => [item.group, item])
  )
  const categoryOrder = buildCategoryOrderMap(displayConfig.categories)
  const names = new Set([...Object.keys(ratioMap), ...Object.keys(usableMap)])

  return Array.from(names)
    .map((name, index) => {
      const display = displayGroupMap.get(name)
      return {
        _id: createGroupPricingId(),
        name,
        ratio: normalizeRatio(ratioMap[name]),
        selectable: Object.prototype.hasOwnProperty.call(usableMap, name),
        description: String(usableMap[name] ?? ''),
        categoryId: display?.category_id ?? '',
        order: display?.order ?? (index + 1) * 10,
      }
    })
    .sort((a, b) => compareGroupPricingRows(a, b, categoryOrder))
}

function parseGroupDisplayConfig(value: string): GroupDisplayConfig {
  const parsed = safeJsonParse<GroupDisplayConfig>(value, {
    fallback: { categories: [], groups: [] },
    context: 'group display config',
  })
  return normalizeGroupDisplayConfig(parsed)
}

function normalizeGroupDisplayConfig(
  config: Partial<GroupDisplayConfig> | null | undefined
): GroupDisplayConfig {
  const seenCategories = new Set<string>()
  const categories = (
    Array.isArray(config?.categories) ? config?.categories : []
  )
    .map((category) => ({
      id: String(category.id ?? '').trim(),
      name: String(category.name ?? '').trim(),
      order: normalizeDisplayOrder(category.order),
    }))
    .filter((category) => {
      if (!category.id || seenCategories.has(category.id)) return false
      seenCategories.add(category.id)
      return true
    })
    .map((category) => ({
      ...category,
      name: category.name || category.id,
    }))
    .sort(compareCategories)

  const categoryIds = new Set(categories.map((category) => category.id))
  const seenGroups = new Set<string>()
  const groups = (Array.isArray(config?.groups) ? config?.groups : [])
    .map((group) => ({
      group: String(group.group ?? '').trim(),
      category_id: String(group.category_id ?? '').trim(),
      order: normalizeDisplayOrder(group.order),
    }))
    .filter((group) => {
      if (!group.group || seenGroups.has(group.group)) return false
      seenGroups.add(group.group)
      return true
    })
    .map((group) => ({
      ...group,
      category_id: categoryIds.has(group.category_id) ? group.category_id : '',
    }))

  const categoryOrder = buildCategoryOrderMap(categories)
  groups.sort((a, b) =>
    compareGroupPricingRows(
      {
        _id: '',
        name: a.group,
        ratio: 1,
        selectable: true,
        description: '',
        categoryId: a.category_id,
        order: a.order,
      },
      {
        _id: '',
        name: b.group,
        ratio: 1,
        selectable: true,
        description: '',
        categoryId: b.category_id,
        order: b.order,
      },
      categoryOrder
    )
  )

  return { categories, groups }
}

function serializeGroupPricingRows(
  rows: GroupPricingRow[],
  categories: GroupDisplayCategory[]
) {
  const groupRatio: Record<string, number> = {}
  const userUsableGroups: Record<string, string> = {}
  const normalizedCategories = normalizeGroupDisplayConfig({
    categories,
    groups: [],
  }).categories
  const categoryIds = new Set(normalizedCategories.map((item) => item.id))
  const groups: GroupDisplayGroup[] = []

  for (const [index, row] of rows.entries()) {
    const name = row.name.trim()
    if (!name) continue
    groupRatio[name] = normalizeRatio(row.ratio)
    if (row.selectable) {
      userUsableGroups[name] = row.description
    }
    groups.push({
      group: name,
      category_id: categoryIds.has(row.categoryId) ? row.categoryId : '',
      order: normalizeDisplayOrder(row.order || (index + 1) * 10),
    })
  }

  return {
    GroupRatio: JSON.stringify(groupRatio, null, 2),
    UserUsableGroups: JSON.stringify(userUsableGroups, null, 2),
    GroupDisplayConfig: JSON.stringify(
      normalizeGroupDisplayConfig({
        categories: normalizedCategories,
        groups,
      }),
      null,
      2
    ),
  }
}

function normalizeDisplayOrder(value: unknown): number {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : 0
}

function compareCategories(
  left: GroupDisplayCategory,
  right: GroupDisplayCategory
) {
  if (left.order !== right.order) return left.order - right.order
  if (left.name !== right.name) return left.name.localeCompare(right.name)
  return left.id.localeCompare(right.id)
}

function buildCategoryOrderMap(categories: GroupDisplayCategory[]) {
  return new Map(categories.map((category, index) => [category.id, index]))
}

function compareGroupPricingRows(
  left: GroupPricingRow,
  right: GroupPricingRow,
  categoryOrder: Map<string, number>
) {
  const leftCategoryOrder =
    categoryOrder.get(left.categoryId) ?? Number.MAX_SAFE_INTEGER
  const rightCategoryOrder =
    categoryOrder.get(right.categoryId) ?? Number.MAX_SAFE_INTEGER
  if (leftCategoryOrder !== rightCategoryOrder) {
    return leftCategoryOrder - rightCategoryOrder
  }
  if (left.order !== right.order) return left.order - right.order
  return left.name.localeCompare(right.name)
}

function normalizeRowOrders(rows: GroupPricingRow[]) {
  return rows.map((row, index) => ({ ...row, order: (index + 1) * 10 }))
}

function normalizeCategoryOrders(categories: GroupDisplayCategory[]) {
  return categories.map((category, index) => ({
    ...category,
    order: (index + 1) * 10,
  }))
}

function stableConfigSignature(value: unknown) {
  return JSON.stringify(value)
}

function groupPricingSignature(
  rows: GroupPricingRow[],
  categories: GroupDisplayCategory[]
): string {
  const serialized = serializeGroupPricingRows(rows, categories)
  return JSON.stringify({
    groupRatio: safeJsonParse(serialized.GroupRatio, {
      fallback: {},
      silent: true,
    }),
    userUsableGroups: safeJsonParse(serialized.UserUsableGroups, {
      fallback: {},
      silent: true,
    }),
    groupDisplay: safeJsonParse(serialized.GroupDisplayConfig, {
      fallback: { categories: [], groups: [] },
      silent: true,
    }),
  })
}

function sourceGroupPricingSignature(
  groupRatio: string,
  userUsableGroups: string,
  groupDisplayConfig: string
): string {
  return JSON.stringify({
    groupRatio: safeJsonParse(groupRatio, { fallback: {}, silent: true }),
    userUsableGroups: safeJsonParse(userUsableGroups, {
      fallback: {},
      silent: true,
    }),
    groupDisplay: parseGroupDisplayConfig(groupDisplayConfig),
  })
}

export const GroupRatioVisualEditor = memo(function GroupRatioVisualEditor({
  groupRatio,
  topupGroupRatio,
  userUsableGroups,
  groupGroupRatio,
  autoGroups,
  groupDisplayConfig,
  onChange,
}: GroupRatioVisualEditorProps) {
  const { t } = useTranslation()
  const [simpleDialogOpen, setSimpleDialogOpen] = useState(false)
  const [simpleDialogType, setSimpleDialogType] = useState<
    'groupRatio' | 'topupGroupRatio' | null
  >(null)
  const [simpleEditData, setSimpleEditData] = useState<SimpleGroup | null>(null)

  const [autoGroupDialogOpen, setAutoGroupDialogOpen] = useState(false)
  const [autoGroupInput, setAutoGroupInput] = useState('')

  const [groupOverrideDialogOpen, setGroupOverrideDialogOpen] = useState(false)
  const [groupOverrideUserGroup, setGroupOverrideUserGroup] = useState<
    string | null
  >(null)
  const [groupOverrideEditData, setGroupOverrideEditData] =
    useState<GroupOverride | null>(null)

  const [userGroupDialogOpen, setUserGroupDialogOpen] = useState(false)
  const [userGroupInput, setUserGroupInput] = useState('')

  // Parse topup group ratios
  const topupRatioList = useMemo(() => {
    const map = safeJsonParse<Record<string, number>>(topupGroupRatio, {
      fallback: {},
      context: 'topup group ratios',
    })
    return Object.entries(map).map(([name, value]) => ({
      name,
      value: String(value),
    }))
  }, [topupGroupRatio])

  // Parse auto groups
  const autoGroupsList = useMemo(() => {
    return safeJsonParse<string[]>(autoGroups, {
      fallback: [],
      context: 'auto groups',
    })
  }, [autoGroups])

  // Parse group-group ratios
  const groupGroupRatioList = useMemo(() => {
    const map = safeJsonParse<Record<string, Record<string, number>>>(
      groupGroupRatio,
      {
        fallback: {},
        context: 'group-group ratios',
      }
    )
    return Object.entries(map).map(([userGroup, overrides]) => ({
      userGroup,
      overrides: Object.entries(overrides).map(([targetGroup, ratio]) => ({
        targetGroup,
        ratio,
      })),
    }))
  }, [groupGroupRatio])

  // Simple group handlers (for groupRatio and topupGroupRatio)
  const handleSimpleAdd = (type: 'groupRatio' | 'topupGroupRatio') => {
    setSimpleDialogType(type)
    setSimpleEditData(null)
    setSimpleDialogOpen(true)
  }

  const handleSimpleEdit = (
    type: 'groupRatio' | 'topupGroupRatio',
    group: SimpleGroup
  ) => {
    setSimpleDialogType(type)
    setSimpleEditData(group)
    setSimpleDialogOpen(true)
  }

  const handleSimpleSave = (name: string, value: string) => {
    if (!simpleDialogType) return

    const fieldName =
      simpleDialogType === 'groupRatio' ? groupRatio : topupGroupRatio
    const map = safeJsonParse<Record<string, number>>(fieldName, {
      fallback: {},
      silent: true,
    })

    if (simpleEditData && simpleEditData.name !== name) {
      delete map[simpleEditData.name]
    }

    map[name] = parseFloat(value)

    const field =
      simpleDialogType === 'groupRatio' ? 'GroupRatio' : 'TopupGroupRatio'
    onChange(field, JSON.stringify(map, null, 2))
    setSimpleDialogOpen(false)
  }

  const handleSimpleDelete = (
    type: 'groupRatio' | 'topupGroupRatio',
    name: string
  ) => {
    const fieldName = type === 'groupRatio' ? groupRatio : topupGroupRatio
    const map = safeJsonParse<Record<string, number>>(fieldName, {
      fallback: {},
      silent: true,
    })
    delete map[name]

    const field = type === 'groupRatio' ? 'GroupRatio' : 'TopupGroupRatio'
    onChange(field, JSON.stringify(map, null, 2))
  }

  // Auto groups handlers
  const handleAutoGroupAdd = () => {
    setAutoGroupInput('')
    setAutoGroupDialogOpen(true)
  }

  const handleAutoGroupSave = () => {
    if (!autoGroupInput.trim()) return

    const list = [...autoGroupsList, autoGroupInput.trim()]
    onChange('AutoGroups', JSON.stringify(list, null, 2))
    setAutoGroupDialogOpen(false)
  }

  const handleAutoGroupDelete = (index: number) => {
    const list = autoGroupsList.filter((_, i) => i !== index)
    onChange('AutoGroups', JSON.stringify(list, null, 2))
  }

  const handleAutoGroupMove = (index: number, direction: 'up' | 'down') => {
    const list = [...autoGroupsList]
    const newIndex = direction === 'up' ? index - 1 : index + 1

    if (newIndex < 0 || newIndex >= list.length) return
    ;[list[index], list[newIndex]] = [list[newIndex], list[index]]
    onChange('AutoGroups', JSON.stringify(list, null, 2))
  }

  // Group-group ratio handlers
  const handleUserGroupAdd = () => {
    setUserGroupInput('')
    setUserGroupDialogOpen(true)
  }

  const handleUserGroupSave = () => {
    if (!userGroupInput.trim()) return

    const map = safeJsonParse<Record<string, Record<string, number>>>(
      groupGroupRatio,
      {
        fallback: {},
        silent: true,
      }
    )

    if (!map[userGroupInput.trim()]) {
      map[userGroupInput.trim()] = {}
    }

    onChange('GroupGroupRatio', JSON.stringify(map, null, 2))
    setUserGroupDialogOpen(false)
  }

  const handleUserGroupDelete = (userGroup: string) => {
    const map = safeJsonParse<Record<string, Record<string, number>>>(
      groupGroupRatio,
      {
        fallback: {},
        silent: true,
      }
    )
    delete map[userGroup]
    onChange('GroupGroupRatio', JSON.stringify(map, null, 2))
  }

  const handleOverrideAdd = (userGroup: string) => {
    setGroupOverrideUserGroup(userGroup)
    setGroupOverrideEditData(null)
    setGroupOverrideDialogOpen(true)
  }

  const handleOverrideEdit = (userGroup: string, override: GroupOverride) => {
    setGroupOverrideUserGroup(userGroup)
    setGroupOverrideEditData(override)
    setGroupOverrideDialogOpen(true)
  }

  const handleOverrideSave = (
    targetGroup: string,
    ratio: number,
    oldTargetGroup?: string
  ) => {
    if (!groupOverrideUserGroup) return

    const map = safeJsonParse<Record<string, Record<string, number>>>(
      groupGroupRatio,
      {
        fallback: {},
        silent: true,
      }
    )

    if (!map[groupOverrideUserGroup]) {
      map[groupOverrideUserGroup] = {}
    }

    if (oldTargetGroup && oldTargetGroup !== targetGroup) {
      delete map[groupOverrideUserGroup][oldTargetGroup]
    }

    map[groupOverrideUserGroup][targetGroup] = ratio

    onChange('GroupGroupRatio', JSON.stringify(map, null, 2))
    setGroupOverrideDialogOpen(false)
  }

  const handleOverrideDelete = (userGroup: string, targetGroup: string) => {
    const map = safeJsonParse<Record<string, Record<string, number>>>(
      groupGroupRatio,
      {
        fallback: {},
        silent: true,
      }
    )

    if (map[userGroup]) {
      delete map[userGroup][targetGroup]
      if (Object.keys(map[userGroup]).length === 0) {
        delete map[userGroup]
      }
    }

    onChange('GroupGroupRatio', JSON.stringify(map, null, 2))
  }

  return (
    <div className='space-y-4'>
      <GroupPricingTable
        groupRatio={groupRatio}
        userUsableGroups={userUsableGroups}
        groupDisplayConfig={groupDisplayConfig}
        onChange={onChange}
      />

      {/* Topup Group Ratios */}
      <Card className={sectionCardClassName}>
        <CardHeader className={sectionHeaderClassName}>
          <CardTitle>{t('Top-up group ratios')}</CardTitle>
          <CardDescription>
            {t('Multipliers for recharge pricing based on user groups.')}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className='space-y-4'>
            <Button
              onClick={() => handleSimpleAdd('topupGroupRatio')}
              size='sm'
            >
              <Plus className='mr-2 h-4 w-4' />
              {t('Add group')}
            </Button>
            {topupRatioList.length > 0 && (
              <div className='rounded-md border'>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('Group name')}</TableHead>
                      <TableHead>{t('Multiplier')}</TableHead>
                      <TableHead className='text-right'>
                        {t('Actions')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {topupRatioList.map((group) => (
                      <TableRow key={group.name}>
                        <TableCell className='font-medium'>
                          {group.name}
                        </TableCell>
                        <TableCell>{group.value}</TableCell>
                        <TableCell className='text-right'>
                          <div className='flex justify-end gap-2'>
                            <Button
                              variant='ghost'
                              size='sm'
                              onClick={() =>
                                handleSimpleEdit('topupGroupRatio', group)
                              }
                            >
                              <Pencil className='h-4 w-4' />
                            </Button>
                            <Button
                              variant='ghost'
                              size='sm'
                              onClick={() =>
                                handleSimpleDelete(
                                  'topupGroupRatio',
                                  group.name
                                )
                              }
                            >
                              <Trash2 className='h-4 w-4' />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Inter-group ratio overrides */}
      <Card className={sectionCardClassName}>
        <CardHeader className={sectionHeaderClassName}>
          <CardTitle>{t('Inter-group ratio overrides')}</CardTitle>
          <CardDescription>
            {t(
              'Custom multipliers when specific user groups use specific token groups. Example: VIP users get 0.9x rate when using "edit_this" group tokens.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className='space-y-4'>
            <Button onClick={handleUserGroupAdd} size='sm'>
              <Plus className='mr-2 h-4 w-4' />
              {t('Add user group')}
            </Button>
            {groupGroupRatioList.length > 0 && (
              <div className='space-y-3'>
                {groupGroupRatioList.map((userGroupData) => (
                  <Collapsible key={userGroupData.userGroup}>
                    <div className='rounded-lg border'>
                      <div className='flex items-center justify-between p-4'>
                        <div className='flex items-center gap-2'>
                          <CollapsibleTrigger
                            render={<Button variant='ghost' size='sm' />}
                          >
                            <ChevronDown className='h-4 w-4' />
                          </CollapsibleTrigger>
                          <span className='font-semibold'>
                            {userGroupData.userGroup}
                          </span>
                          <span className='text-muted-foreground text-sm'>
                            {t('{{count}} override', {
                              count: userGroupData.overrides.length,
                            })}
                          </span>
                        </div>
                        <div className='flex gap-2'>
                          <Button
                            variant='ghost'
                            size='sm'
                            onClick={() =>
                              handleOverrideAdd(userGroupData.userGroup)
                            }
                          >
                            <Plus className='h-4 w-4' />
                          </Button>
                          <Button
                            variant='ghost'
                            size='sm'
                            onClick={() =>
                              handleUserGroupDelete(userGroupData.userGroup)
                            }
                          >
                            <Trash2 className='h-4 w-4' />
                          </Button>
                        </div>
                      </div>
                      <CollapsibleContent>
                        {userGroupData.overrides.length > 0 && (
                          <div className='border-t'>
                            <Table>
                              <TableHeader>
                                <TableRow>
                                  <TableHead>{t('Target group')}</TableHead>
                                  <TableHead>{t('Ratio')}</TableHead>
                                  <TableHead className='text-right'>
                                    {t('Actions')}
                                  </TableHead>
                                </TableRow>
                              </TableHeader>
                              <TableBody>
                                {userGroupData.overrides.map((override) => (
                                  <TableRow key={override.targetGroup}>
                                    <TableCell className='font-medium'>
                                      {override.targetGroup}
                                    </TableCell>
                                    <TableCell>{override.ratio}</TableCell>
                                    <TableCell className='text-right'>
                                      <div className='flex justify-end gap-2'>
                                        <Button
                                          variant='ghost'
                                          size='sm'
                                          onClick={() =>
                                            handleOverrideEdit(
                                              userGroupData.userGroup,
                                              override
                                            )
                                          }
                                        >
                                          <Pencil className='h-4 w-4' />
                                        </Button>
                                        <Button
                                          variant='ghost'
                                          size='sm'
                                          onClick={() =>
                                            handleOverrideDelete(
                                              userGroupData.userGroup,
                                              override.targetGroup
                                            )
                                          }
                                        >
                                          <Trash2 className='h-4 w-4' />
                                        </Button>
                                      </div>
                                    </TableCell>
                                  </TableRow>
                                ))}
                              </TableBody>
                            </Table>
                          </div>
                        )}
                      </CollapsibleContent>
                    </div>
                  </Collapsible>
                ))}
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Auto Groups */}
      <Card className={sectionCardClassName}>
        <CardHeader className={sectionHeaderClassName}>
          <CardTitle>{t('Auto assignment order')}</CardTitle>
          <CardDescription>
            {t(
              'Priority order for automatic group assignment. New tokens rotate through this list.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className='space-y-4'>
            <Button onClick={handleAutoGroupAdd} size='sm'>
              <Plus className='mr-2 h-4 w-4' />
              {t('Add group')}
            </Button>
            {autoGroupsList.length > 0 && (
              <div className='space-y-2'>
                {autoGroupsList.map((group, index) => (
                  <div
                    key={index}
                    className='flex items-center gap-2 rounded-md border p-3'
                  >
                    <GripVertical className='text-muted-foreground h-4 w-4' />
                    <span className='flex-1 font-medium'>{group}</span>
                    <div className='flex gap-1'>
                      <Button
                        variant='ghost'
                        size='sm'
                        disabled={index === 0}
                        onClick={() => handleAutoGroupMove(index, 'up')}
                      >
                        ↑
                      </Button>
                      <Button
                        variant='ghost'
                        size='sm'
                        disabled={index === autoGroupsList.length - 1}
                        onClick={() => handleAutoGroupMove(index, 'down')}
                      >
                        ↓
                      </Button>
                      <Button
                        variant='ghost'
                        size='sm'
                        onClick={() => handleAutoGroupDelete(index)}
                      >
                        <Trash2 className='h-4 w-4' />
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Simple Group Dialog */}
      <SimpleGroupDialog
        open={simpleDialogOpen}
        onOpenChange={setSimpleDialogOpen}
        onSave={handleSimpleSave}
        editData={simpleEditData}
        type={simpleDialogType}
      />

      {/* Auto Group Dialog */}
      <Dialog open={autoGroupDialogOpen} onOpenChange={setAutoGroupDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('Add auto group')}</DialogTitle>
            <DialogDescription>
              {t('Add a group identifier to the auto assignment list.')}
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-4 py-4'>
            <div className='space-y-2'>
              <Label>{t('Group identifier')}</Label>
              <Input
                value={autoGroupInput}
                onChange={(e) => setAutoGroupInput(e.target.value)}
                placeholder={t('default')}
              />
            </div>
          </div>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setAutoGroupDialogOpen(false)}
            >
              {t('Cancel')}
            </Button>
            <Button onClick={handleAutoGroupSave}>{t('Add')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* User Group Dialog */}
      <Dialog open={userGroupDialogOpen} onOpenChange={setUserGroupDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('Add user group')}</DialogTitle>
            <DialogDescription>
              {t('Create a new user group to configure ratio overrides for.')}
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-4 py-4'>
            <div className='space-y-2'>
              <Label>{t('User group name')}</Label>
              <Input
                value={userGroupInput}
                onChange={(e) => setUserGroupInput(e.target.value)}
                placeholder={t('vip')}
              />
            </div>
          </div>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setUserGroupDialogOpen(false)}
            >
              {t('Cancel')}
            </Button>
            <Button onClick={handleUserGroupSave}>{t('Add')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Group Override Dialog */}
      <GroupOverrideDialog
        open={groupOverrideDialogOpen}
        onOpenChange={setGroupOverrideDialogOpen}
        onSave={handleOverrideSave}
        editData={groupOverrideEditData}
        userGroup={groupOverrideUserGroup}
      />
    </div>
  )
})

type GroupPricingTableProps = {
  groupRatio: string
  userUsableGroups: string
  groupDisplayConfig: string
  onChange: (field: string, value: string) => void
}

function GroupPricingTable({
  groupRatio,
  userUsableGroups,
  groupDisplayConfig,
  onChange,
}: GroupPricingTableProps) {
  const { t } = useTranslation()
  const [categories, setCategories] = useState<GroupDisplayCategory[]>(
    () => parseGroupDisplayConfig(groupDisplayConfig).categories
  )
  const [rows, setRows] = useState<GroupPricingRow[]>(() =>
    buildGroupPricingRows(groupRatio, userUsableGroups, groupDisplayConfig)
  )

  useEffect(() => {
    const incomingDisplayConfig = parseGroupDisplayConfig(groupDisplayConfig)
    const incomingSignature = sourceGroupPricingSignature(
      groupRatio,
      userUsableGroups,
      groupDisplayConfig
    )
    setCategories((currentCategories) => {
      if (
        stableConfigSignature(currentCategories) ===
        stableConfigSignature(incomingDisplayConfig.categories)
      ) {
        return currentCategories
      }
      return incomingDisplayConfig.categories
    })
    setRows((currentRows) => {
      if (
        groupPricingSignature(currentRows, incomingDisplayConfig.categories) ===
        incomingSignature
      ) {
        return currentRows
      }
      return buildGroupPricingRows(
        groupRatio,
        userUsableGroups,
        groupDisplayConfig
      )
    })
  }, [groupDisplayConfig, groupRatio, userUsableGroups])

  const emitRows = useCallback(
    (
      nextRows: GroupPricingRow[],
      nextCategories: GroupDisplayCategory[] = categories
    ) => {
      const categoryOrder = buildCategoryOrderMap(nextCategories)
      const sortedRows = [...nextRows].sort((a, b) =>
        compareGroupPricingRows(a, b, categoryOrder)
      )
      setRows(sortedRows)
      setCategories(nextCategories)
      const serialized = serializeGroupPricingRows(sortedRows, nextCategories)
      onChange('GroupRatio', serialized.GroupRatio)
      onChange('UserUsableGroups', serialized.UserUsableGroups)
      onChange('GroupDisplayConfig', serialized.GroupDisplayConfig)
    },
    [categories, onChange]
  )

  const updateRow = useCallback(
    (
      id: string,
      field: Exclude<keyof GroupPricingRow, '_id'>,
      value: string | number | boolean
    ) => {
      emitRows(
        rows.map((row) => (row._id === id ? { ...row, [field]: value } : row))
      )
    },
    [emitRows, rows]
  )

  const addRow = useCallback(() => {
    const existingNames = new Set(rows.map((row) => row.name))
    let index = 1
    let name = `group_${index}`
    while (existingNames.has(name)) {
      index += 1
      name = `group_${index}`
    }
    emitRows([
      ...rows,
      {
        _id: createGroupPricingId(),
        name,
        ratio: 1,
        selectable: true,
        description: '',
        categoryId: '',
        order: (rows.length + 1) * 10,
      },
    ])
  }, [emitRows, rows])

  const removeRow = useCallback(
    (id: string) => {
      emitRows(rows.filter((row) => row._id !== id))
    },
    [emitRows, rows]
  )

  const addCategory = useCallback(() => {
    const existingIds = new Set(categories.map((category) => category.id))
    let index = 1
    let id = `category_${index}`
    while (existingIds.has(id)) {
      index += 1
      id = `category_${index}`
    }
    emitRows(rows, [
      ...categories,
      { id, name: t('New category'), order: (categories.length + 1) * 10 },
    ])
  }, [categories, emitRows, rows, t])

  const updateCategory = useCallback(
    (
      oldId: string,
      field: keyof GroupDisplayCategory,
      value: string | number
    ) => {
      const nextCategories = categories.map((category) =>
        category.id === oldId ? { ...category, [field]: value } : category
      )
      const nextRows =
        field === 'id'
          ? rows.map((row) =>
              row.categoryId === oldId
                ? { ...row, categoryId: String(value) }
                : row
            )
          : rows
      emitRows(nextRows, nextCategories)
    },
    [categories, emitRows, rows]
  )

  const removeCategory = useCallback(
    (categoryId: string) => {
      emitRows(
        rows.map((row) =>
          row.categoryId === categoryId ? { ...row, categoryId: '' } : row
        ),
        categories.filter((category) => category.id !== categoryId)
      )
    },
    [categories, emitRows, rows]
  )

  const moveCategory = useCallback(
    (index: number, direction: 'up' | 'down') => {
      const nextIndex = direction === 'up' ? index - 1 : index + 1
      if (nextIndex < 0 || nextIndex >= categories.length) return
      const nextCategories = [...categories]
      ;[nextCategories[index], nextCategories[nextIndex]] = [
        nextCategories[nextIndex],
        nextCategories[index],
      ]
      emitRows(rows, normalizeCategoryOrders(nextCategories))
    },
    [categories, emitRows, rows]
  )

  const moveRow = useCallback(
    (index: number, direction: 'up' | 'down') => {
      const nextIndex = direction === 'up' ? index - 1 : index + 1
      if (nextIndex < 0 || nextIndex >= rows.length) return
      const nextRows = [...rows]
      ;[nextRows[index], nextRows[nextIndex]] = [
        nextRows[nextIndex],
        nextRows[index],
      ]
      emitRows(normalizeRowOrders(nextRows))
    },
    [emitRows, rows]
  )

  const duplicateNames = useMemo(() => {
    const counts = new Map<string, number>()
    for (const row of rows) {
      const name = row.name.trim()
      if (!name) continue
      counts.set(name, (counts.get(name) ?? 0) + 1)
    }
    return Array.from(counts.entries())
      .filter(([, count]) => count > 1)
      .map(([name]) => name)
  }, [rows])

  return (
    <Card className={sectionCardClassName}>
      <CardHeader className={sectionHeaderClassName}>
        <div className='flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between'>
          <div>
            <CardTitle>{t('Pricing groups')}</CardTitle>
            <CardDescription>
              {t(
                'Edit billing ratios and user-selectable groups in one table.'
              )}
            </CardDescription>
          </div>
          <Button onClick={addRow} size='sm' className='sm:self-start'>
            <Plus className='mr-2 h-4 w-4' />
            {t('Add group')}
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        <div className='space-y-5'>
          <div className='bg-muted/15 rounded-lg border p-3'>
            <div className='mb-3 flex items-center justify-between gap-3'>
              <div>
                <div className='text-sm font-medium'>
                  {t('Group categories')}
                </div>
                <p className='text-muted-foreground text-xs'>
                  {t('Manage category names and display order.')}
                </p>
              </div>
              <Button onClick={addCategory} size='sm' variant='outline'>
                <Plus className='mr-2 h-4 w-4' />
                {t('Add category')}
              </Button>
            </div>

            {categories.length === 0 ? (
              <div className='text-muted-foreground rounded-md border border-dashed px-3 py-4 text-center text-sm'>
                {t('No categories yet. Groups without a category appear last.')}
              </div>
            ) : (
              <div className='space-y-2'>
                {categories.map((category, index) => (
                  <div
                    key={`${category.id}-${index}`}
                    className='bg-background grid gap-2 rounded-md border p-2 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]'
                  >
                    <Input
                      value={category.id}
                      placeholder={t('Category ID')}
                      onChange={(event) =>
                        updateCategory(category.id, 'id', event.target.value)
                      }
                    />
                    <Input
                      value={category.name}
                      placeholder={t('Category name')}
                      onChange={(event) =>
                        updateCategory(category.id, 'name', event.target.value)
                      }
                    />
                    <div className='flex items-center justify-end gap-1'>
                      <Button
                        variant='ghost'
                        size='sm'
                        disabled={index === 0}
                        onClick={() => moveCategory(index, 'up')}
                        aria-label={t('Move up')}
                      >
                        <ArrowUp className='h-4 w-4' />
                      </Button>
                      <Button
                        variant='ghost'
                        size='sm'
                        disabled={index === categories.length - 1}
                        onClick={() => moveCategory(index, 'down')}
                        aria-label={t('Move down')}
                      >
                        <ArrowDown className='h-4 w-4' />
                      </Button>
                      <Button
                        variant='ghost'
                        size='sm'
                        onClick={() => removeCategory(category.id)}
                        aria-label={t('Delete category')}
                      >
                        <Trash2 className='h-4 w-4' />
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          <div className='overflow-hidden rounded-md border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className='w-24'>{t('Order')}</TableHead>
                  <TableHead className='min-w-40'>{t('Group name')}</TableHead>
                  <TableHead className='min-w-36'>
                    {t('Display category')}
                  </TableHead>
                  <TableHead className='w-28'>{t('Ratio')}</TableHead>
                  <TableHead className='w-28 text-center'>
                    {t('User selectable')}
                  </TableHead>
                  <TableHead className='min-w-56'>{t('Description')}</TableHead>
                  <TableHead className='w-16 text-right'>
                    {t('Actions')}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={7}
                      className='text-muted-foreground h-20 text-center text-sm'
                    >
                      {t('No groups yet. Add a group to get started.')}
                    </TableCell>
                  </TableRow>
                ) : (
                  rows.map((row) => (
                    <TableRow key={row._id}>
                      <TableCell>
                        <div className='flex items-center gap-1'>
                          <GripVertical className='text-muted-foreground h-4 w-4' />
                          <Button
                            variant='ghost'
                            size='sm'
                            disabled={rows.indexOf(row) === 0}
                            onClick={() => moveRow(rows.indexOf(row), 'up')}
                            aria-label={t('Move up')}
                          >
                            <ArrowUp className='h-4 w-4' />
                          </Button>
                          <Button
                            variant='ghost'
                            size='sm'
                            disabled={rows.indexOf(row) === rows.length - 1}
                            onClick={() => moveRow(rows.indexOf(row), 'down')}
                            aria-label={t('Move down')}
                          >
                            <ArrowDown className='h-4 w-4' />
                          </Button>
                        </div>
                      </TableCell>
                      <TableCell>
                        <Input
                          value={row.name}
                          onChange={(event) =>
                            updateRow(row._id, 'name', event.target.value)
                          }
                          aria-invalid={duplicateNames.includes(
                            row.name.trim()
                          )}
                        />
                      </TableCell>
                      <TableCell>
                        <NativeSelect
                          value={row.categoryId}
                          onChange={(event) =>
                            updateRow(row._id, 'categoryId', event.target.value)
                          }
                          className='w-full'
                        >
                          <NativeSelectOption value=''>
                            {t('Uncategorized')}
                          </NativeSelectOption>
                          {categories.map((category) => (
                            <NativeSelectOption
                              key={category.id}
                              value={category.id}
                            >
                              {category.name}
                            </NativeSelectOption>
                          ))}
                        </NativeSelect>
                      </TableCell>
                      <TableCell>
                        <Input
                          type='number'
                          min={0}
                          step={0.1}
                          value={String(row.ratio)}
                          onChange={(event) =>
                            updateRow(
                              row._id,
                              'ratio',
                              normalizeRatio(event.target.value)
                            )
                          }
                        />
                      </TableCell>
                      <TableCell>
                        <div className='flex justify-center'>
                          <Checkbox
                            checked={row.selectable}
                            onCheckedChange={(checked) =>
                              updateRow(row._id, 'selectable', checked === true)
                            }
                            aria-label={t('User selectable')}
                          />
                        </div>
                      </TableCell>
                      <TableCell>
                        {row.selectable ? (
                          <Input
                            value={row.description}
                            placeholder={t('Group description')}
                            onChange={(event) =>
                              updateRow(
                                row._id,
                                'description',
                                event.target.value
                              )
                            }
                          />
                        ) : (
                          <span className='text-muted-foreground px-3 text-sm'>
                            -
                          </span>
                        )}
                      </TableCell>
                      <TableCell className='text-right'>
                        <Button
                          variant='ghost'
                          size='sm'
                          onClick={() => removeRow(row._id)}
                          aria-label={t('Delete')}
                        >
                          <Trash2 className='h-4 w-4' />
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>

          {duplicateNames.length > 0 && (
            <p className='text-destructive text-sm'>
              {t('Duplicate group names: {{names}}', {
                names: duplicateNames.join(', '),
              })}
            </p>
          )}
        </div>
      </CardContent>
    </Card>
  )
}

// Simple Group Dialog Component
type SimpleGroupDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSave: (name: string, value: string) => void
  editData: SimpleGroup | null
  type: 'groupRatio' | 'topupGroupRatio' | null
}

function SimpleGroupDialog({
  open,
  onOpenChange,
  onSave,
  editData,
  type,
}: SimpleGroupDialogProps) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [value, setValue] = useState('')

  const title = type === 'groupRatio' ? t('group ratio') : t('top-up ratio')

  useEffect(() => {
    if (!open) {
      setName('')
      setValue('')
      return
    }

    setName(editData?.name ?? '')
    setValue(editData?.value ?? '')
  }, [editData, open])

  const handleSave = () => {
    if (!name.trim() || !value.trim()) return
    onSave(name.trim(), value.trim())
    setName('')
    setValue('')
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {editData
              ? t('Edit {{title}}', { title })
              : t('Add {{title}}', { title })}
          </DialogTitle>
          <DialogDescription>
            {t('Configure the ratio for this group.')}
          </DialogDescription>
        </DialogHeader>
        <div className='space-y-4 py-4'>
          <div className='space-y-2'>
            <Label>{t('Group name')}</Label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('default')}
              disabled={!!editData}
            />
          </div>
          <div className='space-y-2'>
            <Label>{t('Ratio')}</Label>
            <Input
              value={value}
              onChange={(e) => {
                const val = e.target.value
                if (val === '' || !isNaN(parseFloat(val))) {
                  setValue(val)
                }
              }}
              placeholder='1.0'
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleSave}>
            {editData ? t('Update') : t('Add')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// Group Override Dialog Component
type GroupOverrideDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSave: (targetGroup: string, ratio: number, oldTargetGroup?: string) => void
  editData: GroupOverride | null
  userGroup: string | null
}

function GroupOverrideDialog({
  open,
  onOpenChange,
  onSave,
  editData,
  userGroup,
}: GroupOverrideDialogProps) {
  const { t } = useTranslation()
  const [targetGroup, setTargetGroup] = useState('')
  const [ratio, setRatio] = useState('')

  useEffect(() => {
    if (!open) {
      setTargetGroup('')
      setRatio('')
      return
    }

    setTargetGroup(editData?.targetGroup ?? '')
    setRatio(editData ? String(editData.ratio) : '')
  }, [editData, open])

  const handleSave = () => {
    if (!targetGroup.trim() || !ratio.trim()) return
    const parsedRatio = parseFloat(ratio)
    if (isNaN(parsedRatio)) return

    onSave(targetGroup.trim(), parsedRatio, editData?.targetGroup)
    setTargetGroup('')
    setRatio('')
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {editData ? t('Edit ratio override') : t('Add ratio override')}
          </DialogTitle>
          <DialogDescription>
            {userGroup
              ? t(
                  'Configure a custom ratio for "{{userGroup}}" users when using a specific token group.',
                  { userGroup }
                )
              : t(
                  'Configure a custom ratio for when users use a specific token group.'
                )}
          </DialogDescription>
        </DialogHeader>
        <div className='space-y-4 py-4'>
          <div className='space-y-2'>
            <Label>{t('Target group')}</Label>
            <Input
              value={targetGroup}
              onChange={(e) => setTargetGroup(e.target.value)}
              placeholder={t('edit_this')}
              disabled={!!editData}
            />
            <p className='text-muted-foreground text-xs'>
              {t('The token group that will have a custom ratio')}
            </p>
          </div>
          <div className='space-y-2'>
            <Label>{t('Ratio')}</Label>
            <Input
              value={ratio}
              onChange={(e) => {
                const val = e.target.value
                if (val === '' || !isNaN(parseFloat(val))) {
                  setRatio(val)
                }
              }}
              placeholder='0.9'
            />
            <p className='text-muted-foreground text-xs'>
              {t('Multiplier applied when {{userGroup}} uses {{targetGroup}}', {
                userGroup: userGroup || t('this user group'),
                targetGroup: targetGroup || t('this token group'),
              })}
            </p>
          </div>
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleSave}>
            {editData ? t('Update') : t('Add')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
