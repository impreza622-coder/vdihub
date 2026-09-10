package main

import (
	"fmt"
	"log"
	"os"

	"vdihub/internal/auth"
	"vdihub/internal/config"
	"vdihub/internal/database"
	"vdihub/internal/handlers"
	"vdihub/internal/pve"
	"vdihub/internal/routes"
	"vdihub/internal/session"

	"github.com/redis/go-redis/v9"
)

func main() {
	configPath := "config.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	log.Println("正在加载配置...")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Printf("警告: 加载配置文件失败，使用默认配置: %v", err)
		cfg = &config.Config{}
	}

	log.Println("正在初始化数据库...")
	if err := database.InitDBConnection(&cfg.Database); err != nil {
		log.Printf("警告: 数据库连接失败（如不需要数据库可忽略）: %v", err)
	}

	log.Println("正在初始化Redis连接...")
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr(),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer redisClient.Close()

	log.Println("正在初始化PVE客户端...")
	pveClient := pve.NewClient(&cfg.PVE)
	if err := pveClient.Login(); err != nil {
		log.Printf("警告: PVE登录失败: %v", err)
	}

	log.Println("正在初始化服务...")
	authService := auth.NewService(&cfg.JWT)
	sessionMgr := session.NewManager(redisClient)

	h := handlers.NewHandler(database.DB, authService, pveClient, sessionMgr)

	r := routes.SetupRoutes(h, authService)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("服务器启动成功: http://%s", addr)

	if err := r.Run(addr); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}
