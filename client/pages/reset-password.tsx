import React, { useState, useEffect } from 'react';
import { supabase } from '@/lib/supabaseClient';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useNavigate } from 'react-router-dom';

export default function ResetPassword() {
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [loading, setLoading] = useState(false);
  const [canShowForm, setCanShowForm] = useState(false);
  const navigate = useNavigate();

  // Only show the form if the URL contains a recovery token or type=recovery
  useEffect(() => {
    const hash = window.location.hash;
    const params = new URLSearchParams(window.location.search);
    if (
      hash.includes('access_token') ||
      params.get('type') === 'recovery' ||
      window.location.href.includes('type=recovery')
    ) {
      setCanShowForm(true);
    } else {
      // Optionally, show an error or redirect
      setCanShowForm(false);
    }
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (newPassword !== confirmPassword) {
      toast.error('Passwords do not match.');
      return;
    }
    setLoading(true);
    // Get current user session
    const { data: { session }, error: sessionError } = await supabase.auth.getSession();
    if (sessionError || !session) {
      setLoading(false);
      toast.error('Session expired or invalid. Please use the reset link again.');
      return;
    }
    // Check if new password matches the old one
    const { user } = session;
    const email = user?.email;
    if (email) {
      const { error: signInError } = await supabase.auth.signInWithPassword({ email, password: newPassword });
      if (!signInError) {
        setLoading(false);
        toast.error('New password cannot be the same as the old password.');
        return;
      }
    }
    // Update password
    const { error } = await supabase.auth.updateUser({ password: newPassword });
    setLoading(false);
    if (error) {
      toast.error(error.message || 'Failed to update password.');
    } else {
      toast.success('Password updated successfully! Redirecting to home...');
      setTimeout(() => navigate('/'), 1500);
    }
  };

  if (!canShowForm) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-green-100 via-emerald-200 to-teal-100 dark:from-slate-900 dark:via-green-900 dark:to-slate-900 p-4">
        <Card className="w-full max-w-md">
          <CardHeader>
            <CardTitle>Invalid or Expired Reset Link</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-center">Please use the password reset link sent to your email, or request a new one.</p>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-green-100 via-emerald-200 to-teal-100 dark:from-slate-900 dark:via-green-900 dark:to-slate-900 p-4">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>Reset Password</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            <Input
              type="password"
              placeholder="New password"
              value={newPassword}
              onChange={e => setNewPassword(e.target.value)}
              required
              disabled={loading}
            />
            <Input
              type="password"
              placeholder="Confirm new password"
              value={confirmPassword}
              onChange={e => setConfirmPassword(e.target.value)}
              required
              disabled={loading}
            />
            <Button type="submit" className="w-full" disabled={loading}>
              {loading ? 'Updating...' : 'Update Password'}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
} 