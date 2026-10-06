import React, { useState } from 'react';
import {
  StyleSheet,
  Text,
  View,
  TextInput,
  TouchableOpacity,
  ActivityIndicator,
  KeyboardAvoidingView,
  Platform,
  ScrollView,
  SafeAreaView,
} from 'react-native';
import { StatusBar } from 'expo-status-bar';

interface LoginScreenProps {
  onLogin: (username: string, password: string, serverUrl?: string) => Promise<boolean>;
  isLoading: boolean;
  error: string | null;
  serverUrl: string;
  onSetServerUrl: (url: string) => void;
  onClearError: () => void;
}

export const LoginScreen: React.FC<LoginScreenProps> = ({
  onLogin,
  isLoading,
  error,
  serverUrl,
  onSetServerUrl,
  onClearError,
}) => {
  const [username, setUsername] = useState<string>('admin');
  const [password, setPassword] = useState<string>('password');
  const [customUrl, setCustomUrl] = useState<string>(serverUrl);
  const [showAdvanced, setShowAdvanced] = useState<boolean>(false);

  const handleSubmit = async () => {
    if (!username.trim()) return;
    onClearError();
    await onLogin(username.trim(), password, customUrl.trim());
  };

  const handleSelectPreset = (presetUsername: string) => {
    setUsername(presetUsername);
    setPassword('password');
    onClearError();
  };

  return (
    <SafeAreaView style={styles.safeArea}>
      <StatusBar style="light" />
      <KeyboardAvoidingView
        behavior={Platform.OS === 'ios' ? 'padding' : undefined}
        style={styles.keyboardView}
      >
        <ScrollView contentContainerStyle={styles.scrollContent} keyboardShouldPersistTaps="handled">
          <View style={styles.brandSection}>
            <View style={styles.logoBadge}>
              <View style={styles.logoDot} />
              <Text style={styles.logoText}>PULSE</Text>
            </View>
            <Text style={styles.title}>Cloud ABR Engine</Text>
            <Text style={styles.subtitle}>Ultra-Low Latency Multi-Tenant Video Streamer</Text>
          </View>

          <View style={styles.card}>
            <Text style={styles.formTitle}>Sign In</Text>

            {error && (
              <View style={styles.errorBox} testID="login-error-banner">
                <Text style={styles.errorText}>{error}</Text>
              </View>
            )}

            <View style={styles.inputGroup}>
              <Text style={styles.inputLabel}>Username</Text>
              <TextInput
                style={styles.input}
                value={username}
                onChangeText={setUsername}
                placeholder="Enter username"
                placeholderTextColor="#555566"
                autoCapitalize="none"
                autoCorrect={false}
                testID="username-input"
              />
            </View>

            <View style={styles.inputGroup}>
              <Text style={styles.inputLabel}>Password</Text>
              <TextInput
                style={styles.input}
                value={password}
                onChangeText={setPassword}
                placeholder="Enter password"
                placeholderTextColor="#555566"
                secureTextEntry
                autoCapitalize="none"
                testID="password-input"
              />
            </View>

            <TouchableOpacity
              style={styles.advancedToggle}
              onPress={() => setShowAdvanced(!showAdvanced)}
            >
              <Text style={styles.advancedToggleText}>
                {showAdvanced ? 'Hide Server Config' : 'Server Connection Config'}
              </Text>
            </TouchableOpacity>

            {showAdvanced && (
              <View style={styles.inputGroup}>
                <Text style={styles.inputLabel}>Cloud Backend URL</Text>
                <TextInput
                  style={styles.input}
                  value={customUrl}
                  onChangeText={(val) => {
                    setCustomUrl(val);
                    onSetServerUrl(val);
                  }}
                  placeholder="http://127.0.0.1:9091"
                  placeholderTextColor="#555566"
                  autoCapitalize="none"
                  autoCorrect={false}
                  testID="server-url-input"
                />
              </View>
            )}

            <TouchableOpacity
              style={[styles.loginButton, isLoading && styles.loginButtonDisabled]}
              onPress={handleSubmit}
              disabled={isLoading}
              testID="login-submit-btn"
            >
              {isLoading ? (
                <ActivityIndicator color="#000000" />
              ) : (
                <Text style={styles.loginButtonText}>Connect & Authenticate</Text>
              )}
            </TouchableOpacity>
          </View>

          <View style={styles.presetsSection}>
            <Text style={styles.presetsTitle}>Demo Tenant Presets</Text>
            <View style={styles.presetsGrid}>
              <TouchableOpacity
                style={[styles.presetButton, username === 'admin' && styles.presetButtonActive]}
                onPress={() => handleSelectPreset('admin')}
                testID="preset-admin"
              >
                <Text style={styles.presetName}>admin</Text>
                <Text style={styles.presetTenant}>Tenant Alpha (Admin)</Text>
              </TouchableOpacity>

              <TouchableOpacity
                style={[styles.presetButton, username === 'user2' && styles.presetButtonActive]}
                onPress={() => handleSelectPreset('user2')}
                testID="preset-user2"
              >
                <Text style={styles.presetName}>user2</Text>
                <Text style={styles.presetTenant}>Tenant Beta (Viewer)</Text>
              </TouchableOpacity>

              <TouchableOpacity
                style={[
                  styles.presetButton,
                  username === 'tenant1_user' && styles.presetButtonActive,
                ]}
                onPress={() => handleSelectPreset('tenant1_user')}
                testID="preset-tenant1"
              >
                <Text style={styles.presetName}>tenant1_user</Text>
                <Text style={styles.presetTenant}>Tenant 1 (Admin)</Text>
              </TouchableOpacity>

              <TouchableOpacity
                style={[
                  styles.presetButton,
                  username === 'tenant2_user' && styles.presetButtonActive,
                ]}
                onPress={() => handleSelectPreset('tenant2_user')}
                testID="preset-tenant2"
              >
                <Text style={styles.presetName}>tenant2_user</Text>
                <Text style={styles.presetTenant}>Tenant 2 (Admin)</Text>
              </TouchableOpacity>
            </View>
          </View>
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
};

const styles = StyleSheet.create({
  safeArea: {
    flex: 1,
    backgroundColor: '#0a0a0c',
  },
  keyboardView: {
    flex: 1,
  },
  scrollContent: {
    padding: 24,
    paddingTop: 40,
    alignItems: 'center',
  },
  brandSection: {
    alignItems: 'center',
    marginBottom: 28,
  },
  logoBadge: {
    flexDirection: 'row',
    alignItems: 'center',
    backgroundColor: '#16161c',
    paddingHorizontal: 14,
    paddingVertical: 6,
    borderRadius: 20,
    borderWidth: 1,
    borderColor: '#262634',
    marginBottom: 12,
  },
  logoDot: {
    width: 8,
    height: 8,
    borderRadius: 4,
    backgroundColor: '#00ff88',
    marginRight: 8,
  },
  logoText: {
    color: '#ffffff',
    fontSize: 13,
    fontWeight: '800',
    letterSpacing: 2,
  },
  title: {
    fontSize: 26,
    fontWeight: '800',
    color: '#ffffff',
    letterSpacing: -0.5,
  },
  subtitle: {
    fontSize: 13,
    color: '#777788',
    marginTop: 6,
    textAlign: 'center',
  },
  card: {
    width: '100%',
    maxWidth: 420,
    backgroundColor: '#141418',
    borderRadius: 16,
    borderWidth: 1,
    borderColor: '#24242e',
    padding: 20,
  },
  formTitle: {
    fontSize: 18,
    fontWeight: '700',
    color: '#ffffff',
    marginBottom: 16,
  },
  errorBox: {
    backgroundColor: '#331515',
    borderWidth: 1,
    borderColor: '#662222',
    padding: 12,
    borderRadius: 8,
    marginBottom: 16,
  },
  errorText: {
    color: '#ff8888',
    fontSize: 13,
    fontWeight: '500',
  },
  inputGroup: {
    marginBottom: 16,
  },
  inputLabel: {
    fontSize: 12,
    fontWeight: '600',
    color: '#888899',
    marginBottom: 6,
    textTransform: 'uppercase',
    letterSpacing: 0.5,
  },
  input: {
    backgroundColor: '#1b1b22',
    borderWidth: 1,
    borderColor: '#2e2e3a',
    borderRadius: 10,
    paddingHorizontal: 14,
    paddingVertical: 12,
    color: '#ffffff',
    fontSize: 15,
  },
  advancedToggle: {
    alignSelf: 'flex-start',
    marginBottom: 16,
  },
  advancedToggleText: {
    color: '#00ff88',
    fontSize: 12,
    fontWeight: '600',
  },
  loginButton: {
    backgroundColor: '#00ff88',
    borderRadius: 10,
    paddingVertical: 14,
    alignItems: 'center',
    marginTop: 8,
  },
  loginButtonDisabled: {
    opacity: 0.6,
  },
  loginButtonText: {
    color: '#000000',
    fontSize: 15,
    fontWeight: '700',
  },
  presetsSection: {
    width: '100%',
    maxWidth: 420,
    marginTop: 28,
  },
  presetsTitle: {
    fontSize: 13,
    fontWeight: '700',
    color: '#777788',
    marginBottom: 12,
    textTransform: 'uppercase',
    letterSpacing: 0.5,
  },
  presetsGrid: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 10,
  },
  presetButton: {
    flex: 1,
    minWidth: '45%',
    backgroundColor: '#141418',
    borderWidth: 1,
    borderColor: '#24242e',
    borderRadius: 10,
    padding: 12,
  },
  presetButtonActive: {
    borderColor: '#00ff88',
    backgroundColor: '#0e2418',
  },
  presetName: {
    color: '#ffffff',
    fontSize: 14,
    fontWeight: '700',
  },
  presetTenant: {
    color: '#777788',
    fontSize: 11,
    marginTop: 2,
  },
});
