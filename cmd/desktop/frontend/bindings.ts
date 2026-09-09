// Hybrid bindings: uses real Wails Go backend when available, falls back to Mock for browser dev.
// Client-side features (ProtocolConfig, VMProtocolPreference, OfflineVM) always use localStorage.

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
  // 电源控制后端（与协议解耦）: "none" | "pve" | "hyperv" | "horizon"
  power_backend?: string;
  // ---- PVE 后端专用字段（Proxmox VE API）----
  pve_endpoint?: string;
  pve_username?: string;
  pve_password?: string;
  pve_node?: string;
  pve_vmid?: number;
  pve_verify_ssl?: boolean;
  // ---- Hyper-V 后端专用字段（Windows Hyper-V PowerShell Cmdlet）----
  hyperv_host?: string;        // 主机名，空/"."=本机
  hyperv_vmname?: string;      // Hyper-V 虚拟机名称（精确匹配）
  hyperv_use_winrm?: boolean;  // 是否使用 WinRM 远程
  hyperv_username?: string;    // WinRM 账号（本机可空）
  hyperv_password?: string;    // WinRM 密码
  hyperv_force?: boolean;      // 关机/重启是否强制（硬关 vs 软关）
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

// ============== 系统级嵌入 Kiosk（替换 Shell / 自动登录 / 控制面板注册） ==============
export interface SystemKioskStatus {
  uninstallRegistered: boolean;
  uninstallPublisher: string;
  uninstallVersion: string;
  shellReplaced: boolean;
  currentShell: string;
  ourExePath: string;
  autoLogonEnabled: boolean;
  autoLogonUser: string;
  autoLogonDomain: string;
  isElevated: boolean;
}
export interface SystemKioskConfig {
  registerUninstall: boolean;
  uninstallPublisher: string;
  uninstallVersion: string;
  replaceShell: boolean;
  enableAutoLogin: boolean;
  autoLoginUser: string;
  autoLoginPwd: string;
  autoLoginDomain: string;
  scheduleReboot: boolean;
}
export interface ApplyKioskResult {
  scheduledReboot: boolean;
}

// ============== Wails Environment Detection ==============
const isWails = typeof window !== 'undefined' &&
  (window as any).go !== undefined &&
  (window as any).go.main !== undefined;

function wailsCall(service: string, method: string, ...args: any[]): Promise<any> {
  const goObj = (window as any).go.main;
  const fn = goObj[service]?.[method];
  if (typeof fn !== 'function') {
    return Promise.reject(new Error(`Wails 方法 ${service}.${method} 不可用`));
  }
  return fn(...args);
}

// ============== Mock Data (browser dev fallback) ==============
const mockVMs: VMInfo[] = [
  {
    vmid: 101,
    name: 'Windows 11 开发机',
    status: 'running',
    node: 'pve-node-01',
    cpu_cores: 4,
    memory_mb: 8192,
    disk_gb: 120,
    os: 'Windows 11',
    protocol: 'rdp',
    ip_address: '192.168.1.101',
    vm_username: 'Administrator',
    vm_password: 'Win11@Dev2026',
  },
  {
    vmid: 102,
    name: 'Ubuntu 办公机',
    status: 'running',
    node: 'pve-node-01',
    cpu_cores: 2,
    memory_mb: 4096,
    disk_gb: 80,
    os: 'Ubuntu 22.04',
    protocol: 'spice',
    ip_address: '192.168.1.102',
    vm_username: 'user',
    vm_password: 'Ubuntu@Office2026',
  },
  {
    vmid: 103,
    name: 'CentOS 服务器',
    status: 'stopped',
    node: 'pve-node-02',
    cpu_cores: 8,
    memory_mb: 16384,
    disk_gb: 500,
    os: 'CentOS 8',
    protocol: 'vnc',
    ip_address: '192.168.1.103',
    vm_username: 'root',
    vm_password: 'CentOS@Server2026',
  },
  {
    vmid: 104,
    name: 'Horizon 云桌面',
    status: 'running',
    node: 'pve-node-02',
    cpu_cores: 4,
    memory_mb: 8192,
    disk_gb: 200,
    os: 'Windows 10',
    protocol: 'horizon',
    ip_address: '192.168.1.104',
    vm_username: 'domain\\user01',
    vm_password: 'Horizon@2026',
  },
  {
    vmid: 105,
    name: 'Kali 渗透测试',
    status: 'stopped',
    node: 'pve-node-01',
    cpu_cores: 4,
    memory_mb: 8192,
    disk_gb: 100,
    os: 'Kali Linux',
    protocol: 'vnc',
    ip_address: '192.168.1.105',
    vm_username: 'kali',
    vm_password: 'Kali@Pentest2026',
  },
  {
    vmid: 106,
    name: 'macOS 设计机',
    status: 'running',
    node: 'pve-node-03',
    cpu_cores: 6,
    memory_mb: 16384,
    disk_gb: 300,
    os: 'macOS 14',
    protocol: 'spice',
    ip_address: '192.168.1.106',
    vm_username: 'designer',
    vm_password: 'Mac@Design2026',
  },
  {
    vmid: 107,
    name: 'Windows Server 2022',
    status: 'running',
    node: 'pve-node-02',
    cpu_cores: 8,
    memory_mb: 32768,
    disk_gb: 500,
    os: 'Windows Server 2022',
    protocol: 'rdp',
    ip_address: '192.168.1.107',
    vm_username: 'Administrator',
    vm_password: 'WinServer@2026',
  },
  {
    vmid: 108,
    name: 'Debian 12 测试机',
    status: 'stopped',
    node: 'pve-node-03',
    cpu_cores: 2,
    memory_mb: 2048,
    disk_gb: 60,
    os: 'Debian 12',
    protocol: 'vnc',
    ip_address: '192.168.1.108',
    vm_username: 'tester',
    vm_password: 'Debian@Test2026',
  },
  {
    vmid: 109,
    name: 'Red Hat Enterprise',
    status: 'running',
    node: 'pve-node-01',
    cpu_cores: 16,
    memory_mb: 65536,
    disk_gb: 1000,
    os: 'RHEL 9',
    protocol: 'spice',
    ip_address: '192.168.1.109',
    vm_username: 'admin',
    vm_password: 'RHEL@Admin2026',
  },
];

