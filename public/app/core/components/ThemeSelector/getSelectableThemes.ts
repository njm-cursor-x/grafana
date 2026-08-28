import { getBuiltInThemes, type ThemeRegistryItem } from '@grafana/data';

export const ACCESSIBILITY_THEME_IDS = [
  'dim',
  'high_contrast_dark',
  'high_contrast_light',
  'deut_prot_dark',
  'deut_prot_light',
  'tritanopia_dark',
  'tritanopia_light',
] as const;

export const EXPERIMENTAL_THEME_IDS = [
  'desertbloom',
  'gildedgrove',
  'sapphiredusk',
  'tron',
  'gloom',
] as const;

export type ThemePickerCategory = 'core' | 'accessibility' | 'experimental';

const accessibilityOrder = new Map<string, number>(ACCESSIBILITY_THEME_IDS.map((id, index) => [id, index]));

export function getThemePickerCategory(theme: ThemeRegistryItem): ThemePickerCategory {
  if (accessibilityOrder.has(theme.id)) {
    return 'accessibility';
  }
  if (theme.isExtra) {
    return 'experimental';
  }
  return 'core';
}

export function getSelectableThemes() {
  return getBuiltInThemes([...ACCESSIBILITY_THEME_IDS, ...EXPERIMENTAL_THEME_IDS]);
}

export function getSelectableThemesByCategory() {
  const grouped: Record<ThemePickerCategory, ThemeRegistryItem[]> = {
    core: [],
    accessibility: [],
    experimental: [],
  };

  for (const theme of getSelectableThemes()) {
    grouped[getThemePickerCategory(theme)].push(theme);
  }

  grouped.accessibility.sort(
    (a, b) =>
      (accessibilityOrder.get(a.id) ?? Number.MAX_SAFE_INTEGER) -
      (accessibilityOrder.get(b.id) ?? Number.MAX_SAFE_INTEGER)
  );

  return grouped;
}
