'use client';

import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import apiClient from '../lib/api-client';

interface AddMealDialogProps {
  isOpen: boolean;
  onClose: () => void;
  dayId: string;
  dayName: string;
  date: string;
  tenantId: string;
}

const mealTypes = ['Breakfast', 'Snack', 'Lunch', 'Dinner'];

export default function AddMealDialog({ isOpen, onClose, dayId, dayName, date, tenantId }: AddMealDialogProps) {
  const [selectedMealType, setSelectedMealType] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const queryClient = useQueryClient();

  const handleCreateMeal = async () => {
    if (!selectedMealType) return;

    setIsLoading(true);
    try {
      await apiClient.post(`/days/${dayId}/meals`, {
        type: selectedMealType,
      });

      // Refresh the plans data
      queryClient.invalidateQueries({ queryKey: ['plans', tenantId] });

      onClose();
      setSelectedMealType('');
    } catch (error) {
      console.error('Failed to create meal:', error);
    } finally {
      setIsLoading(false);
    }
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 bg-gray-600 bg-opacity-50 overflow-y-auto h-full w-full z-50">
      <div className="relative top-20 mx-auto p-5 border w-96 shadow-lg rounded-md bg-white">
        <div className="mt-3">
          <h3 className="text-lg font-medium text-gray-900 mb-4">
            Add Meal to {dayName}, {date}
          </h3>

          <div className="mb-4">
            <label className="block text-sm font-medium text-gray-700 mb-2">
              Meal Type
            </label>
            <div className="grid grid-cols-2 gap-3">
              {mealTypes.map((mealType) => (
                <button
                  key={mealType}
                  onClick={() => setSelectedMealType(mealType)}
                  className={`p-3 text-sm font-medium rounded-md border transition-colors ${
                    selectedMealType === mealType
                      ? 'bg-indigo-50 border-indigo-200 text-indigo-700'
                      : 'bg-white border-gray-200 text-gray-700 hover:bg-gray-50'
                  }`}
                >
                  {mealType}
                </button>
              ))}
            </div>
          </div>

          <div className="flex justify-end space-x-3 pt-4">
            <button
              onClick={onClose}
              className="px-4 py-2 bg-gray-300 hover:bg-gray-400 text-gray-800 rounded-md text-sm font-medium"
              disabled={isLoading}
            >
              Cancel
            </button>
            <button
              onClick={handleCreateMeal}
              disabled={!selectedMealType || isLoading}
              className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-md text-sm font-medium disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isLoading ? 'Creating...' : 'Add Meal'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}