// ============== App ==============
export interface AutostartStatus {
  normal: boolean;
  kiosk_auto: boolean;
}

export interface StartHint {
  kiosk_auto: boolean;
  exe_path: string;
}

export class App {
  StartUp(_ctx: any): void {}
  Shutdown(_ctx: any): void {}
  GetWindow(): any { return null; }

  async ApplyKioskMode(): Promise<void> {
    if (isWails) { return wailsCall('App', 'ApplyKioskMode'); }
    return Promise.resolve();
  }

  async ExitKioskMode(): Promise<void> {
    if (isWails) { return wailsCall('App', 'ExitKioskMode'); }
    return Promise.resolve();
  }

  async GetAutostart(): Promise<AutostartStatus> {
    if (isWails) {
      const r = await wailsCall('App', 'GetAutostart');
      return {
        normal: Boolean((r as any)?.Normal ?? (r as any)?.normal ?? false),
        kiosk_auto: Boolean((r as any)?.KioskAuto ?? (r as any)?.kiosk_auto ?? false),
      };
    }
    // 浏览器 fallback：读 localStorage
    try {
      return {
        normal: localStorage.getItem('autostart_normal') === 'true',
        kiosk_auto: localStorage.getItem('autostart_kiosk') === 'true',
      };
    } catch {
      return { normal: false, kiosk_auto: false };
    }
  }

  async SetAutostart(enabled: boolean, kioskAuto: boolean): Promise<void> {
    if (isWails) { return wailsCall('App', 'SetAutostart', enabled, kioskAuto); }
    // 浏览器 fallback
    try {
      localStorage.setItem('autostart_normal', (enabled && !kioskAuto) ? 'true' : 'false');
      localStorage.setItem('autostart_kiosk', (enabled && kioskAuto) ? 'true' : 'false');
    } catch {}
    return Promise.resolve();
  }

  async GetStartHint(): Promise<StartHint> {
    if (isWails) {
      const r = await wailsCall('App', 'GetStartHint');
      return {
        kiosk_auto: Boolean((r as any)?.KioskAuto ?? (r as any)?.kiosk_auto ?? false),
        exe_path: String((r as any)?.ExePath ?? (r as any)?.exe_path ?? ''),
      };
    }
    return Promise.resolve({ kiosk_auto: false, exe_path: '' });
  }

  async LockdownKioskClose(): Promise<void> {
    if (isWails) { return wailsCall('App', 'LockdownKioskClose'); }
    return Promise.resolve();
  }

  async UnlockKioskClose(): Promise<void> {
    if (isWails) { return wailsCall('App', 'UnlockKioskClose'); }
    return Promise.resolve();
  }

  // 关闭本地机器电源（Kiosk 模式下用户无法访问 Windows 关机选项）
  async ShutdownMachine(): Promise<void> {
    if (isWails) { return wailsCall('App', 'ShutdownMachine'); }
    return Promise.resolve();
  }

  // 重启本地机器
  async RestartMachine(): Promise<void> {
    if (isWails) { return wailsCall('App', 'RestartMachine'); }
    return Promise.resolve();
  }

  // 真正退出程序（不是隐藏到托盘，供全局设置"退出程序"按钮调用）
  async QuitApplication(): Promise<void> {
    if (isWails) { return wailsCall('App', 'QuitApplication'); }
    return Promise.resolve();
  }

  // ============== 系统级嵌入 Kiosk（替换 Shell / 自动登录 / 控制面板注册） ==============
  async GetSystemKioskStatus(): Promise<SystemKioskStatus> {
    if (isWails) {
      const r = await wailsCall('App', 'GetSystemKioskStatus');
      return {
        uninstallRegistered: Boolean((r as any)?.UninstallRegistered ?? (r as any)?.uninstallRegistered ?? false),
        uninstallPublisher: String((r as any)?.UninstallPublisher ?? (r as any)?.uninstallPublisher ?? ''),
        uninstallVersion: String((r as any)?.UninstallVersion ?? (r as any)?.uninstallVersion ?? ''),
        shellReplaced: Boolean((r as any)?.ShellReplaced ?? (r as any)?.shellReplaced ?? false),
        currentShell: String((r as any)?.CurrentShell ?? (r as any)?.currentShell ?? ''),
        ourExePath: String((r as any)?.OurExePath ?? (r as any)?.ourExePath ?? ''),
        autoLogonEnabled: Boolean((r as any)?.AutoLogonEnabled ?? (r as any)?.autoLogonEnabled ?? false),
        autoLogonUser: String((r as any)?.AutoLogonUser ?? (r as any)?.autoLogonUser ?? ''),
        autoLogonDomain: String((r as any)?.AutoLogonDomain ?? (r as any)?.autoLogonDomain ?? ''),
        isElevated: Boolean((r as any)?.IsElevated ?? (r as any)?.isElevated ?? false),
      };
    }
    return Promise.resolve({
      uninstallRegistered: false, uninstallPublisher: '', uninstallVersion: '',
      shellReplaced: false, currentShell: 'explorer.exe', ourExePath: '',
      autoLogonEnabled: false, autoLogonUser: '', autoLogonDomain: '',
      isElevated: false,
    });
  }

  async ApplySystemKioskConfig(cfg: SystemKioskConfig): Promise<ApplyKioskResult> {
    if (isWails) {
      const r = await wailsCall('App', 'ApplySystemKioskConfig', cfg);
      return { scheduledReboot: Boolean((r as any)?.ScheduledReboot ?? (r as any)?.scheduledReboot ?? false) };
    }
    return Promise.resolve({ scheduledReboot: false });
  }

  async RevertSystemKiosk(scheduleReboot: boolean): Promise<ApplyKioskResult> {
    if (isWails) {
      const r = await wailsCall('App', 'RevertSystemKiosk', scheduleReboot);
      return { scheduledReboot: Boolean((r as any)?.ScheduledReboot ?? (r as any)?.scheduledReboot ?? false) };
    }
    return Promise.resolve({ scheduledReboot: false });
  }

  async RebootSystemNow(): Promise<void> {
    if (isWails) { return wailsCall('App', 'RebootSystemNow'); }
    return Promise.resolve();
  }

