package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/config"
	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/scripts"
)

func main() {
	// --- WAŻNE: Upewnij się, że ten test działa w odpowiednim folderze ---
	// Przejdź do głównego folderu AliceTraINT_pidml_training_module
	// np. C:\Users\...\AliceTraINT_pidml_training_module
	// i uruchom go run test_runner.go

	// Ładowanie configu, który symuluje cfg.LoadConfig() z main.go
	// Konieczne jest, aby plik .env był w folderze AliceTraINT_pidml_training_module
	// i zawierał poprawne ścieżki (ALICETRAINT_PDI_SRC_DIR_PATH itd.)
	cfg := config.LoadConfig()

	// --- Upewnij się, że foldery wynikowe są czyste przed testem ---
	// Aby uniknąć śmieci z poprzednich testów
	_ = os.RemoveAll(cfg.ResultsDirPath)
	_ = os.MkdirAll(cfg.ResultsDirPath, os.ModePerm)
	_ = os.RemoveAll(cfg.DataDirPath)
	_ = os.MkdirAll(cfg.DataDirPath, os.ModePerm)

	// --- Skopiuj pliki potrzebne do Pythona ---
	// W Go nie ma prostego kopiowania, więc załóżmy, że te pliki są w 'data/'
	// Dla tego testu załóż, że train.json i root file są już w cfg.DataDirPath
	// np. ręcznie skopiowane tam
	
	// Stwórzmy dummy train.json, jeśli go nie ma
	trainConfigContent := `{
		"input_file": "LHC23k4g-535069-from-001-to-006.root",
		"bs": 512,
		"max_epochs": 1,
		"embed_hidden": 128,
		"d_model": 32,
		"ff_hidden": 128,
		"pool_hidden": 64,
		"num_heads": 2,
		"num_blocks": 2,
		"start_lr": 0.001,
		"patience": 5,
		"patience_threshold": 0.001,
		"undersample": false,
		"use_gpu": false
	}`
	trainConfigPath := filepath.Join(cfg.DataDirPath, "train.json")
	err := os.WriteFile(trainConfigPath, []byte(trainConfigContent), os.ModePerm)
	if err != nil {
		log.Fatalf("Failed to create dummy train.json: %v", err)
	}
	fmt.Printf("Dummy train.json created at %s\n", trainConfigPath)

	// --- TEST 1: Process Command (nowy skrypt Pythona) ---
	fmt.Println("\n--- TESTING 'process' COMMAND ---")
	rootFilePath := filepath.Join(cfg.DataDirPath, "LHC23k4g-535069-from-001-to-006.root")
	processRunner := scripts.NewPdiRunner(
		scripts.PdiCommandProcess,
		cfg,
		rootFilePath, // Symulacja argumentu input_file
		trainConfigPath,                               // Symulacja argumentu cfg_file
	)
	err = processRunner.Run()
	if err != nil {
		log.Fatalf("Process command failed: %v", err)
	}
	fmt.Println("Process command finished successfully!")

	// --- TEST 2: Train-Particle Command (nowy skrypt Pythona) ---
	fmt.Println("\n--- TESTING 'train-particle' COMMAND for 'pion' ---")
	trainParticleRunner := scripts.NewPdiRunner(
		scripts.PdiCommandTrainParticle,
		cfg,
		trainConfigPath, // Argument dla Pythona
		"--particle", "pion",            // Argument dla Pythona
	)

	err = trainParticleRunner.Run()
	if err != nil {
		log.Fatalf("Train-particle command failed: %v", err)
	}
	fmt.Println("Train-particle command finished successfully! Checking results...")
	
	// --- TEST 3: UploadResults (sprawdzenie rekurencyjnego wyszukiwania) ---
	// Ta funkcja wypisze na konsolę co znalazła, jeśli mock server jest wyłączony.
	fmt.Println("\n--- TESTING UploadResults for 'train-particle' ---")
	err = trainParticleRunner.UploadResults(123) // Używamy dummy ttId = 123
	if err != nil {
		// Błąd połączenia z mock serverem jest oczekiwany, jeśli go nie ma
		if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "no connection could be made") {
			fmt.Println("Expected: Connection to AliceTraINT server refused (mock server is likely off or not configured). This is OK for local test.")
		} else {
			log.Fatalf("UploadResults failed unexpectedly: %v", err)
		}
	}

	fmt.Println("\nAll tests completed. Check your results/ directory for generated files.")
}
