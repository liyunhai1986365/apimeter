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
import { Combobox } from '@/components/ui/combobox'
import type { ComboboxInputOption } from '@/components/ui/combobox-input'

interface FilterComboboxProps {
  id?: string
  value?: string
  options: ComboboxInputOption[]
  allValue: string
  allLabel: string
  placeholder: string
  className?: string
  allowCustomValue?: boolean
  onValueChange: (value: string | undefined) => void
}

export function FilterCombobox({
  id,
  value,
  options,
  allValue,
  allLabel,
  placeholder,
  className,
  allowCustomValue = true,
  onValueChange,
}: FilterComboboxProps) {
  return (
    <Combobox
      id={id}
      options={[{ value: allValue, label: allLabel }, ...options]}
      value={value || allValue}
      onValueChange={(nextValue) =>
        onValueChange(
          nextValue && nextValue !== allValue ? nextValue : undefined
        )
      }
      placeholder={placeholder}
      searchPlaceholder={placeholder}
      emptyText='No results found.'
      allowCustomValue={allowCustomValue}
      className={className}
    />
  )
}
