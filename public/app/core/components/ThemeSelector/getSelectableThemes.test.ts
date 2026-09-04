import { getSelectableThemes } from './getSelectableThemes';

describe('getSelectableThemes', () => {
  it('includes the NDS-9 seasonal and city theme ids', () => {
    const ids = getSelectableThemes().map((theme) => theme.id);

    expect(ids).toEqual(
      expect.arrayContaining([
        'desert',
        'autumn',
        'winterblues',
        'newyork',
        'osaka',
        'santiago',
        'london',
        'paris',
        'kyoto',
        'reykjavik',
        'mumbai',
        'rio',
        'berlin',
      ])
    );
  });

  it('keeps built-in themes selectable ahead of extras', () => {
    const themes = getSelectableThemes();
    const firstExtraIndex = themes.findIndex((theme) => theme.isExtra);

    expect(themes.slice(0, firstExtraIndex).map((theme) => theme.id)).toEqual(
      expect.arrayContaining(['system', 'dark', 'light'])
    );
    expect(themes[firstExtraIndex].isExtra).toBe(true);
  });
});
