import { supabase } from './supabaseClient';

// Types for multi-tenant architecture
export interface MultiTenantCamera {
  id: string;
  name: string;
  source: 'webcam' | 'ip camera' | 'rtsp';
  url?: string;
  detectionTypes: string[];
  isActive: boolean;

  createdAt: Date;
  updatedAt: Date;
}

export interface MultiTenantDetectionRecord {
  id: string;
  cameraId: string;
  cameraName: string;
  detectionType: string;
  confidence?: number;
  imageUrl?: string;
  videoUrl?: string;
  timestamp: Date;
  createdAt: Date;
}

export interface MultiTenantRecording {
  id: string;
  cameraId: string;
  streamId?: string;
  filePath: string;
  fileSize?: number;
  duration?: number;
  startTime?: Date;
  endTime?: Date;
  createdAt: Date;
}

export interface UserSetting {
  id: string;
  settingKey: string;
  settingValue: any;
  createdAt: Date;
  updatedAt: Date;
}

// Helper function to get user schema name
function getUserSchemaName(userId: string): string {
  return `user_${userId.substring(0, 8)}`;
}

// Multi-tenant client class
export class MultiTenantClient {
  private userId: string | null = null;

  constructor() {
    this.initialize();
  }

  private async initialize() {
    try {
      const { data: { user }, error } = await supabase.auth.getUser();
      if (error) {
        // Don't log this as an error if it's just a missing session (user not logged in)
        if (error.message.includes('Auth session missing') || error.message.includes('No session')) {
          console.log('No authenticated user found during initialization');
        } else {
          console.error('Error getting user during initialization:', error);
        }
        this.userId = null;
        return;
      }
      this.userId = user?.id || null;
      console.log('MultiTenantClient initialized with user:', this.userId);
    } catch (error) {
      console.error('Failed to initialize MultiTenantClient:', error);
      this.userId = null;
    }
  }

  // Public method to re-initialize when user changes
  async reinitialize() {
    console.log('Reinitializing MultiTenantClient...');
    await this.initialize();
  }

  private async ensureUserSchema() {
    if (!this.userId) {
      throw new Error('User not authenticated');
    }

    try {
      // Check if user schema exists, if not create it
      const { data: userRecord, error: userError } = await supabase
        .from('users')
        .select('schema_name')
        .eq('id', this.userId)
        .single();

      if (userError && userError.code !== 'PGRST116') {
        console.error('Error checking user record:', userError);
        throw new Error(`Failed to check user record: ${userError.message}`);
      }

      if (!userRecord) {
        console.log('Creating user schema for user:', this.userId);
        
        // Create schema manually using a direct SQL call
        const schemaName = getUserSchemaName(this.userId);
        
        // Create the schema and tables using a function call
        const { error: createError } = await supabase.rpc('create_user_schema', {
          user_uuid: this.userId
        });

        if (createError) {
          console.error('Error creating user schema:', createError);
          throw new Error(`Failed to create user schema: ${createError.message}`);
        }

        // Grant permissions
        const { error: grantError } = await supabase.rpc('grant_user_permissions', {
          user_uuid: this.userId
        });

        if (grantError) {
          console.error('Error granting permissions:', grantError);
          // Don't throw here, as the schema might still work
        }

        // Insert into users table
        const { error: insertError } = await supabase
          .from('users')
          .insert({ id: this.userId, schema_name: schemaName });

        if (insertError) {
          console.error('Error inserting user record:', insertError);
          // Don't throw here, as the schema might still work
        }

        console.log('User schema created successfully for user:', this.userId);
      } else {
        console.log('User schema already exists for user:', this.userId);
      }
    } catch (error) {
      console.error('Error in ensureUserSchema:', error);
      throw error;
    }
  }

  // Camera operations
  async getCameras(): Promise<MultiTenantCamera[]> {
    try {
      await this.ensureUserSchema();
      const schemaName = getUserSchemaName(this.userId!);

      console.log('Fetching cameras for schema:', schemaName);

      // Use raw SQL query to access schema tables
      const { data, error } = await supabase
        .rpc('get_user_cameras', { user_schema: schemaName });

      if (error) {
        console.error('Error getting cameras:', error);
        throw new Error(`Failed to get cameras: ${error.message}`);
      }

      console.log('Cameras fetched successfully:', data);

      return (data || []).map(camera => ({
        ...camera,
        isActive: camera.is_active,
        detectionTypes: camera.detectionTypes || [],
        createdAt: new Date(camera.created_at),
        updatedAt: new Date(camera.updated_at)
      }));
    } catch (error) {
      console.error('Error in getCameras:', error);
      throw error;
    }
  }