  async Tray(): Promise<void> {
    if (isWails) { return wailsCall('App', 'Tray'); }
    return Promise.resolve();
  }

  async WindowMinimize(): Promise<void> {
    if (isWails) { return wailsCall('App', 'WindowMinimize'); }
    // 浏览器 fallback：尝试 window.minimize（Electron-like 环境），否则无操作
    try { (window as any).minimize?.(); } catch {}
    return Promise.resolve();
  }

  async WindowToggleMaximize(): Promise<boolean> {
    if (isWails) {
      const result = await wailsCall('App', 'WindowToggleMaximize');
      return Boolean(result);
    }
    return Promise.resolve(false);
  }

  async WindowClose(): Promise<void> {
    if (isWails) { return wailsCall('App', 'WindowClose'); }
    // 浏览器 fallback：尽量隐藏窗口（不可完全关闭页面）
    try { (window as any).hide?.(); } catch {}
    return Promise.resolve();
  }

  async WindowIsMaximized(): Promise<boolean> {
    if (isWails) {
      const result = await wailsCall('App', 'WindowIsMaximized');
      return Boolean(result);
    }
    return Promise.resolve(false);
  }

  async WindowStartDrag(): Promise<void> {
    if (isWails) { return wailsCall('App', 'WindowStartDrag'); }
    return Promise.resolve();
  }

  async WindowGetRect(): Promise<{x:number;y:number;w:number;h:number}> {
    if (isWails) {
      const r = await wailsCall('App', 'WindowGetRect');
      if (r && typeof r === 'object') {
        return {
          x: Number((r as any).x ?? (r as any).X ?? 0),
          y: Number((r as any).y ?? (r as any).Y ?? 0),
          w: Number((r as any).w ?? (r as any).W ?? (r as any).Width ?? 0),
          h: Number((r as any).h ?? (r as any).H ?? (r as any).Height ?? 0),
        };
      }
    }
    return Promise.resolve({x:0,y:0,w:0,h:0});
  }

  async WindowSetPos(x: number, y: number): Promise<void> {
    if (isWails) { return wailsCall('App', 'WindowSetPos', x, y); }
    return Promise.resolve();
  }

  async WindowCenter(): Promise<void> {
    if (isWails) { return wailsCall('App', 'WindowCenter'); }
    return Promise.resolve();
  }

  // 获取应用版本号（构建时通过 //go:embed VERSION 注入，随 build.ps1 自增；格式如 V1.004）
  async GetAppVersion(): Promise<string> {
    if (isWails) {
      return await wailsCall('App', 'GetAppVersion') as string;
    }
    return Promise.resolve('V1.0.0');
  }
}

// ============== Local User Password Utils ==============
const LOCAL_USER_KEY = 'local_user';
const LOCAL_USER_SALT = 'shoukehuduan_local_2026';

function hashPassword(pwd: string): string {
  // 简单的加盐哈希，防止明文存储（本地模式非高安全场景）
  const salted = LOCAL_USER_SALT + '|' + pwd;
  let hash = 0;
  for (let i = 0; i < salted.length; i++) {
    const chr = salted.charCodeAt(i);
    hash = ((hash << 5) - hash) + chr;
    hash |= 0;
  }
  // 结合 btoa 增加可读性
  try {
    return btoa(String(hash) + '_' + salted.length);
  } catch {
    return String(hash) + '_' + salted.length;
  }
}

interface LocalUserRecord {
  username: string;
  passwordHash: string;
  createdAt: number;
  updatedAt: number;
}

function ensureDefaultLocalUser(): LocalUserRecord {
  try {
    const saved = localStorage.getItem(LOCAL_USER_KEY);
    if (saved) {
      const rec = JSON.parse(saved) as LocalUserRecord;
      if (rec && rec.username && rec.passwordHash) return rec;
    }
  } catch {}
  // 初始化默认用户 admin/admin
  const defaultRec: LocalUserRecord = {
    username: 'admin',
    passwordHash: hashPassword('admin'),
    createdAt: Date.now(),
    updatedAt: Date.now(),
  };
  try {
    localStorage.setItem(LOCAL_USER_KEY, JSON.stringify(defaultRec));
  } catch {}
  return defaultRec;
}

// ============== AuthService ==============
export class AuthService {
  private token = '';
  private baseURL = 'http://localhost:8080';
  private loggedIn = false;
  private app: App;

  constructor(app?: App) {
    this.app = app || new App();
  }

  SetBaseURL(url: string): void {
    this.baseURL = url;
    if (isWails) {
      wailsCall('AuthService', 'SetBaseURL', url);
    }
  }

  GetBaseURL(): string {
    return this.baseURL;
  }

  SetToken(token: string): void {
    this.token = token;
    if (isWails) {
      wailsCall('AuthService', 'SetToken', token);
    }
  }

  GetToken(): string {
    return this.token;
  }

  IsLoggedIn(): boolean {
    return this.loggedIn;
  }

  async Login(username: string, password: string): Promise<Record<string, any>> {
    if (isWails) {
      // Real Wails backend: calls Go HTTP API
      const data = await wailsCall('AuthService', 'Login', username, password);
      if (data && data.token) {
        this.token = data.token;
        this.loggedIn = true;
      }
      return data;
    }
    // Mock fallback (browser dev mode)
    return new Promise((resolve, reject) => {
      setTimeout(() => {
        if (username && password) {
          this.loggedIn = true;
          this.token = 'mock-token-' + Date.now();
          resolve({
            token: this.token,
            user: { id: 1, username, role: 'admin' },
          });
        } else {
          reject(new Error('用户名或密码不能为空'));
        }
      }, 500);
    });
  }

  async Register(username: string, password: string, email: string): Promise<void> {
    if (isWails) {
      await wailsCall('AuthService', 'Register', username, password, email);
      return;
    }
    return Promise.resolve();
  }

  // ---- Local Mode Auth (本地模式账号密码) ----
  LocalLogin(username: string, password: string): Promise<boolean> {
    return new Promise((resolve, reject) => {
      try {
        // 强制输入账号密码，不许空值绕过
        if (!username || !username.trim()) {
          reject(new Error('请输入本地账号'));
          return;
        }
        if (!password) {
          reject(new Error('请输入本地密码'));
          return;
        }
        const rec = ensureDefaultLocalUser();
        if (rec.username !== username.trim()) {
          reject(new Error('用户名或密码错误'));
          return;
        }
        if (rec.passwordHash !== hashPassword(password)) {
          reject(new Error('用户名或密码错误'));
          return;
        }
        this.loggedIn = true;
        resolve(true);
      } catch (e: any) {
        reject(e || new Error('本地登录验证失败'));
      }
    });
  }

