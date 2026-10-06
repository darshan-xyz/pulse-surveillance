import React from 'react';
import { StyleSheet, Text, View, TouchableOpacity } from 'react-native';
import { UserInfo } from '../types';

interface HeaderProps {
  user: UserInfo | null;
  onLogout: () => void;
}

export const Header: React.FC<HeaderProps> = ({ user, onLogout }) => {
  return (
    <View style={styles.container}>
      <View style={styles.brandRow}>
        <View style={styles.logoContainer}>
          <View style={styles.statusDot} />
          <Text style={styles.brandTitle}>Pulse</Text>
          <Text style={styles.brandSubtitle}>Cloud ABR</Text>
        </View>

        <TouchableOpacity
          style={styles.logoutButton}
          onPress={onLogout}
          accessibilityLabel="Sign Out"
          testID="logout-button"
        >
          <Text style={styles.logoutText}>Sign Out</Text>
        </TouchableOpacity>
      </View>

      {user && (
        <View style={styles.metaRow}>
          <View style={styles.tenantBadge}>
            <Text style={styles.tenantLabel}>TENANT:</Text>
            <Text style={styles.tenantValue} testID="header-tenant-id">
              {user.tenant_id || 'tenant-alpha'}
            </Text>
          </View>

          <View style={styles.userBadge}>
            <Text style={styles.userLabel}>USER:</Text>
            <Text style={styles.userValue} testID="header-username">
              {user.username}
            </Text>
            <Text style={styles.roleTag}>({user.role || 'viewer'})</Text>
          </View>
        </View>
      )}
 </View>
 );
};

const styles = StyleSheet.create({
 container: {
 backgroundColor: '#121214',
 paddingHorizontal: 16,
 paddingVertical: 14,
 borderBottomWidth: 1,
 borderBottomColor: '#222226',
 },
 brandRow: {
 flexDirection: 'row',
 alignItems: 'center',
 justifyContent: 'space-between',
 },
 logoContainer: {
 flexDirection: 'row',
 alignItems: 'center',
 },
 statusDot: {
 width: 10,
 height: 10,
 borderRadius: 5,
 backgroundColor: '#00ff88',
 marginRight: 8,
 },
 brandTitle: {
 fontSize: 22,
 fontWeight: '800',
 color: '#ffffff',
 letterSpacing: -0.5,
 },
 brandSubtitle: {
 fontSize: 13,
 fontWeight: '500',
 color: '#888899',
 marginLeft: 8,
 paddingTop: 3,
 },
 logoutButton: {
 backgroundColor: '#26262e',
 paddingHorizontal: 12,
 paddingVertical: 6,
 borderRadius: 8,
 borderWidth: 1,
 borderColor: '#3a3a44',
 },
 logoutText: {
 color: '#ff6666',
 fontSize: 13,
 fontWeight: '600',
 },
 metaRow: {
 flexDirection: 'row',
 marginTop: 10,
 gap: 8,
 },
 tenantBadge: {
 flexDirection: 'row',
 alignItems: 'center',
 backgroundColor: '#1b1b22',
 paddingHorizontal: 8,
 paddingVertical: 4,
 borderRadius: 6,
 borderWidth: 1,
 borderColor: '#2b2b36',
 },
 tenantLabel: {
 fontSize: 11,
 fontWeight: '700',
 color: '#777788',
 marginRight: 4,
 },
 tenantValue: {
 fontSize: 11,
 fontWeight: '700',
 color: '#00e5ff',
 },
 userBadge: {
 flexDirection: 'row',
 alignItems: 'center',
 backgroundColor: '#1b1b22',
 paddingHorizontal: 8,
 paddingVertical: 4,
 borderRadius: 6,
 borderWidth: 1,
 borderColor: '#2b2b36',
 },
 userLabel: {
 fontSize: 11,
 fontWeight: '700',
 color: '#777788',
 marginRight: 4,
 },
 userValue: {
 fontSize: 11,
 fontWeight: '700',
 color: '#ffffff',
 },
 roleTag: {
 fontSize: 11,
 fontWeight: '500',
 color: '#9999aa',
 marginLeft: 4,
 },
});