  async addCamera(camera: Omit<MultiTenantCamera, 'id' | 'createdAt' | 'updatedAt'>): Promise<MultiTenantCamera> {
    try {
      await this.ensureUserSchema();
      const schemaName = getUserSchemaName(this.userId!);

      console.log('Adding camera to schema:', schemaName, camera);

      // Use raw SQL query to insert into schema table
      const { data, error } = await supabase
        .rpc('add_user_camera', {
          user_schema: schemaName,
                  camera_data: {
          name: camera.name,
          source: camera.source,
          url: camera.url,
          detectionTypes: camera.detectionTypes,
          is_active: camera.isActive,

        }
        });

      if (error) {
        console.error('Supabase error adding camera:', error);
        throw new Error(`Database error: ${error.message}`);
      }

      if (!data) {
        throw new Error('No data returned from camera insertion');
      }

      console.log('Camera added successfully:', data);

      return {
        ...data,
        isActive: data.is_active,
        detectionTypes: data.detectionTypes || [],
        createdAt: new Date(data.created_at),
        updatedAt: new Date(data.updated_at)
      };
    } catch (error) {
      console.error('Error in addCamera:', error);
      if (error instanceof Error) {
        throw error;
      } else {
        throw new Error(`Unknown error: ${String(error)}`);
      }
    }
  }

  async updateCamera(id: string, updates: Partial<MultiTenantCamera>): Promise<MultiTenantCamera> {
    try {
      await this.ensureUserSchema();
      const schemaName = getUserSchemaName(this.userId!);

      const updateData: any = {};
      if (updates.name) updateData.name = updates.name;
      if (updates.source) updateData.source = updates.source;
      if (updates.url !== undefined) updateData.url = updates.url;
      if (updates.detectionTypes) updateData.detectionTypes = updates.detectionTypes;
      if (updates.isActive !== undefined) updateData.isActive = updates.isActive;


      console.log('Updating camera in schema:', schemaName, { id, updates: updateData });

      const { data, error } = await supabase
        .rpc('update_user_camera', {
          user_schema: schemaName,
          camera_id: id,
          camera_data: updateData
        });

      if (error) {
        console.error('Supabase error updating camera:', error);
        throw new Error(`Database error: ${error.message}`);
      }

      if (!data || data.length === 0) {
        throw new Error('No data returned from camera update');
      }

      const camera = data[0];
      return {
        ...camera,
        isActive: camera.is_active,
        detectionTypes: camera.detectionTypes || [],
        createdAt: new Date(camera.created_at),
        updatedAt: new Date(camera.updated_at)
      };
    } catch (error) {
      console.error('Error in updateCamera:', error);
      if (error instanceof Error) {
        throw error;
      } else {
        throw new Error(`Unknown error: ${String(error)}`);
      }
    }
  }

  async deleteCamera(id: string): Promise<void> {
    try {
      await this.ensureUserSchema();
      const schemaName = getUserSchemaName(this.userId!);

      console.log('Deleting camera from schema:', schemaName, id);

      const { error } = await supabase
        .rpc('delete_user_camera', {
          user_schema: schemaName,
          camera_id: id
        });

      if (error) {
        console.error('Supabase error deleting camera:', error);
        throw new Error(`Database error: ${error.message}`);
      }

      console.log('Camera deleted successfully');
    } catch (error) {
      console.error('Error in deleteCamera:', error);
      if (error instanceof Error) {
        throw error;
      } else {
        throw new Error(`Unknown error: ${String(error)}`);
      }
    }
  }

  // Detection records operations
  async getDetectionRecords(limit = 50): Promise<MultiTenantDetectionRecord[]> {
    try {
      await this.ensureUserSchema();
      const schemaName = getUserSchemaName(this.userId!);

      const { data, error } = await supabase
        .from(`${schemaName}.detection_records`)
        .select('*')
        .order('timestamp', { ascending: false })
        .limit(limit);

      if (error) throw error;

      return (data || []).map(record => ({
        ...record,
        cameraId: record.camera_id,
        cameraName: record.camera_name,
        detectionType: record.detection_type,
        timestamp: new Date(record.timestamp),
        createdAt: new Date(record.created_at)
      }));
    } catch (error) {
      console.error('Error in getDetectionRecords:', error);
      throw error;
    }
  }

