import { RecaptchaVerifier, signInWithPhoneNumber, ConfirmationResult } from 'firebase/auth';
import { auth } from './firebaseConfig';

// Global reCAPTCHA verifier instance
let recaptchaVerifier: RecaptchaVerifier | null = null;
let isInitializing = false;

/**
 * Initialize reCAPTCHA verifier
 */
export function initializeRecaptcha(): Promise<void> {
  return new Promise((resolve, reject) => {
    // Check if already initialized
    if (recaptchaVerifier) {
      resolve();
      return;
    }

    // Prevent multiple simultaneous initializations
    if (isInitializing) {
      // Wait a bit and check again
      setTimeout(() => {
        if (recaptchaVerifier) {
          resolve();
        } else {
          reject(new Error('reCAPTCHA initialization in progress'));
        }
      }, 500);
      return;
    }

    isInitializing = true;

    try {
      const container = document.getElementById('recaptcha-container');
      if (!container) {
        isInitializing = false;
        reject(new Error('reCAPTCHA container not found'));
        return;
      }

      // Clear any existing verifier
      if (recaptchaVerifier) {
        try {
          recaptchaVerifier.clear();
        } catch (e) {
          // Ignore clear errors
        }
      }

      // Create new verifier
      recaptchaVerifier = new RecaptchaVerifier(auth, 'recaptcha-container', {
        size: 'invisible',
        callback: () => {
          isInitializing = false;
          resolve();
        },
        'expired-callback': () => {
          // Clear verifier on expiration
          if (recaptchaVerifier) {
            try {
              recaptchaVerifier.clear();
            } catch (e) {
              // Ignore clear errors
            }
            recaptchaVerifier = null;
          }
          isInitializing = false;
        },
      });

      // Render the verifier
      recaptchaVerifier.render().then(() => {
        isInitializing = false;
        resolve();
      }).catch((error) => {
        isInitializing = false;
        // Handle "already being initialized" error
        if (error.message?.includes('already being initialized')) {
          // Wait a bit and try to get the existing verifier
          setTimeout(() => {
            if (recaptchaVerifier) {
              resolve();
            } else {
              reject(error);
            }
          }, 500);
        } else {
          reject(error);
        }
      });
    } catch (error) {
      isInitializing = false;
      reject(error);
    }
  });
}

/**
 * Reset reCAPTCHA verifier
 */
export function resetRecaptcha(): void {
  if (recaptchaVerifier) {
    try {
      recaptchaVerifier.clear();
    } catch (error) {
      // Ignore clear errors
    }
    recaptchaVerifier = null;
  }
  isInitializing = false;
}

/**
 * Cleanup reCAPTCHA verifier
 */
export function cleanupRecaptcha(): void {
  if (recaptchaVerifier) {
    try {
      recaptchaVerifier.clear();
    } catch (error) {
      // Ignore clear errors
    }
    recaptchaVerifier = null;
  }
  isInitializing = false;
}

/**
 * Send OTP to phone number
 */
export async function sendOTP(phoneNumber: string): Promise<ConfirmationResult> {
  // Ensure container exists
  const container = document.getElementById('recaptcha-container');
  if (!container) {
    throw new Error('reCAPTCHA container not found. Please refresh the page.');
  }

  // Initialize reCAPTCHA if not already done
  try {
    await initializeRecaptcha();
  } catch (error: any) {
    // If already initialized, that's okay
    if (!error.message?.includes('already being initialized') && !recaptchaVerifier) {
      throw new Error(`Failed to initialize reCAPTCHA: ${error.message || error}`);
    }
  }

  if (!recaptchaVerifier) {
    throw new Error('reCAPTCHA verifier not available');
  }

  try {
    // Send OTP
    const confirmationResult = await signInWithPhoneNumber(auth, phoneNumber, recaptchaVerifier);
    return confirmationResult;
  } catch (error: any) {
    // Clear verifier on error to allow retry
    resetRecaptcha();
    throw error;
  }
}

/**
 * Verify OTP code
 */
export async function verifyOTP(confirmationResult: ConfirmationResult, otp: string): Promise<void> {
  if (!confirmationResult) {
    throw new Error('No confirmation result available. Please request a new OTP.');
  }

  try {
    await confirmationResult.confirm(otp);
    // Cleanup after successful verification
    cleanupRecaptcha();
  } catch (error: any) {
    throw error;
  }
}
