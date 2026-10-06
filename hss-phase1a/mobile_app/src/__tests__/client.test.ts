import { ApiClient } from '../api/client';

describe('ApiClient', () => {
  let client: ApiClient;
  let originalFetch: any;

  beforeEach(() => {
    client = new ApiClient('http://127.0.0.1:9091');
    originalFetch = global.fetch;
  });

  afterEach(() => {
    global.fetch = originalFetch;
  });

  it('computes WebSocket URL correctly from HTTP/HTTPS base URL', () => {
    expect(client.getWsUrl()).toBe('ws://127.0.0.1:9091');

    const secureClient = new ApiClient('https://pulse.domain.com:9091');
    expect(secureClient.getWsUrl()).toBe('wss://pulse.domain.com:9091');
  });

  it('successfully logs in with valid credentials', async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        token: 'mock_jwt_token',
        user: { username: 'admin', tenant_id: 'tenant-alpha', role: 'admin' },
      }),
    });

    const res = await client.login({ username: 'admin', password: 'password' });
    expect(res.token).toBe('mock_jwt_token');
    expect(res.user.username).toBe('admin');
    expect(res.user.tenant_id).toBe('tenant-alpha');
  });

  it('throws error when login fails with 401', async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: false,
      status: 401,
      json: async () => ({ error: 'Unauthorized: invalid credentials' }),
    });

    await expect(client.login({ username: 'admin', password: 'wrong' })).rejects.toThrow(
      'Unauthorized: invalid credentials'
    );
  });

  it('fetches multi-tenant devices with Bearer token', async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => [
        { device_id: 'cam-tenant1-01', role: 'admin' },
        { device_id: 'cam-tenant1-02', role: 'viewer' },
      ],
    });

    const devices = await client.getDevices('mock_jwt_token');
    expect(devices).toHaveLength(2);
    expect(devices[0].device_id).toBe('cam-tenant1-01');
    expect(global.fetch).toHaveBeenCalledWith(
      'http://127.0.0.1:9091/api/devices',
      expect.objectContaining({
        headers: expect.objectContaining({
          Authorization: 'Bearer mock_jwt_token',
        }),
      })
    );
  });

  it('registers push notification token with backend', async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ status: 'registered' }),
    });

    const res = await client.registerPushToken('mock_jwt_token', {
      push_token: 'ExponentPushToken[abc123xyz]',
      platform: 'ios',
    });

    expect(res.status).toBe('registered');
    expect(global.fetch).toHaveBeenCalledWith(
      'http://127.0.0.1:9091/api/push/register',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({
          Authorization: 'Bearer mock_jwt_token',
        }),
      })
    );
  });
});
