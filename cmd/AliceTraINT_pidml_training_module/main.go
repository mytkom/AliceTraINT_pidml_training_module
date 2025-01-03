package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/client"
	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/config"
	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/scripts"
)

func runCommands(commands []scripts.Command, ttId uint) error {
	for _, command := range commands {
		err := command.Run()
		if err != nil {
			command.UploadLogs(ttId)
			return err
		}
		command.UploadLogs(ttId)
		command.UploadResults(ttId)
	}

	return nil
}

func handleError(cfg *config.Config, err error, ttId uint) {
	log.Printf("Training Task of id %d, error occured, setting status to failed. Error text: %s", ttId, err.Error())
	err = client.UpdateTaskStatus(cfg, ttId, client.Failed)
	if err != nil {
		log.Fatal(err.Error())
	}
}

func main() {
	cfg := config.LoadConfig()
	trainingConfigPath := filepath.Join(cfg.DataDirPath, "train.json")

	for {
		err := os.MkdirAll(cfg.DataDirPath, os.ModePerm)
		if err != nil {
			log.Fatal(err.Error())
		}

		err = os.MkdirAll(cfg.ResultsDirPath, os.ModePerm)
		if err != nil {
			log.Fatal(err.Error())
		}

		tt, err := client.GetQueuedTask(cfg)
		if err != nil {
			log.Fatal(err.Error())
		}

		if tt == nil {
			continue
		}

		jsonString, err := json.Marshal(tt.Configuration)
		if err != nil {
			handleError(cfg, err, tt.ID)
			continue
		}
		os.WriteFile(trainingConfigPath, jsonString, os.ModePerm)

		err = scripts.DownloadFromGrid(cfg, tt.AODFiles)
		if err != nil {
			handleError(cfg, err, tt.ID)
			continue
		}

		preprocessedRoot, err := scripts.PidMLProducer(cfg)
		if err != nil {
			handleError(cfg, err, tt.ID)
			continue
		}

		training_commands := []scripts.Command{
			scripts.NewPdiRunner(scripts.PdiCommandProcess, cfg, preprocessedRoot, trainingConfigPath),
			scripts.NewPdiRunner(scripts.PdiCommandDataExploration, cfg),
			scripts.NewPdiRunner(scripts.PdiCommandTrain, cfg, trainingConfigPath),
		}
		err = runCommands(training_commands, tt.ID)
		if err != nil {
			handleError(cfg, err, tt.ID)
			continue
		}

		err = client.UpdateTaskStatus(cfg, tt.ID, client.Benchmarking)
		if err != nil {
			handleError(cfg, err, tt.ID)
			continue
		}

		benchmarking_commands := []scripts.Command{
			scripts.NewPdiRunner(scripts.PdiCommandBenchmark, cfg),
		}
		err = runCommands(benchmarking_commands, tt.ID)
		if err != nil {
			handleError(cfg, err, tt.ID)
			continue
		}

		err = client.UpdateTaskStatus(cfg, tt.ID, client.Completed)
		if err != nil {
			handleError(cfg, err, tt.ID)
			continue
		}
	}
}
