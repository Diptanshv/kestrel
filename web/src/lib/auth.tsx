import type { ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, type User } from './api'
import { AuthContext, type AuthValue } from './auth-context'

// This file exports only AuthProvider. The context and the useAuth hook live
// in auth-context.ts because react-refresh needs a module to export nothing
// but components for fast refresh to work.
export function AuthProvider({ children }: { children: ReactNode }) {
  const qc = useQueryClient()

  const { data, isLoading } = useQuery({
    queryKey: ['me'],
    queryFn: async () => {
      try {
        return await api.get<User>('/api/auth/me')
      } catch (e) {
        // 401 is the normal "not logged in" answer, not a failure.
        if (e instanceof ApiError && e.status === 401) return null
        throw e
      }
    },
    retry: false,
  })

  const value: AuthValue = {
    user: data ?? null,
    loading: isLoading,
    login: async (email, password) => {
      await api.post('/api/auth/login', { email, password })
      await qc.invalidateQueries({ queryKey: ['me'] })
    },
    register: async (email, password) => {
      await api.post('/api/auth/register', { email, password })
      await api.post('/api/auth/login', { email, password })
      await qc.invalidateQueries({ queryKey: ['me'] })
    },
    logout: async () => {
      await api.post('/api/auth/logout')
      qc.clear()
    },
  }

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
