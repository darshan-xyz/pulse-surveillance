import React from 'react';
import { render } from '@testing-library/react-native';
import { MainScreen } from '../screens/MainScreen';
import { ApiClient } from '../api/client';

jest.mock('../hooks/useDevices', () => ({
  useDevices: () => ({
    devices: [{ device_id: 'cam-tenant1-01', role: 'admin' }],
    loading: false,
    error: null,
    refresh: jest.fn(),
  }),
}));

jest.mock('../hooks/usePushNotifications', () => ({
  usePushNotifications: () => ({
    pushToken: 'ExponentPushToken[test_token_12345]',
    isRegistered: true,
    lastNotification: {
      id: 'notif-01',
      title: 'Motion Alert: cam-tenant1-01',
      body: 'Motion detected (Confidence 95%)',
      data: { camera_id: 'cam-tenant1-01' },
      receivedAt: Date.now(),
    },
    notifications: [],
    clearLastNotification: jest.fn(),
  }),
}));

describe('MainScreen Component', () => {
  it('renders header, push status bar, active alert, and camera cards', () => {
    const user = { username: 'tenant1_user', tenant_id: 'tenant-1', role: 'admin' };
    const apiClient = new ApiClient('http://127.0.0.1:9091');

    const { getByTestId, getByText } = render(
      <MainScreen
        user={user}
        token="valid_jwt"
        serverUrl="http://127.0.0.1:9091"
        apiClient={apiClient}
        onLogout={jest.fn()}
      />
    );

    expect(getByTestId('push-status-bar')).toBeTruthy();
    expect(getByText('Push Alerts: Active')).toBeTruthy();
    expect(getByTestId('active-alert-banner')).toBeTruthy();
    expect(getByText('Motion Alert: cam-tenant1-01')).toBeTruthy();
    expect(getByText('Motion detected (Confidence 95%)')).toBeTruthy();
  });
});
