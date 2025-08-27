import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import apiClient from '../lib/api-client';

interface RegisterData {
  email: string;
  name?: string;
}

interface LoginData {
  email: string;
}

interface AuthResponse {
  access_token: string;
  user: {
    id: string;
    email: string;
    name?: string;
  };
}

export function useRegister() {
  return useMutation({
    mutationFn: async (data: RegisterData) => {
      const response = await apiClient.post<AuthResponse>('/auth/register', data);
      return response.data;
    },
    onSuccess: (data) => {
      localStorage.setItem('auth_token', data.access_token);
    },
  });
}

export function useLogin() {
  return useMutation({
    mutationFn: async (data: LoginData) => {
      const response = await apiClient.post<AuthResponse>('/auth/login', data);
      return response.data;
    },
    onSuccess: (data) => {
      localStorage.setItem('auth_token', data.access_token);
    },
  });
}

export function useMe() {
  return useQuery({
    queryKey: ['me'],
    queryFn: async () => {
      const response = await apiClient.get('/auth/me');
      return response.data;
    },
    retry: false,
  });
}

export function useLogout() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      localStorage.removeItem('auth_token');
    },
    onSuccess: () => {
      queryClient.clear();
    },
  });
}