  LocalChangePassword(oldPassword: string, newPassword: string): Promise<boolean> {
    return new Promise((resolve, reject) => {
      try {
        if (!newPassword || newPassword.length < 4) {
          reject(new Error('新密码长度至少 4 位'));
          return;
        }
        const rec = ensureDefaultLocalUser();
        if (rec.passwordHash !== hashPassword(oldPassword)) {
          reject(new Error('原密码错误'));
          return;
        }
        const updated: LocalUserRecord = {
          ...rec,
          passwordHash: hashPassword(newPassword),
          updatedAt: Date.now(),
        };
        localStorage.setItem(LOCAL_USER_KEY, JSON.stringify(updated));
        resolve(true);
      } catch (e: any) {
        reject(e || new Error('修改密码失败'));
      }
    });
  }

  GetLocalUsername(): Promise<string> {
    return Promise.resolve(ensureDefaultLocalUser().username);
  }

  LocalResetDefault(): Promise<boolean> {
    // 重置为默认 admin/admin（忘记密码时可用，暂不暴露UI）
    return new Promise((resolve) => {
      try {
        localStorage.removeItem(LOCAL_USER_KEY);
        ensureDefaultLocalUser();
        resolve(true);
      } catch {
        resolve(false);
      }
    });
  }
}

// ============== VMService ==============
export class VMService {
  private auth: AuthService;
  private vms: VMInfo[];

  constructor(auth: AuthService) {
    this.auth = auth;
    this.vms = JSON.parse(JSON.stringify(mockVMs));
  }

  // ---- Online mode: real backend or mock ----

  async ListVMs(): Promise<VMInfo[]> {
    if (isWails) {
      // Real Wails backend: calls Go HTTP API to get VM list from console server
      const list = await wailsCall('VMService', 'ListVMs');
      return list as VMInfo[];
    }
    // Mock fallback
    return new Promise((resolve) => {
      setTimeout(() => resolve([...this.vms]), 300);
    });
  }

  async StartVM(vmid: number): Promise<void> {
    if (isWails) {
      await wailsCall('VMService', 'StartVM', vmid);
      return;
    }
    return new Promise((resolve, reject) => {
      setTimeout(() => {
        const vm = this.vms.find(v => v.vmid === vmid);
        if (vm) { vm.status = 'running'; resolve(); }
        else reject(new Error('虚拟机不存在'));
      }, 800);
    });
  }

  async StopVM(vmid: number): Promise<void> {
    if (isWails) {
      await wailsCall('VMService', 'StopVM', vmid);
      return;
    }
    return new Promise((resolve, reject) => {
      setTimeout(() => {
        const vm = this.vms.find(v => v.vmid === vmid);
        if (vm) { vm.status = 'stopped'; resolve(); }
        else reject(new Error('虚拟机不存在'));
      }, 800);
    });
  }

  async RebootVM(vmid: number): Promise<void> {
    if (isWails) {
      await wailsCall('VMService', 'RebootVM', vmid);
      return;
    }
    return new Promise((resolve, reject) => {
      setTimeout(() => {
        const vm = this.vms.find(v => v.vmid === vmid);
        if (vm) { vm.status = 'running'; resolve(); }
        else reject(new Error('虚拟机不存在'));
      }, 1500);
    });
  }

  async GetConnection(vmid: number): Promise<ConnectionInfo | null> {
    if (isWails) {
      // Real Wails backend: calls Go HTTP API to get connection info from console
      const conn = await wailsCall('VMService', 'GetConnection', vmid);
      // Apply protocol preference override
      const customProto = this.GetVMProtocolPreferenceSync(vmid);
      if (customProto && conn) {
        const portMap: Record<string, number> = {
          rdp: 3389, spice: 5900, vnc: 5900, horizon: 443,
        };
        conn.protocol = customProto;
        conn.port = portMap[customProto] || conn.port;
      }
      return conn as ConnectionInfo;
    }
    // Mock fallback
    return new Promise((resolve, reject) => {
      setTimeout(() => {
        const vm = this.vms.find(v => v.vmid === vmid);
        if (vm) {
          const customProto = this.GetVMProtocolPreferenceSync(vmid);
          const protocol = customProto || vm.protocol;
          const portMap: Record<string, number> = {
            rdp: 3389, spice: 5900, vnc: 5900, horizon: 443,
          };
          const info: ConnectionInfo = {
            session_id: 'sess-' + Date.now(),
            protocol: protocol,
            host: vm.ip_address,
            port: portMap[protocol] || 3389,
            password: 'access-pass-' + vmid,
            expires_at: new Date(Date.now() + 8 * 3600 * 1000).toISOString(),
            tls_port: protocol === 'spice' ? 5901 : undefined,
            username: vm.vm_username,
            vm_password: vm.vm_password,
            broker_url: protocol === 'horizon' ? 'https://broker.example.com' : undefined,
          };
          resolve(info);
        } else {
          reject(new Error('虚拟机不存在'));
        }
      }, 400);
    });
  }

  // ---- Protocol Preference (always localStorage, client-side feature) ----

  private GetVMProtocolPreferenceSync(vmid: number): string | null {
    try {
      const saved = localStorage.getItem('vm_protocol_prefs');
      if (saved) {
        const prefs = JSON.parse(saved);
        return prefs[String(vmid)] || null;
      }
    } catch {}
    return null;
  }

  GetVMProtocolPreference(vmid: number): Promise<string | null> {
    return Promise.resolve(this.GetVMProtocolPreferenceSync(vmid));
  }

