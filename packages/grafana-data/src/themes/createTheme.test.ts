import { getContrastRatio } from './colorManipulator';
import { createTheme, NewThemeOptionsSchema } from './createTheme';
import { getThemeById } from './registry';

import autumn from './themeDefinitions/autumn.json';
import berlin from './themeDefinitions/berlin.json';
import desert from './themeDefinitions/desert.json';
import kyoto from './themeDefinitions/kyoto.json';
import london from './themeDefinitions/london.json';
import mumbai from './themeDefinitions/mumbai.json';
import newyork from './themeDefinitions/newyork.json';
import osaka from './themeDefinitions/osaka.json';
import paris from './themeDefinitions/paris.json';
import reykjavik from './themeDefinitions/reykjavik.json';
import rio from './themeDefinitions/rio.json';
import santiago from './themeDefinitions/santiago.json';
import winterblues from './themeDefinitions/winterblues.json';

const nds9ThemeDefinitions = [
  desert,
  autumn,
  winterblues,
  newyork,
  osaka,
  santiago,
  london,
  paris,
  kyoto,
  reykjavik,
  mumbai,
  rio,
  berlin,
] as const;

const nds9ThemeExpectations: Record<
  string,
  { mode: 'light' | 'dark'; canvas: string; accent: string; name: string }
> = {
  desert: { mode: 'light', canvas: '#F5E6D3', accent: '#C65D3B', name: 'Desert' },
  autumn: { mode: 'dark', canvas: '#1A1210', accent: '#E07A3D', name: 'Autumn' },
  winterblues: { mode: 'dark', canvas: '#0B1C2C', accent: '#5BA3D9', name: 'Winter Blues' },
  newyork: { mode: 'dark', canvas: '#121212', accent: '#F7B500', name: 'New York' },
  osaka: { mode: 'dark', canvas: '#140818', accent: '#FF2D95', name: 'Osaka' },
  santiago: { mode: 'light', canvas: '#F3E6D8', accent: '#B84A3A', name: 'Santiago' },
  london: { mode: 'dark', canvas: '#1B1E24', accent: '#C8102E', name: 'London' },
  paris: { mode: 'light', canvas: '#F4EFE6', accent: '#2C3A6E', name: 'Paris' },
  kyoto: { mode: 'light', canvas: '#F2EDE4', accent: '#C43B32', name: 'Kyoto' },
  reykjavik: { mode: 'dark', canvas: '#0C1418', accent: '#3DDC97', name: 'Reykjavik' },
  mumbai: { mode: 'light', canvas: '#F6EBD8', accent: '#D97706', name: 'Mumbai' },
  rio: { mode: 'dark', canvas: '#071A16', accent: '#F4C430', name: 'Rio' },
  berlin: { mode: 'dark', canvas: '#161616', accent: '#F0D400', name: 'Berlin' },
};

describe('createTheme', () => {
  it('create custom theme', () => {
    const custom = createTheme({
      colors: {
        mode: 'dark',
        primary: {
          main: 'rgb(240,0,0)',
        },
        background: {
          canvas: '#123',
        },
      },
    });

    expect(custom.colors.primary.main).toBe('rgb(240,0,0)');
    expect(custom.colors.primary.shade).toBe('rgb(242, 38, 38)');
    expect(custom.colors.background.canvas).toBe('#123');
  });

  it('create default theme', () => {
    const theme = createTheme();
    expect(theme.colors.mode).toBe('dark');
  });

  it('deep-merges component overrides on top of the defaults', () => {
    const theme = createTheme({
      components: {
        height: { sm: 99 },
      },
    });

    // overridden value is applied
    expect(theme.components.height.sm).toBe(99);
    // sibling defaults are preserved by the deep merge
    expect(theme.components.height.md).toBe(4);
    expect(theme.components.height.lg).toBe(6);
  });

  it('replaces tag colors wholesale rather than merging by index', () => {
    const theme = createTheme({
      components: {
        tag: {
          colors: [{ background: '#fff', text: '#000' }],
        },
      },
    });

    expect(theme.components.tag.colors).toEqual([{ background: '#fff', text: '#000' }]);
  });

  it.each(['visual_refresh_dark', 'visual_refresh_light'])(
    'meets code editor contrast requirements for %s',
    (themeId) => {
      const theme = getThemeById(themeId);

      for (const background of [theme.components.input.background, theme.colors.background.secondary]) {
        for (const color of Object.values(theme.components.codeEditor)) {
          expect(getContrastRatio(color, background)).toBeGreaterThanOrEqual(4.5);
        }
      }
    }
  );

  it.each(nds9ThemeDefinitions)('accepts the $id theme definition schema', (definition) => {
    const result = NewThemeOptionsSchema.safeParse(definition);
    expect(result.success).toBe(true);
  });

  it.each(Object.entries(nds9ThemeExpectations))(
    'builds %s with the expected mode, canvas, and accent',
    (themeId, expected) => {
      const theme = getThemeById(themeId);
      expect(theme.name).toBe(expected.name);
      expect(theme.colors.mode).toBe(expected.mode);
      expect(theme.colors.background.canvas).toBe(expected.canvas);
      expect(theme.colors.primary.main).toBe(expected.accent);
      expect(theme.colors.accent.main).toBe(expected.accent);
    }
  );

  it('keeps Desert distinct from Desert bloom', () => {
    const desertTheme = getThemeById('desert');
    const bloomTheme = getThemeById('desertbloom');

    expect(desertTheme.colors.background.canvas).toBe('#F5E6D3');
    expect(bloomTheme.colors.background.canvas).toBe('#FFF8F0');
    expect(desertTheme.colors.primary.main).toBe('#C65D3B');
    expect(bloomTheme.colors.primary.main).toBe('#FF6F61');
    expect(desertTheme.colors.background.canvas).not.toBe(bloomTheme.colors.background.canvas);
    expect(desertTheme.colors.primary.main).not.toBe(bloomTheme.colors.primary.main);
  });
});
