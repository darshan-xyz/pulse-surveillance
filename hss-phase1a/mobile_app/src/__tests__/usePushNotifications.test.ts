import { renderHook, act } from '@testing-library/react-native';
import { usePushNotifications } from '../hooks/usePushNotifications';
import { ApiClient } from '../api/client';
import * as NotificationsService from '../services/notifications';

jest.mock('../services/notifications');

describe('usePushNotifications Hook', () => {
  let apiClient: ApiClient;

  beforeEach(() => {
    apiClient = new ApiClient('http://127.0.0.1:9091');
    jest.clearAllMocks();
  });

  it('obtains Expo push token and registers with backend on authentication', async () => {
    jest
      .spyOn(NotificationsService, 'registerForPushNotificationsAsync')
      .mockResolvedValue('ExponentPushToken[mock_token_12345]');

    jest.spyOn(apiClient, 'registerPushToken').mockResolvedValue({
      status: 'registered',
    });

    const { result } = renderHook(() =>
      usePushNotifications(apiClient, 'mock_auth_token')
    );

    // Wait for async token retrieval and registration
    await act(async () => {
      await new Promise((r) => setTimeout(r, 50));
    });

    expect(result.current.pushToken).toBe('ExponentPushToken[mock_token_12345]');
    expect(result.current.isRegistered).toBe(true);
    expect(apiClient.registerPushToken).toHaveBeenCalledWith(
      'mock_auth_token',
      expect.objectContaining({
        push_token: 'ExponentPushToken[mock_token_12345]',
      })
    );
  });

  it('allows clearing the last received notification', () => {
    jest
      .spyOn(NotificationsService, 'registerForPushNotificationsAsync')
      .mockResolvedValue(null);

    const { result } = renderHook(() => usePushNotifications(apiClient, null));

    expect(result.current.lastNotification).toBeNull();
    act(() => {
      result.current.clearLastNotification();
    });
    expect(result.current.lastNotification).toBeNull();
  });
});