  SetVMProtocolPreference(vmid: number, protocol: string): Promise<boolean> {
    return new Promise((resolve) => {
      try {
        let prefs: Record<string, string> = {};
        const saved = localStorage.getItem('vm_protocol_prefs');
        if (saved) prefs = JSON.parse(saved);
        if (protocol) {
          prefs[String(vmid)] = protocol;
        } else {
          delete prefs[String(vmid)];
        }
        localStorage.setItem('vm_protocol_prefs', JSON.stringify(prefs));
        const vm = this.vms.find(v => v.vmid === vmid);
        if (vm) vm.protocol = protocol || vm.protocol;
        resolve(true);
      } catch {
        resolve(false);
      }
    });
  }

  GetAllVMProtocolPreferences(): Record<string, string> {
    try {
      const saved = localStorage.getItem('vm_protocol_prefs');
      if (saved) return JSON.parse(saved);
    } catch {}
    return {};
  }

  // ---- Offline mode (always localStorage, client-side feature) ----

  private offlineVms: VMInfo[] | null = null;

  private loadOfflineVMs(): VMInfo[] {
    if (this.offlineVms) return this.offlineVms;
    try {
      const saved = localStorage.getItem('offline_vms');
      if (saved) {
        this.offlineVms = JSON.parse(saved);
      } else {
        this.offlineVms = [
          {
            vmid: 1, name: '我的 Windows 桌面', status: 'stopped', node: '本地',
            cpu_cores: 4, memory_mb: 8192, disk_gb: 120, os: 'Windows 11',
            protocol: 'rdp', ip_address: '192.168.1.100', vm_username: 'administrator', vm_password: '',
          },
        ];
        this.saveOfflineVMs();
      }
    } catch {
      this.offlineVms = [];
    }
    return this.offlineVms || [];
  }

  private saveOfflineVMs(): void {
    try {
      localStorage.setItem('offline_vms', JSON.stringify(this.offlineVms));
    } catch {}
  }

  ListOfflineVMs(): Promise<VMInfo[]> {
    // 立即返回本地缓存，去掉无意义的 200ms 延迟，让卡片第一时间渲染
    const vms = this.loadOfflineVMs();
    const prefs = this.GetAllVMProtocolPreferences();
    const result = vms.map(vm => {
      const customProto = prefs[String(vm.vmid)];
      return customProto ? { ...vm, protocol: customProto } : { ...vm };
    });
    return Promise.resolve(result);
  }

  AddOfflineVM(vm: Omit<VMInfo, 'vmid'>): Promise<VMInfo> {
    return new Promise((resolve, reject) => {
      setTimeout(() => {
        const vms = this.loadOfflineVMs();
        const newId = vms.length > 0 ? Math.max(...vms.map(v => v.vmid)) + 1 : 1;
        const newVM: VMInfo = { ...vm, vmid: newId };
        vms.push(newVM);
        this.saveOfflineVMs();
        resolve(newVM);
      }, 300);
    });
  }

  UpdateOfflineVM(vm: VMInfo): Promise<boolean> {
    return new Promise((resolve, reject) => {
      setTimeout(() => {
        const vms = this.loadOfflineVMs();
        const idx = vms.findIndex(v => v.vmid === vm.vmid);
        if (idx >= 0) {
          vms[idx] = { ...vm };
          this.saveOfflineVMs();
          resolve(true);
        } else {
          reject(new Error('虚拟机不存在'));
        }
      }, 300);
    });
  }

  DeleteOfflineVM(vmid: number): Promise<boolean> {
    return new Promise((resolve, reject) => {
      setTimeout(() => {
        const vms = this.loadOfflineVMs();
        const idx = vms.findIndex(v => v.vmid === vmid);
        if (idx >= 0) {
          vms.splice(idx, 1);
          this.saveOfflineVMs();
          try {
            const prefs = this.GetAllVMProtocolPreferences();
            delete prefs[String(vmid)];
            localStorage.setItem('vm_protocol_prefs', JSON.stringify(prefs));
          } catch {}
          resolve(true);
        } else {
          reject(new Error('虚拟机不存在'));
        }
      }, 300);
    });
  }

  StartOfflineVM(vmid: number): Promise<void> {
    return new Promise((resolve, reject) => {
      setTimeout(() => {
        const vms = this.loadOfflineVMs();
        const vm = vms.find(v => v.vmid === vmid);
        if (vm) { vm.status = 'running'; this.saveOfflineVMs(); resolve(); }
        else reject(new Error('虚拟机不存在'));
      }, 500);
    });
  }

  StopOfflineVM(vmid: number): Promise<void> {
    return new Promise((resolve, reject) => {
      setTimeout(() => {
        const vms = this.loadOfflineVMs();
        const vm = vms.find(v => v.vmid === vmid);
        if (vm) { vm.status = 'stopped'; this.saveOfflineVMs(); resolve(); }
        else reject(new Error('虚拟机不存在'));
      }, 500);
    });
  }

  RebootOfflineVM(vmid: number): Promise<void> {
    return new Promise((resolve, reject) => {
      setTimeout(() => {
        const vms = this.loadOfflineVMs();
        const vm = vms.find(v => v.vmid === vmid);
        if (vm) { vm.status = 'running'; this.saveOfflineVMs(); resolve(); }
        else reject(new Error('虚拟机不存在'));
      }, 600);
    });
  }

  GetOfflineConnection(vmid: number): Promise<ConnectionInfo | null> {
    return new Promise((resolve, reject) => {
      setTimeout(() => {
        const vms = this.loadOfflineVMs();
        const vm = vms.find(v => v.vmid === vmid);
        if (vm) {
          const customProto = this.GetVMProtocolPreferenceSync(vmid);
          const protocol = customProto || vm.protocol;
          const portMap: Record<string, number> = {
            rdp: 3389, spice: 5900, vnc: 5900, horizon: 443,
          };
          const info: ConnectionInfo = {
            session_id: 'offline-' + Date.now(),
            protocol: protocol,
            host: vm.ip_address,
            port: portMap[protocol] || 3389,
            password: vm.vm_password || '',
            expires_at: new Date(Date.now() + 8 * 3600 * 1000).toISOString(),
            tls_port: protocol === 'spice' ? 5901 : undefined,
            username: vm.vm_username,
            vm_password: vm.vm_password,
            broker_url: protocol === 'horizon' ? 'https://broker.example.com' : undefined,
          };
          resolve(info);
        } else {
          reject(new Error('虚拟机不存在'));
        }
      }, 200);
    });
  }
}

