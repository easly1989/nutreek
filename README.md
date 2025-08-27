# Nutreek - Enterprise Nutrition Planning Platform

A **comprehensive, enterprise-grade multi-tenant nutrition planning platform** built with modern web technologies and advanced collaboration features.

## If you like my work
Help me pay off my home loan → [Donate on PayPal](https://paypal.me/ruggierocarlo)

## 🚀 **Key Features**

### **Core Functionality**
- **Multi-tenant household management**: Users can belong to multiple households with shared weekly meal plans
- **Advanced weekly meal planning**: Plan meals for Monday-Sunday with Breakfast/Snack/Lunch/Dinner slots
- **Intelligent recipe management**: Create, edit, and organize recipes with comprehensive nutritional data
- **Smart shopping list generation**: Automatically generate optimized shopping lists from weekly plans
- **Personal substitutions**: Users can define personal ingredient substitutions
- **Real-time collaboration**: Activity feeds, commenting, and notification systems
- **Comprehensive analytics**: Detailed nutrition tracking and household insights

### **Advanced Features**
- **Recipe creation system**: Full-featured recipe builder with ingredient management
- **Ingredient database integration**: Real-time nutritional data with FatSecret API
- **Activity tracking**: Complete audit trail of household activities
- **Comment system**: Contextual feedback on plans and recipes
- **Analytics dashboard**: Comprehensive nutrition and usage analytics
- **Production monitoring**: Health checks, system monitoring, and alerting

## 🛠 **Technology Stack**

### **Frontend**
- **Next.js 14** with App Router and React 18
- **TailwindCSS** + **shadcn/ui** for modern, responsive design
- **React Query** for efficient API state management
- **TypeScript** for complete type safety
- **Playwright** for comprehensive E2E testing

### **Backend**
- **NestJS** with TypeScript and modular architecture
- **Prisma ORM** with PostgreSQL for robust data management
- **Redis** for high-performance caching
- **JWT authentication** with secure session management
- **Swagger/OpenAPI** for complete API documentation
- **Jest + Supertest** for comprehensive testing

### **Infrastructure & DevOps**
- **Docker & Docker Compose** for containerization
- **Nginx** reverse proxy with SSL termination
- **Production health monitoring** and alerting
- **Automated deployment scripts**
- **Database backup and recovery**
- **Security hardening** with rate limiting and CORS

## 📊 **Enhanced Capabilities**

### **Recipe Management**
- Create recipes with precise ingredient measurements
- Automatic nutritional value calculations
- Ingredient search with external API integration
- Recipe categorization and organization
- Import/export functionality

### **Shopping Intelligence**
- Automatic ingredient consolidation from weekly plans
- Smart quantity optimization
- Recipe attribution tracking
- Shopping list categorization
- Integration with household preferences

### **Collaboration & Communication**
- Real-time activity feeds
- Contextual commenting system
- Notification management
- Household member coordination
- Activity history and audit trails

### **Analytics & Insights**
- Comprehensive nutrition tracking
- Recipe performance analytics
- Household engagement metrics
- Trend analysis and reporting
- Goal setting and achievement tracking

## 🚀 **Quick Start**

### **Prerequisites**
- Node.js 18+
- Docker & Docker Compose
- PostgreSQL (via Docker)
- Redis (via Docker)

### **Installation**

```bash
# Clone the repository
git clone <repository-url>
cd nutreek

# Install dependencies
npm install

# Set up environment variables
cp .env.example .env
# Edit .env with your configuration

# Start the development environment
docker-compose up -d

# Run database migrations
cd backend && npx prisma migrate dev

# Start the development servers
npm run dev
```

### **Access Points**
- **Frontend**: http://localhost:3000
- **Backend API**: http://localhost:4000
- **API Documentation**: http://localhost:4000/api
- **Health Checks**: http://localhost:4000/health

## 📁 **Project Structure**

```
nutreek/
├── frontend/                      # Next.js application
│   ├── app/                       # App Router pages
│   │   ├── (auth)/                # Authentication routes
│   │   ├── (dashboard)/           # Protected routes
│   │   │   ├── households/        # Household management
│   │   │   ├── recipes/           # Recipe management
│   │   │   ├── substitutions/     # Ingredient substitutions
│   │   │   └── ingredients/       # Ingredient database
│   │   └── globals.css            # Global styles
│   ├── components/                # Reusable UI components
│   ├── hooks/                     # React Query hooks
│   ├── lib/                       # Utilities and API client
│   ├── tests/                     # Playwright E2E tests
│   └── playwright.config.ts       # Test configuration
├── backend/                       # NestJS API
│   ├── src/
│   │   ├── auth/                  # Authentication module
│   │   ├── tenants/               # Household management
│   │   ├── plans/                 # Weekly plans
│   │   ├── recipes/               # Recipe creation & management
│   │   ├── ingredients/           # Ingredient database & caching
│   │   ├── substitutions/         # Ingredient substitutions
│   │   ├── shopping-lists/        # Intelligent list generation
│   │   ├── analytics/             # Comprehensive analytics
│   │   ├── collaboration/         # Activity feeds & comments
│   │   ├── health/                # Production monitoring
│   │   ├── prisma/                # Database schema
│   │   └── main.ts                # Application entry point
│   ├── test/                      # Unit tests
│   └── Dockerfile.prod            # Production build
├── scripts/                       # Deployment & monitoring
│   ├── deploy.sh                  # Production deployment
│   └── monitor.sh                 # System monitoring
├── docker-compose.yml             # Development environment
├── docker-compose.prod.yml        # Production environment
├── nginx/                         # Reverse proxy configuration
└── README.md
```

## 🔧 **Development Workflow**

### **TDD Approach**
This project follows strict Test-Driven Development:

1. **Write failing tests first** for new features
2. **Implement minimum code** to make tests pass
3. **Refactor** while maintaining test coverage
4. **Continuous testing** during development

### **Running Tests**
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

## 📡 **API Documentation**

Complete Swagger documentation available at `/api` in development mode.

### **Core API Endpoints**
- **Authentication**: `/auth/*` - User registration and login
- **Tenants**: `/tenants/*` - Household management
- **Weekly Plans**: `/tenants/{id}/plans/*` - Meal planning
- **Recipes**: `/recipes/*` - Recipe creation and management
- **Ingredients**: `/ingredients/*` - Ingredient database and search
- **Shopping Lists**: `/shopping-lists/*` - Automated list generation
- **Analytics**: `/analytics/*` - Comprehensive data insights
- **Collaboration**: `/collaboration/*` - Activity feeds and comments

## 🗄️ **Database Schema**

### **Core Models**
- **User**: Application users with authentication
- **Tenant**: Households/organizations with member management
- **Membership**: User-tenant relationships with roles
- **WeeklyPlan**: Weekly meal plans with date ranges
- **Day**: Individual days within weekly plans
- **Meal**: Specific meals (Breakfast, Lunch, etc.)
- **Recipe**: Detailed recipes with ingredients and nutrition
- **Ingredient**: Ingredient database with nutritional data
- **ShoppingList**: Automatically generated shopping lists
- **Activity**: Audit trail for collaboration features
- **Comment**: Contextual comments on entities

## 🚀 **Deployment**

### **Production Deployment**
```bash
# Automated deployment
./scripts/deploy.sh

# Or manual production build
docker-compose -f docker-compose.prod.yml up -d
```

### **System Monitoring**
```bash
# Health checks
./scripts/monitor.sh check

# Continuous monitoring
./scripts/monitor.sh continuous
```

## 🔒 **Security Features**

- JWT-based authentication with secure tokens
- Rate limiting and DDoS protection
- CORS configuration for cross-origin requests
- Input validation and sanitization
- SQL injection prevention via Prisma ORM
- XSS protection with React's built-in security
- Security headers via Nginx configuration

## 📈 **Performance Optimizations**

- **Redis caching** for API responses and session data
- **Database query optimization** with strategic indexing
- **React Query** for intelligent client-side caching
- **Lazy loading** and code splitting in Next.js
- **Optimized Docker images** with multi-stage builds
- **CDN-ready static asset optimization**

## 🤝 **Contributing**

1. Follow the established TDD workflow
2. Write comprehensive tests for new features
3. Ensure all tests pass before submitting
4. Update documentation and API specs
5. Use conventional commits for version control
6. Maintain code quality and performance standards

## 📄 **License**

This project is licensed under the MIT License.

## 🙏 **Acknowledgments**

- **FatSecret API** for comprehensive nutritional data
- **shadcn/ui** for beautiful, accessible UI components
- **NestJS** and **Next.js** communities for excellent frameworks
- **Prisma** for robust database management
- **Docker** ecosystem for containerization excellence

---

## 🎯 **Ready for Enterprise Use**

Nutreek is now a **production-ready, enterprise-grade nutrition planning platform** featuring:

- ✅ **Advanced Recipe Management**
- ✅ **Intelligent Shopping Lists**
- ✅ **Real-time Collaboration**
- ✅ **Comprehensive Analytics**
- ✅ **Production Deployment**
- ✅ **System Monitoring**
- ✅ **Security Hardening**
- ✅ **Scalable Architecture**

**Perfect for individual users, families, nutritionists, and healthcare organizations!** 🚀✨
