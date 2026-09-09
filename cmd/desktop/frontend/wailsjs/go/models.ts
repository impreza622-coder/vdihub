export namespace main {
	
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
	    }
	}

}

