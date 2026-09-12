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
import { z } from 'zod'
import type { TFunction } from 'i18next'

export function createAgentDomainsSchema(t: TFunction) {
  return z.object({
    domains: z
      .string()
      .transform((value) =>
        value
          .split(/[\n\r,，]+/)
          .map((domain) => domain.trim())
          .filter(Boolean)
      )
      .pipe(
        z
          .array(z.string())
          .min(1, t('Enter at least one domain.'))
          .max(50, t('Add up to 50 domains at a time.'))
          .refine(
            (domains) =>
              domains.every((domain) =>
                /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+\.?(?::\d{1,5})?$/i.test(
                  domain
                )
              ),
            t('Enter valid domain names without a protocol or path.')
          )
      ),
  })
}
