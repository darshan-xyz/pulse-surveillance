import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart';
import 'package:web_socket_channel/web_socket_channel.dart';
import 'dart:math';

class ViewerScreen extends StatefulWidget {
  final String deviceId;
  final String token;

  const ViewerScreen({Key? key, required this.deviceId, required this.token}) : super(key: key);

  @override
  _ViewerScreenState createState() => _ViewerScreenState();
}

class _ViewerScreenState extends State<ViewerScreen> {
  final RTCVideoRenderer _localRenderer = RTCVideoRenderer();
  RTCPeerConnection? _peerConnection;
  WebSocketChannel? _channel;
  String _status = 'Connecting...';
  bool _isConnected = false;

  @override
  void initState() {
    super.initState();
    initRenderers();
    _connectWebRTC();
  }

  Future<void> initRenderers() async {
    await _localRenderer.initialize();
  }

  Future<void> _connectWebRTC() async {
    final sessionId = 'mobile-session-${Random().nextInt(10000)}';
    
    // 1. Connect Signaling
    _channel = WebSocketChannel.connect(
      Uri.parse('ws://10.0.2.2:9090/ws/client?session_id=$sessionId&device_id=${widget.deviceId}'),
    );

    // 2. Initialize PeerConnection
    _peerConnection = await createPeerConnection({
      'iceServers': [
        {'urls': 'stun:stun.l.google.com:19302'}
      ]
    });

    _peerConnection!.onTrack = (RTCTrackEvent event) {
      if (event.track.kind == 'video') {
        setState(() {
          _status = 'Live';
          _isConnected = true;
          _localRenderer.srcObject = event.streams[0];
        });
      }
    };

    _peerConnection!.onIceConnectionState = (RTCIceConnectionState state) {
      if (state == RTCIceConnectionState.RTCIceConnectionStateFailed ||
          state == RTCIceConnectionState.RTCIceConnectionStateDisconnected) {
        setState(() {
          _status = 'Connection Lost';
          _isConnected = false;
        });
      } else {
        setState(() => _status = 'ICE State: ${state.name}');
      }
    };

    _peerConnection!.onIceCandidate = (RTCIceCandidate candidate) {
      _channel!.sink.add(jsonEncode({
        'type': 'candidate',
        'session_id': sessionId,
        'camera_id': 'cam_001_mock',
        'candidate': jsonEncode(candidate.toMap())
      }));
    };

    // 3. Create Offer
    await _peerConnection!.addTransceiver(
      kind: RTCRtpMediaType.RTCRtpMediaTypeVideo,
      init: RTCRtpTransceiverInit(direction: TransceiverDirection.RecvOnly),
    );
    
    RTCSessionDescription offer = await _peerConnection!.createOffer();
    await _peerConnection!.setLocalDescription(offer);

    setState(() => _status = 'Relay Connected. Negotiating...');

    _channel!.sink.add(jsonEncode({
      'type': 'offer',
      'session_id': sessionId,
      'camera_id': 'cam_001_mock',
      'sdp': offer.sdp,
    }));

    // 4. Listen for Answers
    _channel!.stream.listen((message) async {
      final msg = jsonDecode(message);
      if (msg['type'] == 'answer') {
        setState(() => _status = 'Establishing P2P stream...');
        await _peerConnection!.setRemoteDescription(
          RTCSessionDescription(msg['sdp'], msg['type']),
        );
      }
    });
  }

  @override
  void dispose() {
    _localRenderer.dispose();
    _peerConnection?.dispose();
    _channel?.sink.close();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.black,
      appBar: AppBar(
        title: const Text('Live View', style: TextStyle(color: Colors.white)),
        backgroundColor: Colors.grey[900],
        iconTheme: const IconThemeData(color: Colors.white),
      ),
      body: Column(
        children: [
          Container(
            padding: const EdgeInsets.symmetric(vertical: 8, horizontal: 16),
            color: Colors.grey[850],
            child: Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                Text(widget.deviceId, style: const TextStyle(color: Colors.grey)),
                Row(
                  children: [
                    Icon(_isConnected ? Icons.wifi : Icons.wifi_off, 
                         color: _isConnected ? Colors.green : Colors.red, size: 16),
                    const SizedBox(width: 8),
                    Text(_status, style: TextStyle(color: _isConnected ? Colors.green : Colors.grey)),
                  ],
                ),
              ],
            ),
          ),
          Expanded(
            child: Center(
              child: _isConnected
                  ? RTCVideoView(_localRenderer, objectFit: RTCVideoViewObjectFit.RTCVideoViewObjectFitContain)
                  : const CircularProgressIndicator(),
            ),
          ),
        ],
      ),
    );
  }
}
