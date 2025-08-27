# Nutreek - Weekly Nutrition Planning Application

A multi-tenant nutrition planning web application built with Next.js, NestJS, and PostgreSQL.

## Features

- **Multi-tenant household management**: Users can belong to multiple households with shared weekly meal plans
- **Weekly meal planning**: Plan meals for Monday-Sunday with Breakfast/Snack/Lunch/Dinner slots
- **Recipe management**: Create and manage recipes with nutritional information
- **Ingredient tracking**: Track ingredients with quantities and nutritional data
- **Personal substitutions**: Users can define personal ingredient substitutions
- **Shopping list generation**: Automatically generate aggregated shopping lists
- **API integration**: Integration with FatSecret API for nutritional data
- **Caching**: Redis caching for API responses to reduce rate limits

## If you like my work
Help me pay off my home loan → [Donate on PayPal](https://paypal.me/ruggierocarlo)

## Tech Stack

### Frontend
- **Next.js 14** with App Router
- **TailwindCSS** for styling
- **shadcn/ui** for UI components
- **React Query** for API state management
- **TypeScript** for type safety

### Backend
- **NestJS** with TypeScript
- **Prisma ORM** for database management
- **PostgreSQL** for data storage
- **Redis** for caching
- **JWT** for authentication
- **Swagger** for API documentation

### Testing
- **Jest** for unit testing
- **React Testing Library** for frontend testing
- **Supertest** for backend API testing
- **Playwright** for E2E testing

### DevOps
- **Docker** & Docker Compose for containerization
- **Turborepo** for monorepo management

## Getting Started

### Prerequisites
- Node.js 18+
- Docker & Docker Compose
- npm or yarn

### Installation

1. Clone the repository
```bash
git clone <repository-url>
cd nutreek
```

2. Install dependencies
```bash
npm install
```

3. Set up environment variables
```bash
cp backend/.env.example backend/.env
# Edit backend/.env with your configuration
```

4. Start the development environment
```bash
docker-compose up -d
```

5. Run database migrations
```bash
cd backend
npx prisma migrate dev
```

6. Start the development servers
```bash
npm run dev
```

The application will be available at:
- Frontend: http://localhost:3000
- Backend API: http://localhost:4000
- API Documentation: http://localhost:4000/api

## Project Structure

```
nutreek/
├── frontend/                     # Next.js application
│   ├── app/                      # App Router pages
│   │   ├── (auth)/               # Authentication routes
│   │   ├── (dashboard)/          # Protected routes
│   │   └── globals.css           # Global styles
│   ├── components/               # Reusable UI components
│   ├── hooks/                    # React Query hooks
│   ├── lib/                      # Utilities and API client
│   └── package.json
├── backend/                      # NestJS API
│   ├── src/
│   │   ├── auth/                 # Authentication module
│   │   ├── tenants/              # Household management
│   │   ├── plans/                # Weekly plans
│   │   ├── prisma/               # Database schema
│   │   └── main.ts               # Application entry point
│   ├── test/                     # E2E tests
│   └── package.json
├── docker-compose.yml            # Development environment
├── package.json                  # Root package.json
├── turbo.json                    # Turborepo configuration
└── README.md
```

## Development Workflow (TDD)

This project follows strict Test-Driven Development practices:

1. **Write failing tests first** for new features
2. **Implement the minimum code** to make tests pass
3. **Refactor** while maintaining test coverage
4. **Run tests** continuously during development

### Running Tests

```bash
# Run all tests
npm run test

# Run backend tests only
cd backend && npm run test

# Run frontend tests only
cd frontend && npm run test

# Run E2E tests
cd frontend && npm run test:e2e
```

## API Documentation

The backend provides Swagger documentation at `/api` when running in development mode.

### Key Endpoints

- **Authentication**: `/auth/*`
- **Tenants**: `/tenants/*`
- **Weekly Plans**: `/tenants/{id}/plans/*`
- **Ingredients**: `/ingredients/*`

## Database Schema

The application uses Prisma with PostgreSQL. Key models include:

- **User**: Application users
- **Tenant**: Households/organizations
- **Membership**: User-tenant relationships
- **WeeklyPlan**: Weekly meal plans
- **Day**: Days in a weekly plan
- **Meal**: Meals in a day
- **Recipe**: Meal recipes
- **Ingredient**: Recipe ingredients
- **Substitution**: User ingredient substitutions

## Caching Strategy

- **Redis** is used to cache FatSecret API responses
- **TTL-based expiration** to ensure data freshness
- **Fallback to stale cache** when API quota is exceeded
- **Graceful degradation** with local ingredient database

## Deployment

The application is designed for Docker-first deployment:

```bash
# Build for production
docker-compose -f docker-compose.yml -f docker-compose.prod.yml up -d

# Or use the provided build scripts
npm run build
```

## Contributing

1. Follow the established TDD workflow
2. Write tests for new features
3. Ensure all tests pass before submitting
4. Update documentation as needed
5. Use conventional commits

## License

This project is licensed under the MIT License.

## Acknowledgments

- FatSecret API for nutritional data
- shadcn/ui for beautiful UI components
- The NestJS and Next.js communities
