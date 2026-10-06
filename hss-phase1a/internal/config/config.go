package config

import (
	"log"

	"github.com/spf13/viper"
)

type Config struct {
	DataDir string
	API     APIConfig
	Control ControlConfig
	Storage StorageConfig
	Camera  CameraConfig
}

type APIConfig struct {
	Port int
}

type ControlConfig struct {
	Endpoint string
}

type StorageConfig struct {
	RecordingsPath     string
	MaxDiskPercent     int
	WarningDiskPercent int
	RetentionDays      int
}

type CameraConfig struct {
	SegmentDurationSeconds int
}

var AppConfig *Config

func InitConfig() {
	viper.SetDefault("DataDir", "./data")
	viper.SetDefault("API.Port", 8080)
	viper.SetDefault("Control.Endpoint", "ws://localhost:9091/signaling")
	viper.SetDefault("Storage.RecordingsPath", "./storage/recordings")
	viper.SetDefault("Storage.MaxDiskPercent", 90)
	viper.SetDefault("Storage.WarningDiskPercent", 85)
	viper.SetDefault("Storage.RetentionDays", 7)
	viper.SetDefault("Camera.SegmentDurationSeconds", 300)

	viper.SetConfigName("hss")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("/etc/hss/")
	viper.AddConfigPath("$HOME/.hss")
	viper.AddConfigPath(".")

	// Allow overriding from ENV variables (e.g. HSS_API_PORT)
	viper.SetEnvPrefix("hss")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Println("No config file found. Using defaults and environment variables.")
		} else {
			log.Fatalf("Error reading config file: %v", err)
		}
	}

	AppConfig = &Config{}
	err := viper.Unmarshal(AppConfig)
	if err != nil {
		log.Fatalf("Unable to decode config into struct: %v", err)
	}
}