// ============== ProtocolConfig (always localStorage) ==============
export interface ProtocolConfig {
  // RDP
  rdp_resolution: string;
  rdp_colorDepth: number;
  rdp_fullscreen: boolean;
  rdp_audio: boolean;
  rdp_clipboard: boolean;
  rdp_driveMapping: boolean;
  rdp_compression: boolean;
  rdp_keyboard: string;
  rdp_hideToolbar: boolean;   // 隐藏 RDP 顶部连接栏/工具栏
  rdp_blockAltF4: boolean;   // 屏蔽 RDP 会话中 ALT+F4 关闭（同时移除窗口系统菜单）
  rdp_multiMon: boolean;     // 多显示器（/multimon：每块屏独立显示通道；需服务端企业版/服务器版 Windows）
  rdp_usb: boolean;           // USB 设备重定向开关
  rdp_usbMode: string;        // USB 重定向模式: 'auto' (自动全部) | 'manual' (手动选择)
  rdp_usbDevices: string;     // 手动模式：选中的USB设备（逗号分隔的VID:PID，如"046D:C534"）
  // SPICE
  spice_tls: boolean;
  spice_compression: boolean;
  spice_fullscreen: boolean;
  spice_smartcard: boolean;
  spice_usb: boolean;
  spice_shareClipboard: boolean;
  spice_hideToolbar: boolean;
  // VNC
  vnc_colorLevel: string;
  vnc_compression: number;
  vnc_fullscreen: boolean;
  vnc_encodings: string;
  vnc_viewOnly: boolean;
  vnc_scaling: boolean;
  // Horizon
  horizon_protocol: string;
  horizon_fullscreen: boolean;
  horizon_autoReconnect: boolean;
  horizon_usbAutoConnect: boolean;
  horizon_hideClient: boolean;
  // Windows RDP (mstsc.exe 官方客户端 + RDP 文件一键链接)
  windowsrdp_resolution: string;
  windowsrdp_colorDepth: number;
  windowsrdp_fullscreen: boolean;
  windowsrdp_audio: boolean;
  windowsrdp_clipboard: boolean;
  windowsrdp_driveMapping: boolean;
  windowsrdp_printer: boolean;
  windowsrdp_smartcard: boolean;
  windowsrdp_compression: boolean;
  windowsrdp_multiMonitor: boolean;
  windowsrdp_showConnectionBar: boolean;
  windowsrdp_autoLogin: boolean;
}

const defaultProtocolConfig: ProtocolConfig = {
  rdp_resolution: '1920x1080',
  rdp_colorDepth: 32,
  rdp_fullscreen: true,
  rdp_audio: true,
  rdp_clipboard: true,
  rdp_driveMapping: false,
  rdp_compression: true,
  rdp_keyboard: 'auto',
  rdp_hideToolbar: false,   // 默认显示 FreeRDP 顶部悬浮连接栏（用户要求"应该有悬浮菜单"）
  rdp_blockAltF4: true,
  rdp_multiMon: false,      // 默认单屏（多显示器需服务端企业版/服务器版 Windows）
  rdp_usb: false,           // USB 重定向默认关闭（需要时手动开启）
  rdp_usbMode: 'auto',      // 默认自动模式（插上就重定向）
  rdp_usbDevices: '',       // 手动模式默认无选中设备
  spice_tls: true,
  spice_compression: true,
  spice_fullscreen: false,
  spice_smartcard: false,
  spice_usb: false,
  spice_shareClipboard: true,
  spice_hideToolbar: false,  // 默认显示 remote-viewer 顶部悬浮工具栏
  vnc_colorLevel: 'rgb222',
  vnc_compression: 6,
  vnc_fullscreen: false,
  vnc_encodings: 'tight',
  vnc_viewOnly: false,
  vnc_scaling: true,
  horizon_protocol: 'blast',
  horizon_fullscreen: true,
  horizon_autoReconnect: true,
  horizon_usbAutoConnect: false,
  horizon_hideClient: false, // 默认显示 Horizon 悬浮菜单（Shade），开启后隐藏
  windowsrdp_resolution: '1920x1080',
  windowsrdp_colorDepth: 32,
  windowsrdp_fullscreen: true,
  windowsrdp_audio: true,
  windowsrdp_clipboard: true,
  windowsrdp_driveMapping: false,
  windowsrdp_printer: false,
  windowsrdp_smartcard: false,
  windowsrdp_compression: true,
  windowsrdp_multiMonitor: false,
  windowsrdp_showConnectionBar: true,
  windowsrdp_autoLogin: true,
};

// ============== USB Device ==============
export interface USBDevice {
  name: string;
  vid: string;
  pid: string;
  device_id: string;
  available: boolean;
}

// ============== PVE VM Info ==============
// PVE 资源接口返回的虚拟机信息，用于「查询 PVE 虚拟机」弹窗列表
export interface PveVMInfo {
  vmID: number;    // VMID（PVE Web 界面虚拟机名字前的数字）
  name: string;    // 虚拟机名称
  node: string;    // 所在节点名
  status: string;  // running / stopped / suspended
  type: string;    // qemu 或 lxc
  tags?: string;   // 标签
}

// ============== ProtocolService ==============
export class ProtocolService {
  private app: App;

  constructor(app: App) {
    this.app = app;
  }

  // ---- USB 设备枚举 ----
  async ListUSBDevices(): Promise<USBDevice[]> {
    if (isWails) {
      try {
        const r = await wailsCall('ProtocolService', 'ListUSBDevices');
        if (Array.isArray(r)) return r as USBDevice[];
        return [];
      } catch {
        return [];
      }
    }
    return [];
  }

  // ---- Real backend: launches actual remote desktop clients ----

  async LaunchRDP(host: string, port: number, password: string): Promise<void> {
    if (isWails) {
      await wailsCall('ProtocolService', 'LaunchRDP', host, port, password);
      return;
    }
    console.log(`启动 RDP 连接：${host}:${port}`);
  }

  async LaunchRDPWithAuth(host: string, port: number, username: string, password: string): Promise<void> {
    if (isWails) {
      await wailsCall('ProtocolService', 'LaunchRDPWithAuth', host, port, username, password);
      return;
    }
    console.log(`启动 RDP 自动登录：${username}@${host}:${port}`);
  }

