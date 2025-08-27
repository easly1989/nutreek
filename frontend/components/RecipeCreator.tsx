'use client';

import { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import apiClient from '../lib/api-client';
import IngredientSearch from './IngredientSearch';

interface Ingredient {
  id: string;
  name: string;
  unit: string;
  amount: number;
  macros?: {
    kcal: number;
    protein: number;
    carbs: number;
    fat: number;
  };
}

interface RecipeData {
  name: string;
  macros?: {
    kcal: number;
    protein: number;
    carbs: number;
    fat: number;
  };
  source?: string;
}

interface RecipeCreatorProps {
  onSuccess?: () => void;
  onCancel?: () => void;
}

export default function RecipeCreator({ onSuccess, onCancel }: RecipeCreatorProps) {
  const [recipeData, setRecipeData] = useState<RecipeData>({
    name: '',
    macros: {
      kcal: 0,
      protein: 0,
      carbs: 0,
      fat: 0,
    },
  });
  const [ingredients, setIngredients] = useState<Ingredient[]>([]);
  const [selectedIngredient, setSelectedIngredient] = useState<Ingredient | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const queryClient = useQueryClient();

  const createRecipe = useMutation({
    mutationFn: async (data: RecipeData & { ingredients: Ingredient[] }) => {
      const response = await apiClient.post('/recipes', {
        ...data,
        // Convert ingredients to the format expected by the backend
        ingredients: data.ingredients.map(ing => ({
          name: ing.name,
          unit: ing.unit,
          amount: ing.amount,
          macros: ing.macros,
        })),
      });
      return response.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['recipes'] });
      onSuccess?.();
    },
  });

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!recipeData.name.trim() || ingredients.length === 0) return;

    setIsLoading(true);
    try {
      await createRecipe.mutateAsync({
        ...recipeData,
        ingredients,
      });
    } catch (error) {
      console.error('Failed to create recipe:', error);
    } finally {
      setIsLoading(false);
    }
  };

  const handleIngredientSelect = (ingredient: Ingredient) => {
    setIngredients(prev => [...prev, ingredient]);
    setSelectedIngredient(null);
  };

  const handleRemoveIngredient = (index: number) => {
    setIngredients(prev => prev.filter((_, i) => i !== index));
  };

  const handleMacroChange = (macro: 'kcal' | 'protein' | 'carbs' | 'fat', value: string) => {
    const numValue = parseFloat(value) || 0;
    setRecipeData(prev => ({
      ...prev,
      macros: {
        ...prev.macros!,
        [macro]: numValue,
      },
    }));
  };

  const calculateTotalMacros = () => {
    return ingredients.reduce(
      (total, ingredient) => {
        if (ingredient.macros) {
          return {
            kcal: total.kcal + ingredient.macros.kcal,
            protein: total.protein + ingredient.macros.protein,
            carbs: total.carbs + ingredient.macros.carbs,
            fat: total.fat + ingredient.macros.fat,
          };
        }
        return total;
      },
      { kcal: 0, protein: 0, carbs: 0, fat: 0 }
    );
  };

  const totalMacros = calculateTotalMacros();

  return (
    <div className="bg-white p-6 rounded-lg shadow">
      <div className="flex justify-between items-center mb-6">
        <h2 className="text-xl font-semibold text-gray-900">Create New Recipe</h2>
        {onCancel && (
          <button
            onClick={onCancel}
            className="text-gray-400 hover:text-gray-600"
          >
            ✕
          </button>
        )}
      </div>

      <form onSubmit={handleSubmit}>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          <div className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">
                Recipe Name *
              </label>
              <input
                type="text"
                value={recipeData.name}
                onChange={(e) => setRecipeData(prev => ({ ...prev, name: e.target.value }))}
                className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                placeholder="e.g., Chicken Stir Fry"
                required
              />
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 mb-3">
                Nutritional Information (per serving)
              </label>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs text-gray-500 mb-1">Calories</label>
                  <input
                    type="number"
                    value={recipeData.macros?.kcal || ''}
                    onChange={(e) => handleMacroChange('kcal', e.target.value)}
                    className="w-full px-2 py-1 text-sm border border-gray-300 rounded focus:outline-none focus:ring-1 focus:ring-indigo-500"
                    placeholder="0"
                  />
                </div>
                <div>
                  <label className="block text-xs text-gray-500 mb-1">Protein (g)</label>
                  <input
                    type="number"
                    step="0.1"
                    value={recipeData.macros?.protein || ''}
                    onChange={(e) => handleMacroChange('protein', e.target.value)}
                    className="w-full px-2 py-1 text-sm border border-gray-300 rounded focus:outline-none focus:ring-1 focus:ring-indigo-500"
                    placeholder="0"
                  />
                </div>
                <div>
                  <label className="block text-xs text-gray-500 mb-1">Carbs (g)</label>
                  <input
                    type="number"
                    step="0.1"
                    value={recipeData.macros?.carbs || ''}
                    onChange={(e) => handleMacroChange('carbs', e.target.value)}
                    className="w-full px-2 py-1 text-sm border border-gray-300 rounded focus:outline-none focus:ring-1 focus:ring-indigo-500"
                    placeholder="0"
                  />
                </div>
                <div>
                  <label className="block text-xs text-gray-500 mb-1">Fat (g)</label>
                  <input
                    type="number"
                    step="0.1"
                    value={recipeData.macros?.fat || ''}
                    onChange={(e) => handleMacroChange('fat', e.target.value)}
                    className="w-full px-2 py-1 text-sm border border-gray-300 rounded focus:outline-none focus:ring-1 focus:ring-indigo-500"
                    placeholder="0"
                  />
                </div>
              </div>
            </div>
          </div>

          <div className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Add Ingredients
              </label>
              <IngredientSearch
                onSelect={handleIngredientSelect}
                placeholder="Search for ingredients to add..."
                showDetails={false}
              />
            </div>

            {ingredients.length > 0 && (
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  Recipe Ingredients ({ingredients.length})
                </label>
                <div className="space-y-2 max-h-40 overflow-y-auto">
                  {ingredients.map((ingredient, index) => (
                    <div key={index} className="flex items-center justify-between bg-gray-50 p-2 rounded">
                      <div className="flex-1">
                        <span className="text-sm font-medium">{ingredient.name}</span>
                        <span className="text-xs text-gray-500 ml-2">
                          {ingredient.amount} {ingredient.unit}
                        </span>
                      </div>
                      <button
                        type="button"
                        onClick={() => handleRemoveIngredient(index)}
                        className="text-red-500 hover:text-red-700 text-sm"
                      >
                        Remove
                      </button>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {ingredients.length > 0 && (
              <div className="bg-blue-50 p-3 rounded-md">
                <p className="text-sm text-blue-800 mb-1">Calculated from ingredients:</p>
                <div className="text-xs text-blue-700">
                  {Math.round(totalMacros.kcal)} kcal, {totalMacros.protein.toFixed(1)}g protein,
                  {totalMacros.carbs.toFixed(1)}g carbs, {totalMacros.fat.toFixed(1)}g fat
                </div>
              </div>
            )}
          </div>
        </div>

        <div className="flex justify-end space-x-3 mt-6 pt-4 border-t">
          {onCancel && (
            <button
              type="button"
              onClick={onCancel}
              className="px-4 py-2 bg-gray-300 hover:bg-gray-400 text-gray-800 rounded-md text-sm font-medium"
              disabled={isLoading}
            >
              Cancel
            </button>
          )}
          <button
            type="submit"
            disabled={!recipeData.name.trim() || ingredients.length === 0 || isLoading}
            className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-md text-sm font-medium disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isLoading ? 'Creating...' : 'Create Recipe'}
          </button>
        </div>
      </form>
    </div>
  );
}