// @ts-nocheck
import '@testing-library/jest-dom';

// Mock TimeRanges for HTMLVideoElement.buffered
class MockTimeRanges {
  constructor(ranges = []) {
    this._ranges = ranges; // array of [start, end]
  }
  get length() {
    return this._ranges.length;
  }
  start(index) {
    if (index < 0 || index >= this._ranges.length) throw new DOMException('IndexSizeError');
    return this._ranges[index][0];
  }
  end(index) {
    if (index < 0 || index >= this._ranges.length) throw new DOMException('IndexSizeError');
    return this._ranges[index][1];
  }
}

// Stub HTMLVideoElement media methods and buffer management for JSDOM
if (typeof HTMLVideoElement !== 'undefined') {
  Object.defineProperty(HTMLVideoElement.prototype, 'paused', {
    get() {
      return this._paused !== undefined ? this._paused : false;
    },
    set(val) {
      this._paused = Boolean(val);
    },
    configurable: true,
  });

  Object.defineProperty(HTMLVideoElement.prototype, 'seeking', {
    get() {
      return this._seeking !== undefined ? this._seeking : false;
    },
    set(val) {
      this._seeking = Boolean(val);
    },
    configurable: true,
  });

  HTMLVideoElement.prototype.play = function () {
    this._paused = false;
    return Promise.resolve();
  };

  HTMLVideoElement.prototype.pause = function () {
    this._paused = true;
  };

  HTMLVideoElement.prototype.load = function () {};

  Object.defineProperty(HTMLVideoElement.prototype, 'buffered', {
    get() {
      if (!this._bufferedRanges) this._bufferedRanges = [];
      return new MockTimeRanges(this._bufferedRanges);
    },
    set(val) {
      if (val && typeof val.length === 'number') {
        this._bufferedRanges = [];
        for (let i = 0; i < val.length; i++) {
          this._bufferedRanges.push([val.start(i), val.end(i)]);
        }
      } else if (Array.isArray(val)) {
        this._bufferedRanges = val;
      }
    },
    configurable: true,
  });

  Object.defineProperty(HTMLVideoElement.prototype, 'currentTime', {
    get() {
      return this._currentTime !== undefined ? this._currentTime : 0;
    },
    set(val) {
      this._currentTime = Number(val);
    },
    configurable: true,
  });

  Object.defineProperty(HTMLVideoElement.prototype, 'playbackRate', {
    get() {
      return this._playbackRate !== undefined ? this._playbackRate : 1.0;
    },
    set(val) {
      this._playbackRate = Number(val);
    },
    configurable: true,
  });

  Object.defineProperty(HTMLVideoElement.prototype, 'srcObject', {
    get() {
      return this._srcObject !== undefined ? this._srcObject : null;
    },
    set(val) {
      this._srcObject = val;
    },
    configurable: true,
  });

  HTMLVideoElement.prototype.simulateBuffer = function (start, end, current) {
    this._bufferedRanges = [[start, end]];
    if (current !== undefined) {
      this._currentTime = current;
    }
  };

  HTMLVideoElement.prototype.clearBuffer = function () {
    this._bufferedRanges = [];
    this._currentTime = 0;
    this._playbackRate = 1.0;
  };

  HTMLVideoElement.prototype.requestVideoFrameCallback = function (cb) {
    return setTimeout(() => {
      cb(performance.now(), {
        presentationTime: performance.now(),
        expectedDisplayTime: performance.now() + 16.6,
        width: this.videoWidth || 1280,
        height: this.videoHeight || 720,
        mediaTime: this.currentTime || 0,
        presentedFrames: 1,
        processingDuration: 0.005,
      });
    }, 16);
  };

  HTMLVideoElement.prototype.cancelVideoFrameCallback = function (handle) {
    clearTimeout(handle);
  };
}

// Mock HTMLCanvasElement 2D Context for Canvas Overlay Tests
class MockCanvasRenderingContext2D {
  constructor(canvas) {
    this.canvas = canvas;
    this.fillStyle = '#000000';
    this.strokeStyle = '#000000';
    this.lineWidth = 1;
    this.font = '10px sans-serif';
    this.textAlign = 'start';
    this.textBaseline = 'alphabetic';
    this.globalAlpha = 1.0;
    this._calls = [];
  }