  async LaunchSPICE(host: string, port: number, tlsPort: number, password: string): Promise<void> {
    if (isWails) {
      await wailsCall('ProtocolService', 'LaunchSPICE', host, port, tlsPort, password);
      return;
    }
    console.log(`启动 SPICE 连接：${host}:${port}`);
  }

  // 通过 PVE API 获取临时 SPICE 凭据后启动 remote-viewer
  async LaunchSPICEViaPVE(pveEndpoint: string, pveUsername: string, pvePassword: string, pveNode: string, pveVMID: number, verifySSL: boolean): Promise<void> {
    if (isWails) {
      await wailsCall('ProtocolService', 'LaunchSPICEViaPVE', pveEndpoint, pveUsername, pvePassword, pveNode, pveVMID, verifySSL);
      return;
    }
    console.log(`启动 SPICE (PVE API 模式)：${pveEndpoint} 节点=${pveNode} VMID=${pveVMID}`);
  }

  // 通过 PVE API 控制虚拟机电源
  async ControlVMViaPVE(pveEndpoint: string, pveUsername: string, pvePassword: string, pveNode: string, pveVMID: number, action: string, verifySSL: boolean): Promise<void> {
    if (isWails) {
      await wailsCall('ProtocolService', 'ControlVMViaPVE', pveEndpoint, pveUsername, pvePassword, pveNode, pveVMID, action, verifySSL);
      return;
    }
    console.log(`PVE 电源控制：${action} VMID=${pveVMID}`);
  }

  // 通过 PVE API 获取虚拟机状态
  async GetVMStatusViaPVE(pveEndpoint: string, pveUsername: string, pvePassword: string, pveNode: string, pveVMID: number, verifySSL: boolean): Promise<string> {
    if (isWails) {
      return await wailsCall('ProtocolService', 'GetVMStatusViaPVE', pveEndpoint, pveUsername, pvePassword, pveNode, pveVMID, verifySSL);
    }
    return Promise.resolve('running');
  }

  // 通过 PVE API 查询账号可见的所有虚拟机（QEMU VM + LXC），返回 VMID/名称/节点/状态
  async ListVMsViaPVE(pveEndpoint: string, pveUsername: string, pvePassword: string, verifySSL: boolean): Promise<PveVMInfo[]> {
    if (isWails) {
      // Wails 运行时按 Go 的 json tag 序列化，字段名为小写 vmid；这里统一规范化为 vmID，
      // 兼容后端两种字段命名，避免表格 VMID 列空白、选择后 VMID 无法自动填充
      const raw = await wailsCall('ProtocolService', 'ListVMsViaPVE', pveEndpoint, pveUsername, pvePassword, verifySSL) as any[];
      return (raw || []).map((it: any) => ({
        vmID: Number(it.vmid ?? it.vmID ?? 0),
        name: String(it.name ?? ''),
        node: String(it.node ?? ''),
        status: String(it.status ?? ''),
        type: String(it.type ?? ''),
        tags: it.tags ? String(it.tags) : undefined,
      })) as PveVMInfo[];
    }
    // Mock：非 Wails 环境下返回几条示例
    return Promise.resolve([
      { vmID: 100, name: 'Win10-Work', node: 'pve', status: 'running', type: 'qemu', tags: 'office' },
      { vmID: 101, name: 'Linux-DEV', node: 'pve', status: 'stopped', type: 'qemu', tags: 'dev' },
      { vmID: 102, name: 'WinServer2022-AD', node: 'pve', status: 'running', type: 'qemu', tags: 'server' },
    ] satisfies PveVMInfo[] as any);
  }

  // 通过 Hyper-V PowerShell Cmdlet 控制虚拟机电源
  // action: "start" | "stop" | "reboot" | "suspend" | "resume"
  async ControlVMViaHyperV(hypervHost: string, hypervVMName: string, useWinRM: boolean, username: string, password: string, forceShutdown: boolean, action: string): Promise<void> {
    if (isWails) {
      await wailsCall('ProtocolService', 'ControlVMViaHyperV', hypervHost, hypervVMName, useWinRM, username, password, forceShutdown, action);
      return;
    }
    console.log(`Hyper-V 电源控制：${action} VM=${hypervVMName}`);
  }

  // 通过 Hyper-V PowerShell Cmdlet 查询虚拟机状态
  async GetVMStatusViaHyperV(hypervHost: string, hypervVMName: string, useWinRM: boolean, username: string, password: string): Promise<string> {
    if (isWails) {
      return await wailsCall('ProtocolService', 'GetVMStatusViaHyperV', hypervHost, hypervVMName, useWinRM, username, password);
    }
    return Promise.resolve('stopped');
  }

  async CheckRDPPortStatus(host: string, port: number): Promise<string> {
    if (isWails) {
      return await wailsCall('ProtocolService', 'CheckRDPPortStatus', host, port);
    }
    return Promise.resolve('stopped');
  }

  async CheckAnyPortStatus(host: string): Promise<string> {
    if (isWails) {
      return await wailsCall('ProtocolService', 'CheckAnyPortStatus', host);
    }
    return Promise.resolve('stopped');
  }

  async LaunchVNC(host: string, port: number, password: string): Promise<void> {
    if (isWails) {
      await wailsCall('ProtocolService', 'LaunchVNC', host, port, password);
      return;
    }
    console.log(`启动 VNC 连接：${host}:${port}`);
  }

  async LaunchHorizon(brokerURL: string, username: string, password: string, domain: string): Promise<void> {
    if (isWails) {
      await wailsCall('ProtocolService', 'LaunchHorizon', brokerURL, username, password, domain);
      return;
    }
    console.log(`启动 Horizon：${brokerURL}，用户：${username}`);
  }

