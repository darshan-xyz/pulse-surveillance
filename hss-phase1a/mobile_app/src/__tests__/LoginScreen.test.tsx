import React from 'react';
import { render, fireEvent, waitFor } from '@testing-library/react-native';
import { LoginScreen } from '../screens/LoginScreen';

describe('LoginScreen Component', () => {
  it('renders form inputs and demo presets', () => {
    const onLogin = jest.fn().mockResolvedValue(true);
    const { getByText, getByTestId } = render(
      <LoginScreen
        onLogin={onLogin}
        isLoading={false}
        error={null}
        serverUrl="http://127.0.0.1:9091"
        onSetServerUrl={jest.fn()}
        onClearError={jest.fn()}
      />
    );

    expect(getByText('Cloud ABR Engine')).toBeTruthy();
    expect(getByTestId('username-input')).toBeTruthy();
    expect(getByTestId('password-input')).toBeTruthy();
    expect(getByTestId('preset-tenant1')).toBeTruthy();
    expect(getByTestId('preset-tenant2')).toBeTruthy();
  });

  it('selects demo presets when clicked', () => {
    const onLogin = jest.fn().mockResolvedValue(true);
    const { getByTestId } = render(
      <LoginScreen
        onLogin={onLogin}
        isLoading={false}
        error={null}
        serverUrl="http://127.0.0.1:9091"
        onSetServerUrl={jest.fn()}
        onClearError={jest.fn()}
      />
    );

    fireEvent.press(getByTestId('preset-tenant1'));
    expect(getByTestId('username-input').props.value).toBe('tenant1_user');

    fireEvent.press(getByTestId('preset-user2'));
    expect(getByTestId('username-input').props.value).toBe('user2');
  });

  it('submits login when Connect & Authenticate is pressed', async () => {
    const onLogin = jest.fn().mockResolvedValue(true);
    const { getByTestId } = render(
      <LoginScreen
        onLogin={onLogin}
        isLoading={false}
        error={null}
        serverUrl="http://127.0.0.1:9091"
        onSetServerUrl={jest.fn()}
        onClearError={jest.fn()}
      />
    );

    fireEvent.press(getByTestId('login-submit-btn'));
    await waitFor(() => {
      expect(onLogin).toHaveBeenCalledWith('admin', 'password', 'http://127.0.0.1:9091');
    });
  });
});
