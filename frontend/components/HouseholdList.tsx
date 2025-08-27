'use client';

import { useState } from 'react';
import Link from 'next/link';
import { useTenants, useCreateTenant } from '../hooks/use-tenants';

export default function HouseholdList() {
  const [newHouseholdName, setNewHouseholdName] = useState('');
  const [showCreateForm, setShowCreateForm] = useState(false);
  const { data: tenants, isLoading } = useTenants();
  const createTenant = useCreateTenant();

  const handleCreateHousehold = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newHouseholdName.trim()) return;

    try {
      await createTenant.mutateAsync({ name: newHouseholdName });
      setNewHouseholdName('');
      setShowCreateForm(false);
    } catch (error) {
      console.error('Failed to create household:', error);
    }
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
        <h2 className="text-2xl font-bold text-gray-900">My Households</h2>
        <button
          onClick={() => setShowCreateForm(true)}
          className="bg-indigo-600 hover:bg-indigo-700 text-white px-4 py-2 rounded-md text-sm font-medium"
        >
          Create Household
        </button>
      </div>

      {showCreateForm && (
        <div className="bg-white p-6 rounded-lg shadow">
          <h3 className="text-lg font-medium text-gray-900 mb-4">Create New Household</h3>
          <form onSubmit={handleCreateHousehold}>
            <div className="mb-4">
              <label htmlFor="householdName" className="block text-sm font-medium text-gray-700 mb-1">
                Household Name
              </label>
              <input
                type="text"
                id="householdName"
                value={newHouseholdName}
                onChange={(e) => setNewHouseholdName(e.target.value)}
                className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                placeholder="e.g., Smith Family"
                required
              />
            </div>
            <div className="flex space-x-3">
              <button
                type="submit"
                disabled={createTenant.isPending}
                className="bg-indigo-600 hover:bg-indigo-700 text-white px-4 py-2 rounded-md text-sm font-medium disabled:opacity-50"
              >
                {createTenant.isPending ? 'Creating...' : 'Create'}
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

      {tenants && tenants.length > 0 ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {tenants.map((tenant) => (
            <div key={tenant.id} className="bg-white overflow-hidden shadow rounded-lg">
              <div className="p-6">
                <div className="flex items-center justify-between">
                  <h3 className="text-lg font-medium text-gray-900">{tenant.name}</h3>
                  <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800">
                    {tenant.memberships?.length || 0} members
                  </span>
                </div>
                <p className="mt-2 text-sm text-gray-500">
                  Created {new Date(tenant.createdAt).toLocaleDateString()}
                </p>
                <div className="mt-4 flex space-x-3">
                  <Link
                    href={`/dashboard/households/${tenant.id}`}
                    className="bg-indigo-600 hover:bg-indigo-700 text-white px-3 py-2 rounded-md text-sm font-medium"
                  >
                    View Plans
                  </Link>
                  <button className="bg-gray-100 hover:bg-gray-200 text-gray-800 px-3 py-2 rounded-md text-sm font-medium">
                    Manage
                  </button>
                </div>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="text-center py-12">
          <div className="w-24 h-24 mx-auto bg-gray-100 rounded-full flex items-center justify-center mb-4">
            <svg className="w-12 h-12 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" />
            </svg>
          </div>
          <h3 className="text-lg font-medium text-gray-900 mb-2">No households yet</h3>
          <p className="text-gray-500 mb-6">Create your first household to start planning meals together.</p>
          <button
            onClick={() => setShowCreateForm(true)}
            className="bg-indigo-600 hover:bg-indigo-700 text-white px-6 py-3 rounded-md text-sm font-medium"
          >
            Create Your First Household
          </button>
        </div>
      )}
    </div>
  );
}