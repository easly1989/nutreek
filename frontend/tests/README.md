# End-to-End Tests

This directory contains Playwright end-to-end tests for the Nutreek application.

## Setup

1. Install dependencies:
```bash
npm install
```

2. Make sure the development servers are running:
```bash
# Terminal 1: Start the Next.js frontend
npm run dev

# Terminal 2: Start the NestJS backend
cd ../backend && npm run start:dev
```

## Running Tests

### Run all tests
```bash
npm run test:e2e
```

### Run tests in headed mode (visible browser)
```bash
npx playwright test --headed
```

### Run specific test file
```bash
npx playwright test auth.spec.ts
npx playwright test dashboard.spec.ts
npx playwright test household.spec.ts
```

### Run tests in specific browser
```bash
npx playwright test --project=chromium
npx playwright test --project=firefox
npx playwright test --project=webkit
```

### Generate and view test report
```bash
npx playwright show-report
```

## Test Structure

### `auth.spec.ts`
Tests authentication flows including:
- Landing page display
- Login page navigation and validation
- Register page navigation and validation
- Form validation and error handling

### `dashboard.spec.ts`
Tests dashboard functionality including:
- User authentication and display
- Household information display
- Navigation to different sections
- Logout functionality
- Statistics cards display

### `household.spec.ts`
Tests household management including:
- Household creation
- Navigation to household pages
- Weekly planner interface
- Plan creation workflow

## Mock Data

Tests use mocked API responses to ensure reliable testing without external dependencies. Key mocked endpoints include:

- `/auth/me` - User authentication
- `/tenants` - Household management
- `/tenants/*/plans` - Weekly planning

## CI/CD Integration

These tests are designed to run in CI/CD pipelines. Use the `--project=chromium` flag for headless execution in CI environments.

## Debugging Tests

### Visual debugging
```bash
npx playwright test --headed --debug
```

### Step-by-step debugging
```bash
npx playwright test --debug
```

### Browser developer tools
```bash
npx playwright test --headed
# Then use browser dev tools to inspect elements
```

## Best Practices

1. **Use descriptive test names** that explain what the test validates
2. **Mock external API calls** to ensure test reliability
3. **Test user journeys** rather than isolated functionality
4. **Use data-testid attributes** for reliable element selection
5. **Clean up after tests** to avoid state interference
6. **Test both success and error scenarios**

## Adding New Tests

1. Create a new `.spec.ts` file in this directory
2. Follow the existing naming convention
3. Use the same setup pattern with authentication mocking
4. Add appropriate API mocking for your test scenarios
5. Run tests locally before committing
</content>
<line_count>89</line_count>