package pve

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"vdihub/internal/config"
	"vdihub/pkg/types"
)

type Client struct {
	cfg        *config.PVEConfig
	httpClient *http.Client
	ticket     string
	csrfToken  string
}

type TicketResponse struct {
	Data struct {
		Ticket              string `json:"ticket"`
		CSRFPreventionToken string `json:"CSRFPreventionToken"`
	} `json:"data"`
}

type VMListResponse struct {
	Data []VMInfoRaw `json:"data"`
}

// VMInfoRaw 对应 PVE /nodes/{node}/qemu 列表与 /nodes/{node}/qemu/{vmid}/status/current 详情接口。
// 注意：PVE 实际返回的字段是 cpus / maxmem / maxdisk（不是 cpu.cores / memory / disk.total），
// 且列表与 status/current 接口都不返回 ostype 与 node（ostype 在 /config 接口，node 是路径参数）。
type VMInfoRaw struct {
	VMID    int    `json:"vmid"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Node    string `json:"node"`
	OSType  string `json:"ostype"`
	CPUs    int    `json:"cpus"`
	MaxMem  int64  `json:"maxmem"`
	MaxDisk int64  `json:"maxdisk"`
}

type VMPowerResponse struct {
	Data struct {
		TaskID string `json:"data"`
	} `json:"data"`
}

type SPICEInfo struct {
	Data struct {
		Host        string `json:"host"`         // pvespiceproxy:<token>（代理令牌）
		Port        int    `json:"port"`         // 直连明文端口（spiceproxy 不返回，通常为 0）
		TLSPort     int    `json:"tls-port"`     // SPICE TLS 端口
		Password    string `json:"password"`     // SPICE 临时密码
		Proxy       string `json:"proxy"`        // HTTP 代理 http://host:3128
		CA          string `json:"ca"`           // PVE 集群 CA 证书（PEM）
		HostSubject string `json:"host-subject"` // TLS 证书主题
		Title       string `json:"title"`        // 窗口标题
	} `json:"data"`
}

type RDPInfo struct {
	Data struct {
		Address  string `json:"address"`
		Port     int    `json:"port"`
		Password string `json:"password"`
	} `json:"data"`
}

func NewClient(cfg *config.PVEConfig) *Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: !cfg.VerifySSL,
		},
	}

	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

// normalizeUsername 规范化 PVE 登录用户名：
// PVE 要求用户名必须带认证域（realm）后缀，裸用户名会直接返回 401（如 1000 必须写成 1000@pve）。
//   - 已含 @（user@pve / user@pam / user@AD.REALM 等）→ 原样使用
//   - root → root@pam（系统管理员账号位于 Linux PAM 认证域）
//   - 其他裸用户名 → 自动追加 @pve（PVE 内建认证域，普通账号默认创建在这里）
func normalizeUsername(username string) string {
	username = strings.TrimSpace(username)
	if username == "" || strings.Contains(username, "@") {
		return username
	}
	if username == "root" {
		return "root@pam"
	}
	return username + "@pve"
}

func (c *Client) Login() error {
	url := fmt.Sprintf("%s/api2/json/access/ticket", strings.TrimRight(c.cfg.Endpoint, "/"))

	payload := map[string]string{
		"username": normalizeUsername(c.cfg.Username),
		"password": c.cfg.Password,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("创建登录请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("连接PVE服务器失败（请检查地址、网络及SSL证书设置）: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var ticketResp TicketResponse
	if err := json.Unmarshal(respBody, &ticketResp); err != nil {
		return fmt.Errorf("解析登录响应失败: %w（响应: %s）", err, string(respBody))
	}

	if resp.StatusCode != http.StatusOK || ticketResp.Data.Ticket == "" {
		return fmt.Errorf(
			"PVE登录失败（HTTP %d，用户: %s）：用户名或密码错误；注意PVE用户名需带认证域后缀（如 1000@pve、root@pam）。响应: %s",
			resp.StatusCode, normalizeUsername(c.cfg.Username), string(respBody))
	}

	c.ticket = ticketResp.Data.Ticket
	// CSRFPreventionToken 是独立字段（格式如 "6A98B453:xxxx"），不能用 ticket 代替
	c.csrfToken = ticketResp.Data.CSRFPreventionToken

	return nil
}

func (c *Client) request(method, path string, payload interface{}) ([]byte, error) {
	reqURL := fmt.Sprintf("%s/api2/json%s", strings.TrimRight(c.cfg.Endpoint, "/"), path)

	// PVE 写操作（POST/PUT/DELETE）不接受 Content-Type: application/json（会返回 501），
	// 参数须用 application/x-www-form-urlencoded 表单编码；无参数则不带 body/Content-Type。
	var body io.Reader
	jsonContentType := false
	if payload != nil {
		switch m := payload.(type) {
		case map[string]string:
			form := url.Values{}
			for k, v := range m {
				form.Set(k, v)
			}
			body = strings.NewReader(form.Encode())
		default:
			jsonBody, err := json.Marshal(payload)
			if err != nil {
				return nil, fmt.Errorf("序列化请求体失败: %w", err)
			}
			body = bytes.NewBuffer(jsonBody)
			jsonContentType = true
		}
	}

	req, err := http.NewRequest(method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	if payload != nil {
		if jsonContentType {
			req.Header.Set("Content-Type", "application/json")
		} else {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	}
	req.AddCookie(&http.Cookie{
		Name:  "PVEAuthCookie",
		Value: c.ticket,
	})
	req.Header.Set("CSRFPreventionToken", c.csrfToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求PVE API失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("PVE API错误 [%d]: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (c *Client) ListVMs() ([]types.VMInfo, error) {
	respBody, err := c.request("GET", fmt.Sprintf("/nodes/%s/qemu", c.cfg.Node), nil)
	if err != nil {
		return nil, err
	}

	var vmListResp VMListResponse
	if err := json.Unmarshal(respBody, &vmListResp); err != nil {
		return nil, fmt.Errorf("解析VM列表失败: %w", err)
	}

	vms := make([]types.VMInfo, 0)
	for _, raw := range vmListResp.Data {
		node := raw.Node
		if node == "" {
			node = c.cfg.Node // 列表接口不返回 node 字段，用配置中的节点名兜底
		}
		vm := types.VMInfo{
			VMID:     raw.VMID,
			Name:     raw.Name,
			Status:   types.VMStatus(raw.Status),
			Node:     node,
			CPUCores: raw.CPUs,
			MemoryMB: int(raw.MaxMem / (1024 * 1024)),
			DiskGB:   float64(raw.MaxDisk) / (1024 * 1024 * 1024),
			OS:       raw.OSType,
			Protocol: c.detectProtocol(raw.OSType, raw.Name),
		}
		vms = append(vms, vm)
	}

	return vms, nil
}

func (c *Client) GetVM(vmid int) (*types.VMInfo, error) {
	respBody, err := c.request("GET", fmt.Sprintf("/nodes/%s/qemu/%d/status/current", c.cfg.Node, vmid), nil)
	if err != nil {
		return nil, err
	}

	var vmInfo VMInfoRaw
	if err := json.Unmarshal(respBody, &vmInfo); err != nil {
		return nil, fmt.Errorf("解析VM信息失败: %w", err)
	}

	node := vmInfo.Node
	if node == "" {
		node = c.cfg.Node // status/current 接口不返回 node 字段，用配置中的节点名兜底
	}
	vm := &types.VMInfo{
		VMID:     vmInfo.VMID,
		Name:     vmInfo.Name,
		Status:   types.VMStatus(vmInfo.Status),
		Node:     node,
		CPUCores: vmInfo.CPUs,
		MemoryMB: int(vmInfo.MaxMem / (1024 * 1024)),
		DiskGB:   float64(vmInfo.MaxDisk) / (1024 * 1024 * 1024),
		OS:       vmInfo.OSType,
		Protocol: c.detectProtocol(vmInfo.OSType, vmInfo.Name),
	}

	return vm, nil
}

func (c *Client) StartVM(vmid int) error {
	payload := map[string]string{
		"node": c.cfg.Node,
	}

	_, err := c.request("POST", fmt.Sprintf("/nodes/%s/qemu/%d/status/start", c.cfg.Node, vmid), payload)
	if err != nil {
		return fmt.Errorf("启动VM失败: %w", err)
	}

	return nil
}

func (c *Client) StopVM(vmid int) error {
	_, err := c.request("POST", fmt.Sprintf("/nodes/%s/qemu/%d/status/stop", c.cfg.Node, vmid), nil)
	if err != nil {
		return fmt.Errorf("停止VM失败: %w", err)
	}

	return nil
}

func (c *Client) RebootVM(vmid int) error {
	_, err := c.request("POST", fmt.Sprintf("/nodes/%s/qemu/%d/status/reboot", c.cfg.Node, vmid), nil)
	if err != nil {
		return fmt.Errorf("重启VM失败: %w", err)
	}

	return nil
}

func (c *Client) SuspendVM(vmid int) error {
	_, err := c.request("POST", fmt.Sprintf("/nodes/%s/qemu/%d/status/suspend", c.cfg.Node, vmid), nil)
	if err != nil {
		return fmt.Errorf("挂起VM失败: %w", err)
	}

	return nil
}

func (c *Client) ResumeVM(vmid int) error {
	_, err := c.request("POST", fmt.Sprintf("/nodes/%s/qemu/%d/status/resume", c.cfg.Node, vmid), nil)
	if err != nil {
		return fmt.Errorf("恢复VM失败: %w", err)
	}

	return nil
}

func (c *Client) GetConnectionInfo(vmid int, protocol types.ProtocolType) (*types.ConnectionInfo, error) {
	switch protocol {
	case types.ProtocolSPICE:
		return c.getSPICEInfo(vmid)
	case types.ProtocolRDP:
		return c.getRDPInfo(vmid)
	default:
		return nil, fmt.Errorf("不支持的协议类型: %s", protocol)
	}
}

func (c *Client) getSPICEInfo(vmid int) (*types.ConnectionInfo, error) {
	// PVE 8.x 使用 /spiceproxy（Web 控制台同款，走节点 3128 代理）；
	// 旧的 /spice 直连端点在新版 PVE 上返回 501。
	respBody, err := c.request("POST", fmt.Sprintf("/nodes/%s/qemu/%d/spiceproxy", c.cfg.Node, vmid), nil)
	if err != nil {
		return nil, fmt.Errorf("获取SPICE连接信息失败: %w", err)
	}

	var spiceInfo SPICEInfo
	if err := json.Unmarshal(respBody, &spiceInfo); err != nil {
		return nil, fmt.Errorf("解析SPICE信息失败: %w", err)
	}

	// proxy 中的主机名（如 pve.com）瘦客户机通常无法解析，替换为 PVE 服务器实际地址
	spiceProxy := spiceInfo.Data.Proxy
	if epu, perr := url.Parse(c.cfg.Endpoint); perr == nil && epu.Hostname() != "" {
		if pu, perr2 := url.Parse(spiceProxy); perr2 == nil {
			port := pu.Port()
			if port == "" {
				port = "3128"
			}
			pu.Host = epu.Hostname() + ":" + port
			spiceProxy = pu.String()
		}
	}

	return &types.ConnectionInfo{
		Protocol:  types.ProtocolSPICE,
		Host:      spiceProxy,
		Port:      spiceInfo.Data.Port,
		Password:  spiceInfo.Data.Password,
		TLSPort:   spiceInfo.Data.TLSPort,
		CACert:    spiceInfo.Data.CA,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}, nil
}

func (c *Client) getRDPInfo(vmid int) (*types.ConnectionInfo, error) {
	vmInfo, err := c.GetVM(vmid)
	if err != nil {
		return nil, err
	}

	return &types.ConnectionInfo{
		Protocol:  types.ProtocolRDP,
		Host:      vmInfo.IPAddress,
		Port:      3389,
		Password:  fmt.Sprintf("rdp-%d-%s", vmid, time.Now().Format("20060102150405")),
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}, nil
}

// detectProtocol 根据操作系统类型推断连接协议：Windows → RDP，其他（Linux 等）→ SPICE。
// 列表/状态接口不返回 ostype，因此同时参考 VM 名称（如 "Windows10-22H2"）兜底判断。
func (c *Client) detectProtocol(osType, name string) types.ProtocolType {
	combined := strings.ToLower(osType + " " + name)
	if strings.Contains(combined, "win") {
		return types.ProtocolRDP
	}
	return types.ProtocolSPICE
}
