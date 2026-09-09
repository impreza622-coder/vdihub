export interface App {
  StartUp(ctx: any): void;
  Shutdown(ctx: any): void;
  GetWindow(): any;
}

export interface AuthService {
  SetBaseURL(url: string): void;
  GetBaseURL(): string;
  SetToken(token: string): void;
  GetToken(): string;
  IsLoggedIn(): boolean;
  Login(username: string, password: string): Promise<Record<string, any>>;
  Register(username: string, password: string, email: string): Promise<void>;
}

export interface VMService {
  ListVMs(): Promise<VMInfo[]>;
  StartVM(vmid: number): Promise<void>;
  StopVM(vmid: number): Promise<void>;
  RebootVM(vmid: number): Promise<void>;
  GetConnection(vmid: number): Promise<ConnectionInfo | null>;
}

export interface ProtocolService {
  LaunchRDP(host: string, port: number, password: string): Promise<void>;
  LaunchRDPWithAuth(host: string, port: number, username: string, password: string): Promise<void>;
  LaunchSPICE(host: string, port: number, tlsPort: number, password: string): Promise<void>;
  LaunchSPICEViaPVE(pveEndpoint: string, pveUsername: string, pvePassword: string, pveNode: string, pveVMID: number, verifySSL: boolean): Promise<void>;
  LaunchVNC(host: string, port: number, password: string): Promise<void>;
  LaunchHorizon(brokerURL: string, username: string, password: string, domain: string): Promise<void>;
  LaunchProtocolFull(protocolType: string, host: string, port: number, username: string, password: string, tlsPort: number, brokerURL: string): Promise<void>;
  LaunchProtocol(protocolType: string, host: string, port: number, password: string, tlsPort: number): Promise<void>;
  CheckDependency(protocolType: string): Promise<[boolean, string]>;
  GetConfigTemplate(protocolType: string, host: string, port: number, password: string, tlsPort: number): Promise<string>;
}

export interface VMInfo {
  vmid: number;
  name: string;
  status: string;
  node: string;
  cpu_cores: number;
  memory_mb: number;
  disk_gb: number;
  os: string;
  protocol: string;
  ip_address: string;
  vm_username?: string;
  vm_password?: string;
  pve_endpoint?: string;
  pve_username?: string;
  pve_password?: string;
  pve_node?: string;
  pve_vmid?: number;
  pve_verify_ssl?: boolean;
}

export interface ConnectionInfo {
  session_id: string;
  protocol: string;
  host: string;
  port: number;
  password: string;
  expires_at: string;
  tls_port?: number;
  username?: string;
  vm_password?: string;
  broker_url?: string;
}

export type ProtocolType = 'rdp' | 'spice' | 'vnc' | 'ssh' | 'horizon';

export interface ProtocolDependency {
  available: boolean;
  message: string;
}
