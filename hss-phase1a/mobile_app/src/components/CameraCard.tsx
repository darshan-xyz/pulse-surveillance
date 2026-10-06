import React, { useState } from 'react';
import { StyleSheet, Text, View, TouchableOpacity, ActivityIndicator, Platform } from 'react-native';
import * as WebRTC from 'react-native-webrtc';
import { Device } from '../types';
import { useSignaling } from '../hooks/useSignaling';

interface CameraCardProps {
  device: Device;
  wsBaseUrl: string;
  token: string;
}

export const CameraCard: React.FC<CameraCardProps> = ({ device, wsBaseUrl, token }) => {
  const [isPlaying, setIsPlaying] = useState<boolean>(false);

  const { stream, error, status, reconnect, disconnect } = useSignaling(
    wsBaseUrl,
    token,
    device.device_id,
    isPlaying
  );

  const handleTogglePlay = () => {
    if (isPlaying) {
      disconnect();
      setIsPlaying(false);
    } else {
      setIsPlaying(true);
    }
  };

  const RTCViewComponent = (WebRTC as any).RTCView;

  return (
    <View style={styles.card} testID={'camera-card-' + device.device_id}>
      <View style={styles.header}>
        <View style={styles.titleCol}>
          <Text style={styles.deviceId} testID={'device-id-' + device.device_id}>
            {device.device_id}
          </Text>
          <Text style={styles.roleText}>Role: {device.role}</Text>
        </View>

        <View
          style={[
            styles.statusBadge,
            status === 'Live'
              ? styles.statusBadgeLive
              : status === 'Failed'
              ? styles.statusBadgeFailed
              : isPlaying
              ? styles.statusBadgeConnecting
              : styles.statusBadgeIdle,
          ]}
        >
          <View
            style={[
              styles.statusIndicator,
              status === 'Live' && styles.statusIndicatorLive,
              status === 'Failed' && styles.statusIndicatorFailed,
            ]}
          />
          <Text style={styles.statusBadgeText}>
            {isPlaying ? status : 'Standby'}
          </Text>
        </View>
      </View>

      <View style={styles.videoContainer}>
        {isPlaying ? (
          stream && RTCViewComponent ? (
            <RTCViewComponent
              streamURL={typeof stream.toURL === 'function' ? stream.toURL() : 'mock_stream'}
              style={styles.video}
              objectFit="cover"
              mirror={false}
              testID={'rtc-view-' + device.device_id}
            />
          ) : stream ? (
            <View style={styles.webFallbackContainer}>
              <Text style={styles.webFallbackText}>Stream Active (WebRTC)</Text>
              <Text style={styles.streamDetailText}>{status}</Text>
            </View>
          ) : (
            <View style={styles.loadingContainer}>
              <ActivityIndicator size="large" color="#00ff88" />
              <Text style={styles.loadingStatusText} testID={'status-text-' + device.device_id}>
                {status}
              </Text>
            </View>
          )
        ) : (
          <View style={styles.offlinePlaceholder}>
            <Text style={styles.offlineIcon}>📹</Text>
            <Text style={styles.offlineText}>Camera Inactive</Text>
            <Text style={styles.offlineSubtext}>Tap Start Stream to view feed</Text>
          </View>
        )}

        {error && (
          <View style={styles.errorOverlay} testID={'error-overlay-' + device.device_id}>
            <Text style={styles.errorTitle}>Stream Error</Text>
            <Text style={styles.errorMessage}>{error}</Text>
            <TouchableOpacity style={styles.retryButton} onPress={reconnect}>
              <Text style={styles.retryButtonText}>Retry</Text>
            </TouchableOpacity>
          </View>
        )}
      </View>

      <View style={styles.footer}>
        <TouchableOpacity
          style={[styles.actionButton, isPlaying ? styles.stopButton : styles.playButton]}
          onPress={handleTogglePlay}
          testID={'toggle-stream-btn-' + device.device_id}
        >
          <Text
            style={[
              styles.actionButtonText,
              isPlaying ? styles.stopButtonText : styles.playButtonText,
            ]}
          >
            {isPlaying ? 'Stop Stream' : 'Start Stream'}
          </Text>
        </TouchableOpacity>

        {isPlaying && (
          <TouchableOpacity
            style={styles.reconnectButton}
            onPress={reconnect}
            testID={'reconnect-btn-' + device.device_id}
          >
            <Text style={styles.reconnectButtonText}>Reconnect</Text>
          </TouchableOpacity>
        )}
      </View>
    </View>
  );
};

