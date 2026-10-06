import React from 'react';
import {
  StyleSheet,
  Text,
  View,
  ScrollView,
  TouchableOpacity,
  ActivityIndicator,
  RefreshControl,
} from 'react-native';
import { Device } from '../types';
import { CameraCard } from './CameraCard';

interface DeviceGridProps {
  devices: Device[];
  loading: boolean;
  error: string | null;
  onRefresh: () => void;
  wsBaseUrl: string;
  token: string;
  tenantId?: string;
}

export const DeviceGrid: React.FC<DeviceGridProps> = ({
  devices,
  loading,
  error,
  onRefresh,
  wsBaseUrl,
  token,
  tenantId,
}) => {
  return (
    <ScrollView
      style={styles.container}
      contentContainerStyle={styles.content}
      refreshControl={
        <RefreshControl
          refreshing={loading}
          onRefresh={onRefresh}
          tintColor="#00ff88"
          colors={['#00ff88']}
        />
      }
    >
      <View style={styles.sectionHeader}>
        <View>
          <Text style={styles.sectionTitle}>Authorized Cameras</Text>
          <Text style={styles.sectionSubtitle}>
            {devices.length} device{devices.length === 1 ? '' : 's'} in {tenantId || 'Tenant'}
          </Text>
        </View>

        <TouchableOpacity
          style={styles.refreshButton}
          onPress={onRefresh}
          disabled={loading}
          testID="refresh-devices-btn"
        >
          {loading ? (
            <ActivityIndicator size="small" color="#00ff88" />
          ) : (
            <Text style={styles.refreshButtonText}>? Refresh</Text>
          )}
        </TouchableOpacity>
      </View>

      {error && (
        <View style={styles.errorContainer} testID="device-grid-error">
          <Text style={styles.errorTitle}>Error Loading Devices</Text>
          <Text style={styles.errorMessage}>{error}</Text>
          <TouchableOpacity style={styles.retryBtn} onPress={onRefresh}>
            <Text style={styles.retryBtnText}>Retry</Text>
          </TouchableOpacity>
        </View>
      )}

      {devices.length === 0 && !loading && !error && (
        <View style={styles.emptyContainer} testID="empty-devices-view">
          <Text style={styles.emptyIcon}>??</Text>
          <Text style={styles.emptyTitle}>No Cameras Found</Text>
          <Text style={styles.emptySubtitle}>
            No edge cameras are associated with tenant {tenantId || 'this account'}.
          </Text>
        </View>
      )}

      {devices.map((dev) => (
        <CameraCard
          key={dev.device_id}
          device={dev}
          wsBaseUrl={wsBaseUrl}
          token={token}
        />
      ))}
    </ScrollView>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: '#0a0a0c',
  },
  content: {
    padding: 16,
    paddingBottom: 40,
  },
  sectionHeader: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginBottom: 16,
  },
  sectionTitle: {
    fontSize: 18,
    fontWeight: '700',
    color: '#ffffff',
    letterSpacing: -0.3,
  },
  sectionSubtitle: {
    fontSize: 12,
    color: '#777788',
    marginTop: 2,
  },
  refreshButton: {
    backgroundColor: '#1b1b22',
    paddingHorizontal: 12,
    paddingVertical: 6,
    borderRadius: 8,
    borderWidth: 1,
    borderColor: '#2b2b36',
  },
  refreshButtonText: {
    color: '#00ff88',
    fontSize: 12,
    fontWeight: '600',
  },
  errorContainer: {
    backgroundColor: '#261414',
    borderRadius: 10,
    padding: 14,
    borderWidth: 1,
    borderColor: '#4d2020',
    marginBottom: 16,
    alignItems: 'center',
  },
  errorTitle: {
    color: '#ff6666',
    fontSize: 14,
    fontWeight: '700',
  },
  errorMessage: {
    color: '#ffcccc',
    fontSize: 12,
    marginTop: 4,
    textAlign: 'center',
  },
  retryBtn: {
    marginTop: 8,
    backgroundColor: '#4d2020',
    paddingHorizontal: 12,
    paddingVertical: 6,
    borderRadius: 6,
  },
  retryBtnText: {
    color: '#ffffff',
    fontSize: 12,
    fontWeight: '600',
  },
  emptyContainer: {
    padding: 32,
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: '#131316',
    borderRadius: 12,
    borderWidth: 1,
    borderColor: '#22222a',
    marginTop: 20,
  },
  emptyIcon: {
    fontSize: 36,
    marginBottom: 10,
  },
  emptyTitle: {
    color: '#ffffff',
    fontSize: 16,
    fontWeight: '700',
  },
  emptySubtitle: {
    color: '#777788',
    fontSize: 13,
    textAlign: 'center',
    marginTop: 6,
    maxWidth: 240,
  },
});
