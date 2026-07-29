/*
Copyright (C) 2025 QuantumNous

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

export const supportedLanguages = [
  'zh-CN',
  'zh-TW',
  'en',
  'fr',
  'ru',
  'ja',
  'vi',
];

export const normalizeLanguage = (language) => {
  if (!language) {
    return language;
  }

  const normalized = language.trim().replace(/_/g, '-');
  const lower = normalized.toLowerCase();

  if (
    lower === 'zh' ||
    lower === 'zhcn' ||
    lower === 'zh-cn' ||
    lower === 'zh-sg' ||
    lower.startsWith('zh-hans')
  ) {
    return 'zh-CN';
  }

  if (
    lower === 'zhtw' ||
    lower === 'zh-tw' ||
    lower === 'zh-hk' ||
    lower === 'zh-mo' ||
    lower.startsWith('zh-hant')
  ) {
    return 'zh-TW';
  }

  const baseLanguage = lower.split('-')[0];
  const matchedLanguage = supportedLanguages.find(
    (supportedLanguage) =>
      !supportedLanguage.startsWith('zh-') &&
      supportedLanguage.toLowerCase() === baseLanguage,
  );

  return matchedLanguage || normalized;
};

export const getLanguageFromSearch = (search) => {
  const params = new URLSearchParams(search);
  for (const parameter of ['lang', 'language']) {
    const language = normalizeLanguage(params.get(parameter));
    if (supportedLanguages.includes(language)) {
      return language;
    }
  }
  return undefined;
};

export const buildLanguageUrl = (href, language) => {
  const normalizedLanguage = normalizeLanguage(language);
  if (!supportedLanguages.includes(normalizedLanguage)) {
    return href;
  }

  const url = new URL(href);
  url.searchParams.set('lang', normalizedLanguage);
  url.searchParams.delete('language');
  return url.toString();
};

export const replaceCurrentUrlLanguage = (language) => {
  if (typeof window === 'undefined') {
    return;
  }

  const url = new URL(buildLanguageUrl(window.location.href, language));
  window.history.replaceState(
    window.history.state,
    '',
    `${url.pathname}${url.search}${url.hash}`,
  );
};