  clearRect(x, y, w, h) {
    this._calls.push({ method: 'clearRect', args: [x, y, w, h] });
  }

  fillRect(x, y, w, h) {
    this._calls.push({ method: 'fillRect', args: [x, y, w, h], fillStyle: this.fillStyle });
  }

  strokeRect(x, y, w, h) {
    this._calls.push({ method: 'strokeRect', args: [x, y, w, h], strokeStyle: this.strokeStyle, lineWidth: this.lineWidth });
  }

  fillText(text, x, y, maxWidth) {
    this._calls.push({ method: 'fillText', args: [text, x, y, maxWidth], fillStyle: this.fillStyle, font: this.font });
  }

  strokeText(text, x, y, maxWidth) {
    this._calls.push({ method: 'strokeText', args: [text, x, y, maxWidth], strokeStyle: this.strokeStyle, font: this.font });
  }

  measureText(text) {
    const textStr = String(text || '');
    return {
      width: textStr.length * 8,
      actualBoundingBoxAscent: 10,
      actualBoundingBoxDescent: 2,
    };
  }

  beginPath() {
    this._calls.push({ method: 'beginPath' });
  }

  closePath() {
    this._calls.push({ method: 'closePath' });
  }

  moveTo(x, y) {
    this._calls.push({ method: 'moveTo', args: [x, y] });
  }

  lineTo(x, y) {
    this._calls.push({ method: 'lineTo', args: [x, y] });
  }

  stroke() {
    this._calls.push({ method: 'stroke', strokeStyle: this.strokeStyle });
  }

  fill() {
    this._calls.push({ method: 'fill', fillStyle: this.fillStyle });
  }

  rect(x, y, w, h) {
    this._calls.push({ method: 'rect', args: [x, y, w, h] });
  }

  roundRect(x, y, w, h, radii) {
    this._calls.push({ method: 'roundRect', args: [x, y, w, h, radii] });
  }

  save() {
    this._calls.push({ method: 'save' });
  }

  restore() {
    this._calls.push({ method: 'restore' });
  }

  setLineDash(segments) {
    this._calls.push({ method: 'setLineDash', args: [segments] });
  }

  getLineDash() {
    return [];
  }

  drawImage() {}
  createImageData() { return {}; }
  getImageData() { return { data: new Uint8ClampedArray(4) }; }
  putImageData() {}
}

if (typeof HTMLCanvasElement !== 'undefined') {
  HTMLCanvasElement.prototype.getContext = function (contextType) {
    if (contextType === '2d') {
      if (!this._context2d) {
        this._context2d = new MockCanvasRenderingContext2D(this);
      }
      return this._context2d;
    }
    return null;
  };
}


// Mock navigator.mediaDevices
if (typeof navigator !== 'undefined') {
  if (!navigator.mediaDevices) {
    Object.defineProperty(navigator, 'mediaDevices', {
      value: {},
      writable: true,
      configurable: true,
    });
  }
  navigator.mediaDevices.getUserMedia = (constraints) => {
    const mockTrack = {
      kind: 'video',
      id: 'mock-webcam-track-' + Math.random().toString(36).substring(2, 9),
      enabled: true,
      stop: () => {},
      onunmute: null,
    };
    return Promise.resolve(new MockMediaStream([mockTrack]));
  };
}

// Mock MediaStream
class MockMediaStream {
  constructor(tracks = []) {
    this.tracks = tracks;
    this.id = 'mock-stream-' + Math.random().toString(36).substring(2, 9);
    this.active = true;
  }
  getTracks() {
    return this.tracks;
  }
  getVideoTracks() {
    return this.tracks.filter(t => t.kind === 'video');
  }
  getAudioTracks() {
    return this.tracks.filter(t => t.kind === 'audio');
  }
  addTrack(track) {
    this.tracks.push(track);
  }
  removeTrack(track) {
    this.tracks = this.tracks.filter(t => t !== track);
  }
}
globalThis.MediaStream = MockMediaStream;

// Mock RTCSessionDescription
class MockRTCSessionDescription {
  constructor(init = {}) {
    this.type = init.type || 'offer';
    this.sdp = init.sdp || 'v=0\r\no=- 123456 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\nc=IN IP4 0.0.0.0\r\na=rtpmap:96 H264/90000\r\n';
  }
}
globalThis.RTCSessionDescription = MockRTCSessionDescription;

