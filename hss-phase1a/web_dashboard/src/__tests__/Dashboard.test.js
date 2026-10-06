// @ts-nocheck
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import Dashboard from '../lib/Dashboard.svelte';

describe('Dashboard Component', () => {
  const mockToken = 'mock-jwt-token-123';

  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('renders navbar with title and Sign Out button', async () => {
    globalThis.fetch = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => [],
    });

    render(Dashboard, { props: { token: mockToken } });

    expect(screen.getByText('Pulse Dashboard')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /sign out/i })).toBeInTheDocument();
  });

  it('dispatches logout event when Sign Out button is clicked', async () => {
    globalThis.fetch = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => [],
    });

    const logoutHandler = vi.fn();
    render(Dashboard, {
      props: { token: mockToken },
      events: { logout: logoutHandler },
    });

    const logoutBtn = screen.getByRole('button', { name: /sign out/i });
    await fireEvent.click(logoutBtn);

    expect(logoutHandler).toHaveBeenCalled();
  });

  it('fetches device list from backend with Authorization Bearer header', async () => {
    globalThis.fetch = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => [
        { device_id: 'cam-tenant1-01', role: 'edge' },
        { device_id: 'cam-tenant1-02', role: 'edge' },
      ],
    });

    render(Dashboard, { props: { token: mockToken } });

    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalledWith(
        'http://localhost:9091/api/devices',
        expect.objectContaining({
          headers: { Authorization: `Bearer ${mockToken}` },
        })
      );
    });
  });

  it('displays empty state when no cameras are accessible', async () => {
    globalThis.fetch = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => [],
    });

    render(Dashboard, { props: { token: mockToken } });

    await waitFor(() => {
      expect(screen.getByText('No cameras found')).toBeInTheDocument();
      expect(
        screen.getByText('Your account does not have access to any devices.')
      ).toBeInTheDocument();
    });
  });

  it('renders camera grid and active nodes count when devices are present', async () => {
    const mockDevices = [
      { device_id: 'cam-front-door', role: 'edge' },
      { device_id: 'cam-backyard', role: 'edge' },
    ];

    globalThis.fetch = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => mockDevices,
    });

    render(Dashboard, { props: { token: mockToken } });

    await waitFor(() => {
      expect(screen.getByText('Live Overview')).toBeInTheDocument();
      expect(screen.getByText('2 Active Nodes')).toBeInTheDocument();
      expect(screen.getByRole('heading', { name: 'cam-front-door' })).toBeInTheDocument();
      expect(screen.getByRole('heading', { name: 'cam-backyard' })).toBeInTheDocument();
    });
  });

  it('handles 401 Unauthorized by triggering logout event', async () => {
    globalThis.fetch = vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 401,
      json: async () => ({ error: 'Unauthorized' }),
    });

    const logoutHandler = vi.fn();
    render(Dashboard, {
      props: { token: mockToken },
      events: { logout: logoutHandler },
    });

    await waitFor(() => {
      expect(logoutHandler).toHaveBeenCalled();
    });
  });

  it('displays error message when device loading fails', async () => {
    globalThis.fetch = vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 500,
      json: async () => ({ error: 'Internal Server Error' }),
    });

    render(Dashboard, { props: { token: mockToken } });

    await waitFor(() => {
      expect(screen.getByText('Failed to load devices')).toBeInTheDocument();
    });
  });

  it('allows user to switch between 1-Cam, 2-Cam, 3-Cam, and 4-Cam grid layouts', async () => {
    const mockDevices = [
      { device_id: 'cam-01', role: 'edge' },
      { device_id: 'cam-02', role: 'edge' },
      { device_id: 'cam-03', role: 'edge' },
      { device_id: 'cam-04', role: 'edge' },
    ];

    globalThis.fetch = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => mockDevices,
    });

    render(Dashboard, { props: { token: mockToken } });

    await waitFor(() => {
      expect(screen.getByText('4 Active Nodes')).toBeInTheDocument();
    });

    const btn1Cam = screen.getByRole('button', { name: '1 Cam' });
    const btn2Cam = screen.getByRole('button', { name: '2 Cams' });
    const btn3Cam = screen.getByRole('button', { name: '3 Cams' });
    const btn4Cam = screen.getByRole('button', { name: '4 Cams' });

    expect(btn1Cam).toBeInTheDocument();
    expect(btn2Cam).toBeInTheDocument();
    expect(btn3Cam).toBeInTheDocument();
    expect(btn4Cam).toBeInTheDocument();

    await fireEvent.click(btn1Cam);
    expect(btn1Cam.className).toContain('bg-indigo-600');

    await fireEvent.click(btn4Cam);
    expect(btn4Cam.className).toContain('bg-indigo-600');
    expect(btn1Cam.className).not.toContain('bg-indigo-600');
  });

  it('filters cameras according to search query and shows empty message when none match', async () => {
    const mockDevices = [
      { device_id: 'cam-front-door', role: 'edge' },
      { device_id: 'cam-garage', role: 'edge' },
    ];

    globalThis.fetch = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => mockDevices,
    });

    render(Dashboard, { props: { token: mockToken } });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'cam-front-door' })).toBeInTheDocument();
      expect(screen.getByRole('heading', { name: 'cam-garage' })).toBeInTheDocument();
    });

    const searchInput = screen.getByPlaceholderText('Filter cameras...');
    await fireEvent.input(searchInput, { target: { value: 'garage' } });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'cam-garage' })).toBeInTheDocument();
      expect(screen.queryByRole('heading', { name: 'cam-front-door' })).not.toBeInTheDocument();
      expect(screen.getByText('1 Active Node')).toBeInTheDocument();
    });

    await fireEvent.input(searchInput, { target: { value: 'nonexistent' } });
    await waitFor(() => {
      expect(screen.getByText('No matching cameras')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Clear Filter' })).toBeInTheDocument();
    });

    const clearBtn = screen.getByRole('button', { name: 'Clear Filter' });
    // When clearBtn is clicked, it clears filter and calls fetchDevices
    globalThis.fetch = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => mockDevices,
    });
    await fireEvent.click(clearBtn);

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'cam-front-door' })).toBeInTheDocument();
      expect(screen.getByRole('heading', { name: 'cam-garage' })).toBeInTheDocument();
    });
  });

  it('renders camera selection dropdown with All Cameras, Local Webcam, and individual edge cameras', async () => {
    const mockDevices = [
      { device_id: 'cam-lobby', role: 'edge' },
      { device_id: 'cam-warehouse', role: 'edge' },
    ];

    globalThis.fetch = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => mockDevices,
    });

    render(Dashboard, { props: { token: mockToken } });

    await waitFor(() => {
      expect(screen.getByLabelText(/camera selection/i)).toBeInTheDocument();
    });

    const select = screen.getByLabelText(/camera selection/i);
    expect(select).toBeInTheDocument();

    const options = Array.from(select.querySelectorAll('option')).map(o => o.textContent);
    expect(options).toContain('All Cameras (Grid View)');
    expect(options).toContain('Local Webcam');
    expect(options).toContain('cam-lobby');
    expect(options).toContain('cam-warehouse');
  });

  it('renders local webcam stream when Local Webcam option is selected in camera dropdown', async () => {
    const mockDevices = [
      { device_id: 'cam-lobby', role: 'edge' },
    ];

    globalThis.fetch = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => mockDevices,
    });

    render(Dashboard, { props: { token: mockToken } });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'cam-lobby' })).toBeInTheDocument();
    });

    const select = screen.getByLabelText(/camera selection/i);
    await fireEvent.change(select, { target: { value: 'local-webcam' } });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Local Webcam' })).toBeInTheDocument();
      expect(screen.getByText('Browser MediaStream')).toBeInTheDocument();
    });
  });

  it('renders single camera stream when individual edge camera is selected and returns to grid on All Cameras', async () => {
    const mockDevices = [
      { device_id: 'cam-front', role: 'edge' },
      { device_id: 'cam-back', role: 'edge' },
    ];

    globalThis.fetch = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => mockDevices,
    });

    render(Dashboard, { props: { token: mockToken } });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'cam-front' })).toBeInTheDocument();
      expect(screen.getByRole('heading', { name: 'cam-back' })).toBeInTheDocument();
    });

    const select = screen.getByLabelText(/camera selection/i);

    // Select single camera 'cam-front'
    await fireEvent.change(select, { target: { value: 'cam-front' } });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'cam-front' })).toBeInTheDocument();
      expect(screen.queryByRole('heading', { name: 'cam-back' })).not.toBeInTheDocument();
    });

    // Re-select 'all' to return to grid view
    await fireEvent.change(select, { target: { value: 'all' } });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'cam-front' })).toBeInTheDocument();
      expect(screen.getByRole('heading', { name: 'cam-back' })).toBeInTheDocument();
    });
  });
});
