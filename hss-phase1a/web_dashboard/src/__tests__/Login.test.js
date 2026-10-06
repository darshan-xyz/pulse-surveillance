// @ts-nocheck
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import Login from '../lib/Login.svelte';

describe('Login Component', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('renders login form with username, password, and sign in button', () => {
    render(Login);

    expect(screen.getByText('Pulse Cloud')).toBeInTheDocument();
    expect(screen.getByText('Sign in to access your secure cameras')).toBeInTheDocument();
    expect(screen.getByLabelText('Username')).toBeInTheDocument();
    expect(screen.getByLabelText('Password')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /sign in/i })).toBeInTheDocument();
  });

  it('allows user to input username and password', async () => {
    render(Login);

    const usernameInput = screen.getByLabelText('Username');
    const passwordInput = screen.getByLabelText('Password');

    await fireEvent.input(usernameInput, { target: { value: 'admin' } });
    await fireEvent.input(passwordInput, { target: { value: 'admin123' } });

    expect(usernameInput).toHaveValue('admin');
    expect(passwordInput).toHaveValue('admin123');
  });

  it('dispatches success event with JWT token upon successful authentication', async () => {
    const mockToken = 'jwt-token-xyz-12345';
    globalThis.fetch = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ token: mockToken }),
    });

    const successHandler = vi.fn();
    render(Login, {
      events: { success: successHandler },
    });

    const usernameInput = screen.getByLabelText('Username');
    const passwordInput = screen.getByLabelText('Password');
    const submitBtn = screen.getByRole('button', { name: /sign in/i });

    await fireEvent.input(usernameInput, { target: { value: 'admin' } });
    await fireEvent.input(passwordInput, { target: { value: 'pulsepass' } });
    await fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalledWith(
        'http://localhost:9091/api/auth/login',
        expect.objectContaining({
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ username: 'admin', password: 'pulsepass' }),
        })
      );
      expect(successHandler).toHaveBeenCalledWith(
        expect.objectContaining({
          detail: { token: mockToken },
        })
      );
    });
  });

  it('displays error message when login fails with invalid credentials', async () => {
    globalThis.fetch = vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 401,
      json: async () => ({ error: 'Invalid credentials' }),
    });

    render(Login);

    const usernameInput = screen.getByLabelText('Username');
    const passwordInput = screen.getByLabelText('Password');
    const submitBtn = screen.getByRole('button', { name: /sign in/i });

    await fireEvent.input(usernameInput, { target: { value: 'wronguser' } });
    await fireEvent.input(passwordInput, { target: { value: 'wrongpass' } });
    await fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(screen.getByText('Invalid credentials')).toBeInTheDocument();
    });
  });

  it('handles network failure error gracefully', async () => {
    globalThis.fetch = vi.fn().mockRejectedValueOnce(new Error('Network connection failed'));

    render(Login);

    const usernameInput = screen.getByLabelText('Username');
    const passwordInput = screen.getByLabelText('Password');
    const submitBtn = screen.getByRole('button', { name: /sign in/i });

    await fireEvent.input(usernameInput, { target: { value: 'admin' } });
    await fireEvent.input(passwordInput, { target: { value: 'admin' } });
    await fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(screen.getByText('Network connection failed')).toBeInTheDocument();
    });
  });

  it('shows loading state during form submission', async () => {
    let resolveFetch;
    const fetchPromise = new Promise((resolve) => {
      resolveFetch = resolve;
    });
    globalThis.fetch = vi.fn().mockReturnValueOnce(fetchPromise);

    render(Login);

    const usernameInput = screen.getByLabelText('Username');
    const passwordInput = screen.getByLabelText('Password');
    const submitBtn = screen.getByRole('button', { name: /sign in/i });

    await fireEvent.input(usernameInput, { target: { value: 'admin' } });
    await fireEvent.input(passwordInput, { target: { value: 'admin' } });
    await fireEvent.click(submitBtn);

    expect(screen.getByRole('button', { name: /authenticating/i })).toHaveTextContent('Authenticating...');
    expect(screen.getByRole('button', { name: /authenticating/i })).toBeDisabled();

    // Resolve fetch
    resolveFetch({
      ok: true,
      json: async () => ({ token: 'mock-token' }),
    });

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /sign in/i })).toHaveTextContent('Sign In');
    });
  });

  it('renders Google sign-in option and triggers authentication on click', async () => {
    const mockToken = 'google-token-456';
    globalThis.fetch = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ token: mockToken }),
    });

    const successHandler = vi.fn();
    render(Login, {
      events: { success: successHandler },
    });

    const googleBtn = screen.getByRole('button', { name: /continue with google/i });
    expect(googleBtn).toBeInTheDocument();

    await fireEvent.click(googleBtn);

    await waitFor(() => {
      expect(successHandler).toHaveBeenCalledWith(
        expect.objectContaining({
          detail: expect.objectContaining({
            token: expect.any(String),
          }),
        })
      );
    });
  });
});
