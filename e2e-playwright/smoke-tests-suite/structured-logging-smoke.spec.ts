import { test, expect } from '@grafana/plugin-e2e';

test.describe(
  'Structured logging regression smoke',
  {
    tag: ['@acceptance'],
  },
  () => {
    test('open a dashboard, query a panel, and save', async ({
      createDataSourceConfigPage,
      gotoDashboardPage,
      selectors,
      page,
    }) => {
      // Login is provided by the Playwright `authenticate` project (admin storageState).
      // Unauthenticated login UI coverage lives in e2e-playwright/unauthenticated/login.spec.ts.

      const dataSourceConfigPage = await createDataSourceConfigPage({
        name: `e2e-structured-logging-${crypto.randomUUID()}`,
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

      const titleInput = page.getByTestId(selectors.components.Drawer.DashboardSaveDrawer.saveAsTitleInput);
      if (await titleInput.isVisible({ timeout: 3000 }).catch(() => false)) {
        await titleInput.fill(`Structured logging smoke ${Date.now()}`);
      }

      await dashboardPage.getByGrafanaSelector(selectors.components.Drawer.DashboardSaveDrawer.saveButton).click();
      await expect(page.getByRole('status', { name: /dashboard saved/i })).toBeVisible({ timeout: 10_000 });
    });
  }
);
