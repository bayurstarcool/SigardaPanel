export interface User {
  id: number;
  username: string;
  email: string;
  role: 'super_admin' | 'admin' | 'user';
}

export interface ImpersonatorInfo {
  id: number;
  username: string;
  email: string;
  role: string;
}

export interface Server {
  id: number;
  name: string;
  hostname: string;
  status: string;
  agent_url?: string;
  ip_address?: string;
  created_at: string;
  updated_at: string;
  sites_count?: number;
}

export interface Site {
  id: number;
  server_id: number;
  name: string;
  domain: string;
  runtime: string;
  root_path: string;
  status: string;
  owner_id?: number;
  ssl_status?: string;
  ssl_issuer?: string;
  ssl_expires_at?: string;
  git_repo?: string;
  git_branch?: string;
  git_commit?: string;
  managed_by?: string;
  system_user?: string;
  conf_file?: string;
  proxy_pass_url?: string;
  created_at: string;
}

export interface Job {
  id: number;
  type: string;
  status: string;
  payload?: any;
  created_at: string;
  started_at?: string;
  completed_at?: string;
  progress?: number;
  error?: string;
  site_id?: number;
  server_id?: number;
}

export interface Backup {
  id: number;
  site_id: number;
  name: string;
  size?: number;
  status: string;
  created_at: string;
  path?: string;
}

export interface Database {
  id: number;
  server_id: number;
  name: string;
  type: string;
  status: string;
  domain?: string;
}

export interface JobLog {
  id: number;
  job_id: number;
  message: string;
  level: string;
  created_at: string;
}

export interface DBUser {
  id: number;
  username: string;
  host: string;
  db_name?: string;
}
