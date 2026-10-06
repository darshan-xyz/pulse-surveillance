import { supabase } from './supabaseClient';

export async function fetchCameras(userId) {
  try {
    // Check authentication with retry for network issues
    let user;
    let authError;
    const maxAuthRetries = 2;
    
    for (let attempt = 0; attempt <= maxAuthRetries; attempt++) {
      const authResult = await supabase.auth.getUser();
      user = authResult.data?.user;
      authError = authResult.error;
      
      // If we got a user, we're good
      if (user) break;
      
      // If it's a network error and we have retries left, try again
      if (authError && attempt < maxAuthRetries) {
        const isNetworkError = 
          authError.message?.includes('NetworkError') ||
          authError.message?.includes('Failed to fetch') ||
          authError.message?.includes('timeout') ||
          authError.message?.includes('ECONNRESET') ||
          authError.message?.includes('Request timeout');
        
        if (isNetworkError) {
          // Wait a bit before retrying
          await new Promise(resolve => setTimeout(resolve, 300 * (attempt + 1)));
          continue;
        }
      }
      
      // If it's not a network error or we've exhausted retries, throw appropriate error
      if (authError || !user) {
        const isNetworkError = 
          authError?.message?.includes('NetworkError') ||
          authError?.message?.includes('Failed to fetch') ||
          authError?.message?.includes('timeout') ||
          authError?.message?.includes('ECONNRESET') ||
          authError?.message?.includes('Request timeout');
        
        if (isNetworkError) {
          throw new Error('Network error: Unable to verify authentication. Please check your connection and try again.');
        } else {
          throw new Error('Authentication required. Please log in again.');
        }
      }
    }
    
    if (!user) {
      throw new Error('Authentication required. Please log in again.');
    }

    // Use the authenticated user's ID for consistency
    const authenticatedUserId = user.id;

  const { data, error } = await supabase
    .from('cameras')
    .select('*')
      .eq('user_id', authenticatedUserId);
    
    if (error) {
      console.error('Supabase fetchCameras error:', error);
      throw error;
    }
    
    // Transform the data to match frontend expectations
    const transformedData = (data || []).map(cam => ({
      ...cam,
      isActive: cam.is_active, // Map database column to frontend property
      detectionTypes: cam.detectionTypes || []
    }));
    
    return transformedData;
  } catch (error) {
    console.error('Error fetching cameras:', error);
    throw error;
  }
}

export async function addCamera(camera) {
  try {
  // Validate required fields
    if (!camera.name || !camera.source || !camera.detectionTypes) {
    throw new Error('Missing required camera fields');
    }

    // Check authentication with retry for network issues
    let user;
    let authError;
    const maxAuthRetries = 2;
    
    for (let attempt = 0; attempt <= maxAuthRetries; attempt++) {
      const authResult = await supabase.auth.getUser();
      user = authResult.data?.user;
      authError = authResult.error;
      
      // If we got a user, we're good
      if (user) break;
      
      // If it's a network error and we have retries left, try again
      if (authError && attempt < maxAuthRetries) {
        const isNetworkError = 
          authError.message?.includes('NetworkError') ||
          authError.message?.includes('Failed to fetch') ||
          authError.message?.includes('timeout') ||
          authError.message?.includes('ECONNRESET') ||
          authError.message?.includes('Request timeout');
        
        if (isNetworkError) {
          // Wait a bit before retrying
          await new Promise(resolve => setTimeout(resolve, 300 * (attempt + 1)));
          continue;
        }
      }
      
      // If it's not a network error or we've exhausted retries, throw appropriate error
      if (authError || !user) {
        const isNetworkError = 
          authError?.message?.includes('NetworkError') ||
          authError?.message?.includes('Failed to fetch') ||
          authError?.message?.includes('timeout') ||
          authError?.message?.includes('ECONNRESET') ||
          authError?.message?.includes('Request timeout');
        
        if (isNetworkError) {
          throw new Error('Network error: Unable to verify authentication. Please check your connection and try again.');
        } else {
          throw new Error('Authentication required. Please log in again.');
        }
      }
    }
    
    if (!user) {
      throw new Error('Authentication required. Please log in again.');
    }

    // Use the authenticated user's ID directly
    const userId = user.id;

    // Prepare camera data - using the actual column names from the database
    const cameraData = {
      user_id: userId, // Use the authenticated user's ID
      name: camera.name,
      source: camera.source,
      url: camera.url || null,
      detectionTypes: camera.detectionTypes, // Using the correct column name
      is_active: camera.isActive !== undefined ? camera.isActive : true, // Using snake_case
      stream_id: null,
      stream_urls: null
    };



    // Only log in debug mode
    if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
      console.log('Inserting camera data:', cameraData);
    }

    const { data, error, status } = await supabase
      .from('cameras')
      .insert([cameraData])
      .select(); // Add select() to return the inserted data

    // Only log in debug mode
    if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
      console.log('Supabase addCamera response:', { data, error, status });
    }
    
    if (error) {
      console.error('Supabase error:', error);
      throw new Error(`Database error: ${error.message}`);
    }
    
    if (!data || data.length === 0) {
      throw new Error('Insert failed: No data returned. This is likely due to RLS or a schema issue.');
    }
    
    // Transform the returned data to match frontend expectations
    const camData = data[0];
    return {
      ...camData,
      isActive: camData.is_active, // Map database column to frontend property
      detectionTypes: camData.detectionTypes || []
    };
  } catch (error) {
    console.error('Error adding camera:', error);
    throw error;
  }
}

