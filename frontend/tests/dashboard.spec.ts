import { test, expect } from '@playwright/test';

test.describe('Dashboard', () => {
  test.beforeEach(async ({ page }) => {
    // Mock authentication by setting localStorage token
    await page.addInitScript(() => {
      localStorage.setItem('auth_token', 'mock-jwt-token');
    });

    // Mock the user API call
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

    // Mock the tenants API call
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
  });

  test('should display dashboard with user info', async ({ page }) => {
    await page.goto('/dashboard');

    await expect(page.locator('h1')).toContainText('Nutreek Dashboard');
    await expect(page.locator('text=Welcome back, Test User!')).toBeVisible();
    await expect(page.locator('button').filter({ hasText: 'Logout' })).toBeVisible();
  });

  test('should display household information', async ({ page }) => {
    await page.goto('/dashboard');

    await expect(page.locator('text=My Households')).toBeVisible();
    await expect(page.locator('text=Test Household')).toBeVisible();
    await expect(page.locator('text=1 members')).toBeVisible();
  });

  test('should navigate to household page', async ({ page }) => {
    await page.goto('/dashboard');

    // Mock the household page route
    await page.route('/dashboard/households/tenant-1', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'text/html',
        body: '<html><body><h1>Test Household</h1></body></html>'
      });
    });

    await page.click('text=View Plans');
    await expect(page).toHaveURL(/.*dashboard\/households\/tenant-1/);
  });

  test('should navigate to substitutions page', async ({ page }) => {
    await page.goto('/dashboard');

    await page.click('text=Manage →');
    await expect(page).toHaveURL(/.*dashboard\/substitutions/);
  });

  test('should navigate to ingredients page', async ({ page }) => {
    await page.goto('/dashboard');

    await page.click('text=Search →');
    await expect(page).toHaveURL(/.*dashboard\/ingredients/);
  });

  test('should logout successfully', async ({ page }) => {
    await page.goto('/dashboard');

    // Mock logout by removing token
    await page.addInitScript(() => {
      // Override localStorage.removeItem for testing
      const originalRemoveItem = localStorage.removeItem;
      localStorage.removeItem = (key: string) => {
        if (key === 'auth_token') {
          // Simulate successful logout
          return;
        }
        originalRemoveItem.call(localStorage, key);
      };
    });

    await page.click('button:has-text("Logout")');

    // Should redirect to home page
    await expect(page).toHaveURL('/');
  });

  test('should display dashboard statistics cards', async ({ page }) => {
    await page.goto('/dashboard');

    // Check for statistics cards
    await expect(page.locator('text=Weekly Plans')).toBeVisible();
    await expect(page.locator('text=Recipes')).toBeVisible();
    await expect(page.locator('text=Substitutions')).toBeVisible();
    await expect(page.locator('text=Shopping Lists')).toBeVisible();
  });
});