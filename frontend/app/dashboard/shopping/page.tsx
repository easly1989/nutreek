'use client';

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { motion } from 'framer-motion';
import { ShoppingCart, Sparkles, CheckSquare } from 'lucide-react';
import { useMe } from '../../../hooks/use-auth';
import { DashboardLayout } from '../../../components/layout/dashboard-layout';
import ShoppingList from '../../../components/ShoppingList';

export default function ShoppingPage() {
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
            Loading your shopping list...
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
            <ShoppingCart className="w-10 h-10 text-white" />
          </motion.div>
          <h1 className="text-display bg-gradient-to-r from-primary to-accent bg-clip-text text-transparent">
            Shopping List
          </h1>
          <p className="text-body-large text-muted-foreground mt-2 max-w-2xl mx-auto">
            Organize your grocery shopping with smart categorization and progress tracking.
            Never forget an item again! 🛒✅
          </p>
        </motion.div>

        {/* Features Overview */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.2 }}
          className="grid grid-cols-1 md:grid-cols-3 gap-6"
        >
          <motion.div
            whileHover={{ scale: 1.02 }}
            className="text-center p-6 bg-card border border-border rounded-xl"
          >
            <div className="w-12 h-12 bg-nutrition-protein/10 rounded-xl flex items-center justify-center mx-auto mb-4">
              <ShoppingCart className="w-6 h-6 text-nutrition-protein" />
            </div>
            <h3 className="font-semibold mb-2">Smart Categories</h3>
            <p className="text-caption text-muted-foreground">
              Automatically organized by grocery store sections for efficient shopping
            </p>
          </motion.div>

          <motion.div
            whileHover={{ scale: 1.02 }}
            className="text-center p-6 bg-card border border-border rounded-xl"
          >
            <div className="w-12 h-12 bg-nutrition-calories/10 rounded-xl flex items-center justify-center mx-auto mb-4">
              <CheckSquare className="w-6 h-6 text-nutrition-calories" />
            </div>
            <h3 className="font-semibold mb-2">Progress Tracking</h3>
            <p className="text-caption text-muted-foreground">
              Mark items as purchased and track your shopping progress in real-time
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
              Automatically generated from your meal plans and recipes
            </p>
          </motion.div>
        </motion.div>

        {/* Shopping List Component */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.4 }}
        >
          <ShoppingList tenantId="default-tenant" />
        </motion.div>
      </div>
    </DashboardLayout>
  );
}