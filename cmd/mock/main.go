package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/client"
	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/config"
)

func uploadRecursiveWalkForExtension(rootDir, extension string, cfg *config.Config, ttId uint) {
	err := filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			fmt.Println("Error accessing path:", err)
			return err
		}

		if !d.IsDir() {
			if strings.HasSuffix(d.Name(), extension) {
				fmt.Println("Found file:", path)
				var resultType client.TaskResultType
				if extension == ".onnx" {
					resultType = client.Onnx
				} else if extension == ".png" || extension == ".jpg" || extension == ".jpeg" {
					resultType = client.Image
				} else {
					resultType = client.Log
				}

				payload := client.TaskResultPayload{
					Name:        d.Name(),
					Description: d.Name(),
					Type:        resultType,
					FilePath:    path,
				}
				client.UploadTaskResult(cfg, ttId, &payload)
			}
		}
		return nil
	})

	if err != nil {
		fmt.Println("Error walking the directory:", err)
	}
}

func main() {
	cfg := config.LoadConfig()

	// Mock loop
	for {
		duration := 5 * time.Second
		time.Sleep(duration)

		ttr, err := client.GetQueuedTask(cfg)
		if err != nil {
			log.Fatal(err.Error())
		}

		if ttr == nil {
			continue
		}

		time.Sleep(duration)

		uploadRecursiveWalkForExtension("mock_uploads/models", ".onnx", cfg, ttr.ID)

		err = client.UpdateTaskStatus(cfg, ttr.ID, client.Benchmarking)
		if err != nil {
			log.Fatal(err.Error())
		}

		time.Sleep(duration)

		uploadRecursiveWalkForExtension("mock_uploads/graphs", ".png", cfg, ttr.ID)

		err = client.UpdateTaskStatus(cfg, ttr.ID, client.Completed)
		if err != nil {
			log.Fatal(err.Error())
		}
	}
}
