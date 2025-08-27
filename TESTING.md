# Testing Guide

This document explains the testing strategy and setup for the Nutreek application.

## Test Types

### Unit Tests
- **Location**: `backend/src/**/*.spec.ts`
- **Framework**: Jest
- **Purpose**: Test individual functions, services, and controllers in isolation
- **Speed**: Fast (seconds)
- **Requirements**: None (mocks all dependencies)

### End-to-End Tests (E2E)
- **Location**: `frontend/tests/*.spec.ts`
- **Framework**: Playwright
- **Purpose**: Test complete user journeys and integration between frontend and backend
- **Speed**: Slow (minutes)
- **Requirements**: Running frontend and backend servers

## Running Tests

### Quick Unit Tests Only
Run only the backend unit tests (fast, no setup required):

```bash
npm run test
# or
npm run test:unit
```

### Full E2E Test Suite
Run the complete test suite including e2e tests (requires server setup):

```bash
# Option 1: Automated setup (recommended)
npm run test:e2e:setup

# Option 2: Manual setup
# 1. Generate Prisma client
npx prisma generate

# 2. Start backend server (in one terminal)
cd backend && npm run start:dev

# 3. Start frontend server (in another terminal)
cd frontend && npm run dev

# 4. Run e2e tests (in third terminal)
npm run test:e2e
```

### Individual Test Commands

#### Backend Unit Tests
```bash
cd backend
npm run test          # Run all unit tests
npm run test:watch    # Run tests in watch mode
npm run test:cov      # Run with coverage report
```

#### Frontend E2E Tests
```bash
cd frontend
npm run test:e2e      # Run all e2e tests
npx playwright test --headed  # Run with browser visible
npx playwright test auth.spec.ts  # Run specific test file
```

## Test Structure

```
backend/
├── src/
│   ├── app.spec.ts
│   ├── auth/
│   │   └── auth.controller.spec.ts
│   ├── ingredients/
│   │   └── ingredients.controller.spec.ts
│   └── ... (other controller tests)
└── test/
    └── jest-e2e.json  # E2E test configuration (currently unused)

frontend/
├── tests/
│   ├── auth.spec.ts      # Authentication e2e tests
│   ├── dashboard.spec.ts # Dashboard e2e tests
│   └── household.spec.ts # Household management e2e tests
├── playwright.config.ts  # Playwright configuration
└── package.json
```

## Adding New Tests

### Unit Tests (Backend)
1. Create `*.spec.ts` file next to the file you want to test
2. Use Jest syntax with `describe`, `it`, `expect`
3. Mock all dependencies using Jest mocks
4. Follow existing patterns for dependency injection

Example:
```typescript
import { Test, TestingModule } from '@nestjs/testing';
import { MyService } from './my.service';
import { PrismaService } from '../prisma.service';

describe('MyService', () => {
  let service: MyService;

  beforeEach(async () => {
    const mockPrismaService = { /* mock methods */ };

    const module: TestingModule = await Test.createTestingModule({
      providers: [
        MyService,
        {
          provide: PrismaService,
          useValue: mockPrismaService,
        },
      ],
    }).compile();

    service = module.get<MyService>(MyService);
  });

  it('should be defined', () => {
    expect(service).toBeDefined();
  });
});
```

### E2E Tests (Frontend)
1. Create `*.spec.ts` file in `frontend/tests/`
2. Use Playwright syntax with `test.describe`, `test()`, `expect()`
3. Mock API calls and external dependencies
4. Follow existing patterns for authentication and setup

Example:
```typescript
import { test, expect } from '@playwright/test';

test.describe('My Feature', () => {
  test.beforeEach(async ({ page }) => {
    // Setup authentication and mocks
    await page.addInitScript(() => {
      localStorage.setItem('auth_token', 'mock-jwt-token');
    });
  });

  test('should work correctly', async ({ page }) => {
    await page.goto('/my-feature');
    await expect(page.locator('h1')).toContainText('My Feature');
  });
});
```

## CI/CD Integration

### For Unit Tests
Unit tests are fast and reliable, perfect for CI/CD pipelines:

```yaml
# Example GitHub Actions
- name: Run Unit Tests
  run: npm run test:unit
```

### For E2E Tests
E2E tests require special setup in CI/CD:

```yaml
# Example GitHub Actions
- name: Setup E2E Environment
  run: |
    npx prisma generate
    npm run test:e2e:setup
```

## Troubleshooting

### E2E Tests Failing
1. **Prisma client not generated**: Run `npx prisma generate`
2. **Ports already in use**: Kill processes on ports 3000 and 4000
3. **Servers not ready**: Wait for both servers to fully start (60+ seconds)
4. **Database connection**: Ensure database is accessible

### Unit Tests Failing
1. **Missing mocks**: Add mocks for all dependencies
2. **Import issues**: Check import paths and ensure files exist
3. **Type errors**: Fix TypeScript compilation errors first

## Best Practices

1. **Unit tests should be fast** - Mock external dependencies
2. **E2E tests should be reliable** - Use proper waits and error handling
3. **Separate concerns** - Unit tests for logic, E2E tests for integration
4. **Mock external services** - Don't rely on real APIs in tests
5. **Clean up after tests** - Remove test data and reset state

## Performance Tips

- **Run unit tests frequently** during development
- **Run e2e tests before commits** to staging/production
- **Use `--headed` flag** when debugging e2e tests
- **Parallel execution** - Unit tests run in parallel by default
- **Caching** - Turbo caches test results between runs

## Migration Notes

This setup was created to resolve issues with:
- Jest trying to run Playwright test files
- Missing dependency mocks in unit tests
- E2E tests requiring manual server setup
- Slow CI/CD pipelines due to unnecessary e2e test runs

The new structure provides:
- ✅ Fast unit test execution
- ✅ Automated e2e test environment setup
- ✅ Clear separation of concerns
- ✅ Cross-platform compatibility
- ✅ CI/CD friendly configuration