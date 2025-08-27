'use client';

import { useState } from 'react';
import { usePlans, useCreatePlan } from '../hooks/use-plans';
import AddMealDialog from './AddMealDialog';

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
    const days: Array<{ date: string; dayName: string; displayDate: string }> = [];
    for (let i = 0; i < 7; i++) {
      const date = new Date(start);
      date.setDate(start.getDate() + i);
      days.push({
        date: date.toISOString().split('T')[0],
        dayName: date.toLocaleDateString('en-US', { weekday: 'long' }),
        displayDate: date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
      });
    }
    return days;
  };

  const mealTypes = ['Breakfast', 'Snack', 'Lunch', 'Dinner'];

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600"></div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h2 className="text-2xl font-bold text-gray-900">Weekly Meal Plans</h2>
        <button
          onClick={() => setShowCreateForm(true)}
          className="bg-green-600 hover:bg-green-700 text-white px-4 py-2 rounded-md text-sm font-medium"
        >
          Create New Plan
        </button>
      </div>

      {showCreateForm && (
        <div className="bg-white p-6 rounded-lg shadow">
          <h3 className="text-lg font-medium text-gray-900 mb-4">Create New Weekly Plan</h3>
          <form onSubmit={handleCreatePlan}>
            <div className="mb-4">
              <label htmlFor="startDate" className="block text-sm font-medium text-gray-700 mb-1">
                Start Date (Monday)
              </label>
              <input
                type="date"
                id="startDate"
                value={startDate}
                onChange={(e) => setStartDate(e.target.value)}
                className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                required
              />
              <p className="text-sm text-gray-500 mt-1">Select the Monday of the week you want to plan</p>
            </div>
            <div className="flex space-x-3">
              <button
                type="submit"
                disabled={createPlan.isPending}
                className="bg-green-600 hover:bg-green-700 text-white px-4 py-2 rounded-md text-sm font-medium disabled:opacity-50"
              >
                {createPlan.isPending ? 'Creating...' : 'Create Plan'}
              </button>
              <button
                type="button"
                onClick={() => setShowCreateForm(false)}
                className="bg-gray-300 hover:bg-gray-400 text-gray-800 px-4 py-2 rounded-md text-sm font-medium"
              >
                Cancel
              </button>
            </div>
          </form>
        </div>
      )}

      {plans && plans.length > 0 ? (
        <div className="space-y-6">
          {plans.map((plan) => (
            <div key={plan.id} className="bg-white shadow rounded-lg overflow-hidden">
              <div className="px-6 py-4 bg-gray-50 border-b border-gray-200">
                <div className="flex justify-between items-center">
                  <h3 className="text-lg font-medium text-gray-900">
                    Week of {new Date(plan.startDate).toLocaleDateString('en-US', { month: 'long', day: 'numeric', year: 'numeric' })}
                  </h3>
                  <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-800">
                    {plan.days?.length || 0} days planned
                  </span>
                </div>
              </div>

              <div className="overflow-x-auto">
                <table className="min-w-full divide-y divide-gray-200">
                  <thead className="bg-gray-50">
                    <tr>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Day
                      </th>
                      {mealTypes.map((mealType) => (
                        <th key={mealType} className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                          {mealType}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody className="bg-white divide-y divide-gray-200">
                    {plan.days && plan.days.length > 0 ? (
                      plan.days.map((day, index) => (
                        <tr key={day.id}>
                          <td className="px-6 py-4 whitespace-nowrap">
                            <div className="text-sm font-medium text-gray-900">
                              {getWeekDays(plan.startDate)[index]?.dayName}
                            </div>
                            <div className="text-sm text-gray-500">
                              {getWeekDays(plan.startDate)[index]?.displayDate}
                            </div>
                          </td>
                          {mealTypes.map((mealType) => {
                            const meal = day.meals?.find(m => m.type === mealType);
                            return (
                              <td key={mealType} className="px-6 py-4 whitespace-nowrap">
                                {meal ? (
                                  <div className="text-sm">
                                    <div className="font-medium text-gray-900">
                                      {meal.recipes?.length || 0} recipes
                                    </div>
                                    <button className="text-indigo-600 hover:text-indigo-900 text-xs">
                                      Edit
                                    </button>
                                  </div>
                                ) : (
                                  <button
                                    onClick={() => handleAddMeal(
                                      day.id,
                                      getWeekDays(plan.startDate)[index]?.dayName || '',
                                      getWeekDays(plan.startDate)[index]?.date || ''
                                    )}
                                    className="text-gray-400 hover:text-gray-600 text-sm underline"
                                  >
                                    Add meal
                                  </button>
                                )}
                              </td>
                            );
                          })}
                        </tr>
                      ))
                    ) : (
                      <tr>
                        <td colSpan={mealTypes.length + 1} className="px-6 py-8 text-center text-sm text-gray-500">
                          No days planned yet. Start by adding meals to your plan.
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="text-center py-12">
          <div className="w-24 h-24 mx-auto bg-gray-100 rounded-full flex items-center justify-center mb-4">
            <svg className="w-12 h-12 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
            </svg>
          </div>
          <h3 className="text-lg font-medium text-gray-900 mb-2">No weekly plans yet</h3>
          <p className="text-gray-500 mb-6">Create your first weekly meal plan to get started.</p>
          <button
            onClick={() => setShowCreateForm(true)}
            className="bg-green-600 hover:bg-green-700 text-white px-6 py-3 rounded-md text-sm font-medium"
          >
            Create Your First Plan
          </button>
        </div>
      )}

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
    </div>
  );
}