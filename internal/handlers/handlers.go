package handlers

import (
	"fmt"
	"net/http"

	"vdihub/internal/auth"
	"vdihub/internal/pve"
	"vdihub/internal/session"
	"vdihub/pkg/types"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Handler struct {
	DB          *gorm.DB
	AuthService *auth.Service
	PVEClient   *pve.Client
	SessionMgr  *session.Manager
}

func NewHandler(db *gorm.DB, authService *auth.Service, pveClient *pve.Client, sessionMgr *session.Manager) *Handler {
	return &Handler{
		DB:          db,
		AuthService: authService,
		PVEClient:   pveClient,
		SessionMgr:  sessionMgr,
	}
}

func (h *Handler) Register(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
		Email    string `json:"email"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, types.APIResponse{
			Code:    400,
			Message: "请求参数错误",
			Data:    err.Error(),
		})
		return
	}

	var existingUser types.User
	if err := h.DB.Where("username = ?", req.Username).First(&existingUser).Error; err == nil {
		c.JSON(http.StatusConflict, types.APIResponse{
			Code:    409,
			Message: "用户名已存在",
		})
		return
	}

	hashedPassword, err := h.AuthService.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, types.APIResponse{
			Code:    500,
			Message: "密码加密失败",
		})
		return
	}

	user := types.User{
		Username: req.Username,
		Password: hashedPassword,
		Email:    req.Email,
		Role:     "user",
	}

	if err := h.DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, types.APIResponse{
			Code:    500,
			Message: "创建用户失败",
		})
		return
	}

	c.JSON(http.StatusOK, types.APIResponse{
		Code:    0,
		Message: "注册成功",
		Data: gin.H{
			"user_id":  user.ID,
			"username": user.Username,
		},
	})
}

func (h *Handler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, types.APIResponse{
			Code:    400,
			Message: "请求参数错误",
		})
		return
	}

	var user types.User
	if err := h.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, types.APIResponse{
			Code:    401,
			Message: "用户名或密码错误",
		})
		return
	}

	if !h.AuthService.CheckPassword(req.Password, user.Password) {
		c.JSON(http.StatusUnauthorized, types.APIResponse{
			Code:    401,
			Message: "用户名或密码错误",
		})
		return
	}

	token, err := h.AuthService.GenerateToken(&user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, types.APIResponse{
			Code:    500,
			Message: "生成令牌失败",
		})
		return
	}

	c.JSON(http.StatusOK, types.APIResponse{
		Code:    0,
		Message: "登录成功",
		Data: gin.H{
			"token": token,
			"user": gin.H{
				"id":       user.ID,
				"username": user.Username,
				"role":     user.Role,
			},
		},
	})
}

func (h *Handler) ListVMs(c *gin.Context) {
	vms, err := h.PVEClient.ListVMs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, types.APIResponse{
			Code:    500,
			Message: "获取虚拟机列表失败",
			Data:    err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, types.APIResponse{
		Code:    0,
		Message: "获取虚拟机列表成功",
		Data:    vms,
	})
}

func (h *Handler) GetVM(c *gin.Context) {
	vmid := c.Param("vmid")
	var vmidInt int
	if _, err := fmt.Sscanf(vmid, "%d", &vmidInt); err != nil {
		c.JSON(http.StatusBadRequest, types.APIResponse{
			Code:    400,
			Message: "无效的虚拟机ID",
		})
		return
	}

	vm, err := h.PVEClient.GetVM(vmidInt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, types.APIResponse{
			Code:    500,
			Message: "获取虚拟机信息失败",
			Data:    err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, types.APIResponse{
		Code:    0,
		Message: "获取虚拟机信息成功",
		Data:    vm,
	})
}

func (h *Handler) StartVM(c *gin.Context) {
	vmid := c.Param("vmid")
	var vmidInt int
	if _, err := fmt.Sscanf(vmid, "%d", &vmidInt); err != nil {
		c.JSON(http.StatusBadRequest, types.APIResponse{
			Code:    400,
			Message: "无效的虚拟机ID",
		})
		return
	}

	if err := h.PVEClient.StartVM(vmidInt); err != nil {
		c.JSON(http.StatusInternalServerError, types.APIResponse{
			Code:    500,
			Message: "启动虚拟机失败",
			Data:    err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, types.APIResponse{
		Code:    0,
		Message: "启动虚拟机成功",
	})
}

func (h *Handler) StopVM(c *gin.Context) {
	vmid := c.Param("vmid")
	var vmidInt int
	if _, err := fmt.Sscanf(vmid, "%d", &vmidInt); err != nil {
		c.JSON(http.StatusBadRequest, types.APIResponse{
			Code:    400,
			Message: "无效的虚拟机ID",
		})
		return
	}

	if err := h.PVEClient.StopVM(vmidInt); err != nil {
		c.JSON(http.StatusInternalServerError, types.APIResponse{
			Code:    500,
			Message: "停止虚拟机失败",
			Data:    err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, types.APIResponse{
		Code:    0,
		Message: "停止虚拟机成功",
	})
}

func (h *Handler) RebootVM(c *gin.Context) {
	vmid := c.Param("vmid")
	var vmidInt int
	if _, err := fmt.Sscanf(vmid, "%d", &vmidInt); err != nil {
		c.JSON(http.StatusBadRequest, types.APIResponse{
			Code:    400,
			Message: "无效的虚拟机ID",
		})
		return
	}

	if err := h.PVEClient.RebootVM(vmidInt); err != nil {
		c.JSON(http.StatusInternalServerError, types.APIResponse{
			Code:    500,
			Message: "重启虚拟机失败",
			Data:    err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, types.APIResponse{
		Code:    0,
		Message: "重启虚拟机成功",
	})
}

func (h *Handler) GetConnection(c *gin.Context) {
	userID := c.GetUint("user_id")
	vmid := c.Param("vmid")
	var vmidInt int
	if _, err := fmt.Sscanf(vmid, "%d", &vmidInt); err != nil {
		c.JSON(http.StatusBadRequest, types.APIResponse{
			Code:    400,
			Message: "无效的虚拟机ID",
		})
		return
	}

	vm, err := h.PVEClient.GetVM(vmidInt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, types.APIResponse{
			Code:    500,
			Message: "获取虚拟机信息失败",
			Data:    err.Error(),
		})
		return
	}

	connectionInfo, err := h.PVEClient.GetConnectionInfo(vmidInt, vm.Protocol)
	if err != nil {
		c.JSON(http.StatusInternalServerError, types.APIResponse{
			Code:    500,
			Message: "获取连接信息失败",
			Data:    err.Error(),
		})
		return
	}

	sessionID, err := h.SessionMgr.CreateSession(userID, vmidInt, vm.Protocol, connectionInfo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, types.APIResponse{
			Code:    500,
			Message: "创建会话失败",
			Data:    err.Error(),
		})
		return
	}

	response := gin.H{
		"session_id":      sessionID,
		"protocol":        connectionInfo.Protocol,
		"host":            connectionInfo.Host,
		"port":            connectionInfo.Port,
		"password":        connectionInfo.Password,
		"expires_at":      connectionInfo.ExpiresAt,
	}

	if connectionInfo.TLSPort > 0 {
		response["tls_port"] = connectionInfo.TLSPort
	}

	c.JSON(http.StatusOK, types.APIResponse{
		Code:    0,
		Message: "获取连接信息成功",
		Data:    response,
	})
}

func (h *Handler) CloseSession(c *gin.Context) {
	sessionID := c.Param("session_id")

	if err := h.SessionMgr.CloseSession(sessionID); err != nil {
		c.JSON(http.StatusInternalServerError, types.APIResponse{
			Code:    500,
			Message: "关闭会话失败",
			Data:    err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, types.APIResponse{
		Code:    0,
		Message: "关闭会话成功",
	})
}

func (h *Handler) GetUserInfo(c *gin.Context) {
	userID := c.GetUint("user_id")

	var user types.User
	if err := h.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, types.APIResponse{
			Code:    404,
			Message: "用户不存在",
		})
		return
	}

	c.JSON(http.StatusOK, types.APIResponse{
		Code:    0,
		Message: "获取用户信息成功",
		Data: gin.H{
			"id":       user.ID,
			"username": user.Username,
			"email":    user.Email,
			"role":     user.Role,
		},
	})
}

func (h *Handler) ListSessions(c *gin.Context) {
	userID := c.GetUint("user_id")

	sessions, err := h.SessionMgr.ListUserSessions(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, types.APIResponse{
			Code:    500,
			Message: "获取会话列表失败",
			Data:    err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, types.APIResponse{
		Code:    0,
		Message: "获取会话列表成功",
		Data:    sessions,
	})
}
