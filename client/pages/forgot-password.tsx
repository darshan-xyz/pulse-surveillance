import React, { useState } from 'react';
import { supabase } from '@/lib/supabaseClient';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useNavigate } from 'react-router-dom';

export default function ForgotPassword() {
  const [email, setEmail] = useState('');
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    const redirectTo = window.location.origin + '/reset-password';
    const { error } = await supabase.auth.resetPasswordForEmail(email, { redirectTo });
    setLoading(false);
    if (error) {
      toast.error(error.message || 'Failed to send password reset email.');
    } else {
      toast.success('If this email exists in our system, a password reset link has been sent.');
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-green-100 via-emerald-200 to-teal-100 dark:from-slate-900 dark:via-green-900 dark:to-slate-900 p-4">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>Forgot Password</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            <Input
              type="email"
              placeholder="Enter your email"
              value={email}
              onChange={e => setEmail(e.target.value)}
              required
              disabled={loading}
            />
            <Button type="submit" className="w-full" disabled={loading}>
              {loading ? 'Sending...' : 'Send Reset Email'}
            </Button>
          </form>
          <div className="mt-4 text-center">
            <a
              href="/login"
              className="text-sm text-green-600 dark:text-green-400 hover:underline"
            >
              Return to Login
            </a>
          </div>
          <div className="flex justify-center mt-8">
            <Button
              type="button"
              className="w-full max-w-xs"
              onClick={() => window.open('https://mail.google.com', '_blank')}
            >
              Open Gmail
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
} 