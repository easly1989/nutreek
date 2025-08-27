'use client';

import { useParams } from 'next/navigation';
import { useTenant } from '../../../../hooks/use-tenants';
import { usePlans } from '../../../../hooks/use-plans';
import WeeklyPlanner from '../../../../components/WeeklyPlanner';
import Link from 'next/link';

export default function HouseholdPage() {
  const params = useParams();
  const tenantId = params.id as string;
  const { data: tenant, isLoading: tenantLoading } = useTenant(tenantId);
  const { data: plans } = usePlans(tenantId);

  if (tenantLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-32 w-32 border-b-2 border-indigo-600 mx-auto"></div>
      </div>
    );
  }

  if (!tenant) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="text-center">
          <h2 className="text-2xl font-bold text-gray-900 mb-4">Household not found</h2>
          <Link href="/dashboard" className="text-indigo-600 hover:text-indigo-800">
            Back to dashboard
          </Link>
        </div>
      </div>
    );
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
                <h1 className="text-3xl font-bold text-gray-900">{tenant.name}</h1>
                <p className="text-gray-600">
                  {tenant.memberships?.length || 0} member{(tenant.memberships?.length || 0) !== 1 ? 's' : ''}
                </p>
              </div>
            </div>
            <div className="flex items-center space-x-4">
              <div className="text-right">
                <div className="text-sm text-gray-500">Active Plans</div>
                <div className="text-2xl font-bold text-gray-900">{plans?.length || 0}</div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div className="max-w-7xl mx-auto py-6 px-4 sm:px-6 lg:px-8">
        <div className="mb-8">
          <div className="bg-white shadow rounded-lg p-6">
            <h2 className="text-xl font-semibold text-gray-900 mb-4">Household Members</h2>
            <div className="space-y-3">
              {tenant.memberships?.map((membership) => (
                <div key={membership.id} className="flex items-center justify-between py-2 border-b border-gray-100 last:border-b-0">
                  <div>
                    <div className="font-medium text-gray-900">{membership.user.name || membership.user.email}</div>
                    <div className="text-sm text-gray-500">{membership.user.email}</div>
                  </div>
                  <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${
                    membership.role === 'admin'
                      ? 'bg-purple-100 text-purple-800'
                      : 'bg-blue-100 text-blue-800'
                  }`}>
                    {membership.role}
                  </span>
                </div>
              )) || (
                <p className="text-gray-500">No members found</p>
              )}
            </div>
          </div>
        </div>

        <WeeklyPlanner tenantId={tenantId} />
      </div>
    </div>
  );
}