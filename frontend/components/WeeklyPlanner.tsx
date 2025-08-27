'use client';

import { useState, Component, ReactNode, useMemo, useCallback } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { usePlans, useCreatePlan } from '../hooks/use-plans';
import AddMealDialog from './AddMealDialog';
import {
  Calendar,
  Plus,
  ChefHat,
  TrendingUp,
  Clock,
  Sparkles,
  ArrowRight,
  Edit,
  Trash2,
  AlertTriangle
} from 'lucide-react';

// Error Boundary Component
interface ErrorBoundaryState {
  hasError: boolean;
  error?: Error;
}

interface ErrorBoundaryProps {
  children: ReactNode;
  fallback?: ReactNode;
}

class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  constructor(props: ErrorBoundaryProps) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: any) {
    console.error('WeeklyPlanner Error:', error, errorInfo);
  }

  render() {
    if (this.state.hasError) {
      return this.props.fallback || (
        <motion.div
          initial={{ opacity: 0, scale: 0.9 }}
          animate={{ opacity: 1, scale: 1 }}
          className="text-center py-16"
        >
          <motion.div
            animate={{
              rotate: [0, 10, -10, 0],
              scale: [1, 1.1, 1]
            }}
            transition={{
              duration: 2,
              repeat: Infinity,
              ease: "easeInOut"
            }}
            className="w-24 h-24 mx-auto mb-6 bg-gradient-nutrition rounded-2xl flex items-center justify-center"
          >
            <AlertTriangle className="w-12 h-12 text-white" />
          </motion.div>
          <h3 className="text-heading-2 mb-2">Something went wrong</h3>
          <p className="text-body-large text-muted-foreground mb-8 max-w-md mx-auto">
            There was an issue loading your meal planner. Please try refreshing the page.
          </p>
          <motion.button
            whileHover={{ scale: 1.05 }}
            whileTap={{ scale: 0.95 }}
            onClick={() => window.location.reload()}
            className="btn-nutrition px-8 py-4"
          >
            Refresh Page
          </motion.button>
        </motion.div>
      );
    }

    return this.props.children;
  }
}

interface WeeklyPlannerProps {
  tenantId: string;
}

