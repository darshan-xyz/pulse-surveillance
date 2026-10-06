import React, { useState } from 'react';
import { StyleSheet, Text, View, SafeAreaView, TouchableOpacity, ActivityIndicator } from 'react-native';
import { RTCView } from 'react-native-webrtc';
import { StatusBar } from 'expo-status-bar';
import { useSignaling } from './src/useSignaling';

// Mock Config for Phase 7 Testing
const CLOUD_URL = 'ws://127.0.0.1:9091/client';
const USER_TOKEN = 'demo_user';
const TARGET_DEVICE_ID = 'hss_12345';

export default function App() {
  const [isPlaying, setIsPlaying] = useState(false);
  const { stream, error, status } = useSignaling(
    isPlaying ? CLOUD_URL : '',
    USER_TOKEN,
    TARGET_DEVICE_ID
  );

  return (
    <SafeAreaView style={styles.container}>
      <StatusBar style="light" />
      
      <View style={styles.header}>
        <Text style={styles.headerTitle}>Pulse</Text>
        <Text style={styles.headerSubtitle}>Live View</Text>
      </View>

      <View style={styles.playerContainer}>
        {isPlaying ? (
          stream ? (
            <RTCView 
              streamURL={stream.toURL()} 
              style={styles.video} 
              objectFit="cover" 
            />
          ) : (
            <View style={styles.loadingContainer}>
              <ActivityIndicator size="large" color="#00ff00" />
              <Text style={styles.statusText}>{status}</Text>
            </View>
          )
        ) : (
          <View style={styles.placeholderContainer}>
            <Text style={styles.placeholderText}>Camera Offline</Text>
          </View>
        )}
        
        {error && (
          <View style={styles.errorOverlay}>
            <Text style={styles.errorText}>{error}</Text>
          </View>
        )}
      </View>

      <View style={styles.controls}>
        <TouchableOpacity 
          style={[styles.playButton, isPlaying && styles.stopButton]} 
          onPress={() => setIsPlaying(!isPlaying)}
        >
          <Text style={styles.playButtonText}>
            {isPlaying ? 'Stop Stream' : 'Connect to Camera'}
          </Text>
        </TouchableOpacity>
      </View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: '#0a0a0a',
  },
  header: {
    padding: 20,
    paddingTop: 40,
  },
  headerTitle: {
    color: '#ffffff',
    fontSize: 28,
    fontWeight: 'bold',
    letterSpacing: -0.5,
  },
  headerSubtitle: {
    color: '#888888',
    fontSize: 16,
    marginTop: 4,
  },
  playerContainer: {
    width: '100%',
    aspectRatio: 16 / 9,
    backgroundColor: '#1a1a1a',
    position: 'relative',
    justifyContent: 'center',
    alignItems: 'center',
  },
  video: {
    width: '100%',
    height: '100%',
  },
  loadingContainer: {
    alignItems: 'center',
  },
  statusText: {
    color: '#ffffff',
    marginTop: 12,
    fontSize: 14,
  },
  placeholderContainer: {
    alignItems: 'center',
  },
  placeholderText: {
    color: '#444444',
    fontSize: 16,
  },
  errorOverlay: {
    position: 'absolute',
    top: 0, left: 0, right: 0, bottom: 0,
    backgroundColor: 'rgba(255,0,0,0.3)',
    justifyContent: 'center',
    alignItems: 'center',
  },
  errorText: {
    color: '#ffffff',
    fontWeight: 'bold',
  },
  controls: {
    padding: 20,
    marginTop: 20,
  },
  playButton: {
    backgroundColor: '#ffffff',
    padding: 16,
    borderRadius: 12,
    alignItems: 'center',
  },
  stopButton: {
    backgroundColor: '#ff4444',
  },
  playButtonText: {
    color: '#000000',
    fontSize: 16,
    fontWeight: 'bold',
  },
});
