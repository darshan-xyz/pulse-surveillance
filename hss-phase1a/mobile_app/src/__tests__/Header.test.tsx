import React from 'react';
import { render, fireEvent } from '@testing-library/react-native';
import { Header } from '../components/Header';

describe('Header Component', () => {
  it('renders brand title and user / tenant metadata', () => {
    const user = { username: 'tenant1_user', tenant_id: 'tenant-1', role: 'admin' };
    const onLogout = jest.fn();

    const { getByText, getByTestId } = render(<Header user={user} onLogout={onLogout} />);

    expect(getByText('Pulse')).toBeTruthy();
    expect(getByTestId('header-tenant-id').props.children).toBe('tenant-1');
    expect(getByTestId('header-username').props.children).toBe('tenant1_user');
  });

  it('triggers logout when Sign Out is pressed', () => {
    const user = { username: 'admin', tenant_id: 'tenant-alpha', role: 'admin' };
    const onLogout = jest.fn();

    const { getByTestId } = render(<Header user={user} onLogout={onLogout} />);

    fireEvent.press(getByTestId('logout-button'));
    expect(onLogout).toHaveBeenCalledTimes(1);
  });
});