  async addDetectionRecord(record: Omit<MultiTenantDetectionRecord, 'id' | 'createdAt'>): Promise<MultiTenantDetectionRecord> {
    try {
      await this.ensureUserSchema();
      const schemaName = getUserSchemaName(this.userId!);

      const { data, error } = await supabase
        .from(`${schemaName}.detection_records`)
        .insert([{
          camera_id: record.cameraId,
          camera_name: record.cameraName,
          detection_type: record.detectionType,
          confidence: record.confidence,
          image_url: record.imageUrl,
          video_url: record.videoUrl,
          timestamp: record.timestamp
        }])
        .select()
        .single();

      if (error) throw error;

      return {
        ...data,
        cameraId: data.camera_id,
        cameraName: data.camera_name,
        detectionType: data.detection_type,
        timestamp: new Date(data.timestamp),
        createdAt: new Date(data.created_at)
      };
    } catch (error) {
      console.error('Error in addDetectionRecord:', error);
      throw error;
    }
  }

  // Recordings operations
  async getRecordings(): Promise<MultiTenantRecording[]> {
    try {
      await this.ensureUserSchema();
      const schemaName = getUserSchemaName(this.userId!);

      const { data, error } = await supabase
        .from(`${schemaName}.recordings`)
        .select('*')
        .order('created_at', { ascending: false });

      if (error) throw error;

      return (data || []).map(recording => ({
        ...recording,
        cameraId: recording.camera_id,
        streamId: recording.stream_id,
        filePath: recording.file_path,
        startTime: recording.start_time ? new Date(recording.start_time) : undefined,
        endTime: recording.end_time ? new Date(recording.end_time) : undefined,
        createdAt: new Date(recording.created_at)
      }));
    } catch (error) {
      console.error('Error in getRecordings:', error);
      throw error;
    }
  }

  async addRecording(recording: Omit<MultiTenantRecording, 'id' | 'createdAt'>): Promise<MultiTenantRecording> {
    try {
      await this.ensureUserSchema();
      const schemaName = getUserSchemaName(this.userId!);

      const { data, error } = await supabase
        .from(`${schemaName}.recordings`)
        .insert([{
          camera_id: recording.cameraId,
          stream_id: recording.streamId,
          file_path: recording.filePath,
          file_size: recording.fileSize,
          duration: recording.duration,
          start_time: recording.startTime,
          end_time: recording.endTime
        }])
        .select()
        .single();

      if (error) throw error;

      return {
        ...data,
        cameraId: data.camera_id,
        streamId: data.stream_id,
        filePath: data.file_path,
        startTime: data.start_time ? new Date(data.start_time) : undefined,
        endTime: data.end_time ? new Date(data.end_time) : undefined,
        createdAt: new Date(data.created_at)
      };
    } catch (error) {
      console.error('Error in addRecording:', error);
      throw error;
    }
  }

  // User settings operations
  async getSettings(): Promise<UserSetting[]> {
    try {
      await this.ensureUserSchema();
      const schemaName = getUserSchemaName(this.userId!);

      const { data, error } = await supabase
        .from(`${schemaName}.user_settings`)
        .select('*')
        .order('setting_key');

      if (error) throw error;

      return (data || []).map(setting => ({
        ...setting,
        settingKey: setting.setting_key,
        settingValue: setting.setting_value,
        createdAt: new Date(setting.created_at),
        updatedAt: new Date(setting.updated_at)
      }));
    } catch (error) {
      console.error('Error in getSettings:', error);
      throw error;
    }
  }

  async getSetting(key: string): Promise<UserSetting | null> {
    try {
      await this.ensureUserSchema();
      const schemaName = getUserSchemaName(this.userId!);

      const { data, error } = await supabase
        .from(`${schemaName}.user_settings`)
        .select('*')
        .eq('setting_key', key)
        .single();

      if (error) {
        if (error.code === 'PGRST116') return null; // No rows returned
        throw error;
      }

      return {
        ...data,
        settingKey: data.setting_key,
        settingValue: data.setting_value,
        createdAt: new Date(data.created_at),
        updatedAt: new Date(data.updated_at)
      };
    } catch (error) {
      console.error('Error in getSetting:', error);
      throw error;
    }
  }

  async updateSetting(key: string, value: any): Promise<UserSetting> {
    try {
      await this.ensureUserSchema();
      const schemaName = getUserSchemaName(this.userId!);

      const { data, error } = await supabase
        .from(`${schemaName}.user_settings`)
        .upsert({
          setting_key: key,
          setting_value: value
        })
        .select()
        .single();

      if (error) throw error;

      return {
        ...data,
        settingKey: data.setting_key,
        settingValue: data.setting_value,
        createdAt: new Date(data.created_at),
        updatedAt: new Date(data.updated_at)
      };
    } catch (error) {
      console.error('Error in updateSetting:', error);
      throw error;
    }
  }

  // Migration helper
  async migrateFromOldStructure(): Promise<void> {
    // This function can be used to migrate data from the old single-table structure
    // to the new multi-tenant structure
    console.log('Migration not implemented yet');
  }
}

// Export singleton instance
export const multiTenantClient = new MultiTenantClient(); 