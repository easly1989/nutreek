import Link from 'next/link'
import { Users, Calendar, ShoppingCart, ArrowRight, Sparkles, ChefHat } from 'lucide-react'

export default function Home() {
  return (
    <main className="min-h-screen bg-background">
      {/* Hero Section */}
      <section className="relative overflow-hidden">
        <div className="absolute inset-0 bg-gradient-to-br from-primary/5 via-transparent to-accent/5"></div>
        <div className="relative max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 pt-20 pb-16">
          <div className="text-center">
            <div className="inline-flex items-center justify-center w-16 h-16 bg-primary/10 rounded-2xl mb-8">
              <ChefHat className="w-8 h-8 text-primary" />
            </div>

            <h1 className="text-display bg-gradient-to-r from-primary to-accent bg-clip-text text-transparent mb-4">
              Nutreek
            </h1>

            <p className="text-xl text-muted-foreground mb-8 max-w-2xl mx-auto">
              Your intelligent nutrition planning companion for healthier, happier households
            </p>

            <div className="flex flex-col sm:flex-row gap-4 justify-center items-center">
              <Link
                href="/auth/login"
                className="btn-nutrition flex items-center gap-2 group"
              >
                Get Started
                <ArrowRight className="w-4 h-4 group-hover:translate-x-1 transition-transform" />
              </Link>
              <Link
                href="/auth/register"
                className="btn-nutrition-secondary flex items-center gap-2"
              >
                <Sparkles className="w-4 h-4" />
                Create Account
              </Link>
            </div>
          </div>
        </div>
      </section>

      {/* Features Section */}
      <section className="py-20 px-4 sm:px-6 lg:px-8">
        <div className="max-w-7xl mx-auto">
          <div className="text-center mb-16">
            <h2 className="text-heading-1 text-foreground mb-4">
              Why Choose Nutreek?
            </h2>
            <p className="text-body-large text-muted-foreground max-w-2xl mx-auto">
              Everything you need for smart nutrition planning in one beautiful, easy-to-use platform
            </p>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-8">
            <div className="card-nutrition hover-lift group p-8">
              <div className="flex items-center gap-4 mb-6">
                <div className="w-12 h-12 bg-primary/10 rounded-xl flex items-center justify-center group-hover:bg-primary/20 transition-colors flex-shrink-0">
                  <Users className="w-6 h-6 text-primary" />
                </div>
                <h3 className="text-xl font-semibold text-foreground">
                  Multi-tenant households
                </h3>
              </div>
              <p className="text-muted-foreground leading-relaxed">
                Manage nutrition planning for your entire household with shared meal plans and collaborative features.
              </p>
            </div>

            <div className="card-nutrition hover-lift group p-8">
              <div className="flex items-center gap-4 mb-6">
                <div className="w-12 h-12 bg-secondary/10 rounded-xl flex items-center justify-center group-hover:bg-secondary/20 transition-colors flex-shrink-0">
                  <Calendar className="w-6 h-6 text-secondary" />
                </div>
                <h3 className="text-xl font-semibold text-foreground">
                  Weekly meal planning
                </h3>
              </div>
              <p className="text-muted-foreground leading-relaxed">
                Plan your meals for the entire week with our intuitive planner interface and smart suggestions.
              </p>
            </div>

            <div className="card-nutrition hover-lift group p-8">
              <div className="flex items-center gap-4 mb-6">
                <div className="w-12 h-12 bg-accent/10 rounded-xl flex items-center justify-center group-hover:bg-accent/20 transition-colors flex-shrink-0">
                  <ShoppingCart className="w-6 h-6 text-accent" />
                </div>
                <h3 className="text-xl font-semibold text-foreground">
                  Smart shopping lists
                </h3>
              </div>
              <p className="text-muted-foreground leading-relaxed">
                Automatically generate shopping lists based on your weekly meal plan with smart categorization.
              </p>
            </div>
          </div>
        </div>
      </section>

      {/* CTA Section */}
      <section className="py-20 px-4 sm:px-6 lg:px-8 bg-muted/30">
        <div className="max-w-4xl mx-auto text-center">
          <div className="card-nutrition p-8 md:p-12">
            <h2 className="text-heading-2 text-foreground mb-4">
              Ready to transform your nutrition planning?
            </h2>
            <p className="text-body-large text-muted-foreground mb-8">
              Join thousands of households who have simplified their meal planning with Nutreek
            </p>
            <div className="flex flex-col sm:flex-row gap-4 justify-center">
              <Link
                href="/auth/login"
                className="btn-nutrition flex items-center gap-2 group"
              >
                Start Planning Today
                <ArrowRight className="w-4 h-4 group-hover:translate-x-1 transition-transform" />
              </Link>
            </div>
          </div>
        </div>
      </section>
    </main>
  )
}