package config

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	MachineID         uint
	MachineSecretKey  string
	AlicetrainBaseUrl string
}

func LoadConfig() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	machineID, err := getEnvAsUint("MACHINE_ID")
	if err != nil {
		log.Fatal(err.Error())
	}

	MachineSecretKey, err := getEnv("MACHINE_SECRET_KEY")
	if err != nil {
		log.Fatal(err.Error())
	}

	alicetraintBaseUrl, err := getEnv("ALICETRAINT_BASE_URL")
	if err != nil {
		log.Fatal(err.Error())
	}

	return &Config{
		MachineID:         machineID,
		MachineSecretKey:  MachineSecretKey,
		AlicetrainBaseUrl: alicetraintBaseUrl,
	}
}

func getEnv(key string) (string, error) {
	if value, exists := os.LookupEnv(key); exists {
		return value, nil
	}

	return "", fmt.Errorf("ENV: %s not present", key)
}

func getEnvAsUint(key string) (uint, error) {
	valueStr, err := getEnv(key)
	if err != nil {
		return 0, err
	}

	value, err := strconv.ParseUint(valueStr, 10, 32)
	if err != nil {
		return 0, err
	}

	return uint(value), nil
}
