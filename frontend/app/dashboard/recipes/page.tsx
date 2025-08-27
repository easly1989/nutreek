'use client';

import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import Link from 'next/link';
import { useMe } from '../../../hooks/use-auth';
import { useTenants } from '../../../hooks/use-tenants';
import RecipeCreator from '../../../components/RecipeCreator';
import apiClient from '../../../lib/api-client';

export default function RecipesPage() {
  const [showCreateForm, setShowCreateForm] = useState(false);
  const { data: user, isLoading: userLoading } = useMe();
  const { data: tenants } = useTenants();

  const { data: recipes, isLoading: recipesLoading } = useQuery({
    queryKey: ['recipes'],
    queryFn: async () => {
      const response = await apiClient.get('/recipes');
      return response.data;
    },
  });

  if (userLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-32 w-32 border-b-2 border-indigo-600 mx-auto"></div>
      </div>
    );
  }

  if (!user) {
    return null;
  }

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="bg-white shadow">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="flex items-center justify-between py-6">
            <div className="flex items-center">
              <Link href="/dashboard" className="text-indigo-600 hover:text-indigo-800 mr-4">
                ← Back to Dashboard
              </Link>
              <div>
                <h1 className="text-3xl font-bold text-gray-900">Recipe Library</h1>
                <p className="text-gray-600">Create and manage your recipe collection</p>
              </div>
            </div>
            <button
              onClick={() => setShowCreateForm(true)}
              className="bg-green-600 hover:bg-green-700 text-white px-4 py-2 rounded-md text-sm font-medium"
            >
              Create Recipe
            </button>
          </div>
        </div>
      </div>

      <div className="max-w-7xl mx-auto py-6 px-4 sm:px-6 lg:px-8">
        {showCreateForm && (
          <div className="mb-8">
            <RecipeCreator
              onSuccess={() => setShowCreateForm(false)}
              onCancel={() => setShowCreateForm(false)}
            />
          </div>
        )}

        <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
          <div className="lg:col-span-2">
            <div className="bg-white shadow rounded-lg">
              <div className="px-6 py-4 border-b border-gray-200">
                <h2 className="text-lg font-medium text-gray-900">
                  Your Recipes ({recipes?.length || 0})
                </h2>
              </div>

              {recipesLoading ? (
                <div className="flex items-center justify-center py-12">
                  <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600"></div>
                </div>
              ) : recipes && recipes.length > 0 ? (
                <div className="divide-y divide-gray-200">
                  {recipes.map((recipe: any) => (
                    <div key={recipe.id} className="p-6 hover:bg-gray-50">
                      <div className="flex items-center justify-between">
                        <div className="flex-1">
                          <h3 className="text-lg font-medium text-gray-900">{recipe.name}</h3>
                          <p className="text-sm text-gray-500">
                            {recipe.ingredients?.length || 0} ingredients
                            {recipe.macros && (
                              <span className="ml-4">
                                • {recipe.macros.kcal} kcal, {recipe.macros.protein}g protein
                              </span>
                            )}
                          </p>
                          {recipe.source && (
                            <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium mt-1 ${
                              recipe.source === 'fatsecret'
                                ? 'bg-blue-100 text-blue-800'
                                : 'bg-green-100 text-green-800'
                            }`}>
                              {recipe.source}
                            </span>
                          )}
                        </div>
                        <div className="flex space-x-2">
                          <button className="text-indigo-600 hover:text-indigo-800 text-sm font-medium">
                            Edit
                          </button>
                          <button className="text-red-600 hover:text-red-800 text-sm font-medium">
                            Delete
                          </button>
                        </div>
                      </div>

                      {recipe.ingredients && recipe.ingredients.length > 0 && (
                        <div className="mt-3">
                          <p className="text-xs text-gray-500 mb-2">Ingredients:</p>
                          <div className="flex flex-wrap gap-2">
                            {recipe.ingredients.slice(0, 5).map((ingredient: any, index: number) => (
                              <span
                                key={index}
                                className="inline-flex items-center px-2 py-0.5 rounded text-xs bg-gray-100 text-gray-800"
                              >
                                {ingredient.amount} {ingredient.unit} {ingredient.name}
                              </span>
                            ))}
                            {recipe.ingredients.length > 5 && (
                              <span className="text-xs text-gray-500">
                                +{recipe.ingredients.length - 5} more
                              </span>
                            )}
                          </div>
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              ) : (
                <div className="text-center py-12">
                  <div className="w-24 h-24 mx-auto bg-gray-100 rounded-full flex items-center justify-center mb-4">
                    <svg className="w-12 h-12 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
                    </svg>
                  </div>
                  <h3 className="text-lg font-medium text-gray-900 mb-2">No recipes yet</h3>
                  <p className="text-gray-500 mb-6">Create your first recipe to get started with meal planning.</p>
                  <button
                    onClick={() => setShowCreateForm(true)}
                    className="bg-indigo-600 hover:bg-indigo-700 text-white px-6 py-3 rounded-md text-sm font-medium"
                  >
                    Create Your First Recipe
                  </button>
                </div>
              )}
            </div>
          </div>

          <div className="space-y-6">
            <div className="bg-white shadow rounded-lg p-6">
              <h3 className="text-lg font-medium text-gray-900 mb-4">Recipe Tips</h3>
              <ul className="space-y-3 text-sm text-gray-600">
                <li className="flex items-start">
                  <span className="w-2 h-2 bg-indigo-500 rounded-full mt-2 mr-3 flex-shrink-0"></span>
                  <span>Use specific ingredient names for better nutritional accuracy</span>
                </li>
                <li className="flex items-start">
                  <span className="w-2 h-2 bg-indigo-500 rounded-full mt-2 mr-3 flex-shrink-0"></span>
                  <span>Include serving size information for accurate macros</span>
                </li>
                <li className="flex items-start">
                  <span className="w-2 h-2 bg-indigo-500 rounded-full mt-2 mr-3 flex-shrink-0"></span>
                  <span>Add multiple ingredients to calculate total nutritional values</span>
                </li>
                <li className="flex items-start">
                  <span className="w-2 h-2 bg-indigo-500 rounded-full mt-2 mr-3 flex-shrink-0"></span>
                  <span>Use recipes in your weekly meal plans for easy meal prep</span>
                </li>
              </ul>
            </div>

            {tenants && tenants.length > 0 && (
              <div className="bg-white shadow rounded-lg p-6">
                <h3 className="text-lg font-medium text-gray-900 mb-4">Quick Actions</h3>
                <div className="space-y-3">
                  {tenants.map((tenant) => (
                    <Link
                      key={tenant.id}
                      href={`/dashboard/households/${tenant.id}`}
                      className="block w-full bg-indigo-600 hover:bg-indigo-700 text-white px-4 py-2 rounded-md text-sm font-medium text-center"
                    >
                      Plan Meals for {tenant.name}
                    </Link>
                  ))}
                  <Link
                    href="/dashboard/substitutions"
                    className="block w-full bg-gray-600 hover:bg-gray-700 text-white px-4 py-2 rounded-md text-sm font-medium text-center"
                  >
                    Manage Substitutions
                  </Link>
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}