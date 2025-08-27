import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import apiClient from '../lib/api-client';

interface CreateTenantData {
  name: string;
}

interface Tenant {
  id: string;
  name: string;
  memberships: any[];
}

export function useTenants() {
  return useQuery({
    queryKey: ['tenants'],
    queryFn: async () => {
      const response = await apiClient.get<Tenant[]>('/tenants');
      return response.data;
    },
  });
}

export function useCreateTenant() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (data: CreateTenantData) => {
      const response = await apiClient.post<Tenant>('/tenants', data);
      return response.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tenants'] });
    },
  });
}

export function useTenant(id: string) {
  return useQuery({
    queryKey: ['tenants', id],
    queryFn: async () => {
      const response = await apiClient.get<Tenant>(`/tenants/${id}`);
      return response.data;
    },
    enabled: !!id,
  });
}