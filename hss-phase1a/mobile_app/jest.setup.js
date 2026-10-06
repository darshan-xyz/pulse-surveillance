// Mock react-native-webrtc
jest.mock('react-native-webrtc', () => {
  const React = require('react');
  const { View } = require('react-native');
  return {
    RTCPeerConnection: jest.fn().mockImplementation(() => ({
      addTransceiver: jest.fn(),
      createOffer: jest.fn().mockResolvedValue({
        type: 'offer',
        sdp: 'v=0\r\no=- 12345 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n',
      }),
      setLocalDescription: jest.fn().mockResolvedValue(undefined),
      setRemoteDescription: jest.fn().mockResolvedValue(undefined),
      addIceCandidate: jest.fn().mockResolvedValue(undefined),
      close: jest.fn(),
    })),
    RTCSessionDescription: jest.fn().mockImplementation((desc) => desc),
    RTCIceCandidate: jest.fn().mockImplementation((cand) => cand),
    MediaStream: jest.fn().mockImplementation((tracks) => ({
      toURL: () => 'mock_media_stream_url',
      tracks,
    })),
    RTCView: (props) => React.createElement(View, { ...props, testID: props.testID || 'rtc-view' }),
  };
});

// Mock expo-notifications
jest.mock('expo-notifications', () => ({
  setNotificationHandler: jest.fn(),
  setNotificationChannelAsync: jest.fn().mockResolvedValue(undefined),
  getPermissionsAsync: jest.fn().mockResolvedValue({ status: 'granted' }),
  requestPermissionsAsync: jest.fn().mockResolvedValue({ status: 'granted' }),
  getExpoPushTokenAsync: jest.fn().mockResolvedValue({ data: 'ExponentPushToken[mock_token_12345]' }),
  addNotificationReceivedListener: jest.fn().mockReturnValue({ remove: jest.fn() }),
  addNotificationResponseReceivedListener: jest.fn().mockReturnValue({ remove: jest.fn() }),
  AndroidImportance: { MAX: 5, HIGH: 4, DEFAULT: 3 },
}));

// Mock expo-device
jest.mock('expo-device', () => ({
  isDevice: true,
  brand: 'Google',
  modelName: 'Pixel 7',
}));

// Mock expo-constants
jest.mock('expo-constants', () => ({
  expoConfig: {
    extra: {
      eas: {
        projectId: 'mock-eas-project-id',
      },
    },
  },
}));
