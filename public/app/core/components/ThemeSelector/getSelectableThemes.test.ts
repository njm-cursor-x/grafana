import {
  ACCESSIBILITY_THEME_IDS,
  EXPERIMENTAL_THEME_IDS,
  getSelectableThemes,
  getSelectableThemesByCategory,
  getThemePickerCategory,
} from './getSelectableThemes';

describe('getSelectableThemes', () => {
  it('includes core, accessibility, and experimental themes', () => {
    const ids = getSelectableThemes().map((theme) => theme.id);

    expect(ids).toEqual(expect.arrayContaining(['system', 'dark', 'light', ...ACCESSIBILITY_THEME_IDS]));
    expect(ids).toEqual(expect.arrayContaining([...EXPERIMENTAL_THEME_IDS]));
  });

  it('puts dim and high contrast in the accessibility category', () => {
    const byId = Object.fromEntries(getSelectableThemes().map((theme) => [theme.id, theme]));

    expect(getThemePickerCategory(byId.dim)).toBe('accessibility');
    expect(getThemePickerCategory(byId.high_contrast_dark)).toBe('accessibility');
    expect(getThemePickerCategory(byId.high_contrast_light)).toBe('accessibility');
    expect(getThemePickerCategory(byId.deut_prot_dark)).toBe('accessibility');
    expect(getThemePickerCategory(byId.dark)).toBe('core');
    expect(getThemePickerCategory(byId.desertbloom)).toBe('experimental');
  });

  it('groups themes with accessibility ids in the declared order', () => {
    const grouped = getSelectableThemesByCategory();

    expect(grouped.core.map((theme) => theme.id)).toEqual(expect.arrayContaining(['system', 'dark', 'light']));
    expect(grouped.accessibility.map((theme) => theme.id)).toEqual([...ACCESSIBILITY_THEME_IDS]);
    expect(grouped.experimental.map((theme) => theme.id)).toEqual(expect.arrayContaining([...EXPERIMENTAL_THEME_IDS]));
  });
});
