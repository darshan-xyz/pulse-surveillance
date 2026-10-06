import { Router } from 'express';
import { createClient } from '@supabase/supabase-js';

const router = Router();

// Initialize Supabase client with fallback pattern
let supabaseAdmin: any = null;
try {
  const supabaseUrl = process.env.SUPABASE_URL || 'https://bujskrxtswtogpamnflh.supabase.co';
  const supabaseServiceKey = process.env.SUPABASE_SERVICE_ROLE_KEY || process.env.VITE_SUPABASE_ANON_KEY || 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzdXBhYmFzZSIsInJlZiI6ImJ1anNrcnh0c3d0b2dwYW1uZmxoIiwicm9sZSI6ImFub24iLCJpYXQiOjE3NTMxNjMzMjMsImV4cCI6MjA2ODczOTMyM30.0ToD4XmfdZOWZEJCH5wWLKCTBm7QhQv7-YnMuNMK5oQ';
  
  supabaseAdmin = createClient(supabaseUrl, supabaseServiceKey, {
    auth: {
      autoRefreshToken: false,
      persistSession: false
    }
  });
  console.log("✅ Supabase admin client initialized successfully");
} catch (error) {
  console.warn("⚠️ Supabase admin client initialization failed:", error);
  // Mock admin client for development
  supabaseAdmin = {
    auth: {
      admin: {
        getUserByEmail: async (email: string) => ({ data: null, error: { message: 'User not found' } }),
        signInWithPassword: async (credentials: any) => ({ data: null, error: { message: 'Invalid credentials' } })
      }
    }
  };
}

// Check if user exists by email
router.post('/check-user-exists', async (req, res) => {
  try {
    const { email } = req.body;
    
    if (!email) {
      return res.status(400).json({ error: 'Email is required' });
    }

    console.log('Checking if user exists for email:', email);

    // Use admin API to check if user exists
    const { data, error } = await supabaseAdmin.auth.admin.getUserByEmail(email);
    
    if (error) {
      console.log('Admin API error:', error.message);
      // If user not found, return false
      if (error.message.includes('User not found')) {
        return res.json({ userExists: false });
      }
      // For other errors, return error
      return res.status(500).json({ error: error.message });
    }
    
    // User exists
    console.log('User exists:', data.user?.id);
    return res.json({ userExists: true });
  } catch (error) {
    console.error('Error checking user existence:', error);
    return res.status(500).json({ error: 'Internal server error' });
  }
});

// Validate login credentials
router.post('/validate-login', async (req, res) => {
  try {
    const { email, password } = req.body;
    
    if (!email || !password) {
      return res.status(400).json({ error: 'Email and password are required' });
    }

    console.log('Validating login for email:', email);

    // First, check if user exists by querying the public.users table
    let userExists = false;
    try {
      // Use the regular Supabase client to query the public.users table
      const { data: userData, error: userError } = await supabaseAdmin
        .from('users')
        .select('id, email')
        .eq('email', email)
        .single();
      
      if (userError) {
        if (userError.code === 'PGRST116') {
          // No rows returned - user doesn't exist
          userExists = false;
          console.log('User not found in database');
        } else {
          console.log('Error checking user existence:', userError.message);
          userExists = null;
        }
      } else if (userData) {
        userExists = true;
        console.log('User found in database:', userData.id);
      }
    } catch (checkError) {
      console.log('Failed to check user existence:', checkError);
      userExists = null;
    }

    // Now try to authenticate with the provided credentials
    const { data, error } = await supabaseAdmin.auth.signInWithPassword({
      email,
      password,
    });
    
    if (error) {
      console.log('Login attempt failed:', error.message);
      const errorMessage = error.message.toLowerCase();
      
      if (
        errorMessage.includes('invalid login credentials') ||
        errorMessage.includes('invalid email or password') ||
        errorMessage.includes('invalid credentials')
      ) {
        // We have generic invalid credentials error
        if (userExists === true) {
          // User exists but password is wrong
          console.log('User exists but password is wrong');
          return res.json({ 
            userExists: true, 
            isWrongPassword: true, 
            message: 'Wrong password' 
          });
        } else if (userExists === false) {
          // User doesn't exist
          console.log('User does not exist');
          return res.json({ 
            userExists: false, 
            isWrongPassword: false, 
            message: 'User not found' 
          });
        } else {
          // We couldn't determine user existence, assume user doesn't exist
          console.log('Could not determine user existence, assuming user not found');
          return res.json({ 
            userExists: false, 
            isWrongPassword: false, 
            message: 'User not found' 
          });
        }
      }
      
      if (
        errorMessage.includes('user not found') ||
        errorMessage.includes('email not found') ||
        errorMessage.includes('no user found')
      ) {
        return res.json({ 
          userExists: false, 
          isWrongPassword: false, 
          message: 'User not found' 
        });
      }
      
      // Other errors
      return res.status(500).json({ error: error.message });
    }
    
    // Login successful
    console.log('Login successful for user:', data.user.id);
    return res.json({ 
      userExists: true, 
      isWrongPassword: false, 
      message: 'Login successful',
      user: data.user 
    });
  } catch (error) {
    console.error('Error validating login:', error);
    return res.status(500).json({ error: 'Internal server error' });
  }
});

export default router;
