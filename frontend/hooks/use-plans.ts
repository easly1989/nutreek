import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import apiClient from '../lib/api-client';

interface CreatePlanData {
  startDate: string;
}

interface WeeklyPlan {
  id: string;
  tenantId: string;
  startDate: string;
  days: any[];
}

export function usePlans(tenantId: string) {
  return useQuery({
    queryKey: ['plans', tenantId],
    queryFn: async () => {
      const response = await apiClient.get<WeeklyPlan[]>(`/tenants/${tenantId}/plans`);
      return response.data;
    },
    enabled: !!tenantId,
  });
}

export function useCreatePlan() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ tenantId, data }: { tenantId: string; data: CreatePlanData }) => {
      const response = await apiClient.post<WeeklyPlan>(`/tenants/${tenantId}/plans`, data);
      return response.data;
    },
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['plans', data.tenantId] });
    },
  });
}

export function usePlan(tenantId: string, planId: string) {
  return useQuery({
    queryKey: ['plans', tenantId, planId],
    queryFn: async () => {
      const response = await apiClient.get<WeeklyPlan>(`/tenants/${tenantId}/plans/${planId}`);
      return response.data;
    },
    enabled: !!tenantId && !!planId,
  });
}