import React from 'react';
import { render, fireEvent } from '@testing-library/react-native';
import { DeviceGrid } from '../components/DeviceGrid';

jest.mock('../components/CameraCard', () => ({
  CameraCard: ({ device }: any) => {
    const { Text, View } = require('react-native');
    return (
      <View testID={`mock-camera-card-${device.device_id}`}>
        <Text>{device.device_id}</Text>
      </View>
    );
  },
}));

describe('DeviceGrid Component', () => {
  const devices = [
    { device_id: 'cam-tenant1-01', role: 'admin' },
    { device_id: 'cam-tenant1-02', role: 'viewer' },
  ];

  it('renders all authorized camera cards', () => {
    const onRefresh = jest.fn();
    const { getByTestId, getByText } = render(
      <DeviceGrid
        devices={devices}
        loading={false}
        error={null}
        onRefresh={onRefresh}
        wsBaseUrl="ws://127.0.0.1:9091"
        token="mock_jwt"
        tenantId="tenant-1"
      />
    );

    expect(getByText('Authorized Cameras')).toBeTruthy();
    expect(getByTestId('mock-camera-card-cam-tenant1-01')).toBeTruthy();
    expect(getByTestId('mock-camera-card-cam-tenant1-02')).toBeTruthy();
  });

  it('displays empty state when device list is empty', () => {
    const onRefresh = jest.fn();
    const { getByTestId, getByText } = render(
      <DeviceGrid
        devices={[]}
        loading={false}
        error={null}
        onRefresh={onRefresh}
        wsBaseUrl="ws://127.0.0.1:9091"
        token="mock_jwt"
        tenantId="tenant-empty"
      />
    );

    expect(getByTestId('empty-devices-view')).toBeTruthy();
    expect(getByText('No Cameras Found')).toBeTruthy();
  });

  it('triggers refresh when refresh button is pressed', () => {
    const onRefresh = jest.fn();
    const { getByTestId } = render(
      <DeviceGrid
        devices={devices}
        loading={false}
        error={null}
        onRefresh={onRefresh}
        wsBaseUrl="ws://127.0.0.1:9091"
        token="mock_jwt"
        tenantId="tenant-1"
      />
    );

    fireEvent.press(getByTestId('refresh-devices-btn'));
    expect(onRefresh).toHaveBeenCalledTimes(1);
  });
});
