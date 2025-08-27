'use client';

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { motion } from 'framer-motion';
import { useMe } from '../../../hooks/use-auth';
import { DashboardLayout } from '../../../components/layout/dashboard-layout';
import WeeklyPlanner from '../../../components/WeeklyPlanner';
import PageTransition from '../../../components/PageTransition';

export default function PlannerPage() {
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
            Loading your meal planner...
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
      <PageTransition>
        <div id="page-title" className="sr-only">
          Weekly Meal Planner - Nutreek
        </div>
        <WeeklyPlanner tenantId="default-tenant" />
      </PageTransition>
    </DashboardLayout>
  );
}