const styles = StyleSheet.create({
 card: {
 backgroundColor: '#16161a',
 borderRadius: 14,
 borderWidth: 1,
 borderColor: '#26262e',
 marginBottom: 16,
 overflow: 'hidden',
 },
 header: {
 flexDirection: 'row',
 alignItems: 'center',
 justifyContent: 'space-between',
 padding: 12,
 borderBottomWidth: 1,
 borderBottomColor: '#22222a',
 },
 titleCol: {
 flex: 1,
 },
 deviceId: {
 fontSize: 16,
 fontWeight: '700',
 color: '#ffffff',
 letterSpacing: -0.3,
 },
 roleText: {
 fontSize: 12,
 color: '#888899',
 marginTop: 2,
 },
 statusBadge: {
 flexDirection: 'row',
 alignItems: 'center',
 paddingHorizontal: 8,
 paddingVertical: 4,
 borderRadius: 12,
 },
 statusBadgeIdle: {
 backgroundColor: '#26262e',
 },
 statusBadgeConnecting: {
 backgroundColor: '#3b2f15',
 },
 statusBadgeLive: {
 backgroundColor: '#0d3822',
 },
 statusBadgeFailed: {
 backgroundColor: '#441818',
 },
 statusIndicator: {
 width: 6,
 height: 6,
 borderRadius: 3,
 backgroundColor: '#888899',
 marginRight: 6,
 },
 statusIndicatorLive: {
 backgroundColor: '#00ff88',
 },
 statusIndicatorFailed: {
 backgroundColor: '#ff4444',
 },
 statusBadgeText: {
 fontSize: 11,
 fontWeight: '600',
 color: '#ffffff',
 },
 videoContainer: {
 width: '100%',
 aspectRatio: 16 / 9,
 backgroundColor: '#0a0a0c',
 position: 'relative',
 justifyContent: 'center',
 alignItems: 'center',
 },
 video: {
 width: '100%',
 height: '100%',
 },
 webFallbackContainer: {
 alignItems: 'center',
 justifyContent: 'center',
 padding: 16,
 },
 webFallbackText: {
 color: '#00ff88',
 fontSize: 15,
 fontWeight: '700',
 },
 streamDetailText: {
 color: '#888899',
 fontSize: 12,
 marginTop: 4,
 },
 loadingContainer: {
 alignItems: 'center',
 justifyContent: 'center',
 },
 loadingStatusText: {
 color: '#00ff88',
 marginTop: 10,
 fontSize: 13,
 fontWeight: '600',
 },
 offlinePlaceholder: {
 alignItems: 'center',
 justifyContent: 'center',
 },
 offlineIcon: {
 fontSize: 28,
 marginBottom: 6,
 },
 offlineText: {
 color: '#666677',
 fontSize: 14,
 fontWeight: '600',
 },
 offlineSubtext: {
 color: '#444455',
 fontSize: 11,
 marginTop: 2,
 },
 errorOverlay: {
 position: 'absolute',
 top: 0,
 left: 0,
 right: 0,
 bottom: 0,
 backgroundColor: 'rgba(20, 5, 5, 0.85)',
 justifyContent: 'center',
 alignItems: 'center',
 padding: 16,
 },
 errorTitle: {
 color: '#ff4444',
 fontSize: 14,
 fontWeight: '700',
 },
 errorMessage: {
 color: '#ffaaaa',
 fontSize: 12,
 textAlign: 'center',
 marginTop: 4,
 marginBottom: 10,
 },
 retryButton: {
 backgroundColor: '#ff4444',
 paddingHorizontal: 12,
 paddingVertical: 6,
 borderRadius: 6,
 },
 retryButtonText: {
 color: '#ffffff',
 fontSize: 12,
 fontWeight: '700',
 },
 footer: {
 flexDirection: 'row',
 alignItems: 'center',
 padding: 10,
 gap: 8,
 borderTopWidth: 1,
 borderTopColor: '#22222a',
 },
 actionButton: {
 flex: 1,
 paddingVertical: 10,
 borderRadius: 8,
 alignItems: 'center',
 justifyContent: 'center',
 },
 actionButtonText: {
 fontSize: 13,
 fontWeight: '700',
 color: '#ffffff',
 },
 playButton: {
 backgroundColor: '#00ff88',
 },
 playButtonText: {
 color: '#000000',
 fontSize: 13,
 fontWeight: '700',
 },
 stopButton: {
 backgroundColor: '#331515',
 borderWidth: 1,
 borderColor: '#ff4444',
 },
 stopButtonText: {
 color: '#ff6666',
 fontSize: 13,
 fontWeight: '700',
 },
 reconnectButton: {
 backgroundColor: '#26262e',
 paddingVertical: 10,
 paddingHorizontal: 14,
 borderRadius: 8,
 borderWidth: 1,
 borderColor: '#3a3a44',
 },
 reconnectButtonText: {
 color: '#ffffff',
 fontSize: 12,
 fontWeight: '600',
 },
});
