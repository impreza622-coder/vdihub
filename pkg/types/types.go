package types

import "time"

type VMStatus string

const (
	VMStatusRunning  VMStatus = "running"
	VMStatusStopped  VMStatus = "stopped"
	VMStatusPaused   VMStatus = "paused"
	VMStatusRebooting VMStatus = "rebooting"
)

type ProtocolType string

const (
	ProtocolRDP     ProtocolType = "rdp"
	ProtocolSPICE   ProtocolType = "spice"
	ProtocolVNC     ProtocolType = "vnc"
	ProtocolSSH     ProtocolType = "ssh"
	ProtocolHorizon ProtocolType = "horizon"
)

type VMInfo struct {
	VMID      int      `json:"vmid"`
	Name      string   `json:"name"`
	Status    VMStatus `json:"status"`
	Node      string   `json:"node"`
	CPUCores  int      `json:"cpu_cores"`
	MemoryMB  int      `json:"memory_mb"`
	DiskGB    float64  `json:"disk_gb"`
	OS        string   `json:"os"`
	Protocol  ProtocolType `json:"protocol"`
	IPAddress string   `json:"ip_address"`
	VMUsername string  `json:"vm_username,omitempty"`
	VMPassword string  `json:"vm_password,omitempty"`
	// 电源控制后端（与协议解耦）: "none" | "pve" | "hyperv" | "horizon"
	PowerBackend string `json:"power_backend,omitempty"`
	// PVE 后端专用字段
	PVEEndpoint  string `json:"pve_endpoint,omitempty"`
	PVEUsername  string `json:"pve_username,omitempty"`
	PVEPassword  string `json:"pve_password,omitempty"`
	PVENode      string `json:"pve_node,omitempty"`
	PVEVMID      int    `json:"pve_vmid,omitempty"`
	PVEVerifySSL bool   `json:"pve_verify_ssl,omitempty"`
	// Hyper-V 后端专用字段
	HyperVHost          string `json:"hyperv_host,omitempty"`
	HyperVVMName        string `json:"hyperv_vmname,omitempty"`
	HyperVUseWinRM      bool   `json:"hyperv_use_winrm,omitempty"`
	HyperVUsername      string `json:"hyperv_username,omitempty"`
	HyperVPassword      string `json:"hyperv_password,omitempty"`
	HyperVForceShutdown bool   `json:"hyperv_force,omitempty"`
}

type VMCreateRequest struct {
	Name      string   `json:"name" binding:"required"`
	Node      string   `json:"node"`
	Template  string   `json:"template" binding:"required"`
	CPUCores  int      `json:"cpu_cores"`
	MemoryMB  int      `json:"memory_mb"`
	DiskGB    float64  `json:"disk_gb"`
	OS        string   `json:"os"`
	Protocol  ProtocolType `json:"protocol"`
}

type ConnectionInfo struct {
	Protocol   ProtocolType `json:"protocol"`
	Host       string       `json:"host"`
	Port       int          `json:"port"`
	Password   string       `json:"password"`
	ExpiresAt  time.Time    `json:"expires_at"`
	TLSPort    int          `json:"tls_port,omitempty"`
	CACert     string       `json:"ca_cert,omitempty"`
	SubChannel string       `json:"sub_channel,omitempty"`
	Username   string       `json:"username,omitempty"`
	VMPassword string       `json:"vm_password,omitempty"`
	BrokerURL  string       `json:"broker_url,omitempty"`
}

type User struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Username  string    `json:"username" gorm:"uniqueIndex;not null"`
	Password  string    `json:"-" gorm:"not null"`
	Email     string    `json:"email" gorm:"uniqueIndex"`
	Role      string    `json:"role" gorm:"default:user"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UserVM struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"user_id" gorm:"index"`
	VMID      int       `json:"vmid"`
	VMName    string    `json:"vm_name"`
	AccessKey string    `json:"access_key" gorm:"uniqueIndex"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type Session struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"user_id" gorm:"index"`
	VMID      int       `json:"vmid" gorm:"index"`
	Protocol  ProtocolType `json:"protocol"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type APIResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type PaginatedResponse struct {
	Total    int         `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	Data     interface{} `json:"data"`
}
