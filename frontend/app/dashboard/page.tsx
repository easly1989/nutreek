'use client';

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { motion } from 'framer-motion';
import {
  Calendar,
  ChefHat,
  Target,
  ShoppingCart,
  Users,
  TrendingUp,
  Award,
  Clock,
  Plus,
  ArrowRight,
  Sparkles
} from 'lucide-react';
import { useMe, useLogout } from '../../hooks/use-auth';
import { DashboardLayout } from '../../components/layout/dashboard-layout';
import HouseholdList from '../../components/HouseholdList';

export default function DashboardPage() {
  const router = useRouter();
  const { data: user, isLoading: userLoading } = useMe();
  const logout = useLogout();

  useEffect(() => {
    if (!userLoading && !user) {
      router.push('/auth/login');
    }
  }, [user, userLoading, router]);

  const handleLogout = async () => {
    await logout.mutateAsync();
    router.push('/');
  };

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
            Loading your nutrition journey...
          </motion.p>
        </div>
      </div>
    );
  }

  if (!user) {
    return null; // Will redirect in useEffect
  }

  const stats = [
    {
      name: 'Weekly Plans',
      value: '0',
      icon: Calendar,
      color: 'nutrition-calories',
      bgColor: 'bg-nutrition-calories/10',
      href: '/dashboard/planner'
    },
    {
      name: 'Recipes',
      value: 'Manage',
      icon: ChefHat,
      color: 'nutrition-carbs',
      bgColor: 'bg-nutrition-carbs/10',
      href: '/dashboard/recipes'
    },
    {
      name: 'Ingredients',
      value: 'Search',
      icon: Target,
      color: 'nutrition-protein',
      bgColor: 'bg-nutrition-protein/10',
      href: '/dashboard/ingredients'
    },
    {
      name: 'Shopping List',
      value: 'View',
      icon: ShoppingCart,
      color: 'nutrition-fat',
      bgColor: 'bg-nutrition-fat/10',
      href: '/dashboard/shopping'
    }
  ];

  const quickActions = [
    {
      name: 'Create Weekly Plan',
      description: 'Plan your meals for the week',
      icon: Calendar,
      color: 'bg-meal-breakfast',
      href: '/dashboard/planner'
    },
    {
      name: 'Add New Recipe',
      description: 'Create a custom recipe',
      icon: ChefHat,
      color: 'bg-meal-lunch',
      href: '/dashboard/recipes'
    },
    {
      name: 'Find Ingredients',
      description: 'Search nutrition database',
      icon: Target,
      color: 'bg-meal-snack',
      href: '/dashboard/ingredients'
    },
    {
      name: 'Manage Household',
      description: 'Add family members',
      icon: Users,
      color: 'bg-meal-dinner',
      href: '/dashboard/households'
    }
  ];

  return (
    <DashboardLayout>
      <div className="space-y-8">
        {/* Welcome Section */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5 }}
          className="bg-gradient-to-r from-primary/10 via-accent/10 to-secondary/10 rounded-2xl p-8 border border-border/50"
        >
          <div className="flex items-center justify-between">
            <div>
              <motion.h1
                initial={{ opacity: 0, x: -20 }}
                animate={{ opacity: 1, x: 0 }}
                transition={{ delay: 0.1, duration: 0.5 }}
                className="text-display bg-gradient-to-r from-primary to-accent bg-clip-text text-transparent"
              >
                Welcome back, {user.name || user.email}! 👋
              </motion.h1>
              <motion.p
                initial={{ opacity: 0, x: -20 }}
                animate={{ opacity: 1, x: 0 }}
                transition={{ delay: 0.2, duration: 0.5 }}
                className="text-body-large text-muted-foreground mt-2"
              >
                Ready to plan your nutrition journey? Let's create something delicious! ✨
              </motion.p>
            </div>
            <motion.div
              initial={{ opacity: 0, scale: 0.8 }}
              animate={{ opacity: 1, scale: 1 }}
              transition={{ delay: 0.3, duration: 0.5 }}
              className="hidden md:block"
            >
              <div className="w-24 h-24 bg-gradient-nutrition rounded-2xl flex items-center justify-center">
                <Sparkles className="w-12 h-12 text-white" />
              </div>
            </motion.div>
          </div>
        </motion.div>

        {/* Stats Grid */}
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
          {stats.map((stat, index) => (
            <motion.div
              key={stat.name}
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ delay: index * 0.1, duration: 0.5 }}
              whileHover={{ scale: 1.02, y: -2 }}
              className="card-nutrition hover-lift"
            >
              <Link href={stat.href} className="block p-6">
                <div className="flex items-center justify-between">
                  <div>
                    <p className="text-caption text-muted-foreground">{stat.name}</p>
                    <p className="text-2xl font-bold mt-1">{stat.value}</p>
                  </div>
                  <div className={`w-12 h-12 ${stat.bgColor} rounded-xl flex items-center justify-center`}>
                    <stat.icon className={`w-6 h-6 ${stat.color}`} />
                  </div>
                </div>
              </Link>
            </motion.div>
          ))}
        </div>

        {/* Quick Actions */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.4, duration: 0.5 }}
          className="space-y-4"
        >
          <h2 className="text-heading-2">Quick Actions</h2>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {quickActions.map((action, index) => (
              <motion.div
                key={action.name}
                initial={{ opacity: 0, x: index % 2 === 0 ? -20 : 20 }}
                animate={{ opacity: 1, x: 0 }}
                transition={{ delay: 0.5 + index * 0.1, duration: 0.5 }}
                whileHover={{ scale: 1.02 }}
                className="card-nutrition hover-lift group"
              >
                <Link href={action.href} className="block p-6">
                  <div className="flex items-center space-x-4">
                    <div className={`w-12 h-12 ${action.color} rounded-xl flex items-center justify-center group-hover:scale-110 transition-transform duration-200`}>
                      <action.icon className="w-6 h-6 text-white" />
                    </div>
                    <div className="flex-1">
                      <h3 className="font-semibold group-hover:text-primary transition-colors duration-200">
                        {action.name}
                      </h3>
                      <p className="text-caption text-muted-foreground mt-1">
                        {action.description}
                      </p>
                    </div>
                    <ArrowRight className="w-5 h-5 text-muted-foreground group-hover:text-primary group-hover:translate-x-1 transition-all duration-200" />
                  </div>
                </Link>
              </motion.div>
            ))}
          </div>
        </motion.div>

        {/* Household Management */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.6, duration: 0.5 }}
          className="space-y-4"
        >
          <div className="flex items-center justify-between">
            <h2 className="text-heading-2">Household Management</h2>
            <motion.button
              whileHover={{ scale: 1.05 }}
              whileTap={{ scale: 0.95 }}
              className="btn-nutrition-secondary"
            >
              <Plus className="w-4 h-4 mr-2" />
              Add Member
            </motion.button>
          </div>
          <HouseholdList />
        </motion.div>
      </div>
    </DashboardLayout>
  );
}