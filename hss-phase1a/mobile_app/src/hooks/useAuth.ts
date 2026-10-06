import { useState, useCallback } from 'react';
import { UserInfo } from '../types';
import { ApiClient, DEFAULT_SERVER_URL } from '../api/client';

export interface UseAuthReturn {
  user: UserInfo | null;
  token: string | null;
  serverUrl: string;
  isAuthenticated: boolean;
  isLoading: boolean;
  error: string | null;
  apiClient: ApiClient;
  login: (username: string, password?: string, customServerUrl?: string) => Promise<boolean>;
  logout: () => void;
  setServerUrl: (url: string) => void;
  clearError: () => void;
}

export function useAuth(initialServerUrl: string = DEFAULT_SERVER_URL): UseAuthReturn {
  const [serverUrl, setServerUrlState] = useState<string>(initialServerUrl);
  const [apiClient] = useState<ApiClient>(() => new ApiClient(initialServerUrl));
  const [user, setUser] = useState<UserInfo | null>(null);
  const [token, setToken] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);

  const setServerUrl = useCallback((url: string) => {
    setServerUrlState(url);
    apiClient.setBaseUrl(url);
  }, [apiClient]);

  const clearError = useCallback(() => {
    setError(null);
  }, []);

  const login = useCallback(
    async (username: string, password: string = 'password', customServerUrl?: string): Promise<boolean> => {
      setIsLoading(true);
      setError(null);

      const targetUrl = customServerUrl || serverUrl;
      if (customServerUrl && customServerUrl !== serverUrl) {
        setServerUrl(customServerUrl);
      }

      try {
        const result = await apiClient.login({ username, password });
        setToken(result.token);
        setUser(result.user || { username, tenant_id: 'default', role: 'viewer' });
        setIsLoading(false);
        return true;
      } catch (err: any) {
        setIsLoading(false);
        const errMsg = err.message || 'Authentication failed';
        setError(errMsg);
        return false;
      }
    },
    [apiClient, serverUrl, setServerUrl]
  );

  const logout = useCallback(() => {
    setUser(null);
    setToken(null);
    setError(null);
  }, []);

  return {
    user,
    token,
    serverUrl,
    isAuthenticated: !!token && !!user,
    isLoading,
    error,
    apiClient,
    login,
    logout,
    setServerUrl,
    clearError,
  };
}
