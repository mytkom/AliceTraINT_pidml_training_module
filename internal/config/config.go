package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	MachineID         uint
	MachineSecretKey  string
	AlicetrainBaseUrl string
	DataDirPath       string
	ScriptsDirPath    string
	VenvDirPath       string
	ResultsDirPath    string
	PdiDirPath        string
}

func LoadConfig() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	return &Config{
		MachineID:         getEnvAsUint("MACHINE_ID"),
		MachineSecretKey:  getEnv("MACHINE_SECRET_KEY"),
		AlicetrainBaseUrl: getEnv("ALICETRAINT_BASE_URL"),
		DataDirPath:       getEnvPath("ALICETRAINT_DATA_DIR_PATH"),
		ScriptsDirPath:    getEnvPath("ALICETRAINT_SCRIPTS_DIR_PATH"),
		VenvDirPath:       getEnvPath("ALICETRAINT_VENV_DIR_PATH"),
		ResultsDirPath:    getEnvPath("ALICETRAINT_RESULTS_DIR_PATH"),
		PdiDirPath:        getEnvPath("ALICETRAINT_PDI_SRC_DIR_PATH"),
	}
}

func getEnv(key string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		log.Fatal(fmt.Errorf("ENV: %s not present", key))
	}

	return value
}

func getEnvPath(key string) string {
	value := getEnv(key)

	valueAbs, err := filepath.Abs(value)
	if err != nil {
		log.Fatal(fmt.Errorf("ENV: %s cannot calculate absolute path", key))
	}

	return valueAbs
}

func getEnvAsUint(key string) uint {
	valueStr := getEnv(key)

	value, err := strconv.ParseUint(valueStr, 10, 32)
	if err != nil {
		log.Fatal(err)
	}

	return uint(value)
}
