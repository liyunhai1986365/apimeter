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
import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import {
  type SortingState,
  type VisibilityState,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { useMediaQuery } from '@/hooks'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { Input } from '@/components/ui/input'
import {
  DISABLED_ROW_DESKTOP,
  DISABLED_ROW_MOBILE,
  DataTablePage,
} from '@/components/data-table'
import { getGroups, getUsers, searchUsers } from '../api'
import {
  USER_STATUS,
  getUserStatusOptions,
  getUserRoleOptions,
  isUserDeleted,
} from '../constants'
import type { User } from '../types'
import { DataTableBulkActions } from './data-table-bulk-actions'
import { useUsersColumns } from './users-columns'
import { useUsers } from './users-provider'

const route = getRouteApi('/_authenticated/users/')

function isDisabledUserRow(user: User) {
  return isUserDeleted(user) || user.status === USER_STATUS.DISABLED
}

export function UsersTable() {
  const { t } = useTranslation()
  const columns = useUsersColumns()
  const { refreshTrigger } = useUsers()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const [rowSelection, setRowSelection] = useState({})
  const [sorting, setSorting] = useState<SortingState>([])
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})

  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [
      { columnId: 'status', searchKey: 'status', type: 'array' },
      { columnId: 'role', searchKey: 'role', type: 'array' },
      { columnId: 'group', searchKey: 'group', type: 'array' },
      { columnId: 'agent', searchKey: 'agent', type: 'string' },
      { columnId: 'inviterId', searchKey: 'inviterId', type: 'string' },
    ],
  })

  const statusFilter =
    (columnFilters.find((filter) => filter.id === 'status')
      ?.value as string[]) || []
  const roleFilter =
    (columnFilters.find((filter) => filter.id === 'role')?.value as string[]) ||
    []
  const groupFilter =
    (columnFilters.find((filter) => filter.id === 'group')
      ?.value as string[]) || []
  const agentFilter =
    (columnFilters.find((filter) => filter.id === 'agent')?.value as string) ||
    ''
  const inviterIdFilter =
    (columnFilters.find((filter) => filter.id === 'inviterId')
      ?.value as string) || ''

  const selectedStatus = statusFilter[0]
  const selectedRole = roleFilter[0]
  const selectedGroup = groupFilter[0]
  const inviterIdNumber = Number(inviterIdFilter)
  const parsedInviterId =
    inviterIdFilter !== '' &&
    Number.isInteger(inviterIdNumber) &&
    inviterIdNumber >= 0
      ? inviterIdNumber
      : undefined
  const hasFilters = Boolean(
    globalFilter?.trim() ||
    selectedStatus ||
    selectedRole ||
    selectedGroup ||
    agentFilter.trim() ||
    inviterIdFilter.trim()
  )

  const setTextFilter = (filterId: string, value: string) => {
    onColumnFiltersChange((previous) => {
      const remaining = previous.filter((filter) => filter.id !== filterId)
      return value ? [...remaining, { id: filterId, value }] : remaining
    })
  }

  const { data: groupsData } = useQuery({
    queryKey: ['groups'],
    queryFn: getGroups,
  })

  const groupOptions = useMemo(
    () =>
      (groupsData?.data || []).map((group) => ({
        label: group,
        value: group,
      })),
    [groupsData]
  )

  // Fetch data with React Query
  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'users',
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
      selectedStatus,
      selectedRole,
      selectedGroup,
      agentFilter,
      parsedInviterId,
      hasFilters,
      refreshTrigger,
    ],
    queryFn: async () => {
      const params = {
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
      }

      const result = hasFilters
        ? await searchUsers({
            ...params,
            keyword: globalFilter,
            status: selectedStatus ? Number(selectedStatus) : undefined,
            role: selectedRole ? Number(selectedRole) : undefined,
            group: selectedGroup,
            agent: agentFilter.trim() || undefined,
            inviter_id: parsedInviterId,
          })
        : await getUsers(params)

      if (!result.success) {
        toast.error(
          result.message || `Failed to ${hasFilters ? 'search' : 'load'} users`
        )
        return { items: [], total: 0 }
      }

      return {
        items: result.data?.items || [],
        total: result.data?.total || 0,
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const users = data?.items || []

  const table = useReactTable({
    data: users,
    columns,
    state: {
      sorting,
      columnVisibility,
      rowSelection,
      columnFilters,
      globalFilter,
      pagination,
    },
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    onSortingChange: setSorting,
    onColumnVisibilityChange: setColumnVisibility,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    onPaginationChange,
    onGlobalFilterChange,
    onColumnFiltersChange,
    manualPagination: true,
    manualFiltering: true,
    pageCount: Math.ceil((data?.total || 0) / pagination.pageSize),
  })

  const pageCount = table.getPageCount()
  useEffect(() => {
    ensurePageInRange(pageCount)
  }, [pageCount, ensurePageInRange])

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No Users Found')}
      emptyDescription={t(
        'No users available. Try adjusting your search or filters.'
      )}
      skeletonKeyPrefix='users-skeleton'
      toolbarProps={{
        searchPlaceholder: t('Filter by username, name, email or user ID...'),
        additionalSearch: (
          <>
            <Input
              type='search'
              placeholder={t('Filter by agent ID or name...')}
              value={agentFilter}
              onChange={(event) => setTextFilter('agent', event.target.value)}
              className='w-full sm:w-[180px] lg:w-[220px]'
              aria-label={t('Agent')}
            />
            <Input
              type='number'
              min={0}
              step={1}
              placeholder={t('Inviter ID')}
              value={inviterIdFilter}
              onChange={(event) => {
                const value = event.target.value
                if (value === '' || /^\d+$/.test(value)) {
                  setTextFilter('inviterId', value)
                }
              }}
              className='w-full sm:w-[140px]'
              aria-label={t('Inviter ID')}
            />
          </>
        ),
        hasAdditionalFilters: Boolean(agentFilter || inviterIdFilter),
        filters: [
          {
            columnId: 'status',
            title: t('Status'),
            options: getUserStatusOptions(t),
            singleSelect: true,
          },
          {
            columnId: 'role',
            title: t('Role'),
            options: getUserRoleOptions(t),
            singleSelect: true,
          },
          {
            columnId: 'group',
            title: t('User group'),
            options: groupOptions,
            singleSelect: true,
          },
        ],
      }}
      getRowClassName={(row, { isMobile }) =>
        isDisabledUserRow(row.original)
          ? isMobile
            ? DISABLED_ROW_MOBILE
            : DISABLED_ROW_DESKTOP
          : undefined
      }
      bulkActions={<DataTableBulkActions table={table} />}
    />
  )
}
