import { getContrastRatio } from './colorManipulator';
import { NewThemeOptionsSchema } from './createTheme';
import { getThemeById } from './registry';

import dim from './themeDefinitions/dim.json';
import highContrastDark from './themeDefinitions/high_contrast_dark.json';
import highContrastLight from './themeDefinitions/high_contrast_light.json';

describe('accessibility theme definitions', () => {
  it.each([
    ['dim', dim],
    ['high_contrast_dark', highContrastDark],
    ['high_contrast_light', highContrastLight],
  ])('%s parses as a valid theme definition', (_id, json) => {
    expect(NewThemeOptionsSchema.safeParse(json).success).toBe(true);
  });

  it.each(['dim', 'high_contrast_dark', 'high_contrast_light'])('builds %s from the registry', (id) => {
    const theme = getThemeById(id);
    expect(theme.name).toBeTruthy();
    expect(theme.colors.mode).toMatch(/^(light|dark)$/);
  });

  it.each(['high_contrast_dark', 'high_contrast_light'])(
    '%s primary text meets WCAG AAA contrast (7:1) against chrome backgrounds',
    (id) => {
      const theme = getThemeById(id);
      const foreground = theme.colors.text.primary;
      const backgrounds = [
        theme.colors.background.canvas,
        theme.colors.background.page,
        theme.colors.background.primary,
        theme.colors.background.secondary,
      ];

      for (const background of backgrounds) {
        expect(getContrastRatio(foreground, background)).toBeGreaterThanOrEqual(7);
      }
    }
  );

  it('dim is a dark theme with a darker canvas than the default dark theme', () => {
    const dimTheme = getThemeById('dim');
    const darkTheme = getThemeById('dark');

    expect(dimTheme.isDark).toBe(true);
    expect(dimTheme.colors.background.canvas).not.toEqual(darkTheme.colors.background.canvas);
  });
});
