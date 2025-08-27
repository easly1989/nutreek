'use client';

import { useState } from 'react';
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
  Trash2
} from 'lucide-react';

interface WeeklyPlannerProps {
  tenantId: string;
}

export default function WeeklyPlanner({ tenantId }: WeeklyPlannerProps) {
  const [showCreateForm, setShowCreateForm] = useState(false);
  const [startDate, setStartDate] = useState('');
  const [showAddMealDialog, setShowAddMealDialog] = useState(false);
  const [selectedDay, setSelectedDay] = useState<{ id: string; name: string; date: string } | null>(null);
  const { data: plans, isLoading } = usePlans(tenantId);
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
    }
  };

  const handleAddMeal = (dayId: string, dayName: string, date: string) => {
    setSelectedDay({ id: dayId, name: dayName, date });
    setShowAddMealDialog(true);
  };

  const getWeekDays = (startDate: string) => {
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
  };

  const mealTypes = ['Breakfast', 'Snack', 'Lunch', 'Dinner'];
  const mealTypeClasses = {
    'Breakfast': 'meal-breakfast',
    'Snack': 'meal-snack',
    'Lunch': 'meal-lunch',
    'Dinner': 'meal-dinner'
  };

  const getMealIcon = (mealType: string) => {
    switch (mealType) {
      case 'Breakfast': return '🌅';
      case 'Snack': return '🍎';
      case 'Lunch': return '☀️';
      case 'Dinner': return '🌙';
      default: return '🍽️';
    }
  };

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
        <div>
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
          className="btn-nutrition whitespace-nowrap"
        >
          <Plus className="w-4 h-4 mr-2" />
          Create New Plan
        </motion.button>
      </motion.div>

      {/* Create Plan Form */}
      <AnimatePresence>
        {showCreateForm && (
          <motion.div
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: 'auto' }}
            exit={{ opacity: 0, height: 0 }}
            className="card-nutrition overflow-hidden"
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
                    className="btn-nutrition flex-1 disabled:opacity-50"
                  >
                    {createPlan.isPending ? 'Creating...' : 'Create Plan'}
                  </motion.button>
                  <motion.button
                    whileHover={{ scale: 1.02 }}
                    whileTap={{ scale: 0.98 }}
                    type="button"
                    onClick={() => setShowCreateForm(false)}
                    className="btn-nutrition-outline flex-1"
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
                className="card-nutrition p-6"
              >
                <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
                  <div>
                    <h3 className="text-heading-2">
                      Week of {new Date(plan.startDate).toLocaleDateString('en-US', {
                        month: 'long',
                        day: 'numeric',
                        year: 'numeric'
                      })}
                    </h3>
                    <p className="text-body text-muted-foreground mt-1">
                      {plan.days?.length || 0} days planned
                    </p>
                  </div>
                  <div className="flex items-center gap-2">
                    <motion.button
                      whileHover={{ scale: 1.05 }}
                      whileTap={{ scale: 0.95 }}
                      className="btn-nutrition-secondary"
                    >
                      <Edit className="w-4 h-4 mr-2" />
                      Edit Plan
                    </motion.button>
                  </div>
                </div>
              </motion.div>

              {/* Days Grid */}
              <div className="grid grid-cols-1 lg:grid-cols-7 gap-4">
                {getWeekDays(plan.startDate).map((dayInfo, dayIndex) => {
                  const day = plan.days?.[dayIndex];

                  return (
                    <motion.div
                      key={dayInfo.date}
                      initial={{ opacity: 0, y: 20 }}
                      animate={{ opacity: 1, y: 0 }}
                      transition={{ delay: dayIndex * 0.05 }}
                      className="space-y-3"
                    >
                      {/* Day Header */}
                      <div className="text-center p-3 bg-muted/50 rounded-lg">
                        <div className="text-heading-3 font-semibold">{dayInfo.shortName}</div>
                        <div className="text-caption text-muted-foreground">{dayInfo.displayDate}</div>
                      </div>

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
                              className={`p-3 rounded-lg border-2 border-dashed border-muted-foreground/20 hover:border-primary/50 transition-all duration-200 group ${
                                meal ? 'bg-muted/30' : 'hover:bg-muted/20'
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
                                  className="w-full flex flex-col items-center justify-center py-4 text-muted-foreground hover:text-primary transition-colors duration-200"
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
            className="w-24 h-24 mx-auto mb-6 bg-gradient-nutrition rounded-2xl flex items-center justify-center"
          >
            <Calendar className="w-12 h-12 text-white" />
          </motion.div>
          <h3 className="text-heading-2 mb-2">No weekly plans yet</h3>
          <p className="text-body-large text-muted-foreground mb-8 max-w-md mx-auto">
            Create your first weekly meal plan to start organizing nutritious meals for your household.
          </p>
          <motion.button
            whileHover={{ scale: 1.05 }}
            whileTap={{ scale: 0.95 }}
            onClick={() => setShowCreateForm(true)}
            className="btn-nutrition px-8 py-4"
          >
            <Plus className="w-5 h-5 mr-2" />
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