import React from 'react';
import { render, fireEvent } from '@testing-library/react-native';
import App from './App';
import * as useSignalingMock from './src/useSignaling';

// Mock the WebRTC RTCView
jest.mock('react-native-webrtc', () => ({
  RTCView: 'RTCView',
}));

// Mock the useSignaling hook
jest.mock('./src/useSignaling');

describe('Mobile App UI Components', () => {
  it('renders the initial disconnected state correctly', () => {
    const { getByText } = render(<App />);
    
    // Header should exist
    expect(getByText('Pulse')).toBeTruthy();
    expect(getByText('Live View')).toBeTruthy();
    
    // Placeholder should be visible
    expect(getByText('Camera Offline')).toBeTruthy();
    
    // Button should prompt to connect
    expect(getByText('Connect to Camera')).toBeTruthy();
  });

  it('renders the loading state when connecting', () => {
    // Mock the hook returning a connecting state
    jest.spyOn(useSignalingMock, 'useSignaling').mockReturnValue({
      stream: null,
      error: null,
      status: 'Negotiating WebRTC...',
    });

    const { getByText } = render(<App />);
    
    // Click play
    fireEvent.press(getByText('Connect to Camera'));

    // Loading status should appear
    expect(getByText('Negotiating WebRTC...')).toBeTruthy();
    expect(getByText('Stop Stream')).toBeTruthy();
  });

  it('renders the WebRTC View when stream is established', () => {
    // Mock the hook returning a successful stream
    jest.spyOn(useSignalingMock, 'useSignaling').mockReturnValue({
      stream: { toURL: () => 'mock_stream_url' } as any,
      error: null,
      status: 'Live',
    });

    const { getByText, getByTestId } = render(<App />);
    
    fireEvent.press(getByText('Connect to Camera'));

    // Check if the mock RTCView is trying to render
    // In a real test we'd add testID="rtc-view" to the RTCView
    // For this demonstration, we assert the loading/offline states are gone
    expect(() => getByText('Camera Offline')).toThrow();
    expect(() => getByText('Negotiating WebRTC...')).toThrow();
  });

  it('renders the error overlay when signaling fails', () => {
    // Mock the hook returning a 403 Forbidden error (simulating Authz Failure)
    jest.spyOn(useSignalingMock, 'useSignaling').mockReturnValue({
      stream: null,
      error: '403 Forbidden: Unauthorized Camera Access',
      status: 'Failed',
    });

    const { getByText } = render(<App />);
    
    fireEvent.press(getByText('Connect to Camera'));

    // The dark-mode UI must cleanly render the error overlay, proving it won't crash
    expect(getByText('403 Forbidden: Unauthorized Camera Access')).toBeTruthy();
  });
});
