import React, { useEffect, useState } from 'react';
import { RemoteViewer } from './RemoteViewer';
import { PairingModal } from './PairingModal';
import { Home, Server, Video, LogOut, Plus } from 'lucide-react';

interface DashboardProps {
  token: string;
  onLogout: () => void;
}

export const Dashboard: React.FC<DashboardProps> = ({ token, onLogout }) => {
  const [selectedDevice, setSelectedDevice] = useState<string | null>(null);
  const [homes, setHomes] = useState<{ id: string; name: string }[]>([]);
  const [devices, setDevices] = useState<{ id: string; name: string; home_id: string; status: string }[]>([]);
  const [showPairingModal, setShowPairingModal] = useState(false);
  const [refreshKey, setRefreshKey] = useState(0);

  useEffect(() => {
    const fetchHydration = async () => {
      try {
        const [homesRes, devicesRes] = await Promise.all([
          fetch('/v1/homes', { headers: { 'Authorization': `Bearer ${token}` } }),
          fetch('/v1/devices', { headers: { 'Authorization': `Bearer ${token}` } })
        ]);
        
        if (homesRes.ok) {
          const hData = await homesRes.json();
          setHomes(hData.homes || []);
        }
        if (devicesRes.ok) {
          const dData = await devicesRes.json();
          setDevices(dData.devices || []);
        }
      } catch (err) {
        console.error("Failed to fetch dashboard data:", err);
      }
    };
    fetchHydration();
  }, [token, refreshKey]);

  return (
    <div className="min-h-screen bg-gray-950 text-white flex flex-col">
      <header className="bg-gray-900 border-b border-gray-800 p-4 flex justify-between items-center">
        <div className="flex items-center gap-2">
          <Home className="text-emerald-400" />
          <h1 className="text-xl font-bold">My Homes</h1>
        </div>
        <button 
          onClick={onLogout}
          className="flex items-center gap-2 text-sm text-gray-400 hover:text-white transition"
        >
          <LogOut className="w-4 h-4" />
          Sign Out
        </button>
      </header>

      <div className="flex-1 flex overflow-hidden">
        {/* Sidebar: Device List */}
        <div className="w-64 bg-gray-900/50 border-r border-gray-800 p-4 overflow-y-auto">
          {homes.length === 0 ? (
            <div className="text-sm text-gray-500 text-center mt-10">No homes found.</div>
          ) : (
            homes.map(home => (
              <div key={home.id} className="mb-6">
                <h2 className="text-xs font-semibold text-gray-500 uppercase tracking-wider mb-3 flex justify-between items-center">
                  {home.name}
                  <button 
                    onClick={() => setShowPairingModal(true)}
                    className="hover:text-blue-400 transition"
                    title="Pair New Camera"
                  >
                    <Plus className="w-4 h-4" />
                  </button>
                </h2>
                <div className="space-y-2">
                  {devices.filter(d => d.home_id === home.id).map(device => (
                    <button
                      key={device.id}
                      onClick={() => setSelectedDevice(device.id)}
                      className={`w-full flex items-center gap-3 p-3 rounded-lg text-left transition ${
                        selectedDevice === device.id 
                          ? 'bg-blue-600 text-white' 
                          : 'bg-gray-800 hover:bg-gray-700 text-gray-300'
                      }`}
                    >
                      <Server className="w-5 h-5 flex-shrink-0" />
                      <div className="overflow-hidden">
                        <p className="text-sm font-medium truncate">{device.name}</p>
                        <p className="text-xs text-emerald-400 flex items-center gap-1 mt-0.5">
                          <span className="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
                          {device.status}
                        </p>
                      </div>
                    </button>
                  ))}
                  {devices.filter(d => d.home_id === home.id).length === 0 && (
                    <div className="text-xs text-gray-500 px-2 py-1">No devices paired.</div>
                  )}
                </div>
              </div>
            ))
          )}
        </div>

        {/* Main Content: Viewer */}
        <div className="flex-1 p-6 bg-black overflow-y-auto">
          {selectedDevice ? (
            <div className="max-w-5xl mx-auto space-y-6">
              <div className="flex items-center gap-2 mb-4">
                <Video className="text-blue-400" />
                <h2 className="text-lg font-semibold">Live Camera Feeds</h2>
              </div>
              <RemoteViewer deviceId={selectedDevice} cameraId="cam_001_mock" token={token} />
            </div>
          ) : (
            <div className="h-full flex flex-col items-center justify-center text-gray-500">
              <Server className="w-16 h-16 mb-4 opacity-50" />
              <p>Select a Home Server to view live feeds</p>
            </div>
          )}
        </div>
      </div>

      {showPairingModal && (
        <PairingModal 
          token={token} 
          homes={homes}
          onClose={() => setShowPairingModal(false)} 
          onSuccess={() => {
            setShowPairingModal(false);
            setRefreshKey(k => k + 1);
          }}
        />
      )}
    </div>
  );
};
