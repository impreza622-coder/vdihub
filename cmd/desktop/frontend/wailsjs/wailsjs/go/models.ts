export namespace main {
	
	export class ApplyKioskResult {
	    scheduledReboot: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ApplyKioskResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scheduledReboot = source["scheduledReboot"];
	    }
	}
	export class AutostartStatus {
	    normal: boolean;
	    kiosk_auto: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AutostartStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.normal = source["normal"];
	        this.kiosk_auto = source["kiosk_auto"];
	    }
	}
	export class ConnectionInfo {
	    session_id: string;
	    protocol: string;
	    host: string;
	    port: number;
	    password: string;
	    // Go type: time
	    expires_at: any;
	    tls_port?: number;
	    username?: string;
	    vm_password?: string;
	    broker_url?: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session_id = source["session_id"];
	        this.protocol = source["protocol"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.password = source["password"];
	        this.expires_at = this.convertValues(source["expires_at"], null);
	        this.tls_port = source["tls_port"];
	        this.username = source["username"];
	        this.vm_password = source["vm_password"];
	        this.broker_url = source["broker_url"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ProtocolDependencyCheckResult {
	    available: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new ProtocolDependencyCheckResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.message = source["message"];
	    }
	}
	export class PveVMInfo {
	    vmid: number;
	    name: string;
	    node: string;
	    status: string;
	    type: string;
	    tags?: string;
	
	    static createFrom(source: any = {}) {
	        return new PveVMInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.vmid = source["vmid"];
	        this.name = source["name"];
	        this.node = source["node"];
	        this.status = source["status"];
	        this.type = source["type"];
	        this.tags = source["tags"];
	    }
	}
	export class Rect {
	    x: number;
	    y: number;
	    w: number;
	    h: number;
	
	    static createFrom(source: any = {}) {
	        return new Rect(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.x = source["x"];
	        this.y = source["y"];
	        this.w = source["w"];
	        this.h = source["h"];
	    }
	}
	export class StartHint {
	    kiosk_auto: boolean;
	    exe_path: string;
	
	    static createFrom(source: any = {}) {
	        return new StartHint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kiosk_auto = source["kiosk_auto"];
	        this.exe_path = source["exe_path"];
	    }
	}
	export class SystemKioskConfig {
	    registerUninstall: boolean;
	    uninstallPublisher: string;
	    uninstallVersion: string;
	    replaceShell: boolean;
	    enableAutoLogin: boolean;
	    autoLoginUser: string;
	    autoLoginPwd: string;
	    autoLoginDomain: string;
	    scheduleReboot: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SystemKioskConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.registerUninstall = source["registerUninstall"];
	        this.uninstallPublisher = source["uninstallPublisher"];
	        this.uninstallVersion = source["uninstallVersion"];
	        this.replaceShell = source["replaceShell"];
	        this.enableAutoLogin = source["enableAutoLogin"];
	        this.autoLoginUser = source["autoLoginUser"];
	        this.autoLoginPwd = source["autoLoginPwd"];
	        this.autoLoginDomain = source["autoLoginDomain"];
	        this.scheduleReboot = source["scheduleReboot"];
	    }
	}
	export class SystemKioskStatus {
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
	
	    static createFrom(source: any = {}) {
	        return new SystemKioskStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uninstallRegistered = source["uninstallRegistered"];
	        this.uninstallPublisher = source["uninstallPublisher"];
	        this.uninstallVersion = source["uninstallVersion"];
	        this.shellReplaced = source["shellReplaced"];
	        this.currentShell = source["currentShell"];
	        this.ourExePath = source["ourExePath"];
	        this.autoLogonEnabled = source["autoLogonEnabled"];
	        this.autoLogonUser = source["autoLogonUser"];
	        this.autoLogonDomain = source["autoLogonDomain"];
	        this.isElevated = source["isElevated"];
	    }
	}
	export class USBDevice {
	    name: string;
	    vid: string;
	    pid: string;
	    device_id: string;
	    available: boolean;
	
	    static createFrom(source: any = {}) {
	        return new USBDevice(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.vid = source["vid"];
	        this.pid = source["pid"];
	        this.device_id = source["device_id"];
	        this.available = source["available"];
	    }
	}
	export class UWFStatus {
	    supported: boolean;
	    featureEnabled: boolean;
	    filterEnabled: boolean;
	    volumeProtected: boolean;
	    pendingEnable: boolean;
	    pendingDisable: boolean;
	    needRebootStep1: boolean;
	    needRebootStep2: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new UWFStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.supported = source["supported"];
	        this.featureEnabled = source["featureEnabled"];
	        this.filterEnabled = source["filterEnabled"];
	        this.volumeProtected = source["volumeProtected"];
	        this.pendingEnable = source["pendingEnable"];
	        this.pendingDisable = source["pendingDisable"];
	        this.needRebootStep1 = source["needRebootStep1"];
	        this.needRebootStep2 = source["needRebootStep2"];
	        this.message = source["message"];
	    }
	}
	export class VMInfo {
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
	    power_backend?: string;
	    pve_endpoint?: string;
	    pve_username?: string;
	    pve_password?: string;
	    pve_node?: string;
	    pve_vmid?: number;
	    pve_verify_ssl?: boolean;
	    hyperv_host?: string;
	    hyperv_vmname?: string;
	    hyperv_use_winrm?: boolean;
	    hyperv_username?: string;
	    hyperv_password?: string;
	    hyperv_force?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new VMInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.vmid = source["vmid"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.node = source["node"];
	        this.cpu_cores = source["cpu_cores"];
	        this.memory_mb = source["memory_mb"];
	        this.disk_gb = source["disk_gb"];
	        this.os = source["os"];
	        this.protocol = source["protocol"];
	        this.ip_address = source["ip_address"];
	        this.vm_username = source["vm_username"];
	        this.vm_password = source["vm_password"];
	        this.power_backend = source["power_backend"];
	        this.pve_endpoint = source["pve_endpoint"];
	        this.pve_username = source["pve_username"];
	        this.pve_password = source["pve_password"];
	        this.pve_node = source["pve_node"];
	        this.pve_vmid = source["pve_vmid"];
	        this.pve_verify_ssl = source["pve_verify_ssl"];
	        this.hyperv_host = source["hyperv_host"];
	        this.hyperv_vmname = source["hyperv_vmname"];
	        this.hyperv_use_winrm = source["hyperv_use_winrm"];
	        this.hyperv_username = source["hyperv_username"];
	        this.hyperv_password = source["hyperv_password"];
	        this.hyperv_force = source["hyperv_force"];
	    }
	}

}

export namespace protocol {
	
	export class ConnectionConfig {
	    host: string;
	    port: number;
	    password: string;
	    tls_port?: number;
	    username?: string;
	    vm_password?: string;
	    broker_url?: string;
	    width?: number;
	    height?: number;
	    fullscreen?: boolean;
	    hide_client?: boolean;
	    color_depth?: number;
	    audio?: boolean;
	    clipboard?: boolean;
	    compression?: boolean;
	    drive_mapping?: boolean;
	    printer?: boolean;
	    smartcard?: boolean;
	    usb_redirect?: boolean;
	    usb_mode?: string;
	    usb_devices?: string;
	    hide_toolbar?: boolean;
	    multi_mon?: boolean;
	    block_alt_f4?: boolean;
	    proxy?: string;
	    ca?: string;
	    host_subject?: string;
	    title?: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.password = source["password"];
	        this.tls_port = source["tls_port"];
	        this.username = source["username"];
	        this.vm_password = source["vm_password"];
	        this.broker_url = source["broker_url"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.fullscreen = source["fullscreen"];
	        this.hide_client = source["hide_client"];
	        this.color_depth = source["color_depth"];
	        this.audio = source["audio"];
	        this.clipboard = source["clipboard"];
	        this.compression = source["compression"];
	        this.drive_mapping = source["drive_mapping"];
	        this.printer = source["printer"];
	        this.smartcard = source["smartcard"];
	        this.usb_redirect = source["usb_redirect"];
	        this.usb_mode = source["usb_mode"];
	        this.usb_devices = source["usb_devices"];
	        this.hide_toolbar = source["hide_toolbar"];
	        this.multi_mon = source["multi_mon"];
	        this.block_alt_f4 = source["block_alt_f4"];
	        this.proxy = source["proxy"];
	        this.ca = source["ca"];
	        this.host_subject = source["host_subject"];
	        this.title = source["title"];
	    }
	}

}

export namespace tray {
	
	export class Manager {
	
	
	    static createFrom(source: any = {}) {
	        return new Manager(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}

}

