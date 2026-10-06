import { Platform } from 'react-native';
import { Device, LoginResponse, PushRegistration } from '../types';

export const DEFAULT_SERVER_URL =
  Platform.OS === 'android' ? 'http://10.0.2.2:9091' : 'http://127.0.0.1:9091';

export class ApiClient {
  private baseUrl: string;

  constructor(baseUrl: string = DEFAULT_SERVER_URL) {
    this.baseUrl = baseUrl.replace(/\/+$/, '');
  }

  public setBaseUrl(url: string) {
    this.baseUrl = url.replace(/\/+$/, '');
  }

  public getBaseUrl(): string {
    return this.baseUrl;
  }

  public getWsUrl(): string {
    const cleanUrl = this.baseUrl.replace(/\/+$/, '');
    if (cleanUrl.startsWith('https://')) {
      return cleanUrl.replace('https://', 'wss://');
    }
    if (cleanUrl.startsWith('http://')) {
      return cleanUrl.replace('http://', 'ws://');
    }
    return 'ws://' + cleanUrl;
  }

  async login(credentials: { username: string; password: string }): Promise<LoginResponse> {
    const endpoints = ['/api/auth/login', '/api/login'];
    let lastError: Error | null = null;

    for (const endpoint of endpoints) {
      try {
        const response = await fetch(this.baseUrl + endpoint, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify(credentials),
        });

        if (response.status === 404 && endpoint !== endpoints[endpoints.length - 1]) {
          continue;
        }

        if (!response.ok) {
          const errData = await response.json().catch(() => ({}));
          throw new Error(errData.error || ('Login failed: HTTP ' + response.status));
        }

        const data: LoginResponse = await response.json();
        return data;
      } catch (err: any) {
        lastError = err;
        if (err.message && !err.message.includes('404')) {
          throw err;
        }
      }
    }

    throw lastError || new Error('Network error during login');
  }

  async getDevices(token: string): Promise<Device[]> {
    const response = await fetch(this.baseUrl + '/api/devices', {
      method: 'GET',
      headers: {
        Authorization: 'Bearer ' + token,
        'Content-Type': 'application/json',
      },
    });

    if (!response.ok) {
      const errData = await response.json().catch(() => ({}));
      throw new Error(errData.error || ('Failed to fetch devices: HTTP ' + response.status));
    }

    const data = await response.json();
    return Array.isArray(data) ? data : [];
  }

  async registerPushToken(
    token: string,
    registration: PushRegistration
  ): Promise<{ status: string }> {
    const response = await fetch(this.baseUrl + '/api/push/register', {
      method: 'POST',
      headers: {
        Authorization: 'Bearer ' + token,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(registration),
    });

    if (!response.ok) {
      const errData = await response.json().catch(() => ({}));
      throw new Error(
        errData.error || ('Failed to register push token: HTTP ' + response.status)
      );
    }

    return response.json();
  }
}

export const defaultClient = new ApiClient();