  async LaunchProtocolFull(
    protocolType: string,
    host: string,
    port: number,
    username: string,
    password: string,
    tlsPort: number,
    brokerURL: string,
    width: number = 0,
    height: number = 0,
    fullscreen: boolean = false,
    colorDepth: number = 32,
    audio: boolean = true,
    clipboard: boolean = true,
    compression: boolean = true,
    driveMapping: boolean = false,
    printer: boolean = false,
    smartcard: boolean = false,
    hideClient: boolean = false,
    hideToolbar: boolean = false,
    blockAltF4: boolean = false,
    usbRedirect: boolean = false,
    usbDevices: string = '',
    usbMode: string = 'auto',
    multiMon: boolean = false
  ): Promise<void> {
    if (isWails) {
      // Real Wails backend: launches actual protocol client (FreeRDP/SPICE/VNC/Horizon)
      await wailsCall('ProtocolService', 'LaunchProtocolFull', protocolType, host, port, username, password, tlsPort, brokerURL, width, height, fullscreen, colorDepth, audio, clipboard, compression, driveMapping, printer, smartcard, hideClient, hideToolbar, blockAltF4, usbRedirect, usbDevices, usbMode, multiMon);
      return;
    }
    // Browser dev mode: log only
    const proto = protocolType.toUpperCase();
    const authInfo = username ? `（自动登录：${username}）` : '（未启用自动登录）';
    const resInfo = width > 0 && height > 0 ? `${width}x${height}` : '默认';
    const fsInfo = fullscreen ? '全屏' : '窗口';
    const feat = [
      audio ? '声音' : null,
      clipboard ? '剪贴板' : null,
      compression ? '压缩' : null,
      driveMapping ? '映射驱动器' : null,
    ].filter(Boolean).join('+')
    console.log(`启动 ${proto} 连接 → ${host}:${port} ${authInfo} ${resInfo} ${fsInfo} 特性:${feat || '无'}`);
  }

  async LaunchProtocol(pt: string, host: string, port: number, pwd: string, tlsPort: number): Promise<void> {
    if (isWails) {
      await wailsCall('ProtocolService', 'LaunchProtocol', pt, host, port, pwd, tlsPort);
      return;
    }
    return this.LaunchProtocolFull(pt, host, port, '', pwd, tlsPort, '');
  }

  // ---- Dependency check ----
  // 返回结构体 {Available, Message}，兼容 Go ProtocolDependencyCheckResult
  async CheckDependency(protocolType: string): Promise<{ Available?: boolean; available?: boolean; Message?: string; message?: string } | [boolean, string]> {
    if (isWails) {
      // Wails 真实后端：加 25 秒超时，避免 Promise 永远 pending 导致界面一直"检测中..."
      const real = wailsCall('ProtocolService', 'CheckDependency', protocolType)
      const timeout = new Promise<never>((_, reject) => setTimeout(() => reject(new Error(`协议依赖检测超时（${protocolType}）`)), 25000))
      const result = await Promise.race([real, timeout]) as any
      if (result && typeof result === 'object' && ('Available' in result || 'Message' in result || 'available' in result)) {
        return result
      }
      // 兼容旧数组返回值（以防万一）
      if (Array.isArray(result)) {
        return { Available: !!result[0], Message: String(result[1] ?? '') }
      }
      return { Available: false, Message: String(result ?? '') }
    }
    // Mock fallback
    return new Promise((resolve) => {
      setTimeout(() => {
        const results: Record<string, [boolean, string]> = {
          rdp: [true, 'Mock: 已找到 FreeRDP 客户端'],
          spice: [true, 'Mock: 已找到 virt-viewer (SPICE 客户端)'],
          vnc: [true, 'Mock: 已找到 VNC Viewer'],
          horizon: [false, 'Mock: 未安装 Horizon Client，请从官网下载安装'],
          ssh: [true, 'Mock: 已找到 OpenSSH 客户端'],
        };
        const r = results[protocolType] || [false, '未知状态'];
        resolve({ Available: r[0], Message: r[1] });
      }, 300);
    });
  }

  async GetConfigTemplate(protocolType: string, host: string, port: number, password: string, tlsPort: number): Promise<string> {
    if (isWails) {
      return await wailsCall('ProtocolService', 'GetConfigTemplate', protocolType, host, port, password, tlsPort);
    }
    return Promise.resolve('');
  }

  // ---- Protocol Config (always localStorage, client-side feature) ----

  GetProtocolConfig(): Promise<ProtocolConfig> {
    return new Promise((resolve) => {
      setTimeout(() => {
        try {
          // ★ 配置版本迁移：
          //   v2: 之前默认 rdp_hideToolbar=true（隐藏悬浮菜单），用户反馈"应该有悬浮菜单" → 重置为 false
          //   v3: 用户反馈"选了全屏但打开是小窗口" → rdp_fullscreen 默认改为 true（全屏启动）
          //   对已保存的旧配置做一次迁移，之后用户主动修改保存后会标记为最新版本，不再重置
          const CFG_VERSION = 3;
          const savedVer = parseInt(localStorage.getItem('protocol_config_version') || '0', 10);
          const saved = localStorage.getItem('protocol_config');
          if (saved) {
            const parsed = JSON.parse(saved);
            if (savedVer < 2) {
              // v1→v2: 重置 rdp_hideToolbar 为新默认值 false（显示悬浮菜单）
              parsed.rdp_hideToolbar = false;
            }
            if (savedVer < 3) {
              // v2→v3: 重置 rdp_fullscreen 为 true（默认全屏启动）
              parsed.rdp_fullscreen = true;
            }
            if (savedVer < CFG_VERSION) {
              localStorage.setItem('protocol_config', JSON.stringify(parsed));
              localStorage.setItem('protocol_config_version', String(CFG_VERSION));
            }
            resolve({ ...defaultProtocolConfig, ...parsed });
          } else {
            localStorage.setItem('protocol_config_version', String(CFG_VERSION));
            resolve({ ...defaultProtocolConfig });
          }
        } catch {
          resolve({ ...defaultProtocolConfig });
        }
      }, 150);
    });
  }

  SaveProtocolConfig(config: ProtocolConfig): Promise<boolean> {
    return new Promise((resolve) => {
      setTimeout(() => {
        try {
          localStorage.setItem('protocol_config', JSON.stringify(config));
        // 保存时更新版本号，标记用户已主动设置过（不再做迁移重置）
        localStorage.setItem('protocol_config_version', '3');
          resolve(true);
        } catch {
          resolve(false);
        }
      }, 150);
    });
  }

  ResetProtocolConfig(): Promise<ProtocolConfig> {
    return new Promise((resolve) => {
      setTimeout(() => {
        localStorage.removeItem('protocol_config');
        resolve({ ...defaultProtocolConfig });
      }, 150);
    });
  }
}
