import { renderHook, act } from '@testing-library/react-native';
import { useAuth } from '../hooks/useAuth';

describe('useAuth Hook', () => {
  it('initializes in unauthenticated state', () => {
    const { result } = renderHook(() => useAuth('http://127.0.0.1:9091'));
    expect(result.current.isAuthenticated).toBe(false);
    expect(result.current.user).toBeNull();
    expect(result.current.token).toBeNull();
    expect(result.current.error).toBeNull();
  });

  it('updates state on successful login', async () => {
    const { result } = renderHook(() => useAuth('http://127.0.0.1:9091'));

    jest.spyOn(result.current.apiClient, 'login').mockResolvedValue({
      token: 'jwt_mock_token',
      user: { username: 'tenant1_user', tenant_id: 'tenant-1', role: 'admin' },
    });

    let success = false;
    await act(async () => {
      success = await result.current.login('tenant1_user', 'password');
    });

    expect(success).toBe(true);
    expect(result.current.isAuthenticated).toBe(true);
    expect(result.current.user?.username).toBe('tenant1_user');
    expect(result.current.user?.tenant_id).toBe('tenant-1');
    expect(result.current.token).toBe('jwt_mock_token');
  });

  it('handles login failure and records error message', async () => {
    const { result } = renderHook(() => useAuth('http://127.0.0.1:9091'));

    jest.spyOn(result.current.apiClient, 'login').mockRejectedValue(
      new Error('Unauthorized: invalid credentials')
    );

    let success = true;
    await act(async () => {
      success = await result.current.login('bad_user', 'bad_pass');
    });

    expect(success).toBe(false);
    expect(result.current.isAuthenticated).toBe(false);
    expect(result.current.error).toBe('Unauthorized: invalid credentials');
  });

  it('resets state on logout', async () => {
    const { result } = renderHook(() => useAuth('http://127.0.0.1:9091'));

    jest.spyOn(result.current.apiClient, 'login').mockResolvedValue({
      token: 'jwt_token',
      user: { username: 'admin', tenant_id: 'tenant-alpha', role: 'admin' },
    });

    await act(async () => {
      await result.current.login('admin', 'password');
    });

    expect(result.current.isAuthenticated).toBe(true);

    act(() => {
      result.current.logout();
    });

    expect(result.current.isAuthenticated).toBe(false);
    expect(result.current.user).toBeNull();
    expect(result.current.token).toBeNull();
  });
});