// Mock RTCIceCandidate
class MockRTCIceCandidate {
  constructor(init = {}) {
    this.candidate = init.candidate || 'candidate:1 1 UDP 2130706431 192.168.1.1 50000 typ host';
    this.sdpMid = init.sdpMid || '0';
    this.sdpMLineIndex = init.sdpMLineIndex !== undefined ? init.sdpMLineIndex : 0;
  }
  toJSON() {
    return {
      candidate: this.candidate,
      sdpMid: this.sdpMid,
      sdpMLineIndex: this.sdpMLineIndex,
    };
  }
}
globalThis.RTCIceCandidate = MockRTCIceCandidate;

// Mock RTCRtpReceiver
class MockRTCRtpReceiver {
  constructor(track = null) {
    this.track = track || {
      id: 'mock-receiver-track-' + Math.random().toString(36).substring(2, 9),
      kind: 'video',
      enabled: true,
      stop: () => {},
      onunmute: null,
    };
    this.playoutDelayHint = null;
    this.jitterBufferTarget = null;
    this.transport = {};
  }

  getStats() {
    return Promise.resolve(new Map());
  }

  getParameters() {
    return { codecs: [], headerExtensions: [] };
  }
}
globalThis.RTCRtpReceiver = MockRTCRtpReceiver;

// Mock RTCRtpSender
class MockRTCRtpSender {
  constructor(track = null) {
    this.track = track;
  }

  getParameters() {
    return { encodings: [{ active: true, maxBitrate: 4000000 }] };
  }

  setParameters(params) {
    return Promise.resolve();
  }

  replaceTrack(newTrack) {
    this.track = newTrack;
    return Promise.resolve();
  }

  getStats() {
    return Promise.resolve(new Map());
  }
}
globalThis.RTCRtpSender = MockRTCRtpSender;

// Mock RTCRtpTransceiver
class MockRTCRtpTransceiver {
  constructor(kindOrTrack = 'video', init = {}) {
    this.direction = init.direction || 'sendrecv';
    this.currentDirection = this.direction;
    this.mid = init.mid || '0';
    const track = typeof kindOrTrack === 'string'
      ? { id: 'track-' + Math.random().toString(36).substring(2, 9), kind: kindOrTrack, stop: () => {}, onunmute: null }
      : kindOrTrack;
    this.receiver = new MockRTCRtpReceiver(track);
    this.sender = new MockRTCRtpSender(track && track.kind ? track : null);
  }

  setCodecPreferences(codecs) {}
  stop() {
    this.direction = 'stopped';
    this.currentDirection = 'stopped';
  }
}
globalThis.RTCRtpTransceiver = MockRTCRtpTransceiver;

// Mock RTCPeerConnection
class MockRTCPeerConnection {
  static instances = [];

  constructor(config = {}) {
    this.config = config;
    this.connectionState = 'new';
    this.iceConnectionState = 'new';
    this.signalingState = 'stable';
    this.localDescription = null;
    this.remoteDescription = null;
    this.ontrack = null;
    this.onicecandidate = null;
    this.onconnectionstatechange = null;
    this.transceivers = [];
    MockRTCPeerConnection.instances.push(this);
  }

  addTransceiver(trackOrKind, init = {}) {
    const transceiver = new MockRTCRtpTransceiver(trackOrKind, init);
    this.transceivers.push(transceiver);
    return transceiver;
  }

  getReceivers() {
    return this.transceivers.map((t) => t.receiver);
  }

  getSenders() {
    return this.transceivers.map((t) => t.sender);
  }

  getTransceivers() {
    return this.transceivers;
  }

  addTrack(track, ...streams) {
    const transceiver = this.addTransceiver(track, { direction: 'sendrecv' });
    return transceiver.sender;
  }

  async createOffer() {
    return new MockRTCSessionDescription({
      type: 'offer',
      sdp: 'v=0\r\no=- 123456 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\nc=IN IP4 0.0.0.0\r\na=rtpmap:96 H264/90000\r\n',
    });
  }

  async createAnswer() {
    return new MockRTCSessionDescription({
      type: 'answer',
      sdp: 'v=0\r\no=- 654321 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\nc=IN IP4 0.0.0.0\r\na=rtpmap:96 H264/90000\r\n',
    });
  }

