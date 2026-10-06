import { renderHook, act } from '@testing-library/react-native';
import { useDevices } from '../hooks/useDevices';
import { ApiClient } from '../api/client';

describe('useDevices Hook', () => {
  let apiClient: ApiClient;

  beforeEach(() => {
    apiClient = new ApiClient('http://127.0.0.1:9091');
  });

  it('returns empty list when token is null', () => {
    const { result } = renderHook(() => useDevices(apiClient, null));
    expect(result.current.devices).toEqual([]);
    expect(result.current.loading).toBe(false);
  });

  it('fetches devices when token is provided', async () => {
    jest.spyOn(apiClient, 'getDevices').mockResolvedValue([
      { device_id: 'cam-tenant1-01', role: 'admin' },
      { device_id: 'cam-tenant1-02', role: 'viewer' },
    ]);

    const { result } = renderHook(() => useDevices(apiClient, 'valid_token'));

    await act(async () => {
      await result.current.refresh();
    });

    expect(result.current.devices).toHaveLength(2);
    expect(result.current.devices[0].device_id).toBe('cam-tenant1-01');
    expect(result.current.error).toBeNull();
  });

  it('captures error when device fetch fails', async () => {
    jest.spyOn(apiClient, 'getDevices').mockRejectedValue(new Error('Network error'));

    const { result } = renderHook(() => useDevices(apiClient, 'valid_token'));

    await act(async () => {
      await result.current.refresh();
    });

    expect(result.current.devices).toEqual([]);
    expect(result.current.error).toBe('Network error');
  });
});
