import React from 'react';
import { StyleSheet, Text, View, SafeAreaView, TouchableOpacity, Platform } from 'react-native';
import { StatusBar } from 'expo-status-bar';
import { UserInfo } from '../types';
import { ApiClient } from '../api/client';
import { Header } from '../components/Header';
import { DeviceGrid } from '../components/DeviceGrid';
import { useDevices } from '../hooks/useDevices';
import { usePushNotifications } from '../hooks/usePushNotifications';

interface MainScreenProps {
  user: UserInfo;
  token: string;
  serverUrl: string;
  apiClient: ApiClient;
  onLogout: () => void;
}

export const MainScreen: React.FC<MainScreenProps> = ({
  user,
  token,
  serverUrl,
  apiClient,
  onLogout,
}) => {
  const { devices, loading, error, refresh } = useDevices(apiClient, token, {
    autoPoll: false,
  });

  const { pushToken, isRegistered, lastNotification, clearLastNotification } =
    usePushNotifications(apiClient, token);

  const wsBaseUrl = apiClient.getWsUrl();

  return (
    <SafeAreaView style={styles.container}>
      <StatusBar style="light" />

      <Header user={user} onLogout={onLogout} />

      {/* Push Notification Status Banner */}
      <View style={styles.pushStatusBar} testID="push-status-bar">
        <View style={styles.pushStatusLeft}>
          <View
            style={[
              styles.pushDot,
              isRegistered ? styles.pushDotActive : styles.pushDotPending,
            ]}
          />
          <Text style={styles.pushStatusText}>
            Push Alerts: {isRegistered ? 'Active' : 'Connecting...'}
          </Text>
        </View>
        {pushToken && (
          <Text style={styles.pushTokenText} numberOfLines={1} ellipsizeMode="middle">
            {pushToken.substring(0, 20)}...
          </Text>
        )}
      </View>

      {/* Active Camera Event / Notification Alert Banner */}
      {lastNotification && (
        <View style={styles.alertBanner} testID="active-alert-banner">
          <View style={styles.alertHeader}>
            <Text style={styles.alertBadge}>EVENT ALERT</Text>
            <TouchableOpacity onPress={clearLastNotification} testID="dismiss-alert-btn">
              <Text style={styles.dismissText}>Dismiss</Text>
            </TouchableOpacity>
          </View>
          <Text style={styles.alertTitle}>{lastNotification.title}</Text>
          <Text style={styles.alertBody}>{lastNotification.body}</Text>
          {lastNotification.data?.camera_id && (
            <Text style={styles.alertCamera}>
              Camera: {lastNotification.data.camera_id}
            </Text>
          )}
        </View>
      )}

      {/* Main Multi-tenant Device Grid */}
      <DeviceGrid
        devices={devices}
        loading={loading}
        error={error}
        onRefresh={refresh}
        wsBaseUrl={wsBaseUrl}
        token={token}
        tenantId={user.tenant_id}
      />
    </SafeAreaView>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: '#0a0a0c',
  },
  pushStatusBar: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    backgroundColor: '#121216',
    paddingHorizontal: 16,
    paddingVertical: 6,
    borderBottomWidth: 1,
    borderBottomColor: '#1c1c22',
  },
  pushStatusLeft: {
    flexDirection: 'row',
    alignItems: 'center',
  },
  pushDot: {
    width: 6,
    height: 6,
    borderRadius: 3,
    marginRight: 6,
  },
  pushDotActive: {
    backgroundColor: '#00ff88',
  },
  pushDotPending: {
    backgroundColor: '#ffaa00',
  },
  pushStatusText: {
    fontSize: 11,
    color: '#888899',
    fontWeight: '500',
  },
  pushTokenText: {
    fontSize: 10,
    color: '#555566',
    maxWidth: 120,
    fontFamily: Platform.OS === 'ios' ? 'Courier' : 'monospace',
  },
  alertBanner: {
    backgroundColor: '#2b1500',
    borderLeftWidth: 4,
    borderLeftColor: '#ffaa00',
    padding: 12,
    margin: 12,
    marginBottom: 4,
    borderRadius: 8,
  },
  alertHeader: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 4,
  },
  alertBadge: {
    backgroundColor: '#ffaa00',
    color: '#000000',
    fontSize: 10,
    fontWeight: '800',
    paddingHorizontal: 6,
    paddingVertical: 2,
    borderRadius: 4,
  },
  dismissText: {
    color: '#ffaa00',
    fontSize: 11,
    fontWeight: '600',
  },
  alertTitle: {
    color: '#ffffff',
    fontSize: 14,
    fontWeight: '700',
  },
  alertBody: {
    color: '#ddccaa',
    fontSize: 12,
    marginTop: 2,
  },
  alertCamera: {
    color: '#ffaa00',
    fontSize: 11,
    marginTop: 4,
    fontWeight: '600',
  },
});