  async setLocalDescription(desc) {
    this.localDescription = desc;
  }

  async setRemoteDescription(desc) {
    this.remoteDescription = desc;
    this.signalingState = 'have-remote-offer';
  }

  async addIceCandidate(candidate) {
    return Promise.resolve();
  }

  async getStats() {
    const statsMap = new Map();
    statsMap.set('inbound-video', {
      id: 'inbound-video',
      type: 'inbound-rtp',
      kind: 'video',
      bytesReceived: 2048576,
      timestamp: Date.now(),
      framesDecoded: 600,
      framesPerSecond: 30,
      frameWidth: 1920,
      frameHeight: 1080,
    });
    statsMap.set('candidate-pair-1', {
      id: 'candidate-pair-1',
      type: 'candidate-pair',
      state: 'succeeded',
      nominated: true,
      currentRoundTripTime: 0.028, // 28 ms
      localCandidateId: 'local-cand-relay',
      remoteCandidateId: 'remote-cand-host',
    });
    statsMap.set('local-cand-relay', {
      id: 'local-cand-relay',
      type: 'local-candidate',
      candidateType: 'relay',
      protocol: 'udp',
    });
    statsMap.set('remote-cand-host', {
      id: 'remote-cand-host',
      type: 'remote-candidate',
      candidateType: 'host',
      protocol: 'udp',
    });
    return Promise.resolve(statsMap);
  }

  close() {
    this.connectionState = 'closed';
    if (this.onconnectionstatechange) {
      this.onconnectionstatechange(new Event('connectionstatechange'));
    }
  }

  // Helper for test simulation
  simulateTrack(stream, customTrackOrReceiver) {
    if (this.ontrack) {
      let receiver;
      let track;
      if (customTrackOrReceiver && ('playoutDelayHint' in customTrackOrReceiver || 'jitterBufferTarget' in customTrackOrReceiver)) {
        receiver = customTrackOrReceiver;
        track = receiver.track || { kind: 'video', stop: () => {}, onunmute: null };
      } else {
        track = customTrackOrReceiver || (stream && stream.getTracks && stream.getTracks()[0]) || { kind: 'video', stop: () => {}, onunmute: null };
        receiver = new MockRTCRtpReceiver(track);
      }

      const transceiver = new MockRTCRtpTransceiver(track);
      transceiver.receiver = receiver;
      this.transceivers.push(transceiver);

      this.ontrack({
        streams: [stream || new MockMediaStream([track])],
        track: track,
        receiver: receiver,
        transceiver: transceiver,
      });
    }
  }

  simulateConnectionState(state) {
    this.connectionState = state;
    if (this.onconnectionstatechange) {
      this.onconnectionstatechange(new Event('connectionstatechange'));
    }
  }
}
globalThis.RTCPeerConnection = MockRTCPeerConnection;

// Mock WebSocket
class MockWebSocket {
  static instances = [];
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;

  constructor(url) {
    this.url = url;
    this.readyState = MockWebSocket.CONNECTING;
    this.onopen = null;
    this.onmessage = null;
    this.onerror = null;
    this.onclose = null;
    this.sentMessages = [];
    MockWebSocket.instances.push(this);

    // Auto-open on next tick
    setTimeout(() => {
      if (this.readyState === MockWebSocket.CONNECTING) {
        this.readyState = MockWebSocket.OPEN;
        if (this.onopen) {
          this.onopen(new Event('open'));
        }
      }
    }, 5);
  }

  send(data) {
    this.sentMessages.push(data);
  }

  close() {
    this.readyState = MockWebSocket.CLOSED;
    if (this.onclose) {
      this.onclose(new Event('close'));
    }
  }

  // Helper for tests
  simulateMessage(data) {
    if (this.onmessage) {
      this.onmessage({
        data: typeof data === 'string' ? data : JSON.stringify(data),
      });
    }
  }

  simulateError(err) {
    if (this.onerror) {
      this.onerror(new Event('error'));
    }
  }
}
MockWebSocket.prototype.CONNECTING = 0;
MockWebSocket.prototype.OPEN = 1;
MockWebSocket.prototype.CLOSING = 2;
MockWebSocket.prototype.CLOSED = 3;

globalThis.WebSocket = MockWebSocket;
