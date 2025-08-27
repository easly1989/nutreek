import { test, expect } from '@playwright/test';

test.describe('Authentication', () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to the home page before each test
    await page.goto('/');
  });

  test('should display the landing page', async ({ page }) => {
    await expect(page).toHaveTitle(/Nutreek/);
    await expect(page.locator('h1')).toContainText('Nutreek');
    await expect(page.locator('text=Weekly Nutrition Planner')).toBeVisible();
  });

  test('should navigate to login page', async ({ page }) => {
    await page.click('text=Login');
    await expect(page).toHaveURL(/.*auth\/login/);
    await expect(page.locator('h2')).toContainText('Sign in to Nutreek');
  });

  test('should navigate to register page', async ({ page }) => {
    await page.click('text=Register');
    await expect(page).toHaveURL(/.*auth\/register/);
    await expect(page.locator('h2')).toContainText('Create your account');
  });

  test('should show validation errors on empty login form', async ({ page }) => {
    await page.goto('/auth/login');
    await page.click('button[type="submit"]');

    // Check that we're still on the login page (form validation prevents submission)
    await expect(page).toHaveURL(/.*auth\/login/);
  });

  test('should show validation errors on empty register form', async ({ page }) => {
    await page.goto('/auth/register');
    await page.click('button[type="submit"]');

    // Check that we're still on the register page (form validation prevents submission)
    await expect(page).toHaveURL(/.*auth\/register/);
  });

  test('should allow navigation between login and register', async ({ page }) => {
    // Go to login page
    await page.goto('/auth/login');
    await expect(page.locator('h2')).toContainText('Sign in to Nutreek');

    // Click register link
    await page.click('text=create a new account');
    await expect(page.locator('h2')).toContainText('Create your account');

    // Click login link
    await page.click('text=sign in to existing account');
    await expect(page.locator('h2')).toContainText('Sign in to Nutreek');
  });
});