import { renderHook, act } from '@testing-library/react-native';
import { useSignaling } from '../hooks/useSignaling';

// Mock WebRTC PeerConnection
class MockRTCPeerConnection {
  ontrack: any = null;
  onicecandidate: any = null;
  onconnectionstatechange: any = null;
  connectionState: string = 'new';

  addTransceiver = jest.fn();
  createOffer = jest.fn().mockResolvedValue({
    type: 'offer',
    sdp: 'v=0\r\no=- 12345 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n',
  });
  setLocalDescription = jest.fn().mockResolvedValue(undefined);
  setRemoteDescription = jest.fn().mockResolvedValue(undefined);
  addIceCandidate = jest.fn().mockResolvedValue(undefined);
  close = jest.fn();
}

// Mock WebSocket
class MockWebSocket {
  static instances: MockWebSocket[] = [];
  url: string;
  readyState: number = 0;
  onopen: any = null;
  onmessage: any = null;
  onerror: any = null;
  onclose: any = null;
  sentMessages: string[] = [];

  constructor(url: string) {
    this.url = url;
    this.readyState = 0; // CONNECTING
    MockWebSocket.instances.push(this);
    setTimeout(() => {
      this.readyState = 1; // OPEN
      if (this.onopen) this.onopen();
    }, 10);
  }

  send(data: string) {
    this.sentMessages.push(data);
  }

  close() {
    this.readyState = 3; // CLOSED
    if (this.onclose) this.onclose();
  }
}

jest.mock('react-native-webrtc', () => ({
  RTCPeerConnection: jest.fn().mockImplementation(() => new MockRTCPeerConnection()),
  RTCSessionDescription: jest.fn().mockImplementation((desc) => desc),
  RTCIceCandidate: jest.fn().mockImplementation((cand) => cand),
  MediaStream: jest.fn().mockImplementation((tracks) => ({
    toURL: () => 'mock_media_stream_url',
    tracks,
  })),
}));

(global as any).WebSocket = MockWebSocket as any;

describe('useSignaling Hook', () => {
  beforeEach(() => {
    MockWebSocket.instances = [];
  });

  it('starts disconnected when enabled is false', () => {
    const { result } = renderHook(() =>
      useSignaling('ws://127.0.0.1:9091', 'token_123', 'cam-01', false)
    );
    expect(result.current.status).toBe('Disconnected');
    expect(result.current.stream).toBeNull();
  });

  it('connects to WebSocket and generates SDP offer with recvonly transceiver', async () => {
    const { result } = renderHook(() =>
      useSignaling('ws://127.0.0.1:9091', 'token_123', 'cam-01', true)
    );

    // Wait for mock WebSocket to open
    await act(async () => {
      await new Promise((r) => setTimeout(r, 30));
    });

    const ws = MockWebSocket.instances[0];
    expect(ws).toBeDefined();
    expect(ws.url).toContain('/client?token=token_123');
    expect(ws.sentMessages.length).toBeGreaterThanOrEqual(1);

    const firstMsg = JSON.parse(ws.sentMessages[0]);
    expect(firstMsg.type).toBe('offer');
    expect(firstMsg.device_id).toBe('cam-01');
    expect(firstMsg.session_id).toBeDefined();
    expect(firstMsg.sdp).toContain('v=0');
  });

  it('processes incoming SDP answer and updates state', async () => {
    const { result } = renderHook(() =>
      useSignaling('ws://127.0.0.1:9091', 'token_123', 'cam-01', true)
    );

    await act(async () => {
      await new Promise((r) => setTimeout(r, 30));
    });

    const ws = MockWebSocket.instances[0];
    await act(async () => {
      if (ws.onmessage) {
        ws.onmessage({
          data: JSON.stringify({
            type: 'answer',
            session_id: 'sess_123',
            sdp: 'v=0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n',
          }),
        } as any);
      }
    });

    expect(result.current.error).toBeNull();
  });

  it('captures Forbidden error message from backend', async () => {
    const { result } = renderHook(() =>
      useSignaling('ws://127.0.0.1:9091', 'token_123', 'cam-01', true)
    );

    await act(async () => {
      await new Promise((r) => setTimeout(r, 30));
    });

    const ws = MockWebSocket.instances[0];
    await act(async () => {
      if (ws.onmessage) {
        ws.onmessage({
          data: JSON.stringify({
            error: 'Forbidden: unauthorized camera access',
          }),
        } as any);
      }
    });

    expect(result.current.error).toBe('Forbidden: unauthorized camera access');
    expect(result.current.status).toBe('Failed');
  });
});
