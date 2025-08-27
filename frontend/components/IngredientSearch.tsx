'use client';

import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import apiClient from '../lib/api-client';

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
  source?: string;
}

interface IngredientSearchProps {
  onSelect?: (ingredient: Ingredient) => void;
  placeholder?: string;
  showDetails?: boolean;
}

export default function IngredientSearch({
  onSelect,
  placeholder = "Search for ingredients...",
  showDetails = true
}: IngredientSearchProps) {
  const [query, setQuery] = useState('');
  const [selectedIngredient, setSelectedIngredient] = useState<Ingredient | null>(null);

  const { data: ingredients, isLoading, error } = useQuery({
    queryKey: ['ingredients-search', query],
    queryFn: async () => {
      if (!query.trim()) return [];
      const response = await apiClient.get<Ingredient[]>(`/ingredients/search?q=${encodeURIComponent(query)}`);
      return response.data;
    },
    enabled: query.length > 2,
  });

  const handleIngredientClick = (ingredient: Ingredient) => {
    setSelectedIngredient(ingredient);
    onSelect?.(ingredient);
  };

  return (
    <div className="space-y-4">
      <div className="relative">
        <input
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={placeholder}
          className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
        />

        {isLoading && query.length > 2 && (
          <div className="absolute right-3 top-2">
            <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-indigo-600"></div>
          </div>
        )}

        {ingredients && ingredients.length > 0 && (
          <div className="absolute z-10 w-full bg-white border border-gray-300 rounded-md mt-1 shadow-lg max-h-64 overflow-y-auto">
            {ingredients.map((ingredient) => (
              <button
                key={ingredient.id}
                onClick={() => handleIngredientClick(ingredient)}
                className="w-full px-4 py-3 text-left hover:bg-gray-50 border-b border-gray-100 last:border-b-0"
              >
                <div className="flex items-center justify-between">
                  <div>
                    <div className="font-medium text-gray-900">{ingredient.name}</div>
                    {showDetails && ingredient.macros && (
                      <div className="text-sm text-gray-500">
                        {ingredient.macros.kcal} kcal, {ingredient.macros.protein}g protein
                      </div>
                    )}
                  </div>
                  {ingredient.source && (
                    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
                      ingredient.source === 'fatsecret'
                        ? 'bg-blue-100 text-blue-800'
                        : 'bg-green-100 text-green-800'
                    }`}>
                      {ingredient.source}
                    </span>
                  )}
                </div>
              </button>
            ))}
          </div>
        )}

        {error && (
          <div className="absolute z-10 w-full bg-red-50 border border-red-200 rounded-md mt-1 p-3">
            <p className="text-sm text-red-600">
              Error searching ingredients. Please try again.
            </p>
          </div>
        )}
      </div>

      {selectedIngredient && showDetails && (
        <div className="bg-gray-50 p-4 rounded-md">
          <h4 className="font-medium text-gray-900 mb-2">Selected Ingredient</h4>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
            <div>
              <span className="text-gray-500">Name:</span>
              <p className="font-medium">{selectedIngredient.name}</p>
            </div>
            <div>
              <span className="text-gray-500">Amount:</span>
              <p className="font-medium">{selectedIngredient.amount} {selectedIngredient.unit}</p>
            </div>
            {selectedIngredient.macros && (
              <>
                <div>
                  <span className="text-gray-500">Calories:</span>
                  <p className="font-medium">{selectedIngredient.macros.kcal}</p>
                </div>
                <div>
                  <span className="text-gray-500">Protein:</span>
                  <p className="font-medium">{selectedIngredient.macros.protein}g</p>
                </div>
              </>
            )}
          </div>
          {selectedIngredient.macros && (
            <div className="grid grid-cols-2 gap-4 mt-2 text-sm">
              <div>
                <span className="text-gray-500">Carbs:</span>
                <p className="font-medium">{selectedIngredient.macros.carbs}g</p>
              </div>
              <div>
                <span className="text-gray-500">Fat:</span>
                <p className="font-medium">{selectedIngredient.macros.fat}g</p>
              </div>
            </div>
          )}
        </div>
      )}

      {query.length > 0 && query.length <= 2 && (
        <p className="text-sm text-gray-500">Type at least 3 characters to search</p>
      )}
    </div>
  );
}