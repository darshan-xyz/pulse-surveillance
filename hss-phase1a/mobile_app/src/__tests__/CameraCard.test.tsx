import React from 'react';
import { render, fireEvent } from '@testing-library/react-native';
import { CameraCard } from '../components/CameraCard';
import * as useSignalingModule from '../hooks/useSignaling';

jest.mock('../hooks/useSignaling');
jest.mock('react-native-webrtc', () => ({
  RTCView: 'RTCView',
}));

describe('CameraCard Component', () => {
  const mockDevice = { device_id: 'cam-tenant1-01', role: 'admin' };

  it('renders camera card in standby mode initially', () => {
    jest.spyOn(useSignalingModule, 'useSignaling').mockReturnValue({
      stream: null,
      error: null,
      status: 'Disconnected',
      reconnect: jest.fn(),
      disconnect: jest.fn(),
    });

    const { getByText, getByTestId } = render(
      <CameraCard device={mockDevice} wsBaseUrl="ws://127.0.0.1:9091" token="token_123" />
    );

    expect(getByTestId('device-id-cam-tenant1-01').props.children).toBe('cam-tenant1-01');
    expect(getByText('Start Stream')).toBeTruthy();
    expect(getByText('Camera Inactive')).toBeTruthy();
  });

  it('starts streaming and shows loading / connecting status when Start Stream pressed', () => {
    jest.spyOn(useSignalingModule, 'useSignaling').mockReturnValue({
      stream: null,
      error: null,
      status: 'Negotiating WebRTC...',
      reconnect: jest.fn(),
      disconnect: jest.fn(),
    });

    const { getByText, getAllByText, getByTestId } = render(
      <CameraCard device={mockDevice} wsBaseUrl="ws://127.0.0.1:9091" token="token_123" />
    );

    fireEvent.press(getByTestId('toggle-stream-btn-cam-tenant1-01'));
    expect(getByText('Stop Stream')).toBeTruthy();
    expect(getAllByText('Negotiating WebRTC...').length).toBeGreaterThanOrEqual(1);
  });

  it('renders error overlay when signaling fails', () => {
    jest.spyOn(useSignalingModule, 'useSignaling').mockReturnValue({
      stream: null,
      error: '403 Forbidden: Tenant Isolation Violation',
      status: 'Failed',
      reconnect: jest.fn(),
      disconnect: jest.fn(),
    });

    const { getByText, getByTestId } = render(
      <CameraCard device={mockDevice} wsBaseUrl="ws://127.0.0.1:9091" token="token_123" />
    );

    fireEvent.press(getByTestId('toggle-stream-btn-cam-tenant1-01'));
    expect(getByText('403 Forbidden: Tenant Isolation Violation')).toBeTruthy();
  });
});
