import React, { useState, useEffect, useCallback } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { ArrowLeft, Smartphone, CheckCircle, XCircle } from 'lucide-react';
import { toast } from 'sonner';

interface PhoneOTPInputProps {
  phoneNumber: string;
  onBack: () => void;
  onSuccess: () => void;
  onError: (error: string) => void;
}

export function PhoneOTPInput({ phoneNumber, onBack, onSuccess, onError }: PhoneOTPInputProps) {
  const [otp, setOtp] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [timeLeft, setTimeLeft] = useState(60);
  const [canResend, setCanResend] = useState(false);
  const [confirmationResult, setConfirmationResult] = useState<any>(null);
  const [hasSentOTP, setHasSentOTP] = useState(false);

  // Timer for resend button
  useEffect(() => {
    if (timeLeft > 0) {
      const timer = setTimeout(() => setTimeLeft(timeLeft - 1), 1000);
      return () => clearTimeout(timer);
    } else {
      setCanResend(true);
    }
  }, [timeLeft]);

  // Define sendOTP function using useCallback to avoid closure issues
  const sendOTP = useCallback(async () => {
    // Validate phone number before sending
    if (!phoneNumber || phoneNumber.trim().length === 0) {
      onError('Phone number is required');
      return;
    }
    
    // Basic validation - should be at least 8 characters (country code + number)
    const cleanNumber = phoneNumber.replace(/[^\d+]/g, '');
    if (cleanNumber.length < 8) {
      onError('Please enter a complete phone number');
      return;
    }
    
    setIsLoading(true);
    try {
      // Ensure container exists before sending
      const container = document.getElementById('recaptcha-container');
      if (!container) {
        throw new Error('reCAPTCHA container not found. Please refresh the page.');
      }

      const { sendOTP: sendOTPFunction, resetRecaptcha } = await import('@/lib/firebasePhoneAuth');
      
      // Reset reCAPTCHA before sending to ensure fresh state
      resetRecaptcha();
      
      // Small delay to ensure reCAPTCHA is ready
      await new Promise(resolve => setTimeout(resolve, 200));
      
      const result = await sendOTPFunction(phoneNumber);
      setConfirmationResult(result);
      setTimeLeft(60);
      setCanResend(false);
      setHasSentOTP(true);
      toast.success('OTP sent successfully!');
    } catch (error: any) {
      console.error('Error sending OTP:', error);
      
      // Provide more specific error messages
      let errorMessage = 'Failed to send OTP';
      if (error.message?.includes('billing-not-enabled')) {
        errorMessage = 'Firebase Phone Auth requires billing to be enabled. Please enable billing in your Firebase Console (Settings → Billing). Note: Phone Auth is free for testing with generous quotas.';
      } else if (error.message?.includes('Invalid phone number format') || error.message?.includes('invalid-phone-number')) {
        errorMessage = 'Invalid phone number format. Please check your number.';
      } else if (error.message?.includes('too-many-requests')) {
        errorMessage = 'Too many requests. Please try again later.';
      } else if (error.message?.includes('quota-exceeded')) {
        errorMessage = 'SMS quota exceeded. Please try again later.';
      } else if (error.message?.includes('reCAPTCHA')) {
        errorMessage = 'reCAPTCHA error. Please refresh the page and try again.';
      } else if (error.message) {
        errorMessage = error.message;
      }
      
      onError(errorMessage);
      setHasSentOTP(false); // Allow retry
    } finally {
      setIsLoading(false);
    }
  }, [phoneNumber, onError]); // Include dependencies

  // Send OTP when component mounts or phoneNumber changes (only if not already sent)
  useEffect(() => {
    if (!hasSentOTP && phoneNumber) {
      // Small delay to ensure DOM is ready
      const timer = setTimeout(() => {
        sendOTP();
      }, 100);
      return () => clearTimeout(timer);
    }
  }, [phoneNumber, hasSentOTP, sendOTP]); // Include all dependencies

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      // Clean up reCAPTCHA when component unmounts
      import('@/lib/firebasePhoneAuth').then(({ cleanupRecaptcha }) => {
        cleanupRecaptcha();
      }).catch(err => {
        console.warn('Error cleaning up reCAPTCHA:', err);
      });
    };
  }, []);

  const handleVerifyOTP = async () => {
    if (otp.length !== 6) {
      toast.error('Please enter a valid 6-digit OTP');
      return;
    }

    setIsLoading(true);
    try {
      const { verifyOTP } = await import('@/lib/firebasePhoneAuth');
      await verifyOTP(confirmationResult, otp);
      toast.success('Phone number verified successfully!');
      onSuccess();
    } catch (error: any) {
      console.error('Error verifying OTP:', error);
      onError(error.message || 'Invalid OTP. Please try again.');
    } finally {
      setIsLoading(false);
    }
  };

  const handleResendOTP = async () => {
    setOtp('');
    setTimeLeft(60);
    setCanResend(false);
    setHasSentOTP(false); // Reset to allow resend
    await sendOTP();
  };

  const handleOtpChange = (value: string) => {
    // Only allow digits and limit to 6 characters
    const digits = value.replace(/\D/g, '').slice(0, 6);
    setOtp(digits);
  };

  return (
    <Card className="w-full max-w-md mx-auto">
      <CardHeader className="text-center">
        <div className="flex items-center justify-center mb-4">
          <div className="w-12 h-12 bg-blue-100 dark:bg-blue-900/20 rounded-full flex items-center justify-center">
            <Smartphone className="w-6 h-6 text-blue-600 dark:text-blue-400" />
          </div>
        </div>
        <CardTitle className="text-xl font-semibold">Verify Phone Number</CardTitle>
        <p className="text-sm text-gray-600 dark:text-gray-400">
          We sent a 6-digit code to <br />
          <span className="font-medium text-gray-900 dark:text-gray-100">{phoneNumber}</span>
        </p>
      </CardHeader>
      
      <CardContent className="space-y-6">
        {/* OTP Input */}
        <div className="space-y-2">
          <Label htmlFor="otp">Enter verification code</Label>
          <Input
            id="otp"
            type="text"
            value={otp}
            onChange={(e) => handleOtpChange(e.target.value)}
            placeholder="000000"
            className="text-center text-lg tracking-widest"
            maxLength={6}
            disabled={isLoading}
          />
        </div>

        {/* Timer and Resend */}
        <div className="text-center">
          {!canResend ? (
            <p className="text-sm text-gray-600 dark:text-gray-400">
              Resend code in {timeLeft}s
            </p>
          ) : (
            <Button
              variant="link"
              onClick={handleResendOTP}
              disabled={isLoading}
              className="text-blue-600 hover:text-blue-700"
            >
              Resend code
            </Button>
          )}
        </div>

        {/* Action Buttons */}
        <div className="space-y-3">
          <Button
            onClick={handleVerifyOTP}
            disabled={otp.length !== 6 || isLoading}
            className="w-full"
          >
            {isLoading ? (
              <div className="flex items-center gap-2">
                <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
                Verifying...
              </div>
            ) : (
              <div className="flex items-center gap-2">
                <CheckCircle className="w-4 h-4" />
                Verify Code
              </div>
            )}
          </Button>

          <Button
            variant="outline"
            onClick={onBack}
            disabled={isLoading}
            className="w-full"
          >
            <ArrowLeft className="w-4 h-4 mr-2" />
            Back to Login
          </Button>
        </div>

        {/* reCAPTCHA Container - required for Firebase Phone Auth (invisible reCAPTCHA)
            Note: Container must exist in DOM but can be visually hidden for invisible reCAPTCHA */}
        <div 
          id="recaptcha-container" 
          aria-hidden="true"
          style={{ 
            position: 'absolute',
            left: '-9999px',
            width: '1px',
            height: '1px',
            overflow: 'hidden'
          }}
        />
      </CardContent>
    </Card>
  );
}









