import { getBuiltInThemes } from '@grafana/data';

export function getSelectableThemes() {
  const allowedExtraThemes = [
    'deut_prot_dark',
    'deut_prot_light',
    'tritanopia_dark',
    'tritanopia_light',
    'desertbloom',
    'gildedgrove',
    'sapphiredusk',
    'tron',
    'gloom',
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
  ];

  return getBuiltInThemes(allowedExtraThemes);
}
