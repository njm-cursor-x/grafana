import { test, expect } from '@grafana/plugin-e2e';

/**
 * Focused structured-logging e2e smoke: login (via authenticate project) →
 * open dashboard → query a testdata panel → save.
 * Requires a live Grafana (Playwright webServer or GRAFANA_URL).
 */
test.describe(
  'Structured logging smoke',
  {
    tag: ['@acceptance', '@structured-logging'],
  },
  () => {
    test('login, open dashboard, query panel, save', async ({
      createDataSourceConfigPage,
      gotoDashboardPage,
      selectors,
      page,
    }) => {
      const dataSourceConfigPage = await createDataSourceConfigPage({
        name: `structured-logging-${crypto.randomUUID()}`,
        type: 'grafana-testdata-datasource',
      });
      const { datasource } = dataSourceConfigPage;
      await dataSourceConfigPage.saveAndTest({
        path: `/api/datasources/uid/${datasource.uid}?accesscontrol=true`,
      });

      const dashboardPage = await gotoDashboardPage({});
      await dashboardPage.addPanel();

      const scenarioSelect = dashboardPage.getByGrafanaSelector(
        selectors.components.DataSource.TestData.QueryTab.scenarioSelectContainer
      );
      await expect(scenarioSelect).toBeVisible();
      await scenarioSelect.locator('input[id*="test-data-scenario-select-"]').click();
      await page.getByText('CSV Metric Values').click();

      await expect(
        dashboardPage.getByGrafanaSelector(selectors.components.VizLegend.seriesName('A-series'))
      ).toBeVisible();

      await dashboardPage
        .getByGrafanaSelector(selectors.components.NavToolbar.editDashboard.backToDashboardButton)
        .click();
      await expect(
        dashboardPage.getByGrafanaSelector(selectors.components.VizLegend.seriesName('A-series'))
      ).toBeVisible();

      await dashboardPage.getByGrafanaSelector(selectors.components.NavToolbar.editDashboard.saveButton).click();

      const titleInput = dashboardPage.getByGrafanaSelector(
        selectors.components.Drawer.DashboardSaveDrawer.saveAsTitleInput
      );
      if (await titleInput.isVisible().catch(() => false)) {
        await titleInput.fill('structured-logging-smoke');
      }

      await dashboardPage.getByGrafanaSelector(selectors.components.Drawer.DashboardSaveDrawer.saveButton).click();
      await expect(page.getByRole('status', { name: 'Dashboard saved' })).toBeVisible({ timeout: 10_000 });
    });
  }
);
