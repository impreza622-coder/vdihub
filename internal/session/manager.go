package session

import (
	"context"
	"fmt"
	"time"

	"shoukehuduan/pkg/types"

	"github.com/redis/go-redis/v9"
)

type Manager struct {
	redisClient *redis.Client
	ctx         context.Context
}

func NewManager(redisClient *redis.Client) *Manager {
	return &Manager{
		redisClient: redisClient,
		ctx:         context.Background(),
	}
}

func (m *Manager) CreateSession(userID uint, vmid int, protocol types.ProtocolType, connectionInfo *types.ConnectionInfo) (string, error) {
	sessionID := fmt.Sprintf("session-%d-%d-%d", userID, vmid, time.Now().Unix())

	sessionData := map[string]interface{}{
		"user_id":          userID,
		"vmid":             vmid,
		"protocol":         protocol,
		"status":           "active",
		"host":             connectionInfo.Host,
		"port":             connectionInfo.Port,
		"password":         connectionInfo.Password,
		"tls_port":         connectionInfo.TLSPort,
		"expires_at":       connectionInfo.ExpiresAt.Format(time.RFC3339),
	}

	err := m.redisClient.HSet(m.ctx, sessionID, sessionData).Err()
	if err != nil {
		return "", fmt.Errorf("创建会话失败: %w", err)
	}

	err = m.redisClient.Expire(m.ctx, sessionID, time.Duration(connectionInfo.ExpiresAt.Sub(time.Now()).Seconds())*time.Second).Err()
	if err != nil {
		return "", fmt.Errorf("设置会话过期时间失败: %w", err)
	}

	return sessionID, nil
}

func (m *Manager) GetSession(sessionID string) (map[string]string, error) {
	session, err := m.redisClient.HGetAll(m.ctx, sessionID).Result()
	if err != nil {
		return nil, fmt.Errorf("获取会话失败: %w", err)
	}

	if len(session) == 0 {
		return nil, fmt.Errorf("会话不存在或已过期")
	}

	return session, nil
}

func (m *Manager) CloseSession(sessionID string) error {
	err := m.redisClient.Del(m.ctx, sessionID).Err()
	if err != nil {
		return fmt.Errorf("关闭会话失败: %w", err)
	}

	return nil
}

func (m *Manager) ListUserSessions(userID uint) ([]map[string]string, error) {
	pattern := fmt.Sprintf("session-%d-*", userID)
	var sessions []map[string]string

	var cursor uint64
	for {
		keys, nextCursor, err := m.redisClient.Scan(m.ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, fmt.Errorf("扫描会话失败: %w", err)
		}

		for _, key := range keys {
			session, err := m.GetSession(key)
			if err == nil {
				sessions = append(sessions, session)
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return sessions, nil
}

func (m *Manager) IsSessionActive(sessionID string) bool {
	exists, _ := m.redisClient.Exists(m.ctx, sessionID).Result()
	return exists > 0
}
