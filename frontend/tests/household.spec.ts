import { test, expect } from '@playwright/test';

test.describe('Household Management', () => {
  test.beforeEach(async ({ page }) => {
    // Mock authentication
    await page.addInitScript(() => {
      localStorage.setItem('auth_token', 'mock-jwt-token');
    });

    // Mock user API
    await page.route('/auth/me', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          id: 'user-1',
          email: 'test@example.com',
          name: 'Test User'
        })
      });
    });
  });

  test('should create a new household', async ({ page }) => {
    // Mock empty tenants list initially
    await page.route('/tenants', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([])
      });
    });

    await page.goto('/dashboard');

    // Click create household button
    await page.click('text=Create Household');

    // Fill out the form
    await page.fill('input[placeholder="e.g., Smith Family"]', 'Test Family');

    // Mock the create tenant API call
    await page.route('/tenants', async route => {
      if (route.request().method() === 'POST') {
        await route.fulfill({
          status: 201,
          contentType: 'application/json',
          body: JSON.stringify({
            id: 'new-tenant-1',
            name: 'Test Family'
          })
        });
      } else {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify([
            {
              id: 'new-tenant-1',
              name: 'Test Family',
              memberships: []
            }
          ])
        });
      }
    });

    // Submit the form
    await page.click('button[type="submit"]');

    // Check that the household appears in the list
    await expect(page.locator('text=Test Family')).toBeVisible();
  });

  test('should navigate to household weekly planner', async ({ page }) => {
    // Mock tenants with existing household
    await page.route('/tenants', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([
          {
            id: 'tenant-1',
            name: 'Test Household',
            memberships: [
              {
                id: 'membership-1',
                userId: 'user-1',
                tenantId: 'tenant-1',
                role: 'admin',
                user: {
                  id: 'user-1',
                  email: 'test@example.com',
                  name: 'Test User'
                }
              }
            ]
          }
        ])
      });
    });

    await page.goto('/dashboard');

    // Click view plans for the household
    await page.click('text=View Plans');

    // Should navigate to household page
    await expect(page).toHaveURL(/.*dashboard\/households\/tenant-1/);

    // Check household page content
    await expect(page.locator('h1')).toContainText('Test Household');
    await expect(page.locator('text=Test User')).toBeVisible();
    await expect(page.locator('text=admin')).toBeVisible();
  });

  test('should display weekly planner interface', async ({ page }) => {
    // Mock household data
    await page.route('/tenants/tenant-1', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          id: 'tenant-1',
          name: 'Test Household',
          memberships: [
            {
              id: 'membership-1',
              userId: 'user-1',
              tenantId: 'tenant-1',
              role: 'admin',
              user: {
                id: 'user-1',
                email: 'test@example.com',
                name: 'Test User'
              }
            }
          ]
        })
      });
    });

    // Mock empty plans list
    await page.route('/tenants/tenant-1/plans', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([])
      });
    });

    await page.goto('/dashboard/households/tenant-1');

    // Check weekly planner elements
    await expect(page.locator('text=Weekly Meal Plans')).toBeVisible();
    await expect(page.locator('text=Create New Plan')).toBeVisible();
    await expect(page.locator('text=No weekly plans yet')).toBeVisible();
  });

  test('should create a new weekly plan', async ({ page }) => {
    // Mock household data
    await page.route('/tenants/tenant-1', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          id: 'tenant-1',
          name: 'Test Household',
          memberships: []
        })
      });
    });

    // Mock empty plans initially, then with new plan
    let planCreated = false;
    await page.route('/tenants/tenant-1/plans', async route => {
      if (route.request().method() === 'POST') {
        planCreated = true;
        await route.fulfill({
          status: 201,
          contentType: 'application/json',
          body: JSON.stringify({
            id: 'plan-1',
            tenantId: 'tenant-1',
            startDate: '2024-01-15T00:00:00.000Z'
          })
        });
      } else {
        const plans = planCreated ? [{
          id: 'plan-1',
          tenantId: 'tenant-1',
          startDate: '2024-01-15T00:00:00.000Z',
          days: []
        }] : [];
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify(plans)
        });
      }
    });

    await page.goto('/dashboard/households/tenant-1');

    // Click create new plan
    await page.click('text=Create New Plan');

    // Fill in the start date (next Monday)
    const nextMonday = new Date();
    nextMonday.setDate(nextMonday.getDate() + (1 - nextMonday.getDay() + 7) % 7 + 1);
    const dateString = nextMonday.toISOString().split('T')[0];

    await page.fill('input[type="date"]', dateString);

    // Submit the form
    await page.click('button[type="submit"]');

    // Check that plan appears
    await expect(page.locator('text=Week of')).toBeVisible();
  });
});