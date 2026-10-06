import React from 'react';
import { StyleSheet, View } from 'react-native';
import { useAuth } from './src/hooks/useAuth';
import { LoginScreen } from './src/screens/LoginScreen';
import { MainScreen } from './src/screens/MainScreen';

export default function App() {
  const {
    user,
    token,
    serverUrl,
    isAuthenticated,
    isLoading,
    error,
    apiClient,
    login,
    logout,
    setServerUrl,
    clearError,
  } = useAuth();

  return (
    <View style={styles.container}>
      {isAuthenticated && user && token ? (
        <MainScreen
          user={user}
          token={token}
          serverUrl={serverUrl}
          apiClient={apiClient}
          onLogout={logout}
        />
      ) : (
        <LoginScreen
          onLogin={login}
          isLoading={isLoading}
          error={error}
          serverUrl={serverUrl}
          onSetServerUrl={setServerUrl}
          onClearError={clearError}
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: '#0a0a0c',
  },
});
