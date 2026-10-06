import { useState, useEffect, useCallback, useRef } from 'react';
import { Device } from '../types';
import { ApiClient } from '../api/client';

export interface UseDevicesOptions {
  autoPoll?: boolean;
  pollIntervalMs?: number;
}

export function useDevices(
  apiClient: ApiClient,
  token: string | null,
  options: UseDevicesOptions = {}
) {
  const { autoPoll = false, pollIntervalMs = 5000 } = options;
  const [devices, setDevices] = useState<Device[]>([]);
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const pollTimerRef = useRef<NodeJS.Timeout | null>(null);

  const fetchDevices = useCallback(async () => {
    if (!token) {
      setDevices([]);
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const data = await apiClient.getDevices(token);
      setDevices(data);
    } catch (err: any) {
      setError(err.message || 'Failed to retrieve devices');
    } finally {
      setLoading(false);
    }
  }, [apiClient, token]);

  useEffect(() => {
    if (token) {
      fetchDevices();
    } else {
      setDevices([]);
      setError(null);
    }
  }, [token, fetchDevices]);

  useEffect(() => {
    if (autoPoll && token) {
      pollTimerRef.current = setInterval(() => {
        fetchDevices();
      }, pollIntervalMs);
    }

    return () => {
      if (pollTimerRef.current) {
        clearInterval(pollTimerRef.current);
      }
    };
  }, [autoPoll, token, pollIntervalMs, fetchDevices]);

  return {
    devices,
    loading,
    error,
    refresh: fetchDevices,
  };
}
