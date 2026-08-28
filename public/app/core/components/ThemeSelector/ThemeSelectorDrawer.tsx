import { css } from '@emotion/css';

import { type GrafanaTheme2, type ThemeRegistryItem } from '@grafana/data';
import { Trans, t } from '@grafana/i18n';
import { config, reportInteraction } from '@grafana/runtime';
import { Drawer, Text, TextLink, useStyles2, useTheme2 } from '@grafana/ui';
import { changeTheme } from 'app/core/services/theme';

import { ThemeCard } from './ThemeCard';
import { getSelectableThemesByCategory, getThemePickerCategory } from './getSelectableThemes';

interface Props {
  onClose: () => void;
}

export function ThemeSelectorDrawer({ onClose }: Props) {
  const styles = useStyles2(getStyles);
  const groupedThemes = getSelectableThemesByCategory();
  const currentTheme = useTheme2();

  const onChange = (theme: ThemeRegistryItem) => {
    reportInteraction('grafana_preferences_theme_changed', {
      toTheme: theme.id,
      preferenceType: 'theme_drawer',
    });
    changeTheme(theme.id, false);
  };

  const subTitle = (
    <Trans i18nKey="shared-preferences.fields.theme-description">
      Enjoying the experimental themes? Tell us what you&apos;d like to see{' '}
      <TextLink
        variant="bodySmall"
        external
        href="https://docs.google.com/forms/d/e/1FAIpQLSeRKAY8nUMEVIKSYJ99uOO-dimF6Y69_If1Q1jTLOZRWqK1cw/viewform?usp=dialog"
      >
        here.
      </TextLink>
    </Trans>
  );

  const sections: Array<{ id: 'core' | 'accessibility' | 'experimental'; title: string }> = [
    { id: 'core', title: t('shared-preferences.theme.core', 'Core') },
    { id: 'accessibility', title: t('shared-preferences.theme.accessibility', 'Accessibility') },
    { id: 'experimental', title: t('shared-preferences.theme.experimental', 'Experimental') },
  ];

  return (
    <Drawer
      title={t('profile.change-theme', 'Change theme')}
      onClose={onClose}
      size="md"
      subtitle={config.feedbackLinksEnabled ? subTitle : undefined}
    >
      <div className={styles.sections} role="radiogroup">
        {sections.map((section) => (
          <section key={section.id} className={styles.section} aria-label={section.title}>
            <Text variant="h5" element="h3">
              {section.title}
            </Text>
            <div className={styles.grid}>
              {groupedThemes[section.id].map((themeOption) => (
                <ThemeCard
                  themeOption={themeOption}
                  isExperimental={getThemePickerCategory(themeOption) === 'experimental'}
                  key={themeOption.id}
                  onSelect={() => onChange(themeOption)}
                  isSelected={currentTheme.name === themeOption.name}
                />
              ))}
            </div>
          </section>
        ))}
      </div>
    </Drawer>
  );
}

const getStyles = (theme: GrafanaTheme2) => {
  return {
    sections: css({
      display: 'flex',
      flexDirection: 'column',
      gap: theme.spacing(3),
    }),
    section: css({
      display: 'flex',
      flexDirection: 'column',
      gap: theme.spacing(1.5),
    }),
    grid: css({
      display: 'grid',
      gridTemplateColumns: 'repeat(auto-fit, minmax(300px, 1fr))',
      gridAutoRows: `250px`,
      gap: theme.spacing(2),
    }),
  };
};
