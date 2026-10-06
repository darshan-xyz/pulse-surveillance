import { useState, useEffect, useCallback, useRef } from 'react';
import { Platform } from 'react-native';
import { ApiClient } from '../api/client';
import { PushNotificationPayload } from '../types';
import {
  registerForPushNotificationsAsync,
  addNotificationReceivedListener,
  addNotificationResponseReceivedListener,
} from '../services/notifications';

export function usePushNotifications(apiClient: ApiClient, userToken: string | null) {
  const [pushToken, setPushToken] = useState<string | null>(null);
  const [isRegistered, setIsRegistered] = useState<boolean>(false);
  const [lastNotification, setLastNotification] = useState<PushNotificationPayload | null>(null);
  const [notifications, setNotifications] = useState<PushNotificationPayload[]>([]);
  const registeredTokenRef = useRef<string | null>(null);

  // 1. Obtain push token from Expo
  useEffect(() => {
    let isMounted = true;

    registerForPushNotificationsAsync().then((token) => {
      if (isMounted && token) {
        setPushToken(token);
      }
    });

    return () => {
      isMounted = false;
    };
  }, []);

  // 2. Register push token with backend API once authenticated
  const registerWithBackend = useCallback(
    async (token: string, expoPushToken: string) => {
      try {
        const platform = Platform.OS === 'ios' ? 'ios' : Platform.OS === 'android' ? 'android' : 'web';
        await apiClient.registerPushToken(token, {
          push_token: expoPushToken,
          platform,
        });
        setIsRegistered(true);
        registeredTokenRef.current = expoPushToken;
      } catch (err) {
        console.warn('Failed to register push token with backend:', err);
        setIsRegistered(false);
      }
    },
    [apiClient]
  );

  useEffect(() => {
    if (userToken && pushToken && registeredTokenRef.current !== pushToken) {
      registerWithBackend(userToken, pushToken);
    }
  }, [userToken, pushToken, registerWithBackend]);

  // 3. Listen for incoming foreground notifications
  useEffect(() => {
    let notificationSub: any;
    let responseSub: any;

    try {
      notificationSub = addNotificationReceivedListener((notification) => {
        const payload: PushNotificationPayload = {
          id: notification.request.identifier,
          title: notification.request.content.title || 'Pulse Alert',
          body: notification.request.content.body || 'New camera event detected',
          data: notification.request.content.data,
          receivedAt: Date.now(),
        };
        setLastNotification(payload);
        setNotifications((prev) => [payload, ...prev.slice(0, 19)]);
      });

      responseSub = addNotificationResponseReceivedListener((response) => {
        const notification = response.notification;
        const payload: PushNotificationPayload = {
          id: notification.request.identifier,
          title: notification.request.content.title || 'Pulse Alert',
          body: notification.request.content.body || 'New camera event detected',
          data: notification.request.content.data,
          receivedAt: Date.now(),
        };
        setLastNotification(payload);
      });
    } catch (_) {}

    return () => {
      if (notificationSub && typeof notificationSub.remove === 'function') {
        notificationSub.remove();
      }
      if (responseSub && typeof responseSub.remove === 'function') {
        responseSub.remove();
      }
    };
  }, []);

  const clearLastNotification = useCallback(() => {
    setLastNotification(null);
  }, []);

  return {
    pushToken,
    isRegistered,
    lastNotification,
    notifications,
    clearLastNotification,
  };
}
