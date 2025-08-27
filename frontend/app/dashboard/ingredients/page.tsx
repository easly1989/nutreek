'use client';

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { motion } from 'framer-motion';
import { Search, Sparkles, Target } from 'lucide-react';
import { useMe } from '../../../hooks/use-auth';
import { DashboardLayout } from '../../../components/layout/dashboard-layout';
import IngredientSearch from '../../../components/IngredientSearch';

export default function IngredientsPage() {
  const router = useRouter();
  const { data: user, isLoading: userLoading } = useMe();

  useEffect(() => {
    if (!userLoading && !user) {
      router.push('/auth/login');
    }
  }, [user, userLoading, router]);

  if (userLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="text-center">
          <motion.div
            animate={{ rotate: 360 }}
            transition={{ duration: 2, repeat: Infinity, ease: "linear" }}
            className="w-16 h-16 border-4 border-primary border-t-transparent rounded-full mx-auto"
          />
          <motion.p
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ delay: 0.2 }}
            className="mt-4 text-muted-foreground"
          >
            Loading nutrition database...
          </motion.p>
        </div>
      </div>
    );
  }

  if (!user) {
    return null; // Will redirect in useEffect
  }

  return (
    <DashboardLayout>
      <div className="space-y-8">
        {/* Header */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          className="text-center"
        >
          <motion.div
            animate={{
              scale: [1, 1.1, 1],
              rotate: [0, 10, -10, 0]
            }}
            transition={{
              duration: 2,
              repeat: Infinity,
              ease: "easeInOut"
            }}
            className="w-20 h-20 bg-gradient-nutrition rounded-2xl flex items-center justify-center mx-auto mb-6"
          >
            <Target className="w-10 h-10 text-white" />
          </motion.div>
          <h1 className="text-display bg-gradient-to-r from-primary to-accent bg-clip-text text-transparent">
            Ingredient Search
          </h1>
          <p className="text-body-large text-muted-foreground mt-2 max-w-2xl mx-auto">
            Discover nutritious ingredients with detailed nutritional information.
            Search our comprehensive database to find the perfect ingredients for your recipes! 🥗✨
          </p>
        </motion.div>

        {/* Search Section */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.2 }}
          className="max-w-4xl mx-auto"
        >
          <div className="bg-card border border-border rounded-2xl p-8 shadow-sm">
            <div className="text-center mb-6">
              <h2 className="text-heading-2 mb-2">Search Nutrition Database</h2>
              <p className="text-body text-muted-foreground">
                Type at least 3 characters to start searching
              </p>
            </div>
            <IngredientSearch
              placeholder="Search for any ingredient (e.g., chicken breast, spinach, almonds)..."
              showDetails={true}
            />
          </div>
        </motion.div>

        {/* Features */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.4 }}
          className="grid grid-cols-1 md:grid-cols-3 gap-6"
        >
          <motion.div
            whileHover={{ scale: 1.02 }}
            className="text-center p-6 bg-card border border-border rounded-xl"
          >
            <div className="w-12 h-12 bg-nutrition-protein/10 rounded-xl flex items-center justify-center mx-auto mb-4">
              <Target className="w-6 h-6 text-nutrition-protein" />
            </div>
            <h3 className="font-semibold mb-2">Detailed Nutrition</h3>
            <p className="text-caption text-muted-foreground">
              Get comprehensive nutritional information including macros, vitamins, and minerals
            </p>
          </motion.div>

          <motion.div
            whileHover={{ scale: 1.02 }}
            className="text-center p-6 bg-card border border-border rounded-xl"
          >
            <div className="w-12 h-12 bg-nutrition-calories/10 rounded-xl flex items-center justify-center mx-auto mb-4">
              <Search className="w-6 h-6 text-nutrition-calories" />
            </div>
            <h3 className="font-semibold mb-2">Smart Search</h3>
            <p className="text-caption text-muted-foreground">
              Advanced search with typeahead suggestions and instant results
            </p>
          </motion.div>

          <motion.div
            whileHover={{ scale: 1.02 }}
            className="text-center p-6 bg-card border border-border rounded-xl"
          >
            <div className="w-12 h-12 bg-nutrition-carbs/10 rounded-xl flex items-center justify-center mx-auto mb-4">
              <Sparkles className="w-6 h-6 text-nutrition-carbs" />
            </div>
            <h3 className="font-semibold mb-2">Recipe Integration</h3>
            <p className="text-caption text-muted-foreground">
              Easily add found ingredients to your recipes and meal plans
            </p>
          </motion.div>
        </motion.div>
      </div>
    </DashboardLayout>
  );
}