export async function updateCamera(id, updates) {
  try {
    // Only log in debug mode
    if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
      console.log('updateCamera called with:', { id, updates });
    }
    
    // Check authentication with retry for network issues
    let user;
    let authError;
    const maxAuthRetries = 2;
    
    for (let attempt = 0; attempt <= maxAuthRetries; attempt++) {
      const authResult = await supabase.auth.getUser();
      user = authResult.data?.user;
      authError = authResult.error;
      
      // If we got a user, we're good
      if (user) break;
      
      // If it's a network error and we have retries left, try again
      if (authError && attempt < maxAuthRetries) {
        const isNetworkError = 
          authError.message?.includes('NetworkError') ||
          authError.message?.includes('Failed to fetch') ||
          authError.message?.includes('timeout') ||
          authError.message?.includes('ECONNRESET');
        
        if (isNetworkError) {
          // Wait a bit before retrying
          await new Promise(resolve => setTimeout(resolve, 300 * (attempt + 1)));
          continue;
        }
      }
      
      // If it's not a network error or we've exhausted retries, check if we should throw
      if (authError || !user) {
        // Only throw if it's a real auth error (not a network issue)
        const isNetworkError = 
          authError?.message?.includes('NetworkError') ||
          authError?.message?.includes('Failed to fetch') ||
          authError?.message?.includes('timeout') ||
          authError?.message?.includes('ECONNRESET');
        
        if (!isNetworkError) {
          throw new Error('Authentication required');
        } else {
          // If it's a network error after retries, throw a more descriptive error
          throw new Error('Network error: Unable to verify authentication. Please check your connection.');
        }
      }
    }
    
    if (!user) {
      throw new Error('Authentication required');
    }

    // Convert updates to match database column names
    const dbUpdates: any = {};
    if (updates.detectionTypes) dbUpdates.detectionTypes = updates.detectionTypes;
    if (updates.isActive !== undefined) dbUpdates.is_active = updates.isActive;
    if (updates.name) dbUpdates.name = updates.name;
    if (updates.source) dbUpdates.source = updates.source;
    if (updates.url) dbUpdates.url = updates.url;

    // Only log in debug mode
    if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
      console.log('Database updates object:', dbUpdates);
    }

  const { data, error } = await supabase
    .from('cameras')
      .update(dbUpdates)
      .eq('id', id)
      .eq('user_id', user.id) // Ensure user owns the camera
      .select();

    // Only log in debug mode
    if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
      console.log('Supabase update response:', { data, error });
    }

  if (error) {
    // Enhance error message with more context
    const enhancedError = new Error(error.message || 'Failed to update camera');
    (enhancedError as any).code = error.code;
    (enhancedError as any).details = error.details;
    (enhancedError as any).hint = error.hint;
    throw enhancedError;
  }
    
    // Transform the returned data to match frontend expectations
    if (data && data.length > 0) {
      const camData = data[0];
      return {
        ...camData,
        isActive: camData.is_active, // Map database column to frontend property
        detectionTypes: camData.detectionTypes || []
      };
    }
    return null;
  } catch (error) {
    console.error('Error updating camera:', error);
    throw error;
  }
}

export async function deleteCamera(id) {
  try {
    // Check authentication with retry for network issues
    let user;
    let authError;
    const maxAuthRetries = 2;
    
    for (let attempt = 0; attempt <= maxAuthRetries; attempt++) {
      const authResult = await supabase.auth.getUser();
      user = authResult.data?.user;
      authError = authResult.error;
      
      // If we got a user, we're good
      if (user) break;
      
      // If it's a network error and we have retries left, try again
      if (authError && attempt < maxAuthRetries) {
        const isNetworkError = 
          authError.message?.includes('NetworkError') ||
          authError.message?.includes('Failed to fetch') ||
          authError.message?.includes('timeout') ||
          authError.message?.includes('ECONNRESET') ||
          authError.message?.includes('Request timeout');
        
        if (isNetworkError) {
          // Wait a bit before retrying
          await new Promise(resolve => setTimeout(resolve, 300 * (attempt + 1)));
          continue;
        }
      }
      
      // If it's not a network error or we've exhausted retries, throw appropriate error
      if (authError || !user) {
        const isNetworkError = 
          authError?.message?.includes('NetworkError') ||
          authError?.message?.includes('Failed to fetch') ||
          authError?.message?.includes('timeout') ||
          authError?.message?.includes('ECONNRESET') ||
          authError?.message?.includes('Request timeout');
        
        if (isNetworkError) {
          throw new Error('Network error: Unable to verify authentication. Please check your connection and try again.');
        } else {
          throw new Error('Authentication required. Please log in again.');
        }
      }
    }
    
    if (!user) {
      throw new Error('Authentication required. Please log in again.');
    }

    // Get camera details to check if it has a stream
    const { data: camera, error: fetchError } = await supabase
      .from('cameras')
      .select('stream_id, source')
      .eq('id', id)
      .eq('user_id', user.id) // Ensure user owns the camera
      .single();

    if (fetchError) {
      console.error('Error fetching camera for deletion:', fetchError);
      throw fetchError;
    }



  const { data, error } = await supabase
    .from('cameras')
    .delete()
      .eq('id', id)
      .eq('user_id', user.id) // Ensure user owns the camera
      .select();

  if (error) throw error;
    return data && data.length > 0 ? data[0] : null;
  } catch (error) {
    console.error('Error deleting camera:', error);
    throw error;
  }
} 