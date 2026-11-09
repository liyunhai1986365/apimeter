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
import type { InterfaceLanguageCode } from './languages'

// Keep startup and recovery messages available without downloading a locale.
export const bootstrapResources = {
  en: {
    bootstrap: {
      loading: 'Loading...',
      error:
        'Unable to load the language. Check your connection and try again.',
      retry: 'Retry',
    },
  },
  zhCN: {
    bootstrap: {
      loading: '加载中...',
      error: '语言加载失败，请检查网络连接后重试。',
      retry: '重试',
    },
  },
  fr: {
    bootstrap: {
      loading: 'Chargement...',
      error:
        'Impossible de charger la langue. Vérifiez votre connexion et réessayez.',
      retry: 'Réessayer',
    },
  },
  ru: {
    bootstrap: {
      loading: 'Загрузка...',
      error:
        'Не удалось загрузить язык. Проверьте подключение и повторите попытку.',
      retry: 'Повторить попытку',
    },
  },
  ja: {
    bootstrap: {
      loading: '読み込み中...',
      error:
        '言語を読み込めませんでした。接続を確認して、もう一度お試しください。',
      retry: '再試行',
    },
  },
  vi: {
    bootstrap: {
      loading: 'Đang tải...',
      error: 'Không thể tải ngôn ngữ. Vui lòng kiểm tra kết nối và thử lại.',
      retry: 'Thử lại',
    },
  },
  zhTW: {
    bootstrap: {
      loading: '載入中...',
      error: '語言載入失敗，請檢查網路連線後重試。',
      retry: '重試',
    },
  },
} satisfies Record<
  InterfaceLanguageCode,
  { bootstrap: { loading: string; error: string; retry: string } }
>
