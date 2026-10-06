import { useState } from 'react';
import { Auth } from './Auth';
import { Dashboard } from './Dashboard';
import './App.css';

function App() {
  const [token, setToken] = useState<string | null>(localStorage.getItem('hss_token'));

  const handleLogin = (newToken: string) => {
    localStorage.setItem('hss_token', newToken);
    setToken(newToken);
  };

  const handleLogout = () => {
    localStorage.removeItem('hss_token');
    setToken(null);
  };

  return (
    <>
      {!token ? (
        <Auth onLogin={handleLogin} />
      ) : (
        <Dashboard token={token} onLogout={handleLogout} />
      )}
    </>
  );
}

export default App;
