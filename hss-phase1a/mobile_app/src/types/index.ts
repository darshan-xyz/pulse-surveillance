export interface UserInfo {
  username: string;
  tenant_id?: string;
  role?: string;
}

export interface LoginResponse {
  token: string;
  user: UserInfo;
}

export interface Device {
  device_id: string;
  role: string;
  name?: string;
  status?: 'online' | 'offline' | 'streaming';
}

export interface SignalingMessage {
  type: 'offer' | 'answer' | 'candidate';
  session_id: string;
  device_id?: string;
  camera_id?: string;
  sdp?: string;
  candidate?: string;
  turn_server?: string;
  turn_username?: string;
  turn_password?: string;
  error?: string;
}

export interface PushRegistration {
  push_token: string;
  platform: 'ios' | 'android' | 'web';
}

export interface PushNotificationPayload {
  id: string;
  title: string;
  body: string;
  data?: Record<string, any>;
  receivedAt: number;
}
