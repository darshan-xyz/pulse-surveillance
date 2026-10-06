import React from 'react';
import { render } from '@testing-library/react-native';
import App from '../../App';
import * as useAuthModule from '../hooks/useAuth';
import { ApiClient } from '../api/client';

jest.mock('../hooks/useAuth');
jest.mock('react-native-webrtc', () => ({
  RTCView: 'RTCView',
}));

describe('App Root Integration', () => {
  it('renders LoginScreen when not authenticated', () => {
    jest.spyOn(useAuthModule, 'useAuth').mockReturnValue({
      user: null,
      token: null,
      serverUrl: 'http://127.0.0.1:9091',
      isAuthenticated: false,
      isLoading: false,
      error: null,
      apiClient: new ApiClient('http://127.0.0.1:9091'),
      login: jest.fn(),
      logout: jest.fn(),
      setServerUrl: jest.fn(),
      clearError: jest.fn(),
    });

    const { getByText } = render(<App />);
    expect(getByText('Cloud ABR Engine')).toBeTruthy();
    expect(getByText('Sign In')).toBeTruthy();
  });

  it('renders MainScreen when authenticated', () => {
    jest.spyOn(useAuthModule, 'useAuth').mockReturnValue({
      user: { username: 'tenant1_user', tenant_id: 'tenant-1', role: 'admin' },
      token: 'jwt_valid_token',
      serverUrl: 'http://127.0.0.1:9091',
      isAuthenticated: true,
      isLoading: false,
      error: null,
      apiClient: new ApiClient('http://127.0.0.1:9091'),
      login: jest.fn(),
      logout: jest.fn(),
      setServerUrl: jest.fn(),
      clearError: jest.fn(),
    });

    const { getByTestId, getByText } = render(<App />);
    expect(getByText('Authorized Cameras')).toBeTruthy();
    expect(getByTestId('header-tenant-id').props.children).toBe('tenant-1');
  });
});
