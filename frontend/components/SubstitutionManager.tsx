'use client';

import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import apiClient from '../lib/api-client';

interface Substitution {
  id: string;
  originalId: string;
  substituteId: string;
  user: {
    id: string;
    name?: string;
    email: string;
  };
  original?: {
    id: string;
    name: string;
  };
  substitute?: {
    id: string;
    name: string;
  };
}

export default function SubstitutionManager() {
  const [showCreateForm, setShowCreateForm] = useState(false);
  const [originalIngredient, setOriginalIngredient] = useState('');
  const [substituteIngredient, setSubstituteIngredient] = useState('');
  const [searchQuery, setSearchQuery] = useState('');
  const queryClient = useQueryClient();

  // Fetch user's substitutions
  const { data: substitutions, isLoading } = useQuery({
    queryKey: ['substitutions'],
    queryFn: async () => {
      const response = await apiClient.get<Substitution[]>('/users/substitutions');
      return response.data;
    },
  });

  // Search ingredients
  const { data: searchResults } = useQuery({
    queryKey: ['ingredients-search', searchQuery],
    queryFn: async () => {
      if (!searchQuery.trim()) return [];
      const response = await apiClient.get(`/ingredients/search?q=${encodeURIComponent(searchQuery)}`);
      return response.data;
    },
    enabled: searchQuery.length > 2,
  });

  // Create substitution
  const createSubstitution = useMutation({
    mutationFn: async (data: { originalId: string; substituteId: string }) => {
      const response = await apiClient.post('/users/substitutions', data);
      return response.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['substitutions'] });
      setShowCreateForm(false);
      setOriginalIngredient('');
      setSubstituteIngredient('');
    },
  });

  // Delete substitution
  const deleteSubstitution = useMutation({
    mutationFn: async (id: string) => {
      await apiClient.delete(`/users/substitutions/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['substitutions'] });
    },
  });

  const handleCreateSubstitution = (e: React.FormEvent) => {
    e.preventDefault();
    if (!originalIngredient || !substituteIngredient) return;

    createSubstitution.mutate({
      originalId: originalIngredient,
      substituteId: substituteIngredient,
    });
  };

  const handleIngredientSelect = (ingredientId: string, type: 'original' | 'substitute') => {
    if (type === 'original') {
      setOriginalIngredient(ingredientId);
    } else {
      setSubstituteIngredient(ingredientId);
    }
    setSearchQuery('');
  };

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
        <h2 className="text-2xl font-bold text-gray-900">My Ingredient Substitutions</h2>
        <button
          onClick={() => setShowCreateForm(true)}
          className="bg-indigo-600 hover:bg-indigo-700 text-white px-4 py-2 rounded-md text-sm font-medium"
        >
          Add Substitution
        </button>
      </div>

      {showCreateForm && (
        <div className="bg-white p-6 rounded-lg shadow">
          <h3 className="text-lg font-medium text-gray-900 mb-4">Add New Substitution</h3>
          <form onSubmit={handleCreateSubstitution}>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  Original Ingredient
                </label>
                <div className="relative">
                  <input
                    type="text"
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    placeholder="Search for ingredient..."
                    className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                  />
                  {searchResults && searchResults.length > 0 && (
                    <div className="absolute z-10 w-full bg-white border border-gray-300 rounded-md mt-1 shadow-lg">
                      {searchResults.slice(0, 5).map((ingredient: any) => (
                        <button
                          key={ingredient.id}
                          type="button"
                          onClick={() => handleIngredientSelect(ingredient.id, 'original')}
                          className="w-full px-3 py-2 text-left hover:bg-gray-100 first:rounded-t-md last:rounded-b-md"
                        >
                          {ingredient.name}
                        </button>
                      ))}
                    </div>
                  )}
                </div>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  Substitute Ingredient
                </label>
                <input
                  type="text"
                  value={substituteIngredient}
                  onChange={(e) => setSubstituteIngredient(e.target.value)}
                  placeholder="Substitute ingredient..."
                  className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                />
              </div>
            </div>

            <div className="flex justify-end space-x-3 mt-6">
              <button
                type="button"
                onClick={() => setShowCreateForm(false)}
                className="bg-gray-300 hover:bg-gray-400 text-gray-800 px-4 py-2 rounded-md text-sm font-medium"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={createSubstitution.isPending || !originalIngredient || !substituteIngredient}
                className="bg-indigo-600 hover:bg-indigo-700 text-white px-4 py-2 rounded-md text-sm font-medium disabled:opacity-50"
              >
                {createSubstitution.isPending ? 'Adding...' : 'Add Substitution'}
              </button>
            </div>
          </form>
        </div>
      )}

      {substitutions && substitutions.length > 0 ? (
        <div className="bg-white shadow rounded-lg overflow-hidden">
          <div className="px-6 py-4 border-b border-gray-200">
            <h3 className="text-lg font-medium text-gray-900">Your Substitutions</h3>
          </div>
          <ul className="divide-y divide-gray-200">
            {substitutions.map((substitution) => (
              <li key={substitution.id} className="px-6 py-4">
                <div className="flex items-center justify-between">
                  <div className="flex-1">
                    <div className="flex items-center">
                      <span className="text-sm font-medium text-gray-900">
                        {substitution.original?.name || 'Unknown Ingredient'}
                      </span>
                      <span className="mx-2 text-gray-400">→</span>
                      <span className="text-sm font-medium text-gray-900">
                        {substitution.substitute?.name || 'Unknown Substitute'}
                      </span>
                    </div>
                  </div>
                  <button
                    onClick={() => deleteSubstitution.mutate(substitution.id)}
                    className="text-red-600 hover:text-red-800 text-sm font-medium"
                    disabled={deleteSubstitution.isPending}
                  >
                    Remove
                  </button>
                </div>
              </li>
            ))}
          </ul>
        </div>
      ) : (
        <div className="text-center py-12">
          <div className="w-24 h-24 mx-auto bg-gray-100 rounded-full flex items-center justify-center mb-4">
            <svg className="w-12 h-12 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 16V4m0 0L3 8m4-4l4 4m6 0v12m0 0l4-4m-4 4l-4-4" />
            </svg>
          </div>
          <h3 className="text-lg font-medium text-gray-900 mb-2">No substitutions yet</h3>
          <p className="text-gray-500 mb-6">Create ingredient substitutions for your meal planning preferences.</p>
          <button
            onClick={() => setShowCreateForm(true)}
            className="bg-indigo-600 hover:bg-indigo-700 text-white px-6 py-3 rounded-md text-sm font-medium"
          >
            Create Your First Substitution
          </button>
        </div>
      )}
    </div>
  );
}