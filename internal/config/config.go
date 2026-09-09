package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig     `mapstructure:"server"`
	PVE       PVEConfig        `mapstructure:"pve"`
	Database  DatabaseConfig   `mapstructure:"database"`
	Redis     RedisConfig      `mapstructure:"redis"`
	JWT       JWTConfig        `mapstructure:"jwt"`
	Clients   ClientsConfig    `mapstructure:"clients"`
}

type ClientsConfig struct {
	RDP     RDPClientConfig     `mapstructure:"rdp"`
	SPICE   SPICEClientConfig   `mapstructure:"spice"`
	VNC     VNCClientConfig     `mapstructure:"vnc"`
	Horizon HorizonClientConfig `mapstructure:"horizon"`
}

type RDPClientConfig struct {
	WindowsPath string `mapstructure:"windows_path"`
	MacOSPath   string `mapstructure:"macos_path"`
	LinuxPath   string `mapstructure:"linux_path"`
}

type SPICEClientConfig struct {
	WindowsPath string `mapstructure:"windows_path"`
	MacOSPath   string `mapstructure:"macos_path"`
	LinuxPath   string `mapstructure:"linux_path"`
}

type VNCClientConfig struct {
	WindowsPath string `mapstructure:"windows_path"`
	MacOSPath   string `mapstructure:"macos_path"`
	LinuxPath   string `mapstructure:"linux_path"`
}

type HorizonClientConfig struct {
	WindowsPath string `mapstructure:"windows_path"`
	MacOSPath   string `mapstructure:"macos_path"`
	LinuxPath   string `mapstructure:"linux_path"`
}

type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

type PVEConfig struct {
	Endpoint  string `mapstructure:"endpoint"`
	Username  string `mapstructure:"username"`
	Password  string `mapstructure:"password"`
	VerifySSL bool   `mapstructure:"verify_ssl"`
	Node      string `mapstructure:"node"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type JWTConfig struct {
	Secret     string `mapstructure:"secret"`
	ExpireTime int    `mapstructure:"expire_time"`
}

func LoadConfig(configPath string) (*Config, error) {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("pve.endpoint", "https://localhost:8006")
	viper.SetDefault("pve.username", "root@pam")
	viper.SetDefault("pve.verify_ssl", false)
	viper.SetDefault("pve.node", "pve")
	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.user", "postgres")
	viper.SetDefault("database.sslmode", "disable")
	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("jwt.expire_time", 24)

	if err := viper.ReadInConfig(); err != nil {
		if os.IsNotExist(err) {
			return loadDefaultConfig(), nil
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	return &cfg, nil
}

func loadDefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		PVE: PVEConfig{
			Endpoint:  "https://localhost:8006",
			Username:  "root@pam",
			VerifySSL: false,
			Node:      "pve",
		},
		Database: DatabaseConfig{
			Host:    "localhost",
			Port:    5432,
			User:    "postgres",
			SSLMode: "disable",
		},
		Redis: RedisConfig{
			Host: "localhost",
			Port: 6379,
			DB:   0,
		},
		JWT: JWTConfig{
			Secret:     "your-secret-key-change-in-production",
			ExpireTime: 24,
		},
	}
}

func (c *DatabaseConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode)
}

func (c *RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
