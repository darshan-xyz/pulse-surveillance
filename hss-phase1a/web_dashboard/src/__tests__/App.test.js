// @ts-nocheck
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import App from '../App.svelte';

describe('App Root Component', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    localStorage.clear();
  });

  it('renders Login view when no token exists in localStorage', () => {
    render(App);

    expect(screen.getByText('Pulse Cloud')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /sign in/i })).toBeInTheDocument();
  });

  it('renders Dashboard view directly when valid token exists in localStorage', async () => {
    localStorage.setItem('pulse_token', 'persisted-jwt-token-777');
    globalThis.fetch = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => [],
    });

    render(App);

    await waitFor(() => {
      expect(screen.getByText('Pulse Dashboard')).toBeInTheDocument();
    });
  });

  it('transitions from Login to Dashboard on successful authentication and stores token', async () => {
    const testToken = 'auth-token-flow-999';
    globalThis.fetch = vi.fn()
      // First fetch: POST /api/auth/login
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ token: testToken }),
      })
      // Second fetch: GET /api/devices in Dashboard
      .mockResolvedValueOnce({
        ok: true,
        json: async () => [{ device_id: 'cam-01', role: 'edge' }],
      });

    render(App);

    const usernameInput = screen.getByLabelText('Username');
    const passwordInput = screen.getByLabelText('Password');
    const submitBtn = screen.getByRole('button', { name: /sign in/i });

    await fireEvent.input(usernameInput, { target: { value: 'testuser' } });
    await fireEvent.input(passwordInput, { target: { value: 'password123' } });
    await fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(localStorage.getItem('pulse_token')).toBe(testToken);
      expect(screen.getByText('Pulse Dashboard')).toBeInTheDocument();
    });
  });

  it('transitions from Dashboard to Login on logout and removes token', async () => {
    localStorage.setItem('pulse_token', 'active-token-888');
    globalThis.fetch = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => [],
    });

    render(App);

    await waitFor(() => {
      expect(screen.getByText('Pulse Dashboard')).toBeInTheDocument();
    });

    const logoutBtn = screen.getByRole('button', { name: /sign out/i });
    await fireEvent.click(logoutBtn);

    await waitFor(() => {
      expect(localStorage.getItem('pulse_token')).toBeNull();
      expect(screen.getByText('Pulse Cloud')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /sign in/i })).toBeInTheDocument();
    });
  });
});