function WeeklyPlannerContent({ tenantId }: WeeklyPlannerProps) {
  const [showCreateForm, setShowCreateForm] = useState(false);
  const [startDate, setStartDate] = useState('');
  const [showAddMealDialog, setShowAddMealDialog] = useState(false);
  const [selectedDay, setSelectedDay] = useState<{ id: string; name: string; date: string } | null>(null);
  const { data: plans, isLoading, error } = usePlans(tenantId);
  const createPlan = useCreatePlan();

  const handleCreatePlan = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!startDate) return;

    try {
      await createPlan.mutateAsync({
        tenantId,
        data: { startDate: new Date(startDate).toISOString() }
      });
      setStartDate('');
      setShowCreateForm(false);
    } catch (error) {
      console.error('Failed to create plan:', error);
      // Error will be handled by mutation state
    }
  };

  // Handle API errors
  if (error) {
    return (
      <motion.div
        initial={{ opacity: 0, scale: 0.9 }}
        animate={{ opacity: 1, scale: 1 }}
        className="text-center py-16"
      >
        <motion.div
          animate={{
            rotate: [0, 10, -10, 0],
            scale: [1, 1.1, 1]
          }}
          transition={{
            duration: 2,
            repeat: Infinity,
            ease: "easeInOut"
          }}
          className="w-24 h-24 mx-auto mb-6 bg-destructive/20 rounded-2xl flex items-center justify-center"
        >
          <AlertTriangle className="w-12 h-12 text-destructive" />
        </motion.div>
        <h3 className="text-heading-2 mb-2">Unable to load meal plans</h3>
        <p className="text-body-large text-muted-foreground mb-8 max-w-md mx-auto">
          There was an issue connecting to the server. Please check your connection and try again.
        </p>
        <motion.button
          whileHover={{ scale: 1.05 }}
          whileTap={{ scale: 0.95 }}
          onClick={() => window.location.reload()}
          className="btn-nutrition px-8 py-4"
        >
          Try Again
        </motion.button>
      </motion.div>
    );
  }

  const handleAddMeal = useCallback((dayId: string, dayName: string, date: string) => {
    setSelectedDay({ id: dayId, name: dayName, date });
    setShowAddMealDialog(true);
  }, []);

  const getWeekDays = useCallback((startDate: string) => {
    const start = new Date(startDate);
    const days: Array<{ date: string; dayName: string; displayDate: string; shortName: string }> = [];
    for (let i = 0; i < 7; i++) {
      const date = new Date(start);
      date.setDate(start.getDate() + i);
      days.push({
        date: date.toISOString().split('T')[0],
        dayName: date.toLocaleDateString('en-US', { weekday: 'long' }),
        displayDate: date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' }),
        shortName: date.toLocaleDateString('en-US', { weekday: 'short' })
      });
    }
    return days;
  }, []);

  // Memoize static data
  const mealTypes = useMemo(() => ['Breakfast', 'Snack', 'Lunch', 'Dinner'], []);

  const mealTypeClasses = useMemo(() => ({
    'Breakfast': 'bg-gradient-to-br from-yellow-400 via-orange-400 to-red-400 text-white',
    'Snack': 'bg-gradient-to-br from-green-400 via-emerald-400 to-teal-400 text-white',
    'Lunch': 'bg-gradient-to-br from-blue-400 via-cyan-400 to-indigo-400 text-white',
    'Dinner': 'bg-gradient-to-br from-purple-400 via-pink-400 to-rose-400 text-white'
  }), []);

  const getMealIcon = useCallback((mealType: string) => {
    switch (mealType) {
      case 'Breakfast': return '🌅';
      case 'Snack': return '🍎';
      case 'Lunch': return '☀️';
      case 'Dinner': return '🌙';
      default: return '🍽️';
    }
  }, []);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-16">
        <motion.div
          animate={{ rotate: 360 }}
          transition={{ duration: 2, repeat: Infinity, ease: "linear" }}
          className="w-12 h-12 border-4 border-primary border-t-transparent rounded-full"
        />
        <motion.span
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ delay: 0.2 }}
          className="ml-3 text-body"
        >
          Loading your meal plans...
        </motion.span>
      </div>
    );
  }
  
  

  return (
    <div className="space-y-8">
      {/* Header */}
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4"
      >
        <div className="min-w-0 flex-1">
          <h1 className="text-display bg-gradient-to-r from-primary to-accent bg-clip-text text-transparent">
            Weekly Planner
          </h1>
          <p className="text-body-large text-muted-foreground mt-1">
            Plan nutritious meals for your household ✨
          </p>
        </div>
        <motion.button
          whileHover={{ scale: 1.05 }}
          whileTap={{ scale: 0.95 }}
          onClick={() => setShowCreateForm(true)}
          className="btn-nutrition whitespace-nowrap w-full sm:w-auto min-h-[44px] touch-manipulation"
        >
          <Plus className="w-4 h-4 mr-2" />
          <span className="hidden xs:inline">Create New Plan</span>
          <span className="xs:hidden">Create Plan</span>
        </motion.button>
      </motion.div>

      {/* Create Plan Form */}
      <AnimatePresence>
        {showCreateForm && (
          <motion.div
            initial={{ opacity: 0, height: 0, scale: 0.95 }}
            animate={{ opacity: 1, height: 'auto', scale: 1 }}
            exit={{ opacity: 0, height: 0, scale: 0.95 }}
            transition={{ duration: 0.3 }}
            className="bg-gradient-to-br from-white/90 via-primary/5 to-accent/5 backdrop-blur-sm border border-primary/30 rounded-2xl shadow-xl overflow-hidden"
          >
            <div className="p-6">
              <h3 className="text-heading-3 mb-4 flex items-center">
                <Sparkles className="w-5 h-5 mr-2 text-primary" />
                Create New Weekly Plan
              </h3>
              <form onSubmit={handleCreatePlan} className="space-y-4">
                <div>
                  <label htmlFor="startDate" className="block text-sm font-medium mb-2">
                    Start Date (Monday)
                  </label>
                  <input
                    type="date"
                    id="startDate"
                    value={startDate}
                    onChange={(e) => setStartDate(e.target.value)}
                    className="w-full px-4 py-3 bg-muted border border-border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary focus:border-transparent transition-colors duration-200"
                    required
                  />
                  <p className="text-caption text-muted-foreground mt-1">
                    Select the Monday of the week you want to plan
                  </p>
                </div>
                <div className="flex flex-col sm:flex-row gap-3">
                  <motion.button
                    whileHover={{ scale: 1.02 }}
                    whileTap={{ scale: 0.98 }}
                    type="submit"
                    disabled={createPlan.isPending}
                    className="btn-nutrition flex-1 disabled:opacity-50 min-h-[44px] touch-manipulation"
                  >
                    {createPlan.isPending ? 'Creating...' : 'Create Plan'}
                  </motion.button>
                  <motion.button
                    whileHover={{ scale: 1.02 }}
                    whileTap={{ scale: 0.98 }}
                    type="button"
                    onClick={() => setShowCreateForm(false)}
                    className="btn-nutrition-outline flex-1 min-h-[44px] touch-manipulation"
                  >
                    Cancel
                  </motion.button>
                </div>
              </form>
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* Plans List */}
      {plans && plans.length > 0 ? (
        <div className="space-y-8">
          {plans.map((plan, planIndex) => (
            <motion.div
              key={plan.id}
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ delay: planIndex * 0.1 }}
              className="space-y-6"
            >
              {/* Plan Header */}
              <motion.div
                initial={{ opacity: 0, scale: 0.95 }}
                animate={{ opacity: 1, scale: 1 }}
                whileHover={{ scale: 1.01, y: -2 }}
                transition={{ duration: 0.2 }}
                className="bg-gradient-to-r from-primary/10 via-accent/10 to-secondary/10 p-6 rounded-2xl border border-primary/20 hover:border-primary/40 hover:shadow-xl transition-all duration-300"
              >
                <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
                  <div>
                    <h3 className="text-heading-2 bg-gradient-to-r from-primary via-accent to-secondary bg-clip-text text-transparent font-bold">
                      Week of {new Date(plan.startDate).toLocaleDateString('en-US', {
                        month: 'long',
                        day: 'numeric',
                        year: 'numeric'
                      })}
                    </h3>
                    <p className="text-body text-muted-foreground mt-1 font-medium">
                      {plan.days?.length || 0} days planned ✨
                    </p>
                  </div>
                  <div className="flex items-center gap-2">
                    <motion.button
                      whileHover={{ scale: 1.05, y: -1 }}
                      whileTap={{ scale: 0.95 }}
                      className="bg-gradient-to-r from-secondary to-accent text-white font-medium px-6 py-3 rounded-xl shadow-lg hover:shadow-xl transition-all duration-300"
                    >
                      <Edit className="w-4 h-4 mr-2" />
                      Edit Plan
                    </motion.button>
                  </div>
                </div>
              </motion.div>

              {/* Days Grid */}
              <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-7 gap-4">
                {useMemo(() => getWeekDays(plan.startDate), [plan.startDate, getWeekDays]).map((dayInfo, dayIndex) => {
                  const day = plan.days?.[dayIndex];

                  return (
                    <motion.div
                      key={dayInfo.date}
                      initial={{ opacity: 0, y: 20, scale: 0.9 }}
                      animate={{ opacity: 1, y: 0, scale: 1 }}
                      transition={{
                        delay: dayIndex * 0.1,
                        type: "spring",
                        stiffness: 200,
                        damping: 20
                      }}
                      className="space-y-3"
                    >
                      {/* Day Header */}
                      <motion.div
                        whileHover={{ scale: 1.02, y: -2 }}
                        transition={{ duration: 0.2 }}
                        className="text-center p-4 bg-gradient-to-br from-primary/10 via-accent/10 to-secondary/10 rounded-xl min-h-[80px] flex flex-col justify-center border border-primary/20 hover:border-primary/40 hover:shadow-lg transition-all duration-300"
                      >
                        <div className="text-heading-3 font-bold bg-gradient-to-r from-primary to-accent bg-clip-text text-transparent">
                          {dayInfo.shortName}
                        </div>
                        <div className="text-caption text-muted-foreground font-medium">
                          {dayInfo.displayDate}
                        </div>
                      </motion.div>

                      {/* Meals */}
                      <div className="space-y-2">
                        {mealTypes.map((mealType, mealIndex) => {
                          const meal = day?.meals?.find(m => m.type === mealType);

                          return (
                            <motion.div
                              key={mealType}
                              initial={{ opacity: 0, scale: 0.9 }}
                              animate={{ opacity: 1, scale: 1 }}
                              transition={{ delay: mealIndex * 0.1 }}
                              whileHover={{
                                scale: meal ? 1.03 : 1.02,
                                y: meal ? -3 : -2,
                                rotateY: meal ? 2 : 0,
                                boxShadow: "0 20px 25px -5px rgb(0 0 0 / 0.1), 0 10px 10px -5px rgb(0 0 0 / 0.04)",
                                transition: {
                                  type: "spring",
                                  stiffness: 300,
                                  damping: 20,
                                  duration: 0.3
                                }
                              }}
                              whileTap={{ scale: 0.98 }}
                              className={`p-4 rounded-xl border-2 border-dashed border-primary/30 hover:border-primary/60 group overflow-hidden backdrop-blur-sm cursor-pointer ${
                               meal ? 'bg-gradient-to-br from-white/80 to-muted/60 shadow-md hover:shadow-xl' : 'hover:bg-gradient-to-br hover:from-primary/5 hover:to-accent/5'
                             }`}
                            >
                              {meal ? (
                                <motion.div
                                  whileHover={{ scale: 1.02 }}
                                  className="space-y-2"
                                >
                                  <div className="flex items-center justify-between">
                                    <div className="flex items-center space-x-2">
                                      <span className="text-lg">{getMealIcon(mealType)}</span>
                                      <span className="text-sm font-medium">{mealType}</span>
                                    </div>
                                    <motion.button
                                      whileHover={{ scale: 1.1 }}
                                      whileTap={{ scale: 0.9 }}
                                      onClick={() => handleAddMeal(
                                        day.id,
                                        dayInfo.dayName,
                                        dayInfo.date
                                      )}
                                      className="opacity-0 group-hover:opacity-100 p-1 hover:bg-muted rounded transition-all duration-200"
                                    >
                                      <Edit className="w-3 h-3" />
                                    </motion.button>
                                  </div>
                                  <div className="text-caption text-muted-foreground">
                                    {meal.recipes?.length || 0} recipes
                                  </div>
                                  {/* Nutrition Summary */}
                                  <div className="flex items-center justify-between text-xs">
                                    <div className="flex items-center space-x-3">
                                      <div className="flex items-center space-x-1">
                                        <div className="w-2 h-2 bg-nutrition-calories rounded-full"></div>
                                        <span className="nutrition-calories font-medium">
                                          {Math.round(meal.recipes?.reduce((sum, r) => sum + (r.macros?.kcal || 0), 0) || 0)}
                                        </span>
                                      </div>
                                    </div>
                                  </div>
                                </motion.div>
                              ) : (
                                <motion.button
                                  whileHover={{ scale: 1.02 }}
                                  whileTap={{ scale: 0.98 }}
                                  onClick={() => day && handleAddMeal(
                                    day.id,
                                    dayInfo.dayName,
                                    dayInfo.date
                                  )}
                                  className="w-full flex flex-col items-center justify-center py-4 min-h-[80px] text-muted-foreground hover:text-primary transition-colors duration-200 touch-manipulation"
                                >
                                  <span className="text-2xl mb-1">{getMealIcon(mealType)}</span>
                                  <span className="text-sm font-medium">{mealType}</span>
                                  <span className="text-xs mt-1 opacity-70">Add meal</span>
                                </motion.button>
                              )}
                            </motion.div>
                          );
                        })}
                      </div>
                    </motion.div>
                  );
                })}
              </div>
            </motion.div>
          ))}
        </div>
      ) : (
        /* Empty State */
        <motion.div
          initial={{ opacity: 0, scale: 0.9 }}
          animate={{ opacity: 1, scale: 1 }}
          className="text-center py-16"
        >
          <motion.div
            animate={{
              rotate: [0, 10, -10, 0],
              scale: [1, 1.1, 1]
            }}
            transition={{
              duration: 2,
              repeat: Infinity,
              ease: "easeInOut"
            }}
            className="w-32 h-32 mx-auto mb-8 bg-gradient-to-br from-primary via-accent to-secondary rounded-3xl flex items-center justify-center shadow-2xl"
          >
            <Calendar className="w-16 h-16 text-white" />
          </motion.div>
          <motion.h3
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: 0.2 }}
            className="text-heading-1 mb-4 bg-gradient-to-r from-primary via-accent to-secondary bg-clip-text text-transparent font-bold"
          >
            No weekly plans yet
          </motion.h3>
          <motion.p
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: 0.3 }}
            className="text-body-large text-muted-foreground mb-10 max-w-lg mx-auto leading-relaxed"
          >
            Create your first weekly meal plan to start organizing nutritious meals for your household. ✨
          </motion.p>
          <motion.button
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: 0.4 }}
            whileHover={{ scale: 1.05, y: -2 }}
            whileTap={{ scale: 0.95 }}
            onClick={() => setShowCreateForm(true)}
            className="bg-gradient-to-r from-primary via-accent to-secondary text-white font-bold px-10 py-5 rounded-2xl shadow-xl hover:shadow-2xl transition-all duration-300 hover:from-primary/90 hover:via-accent/90 hover:to-secondary/90"
          >
            <Plus className="w-6 h-6 mr-3" />
            Create Your First Plan
          </motion.button>
        </motion.div>
      )}

      {/* Add Meal Dialog */}
      <AnimatePresence>
        {showAddMealDialog && selectedDay && (
          <AddMealDialog
            isOpen={showAddMealDialog}
            onClose={() => {
              setShowAddMealDialog(false);
              setSelectedDay(null);
            }}
            dayId={selectedDay.id}
            dayName={selectedDay.name}
            date={selectedDay.date}
            tenantId={tenantId}
          />
        )}
      </AnimatePresence>
    </div>
  );
}
// Main component wrapped with error boundary
function WeeklyPlanner(props: WeeklyPlannerProps) {
  return (
    <ErrorBoundary>
      <WeeklyPlannerContent {...props} />
    </ErrorBoundary>
  );
}

export default WeeklyPlanner;