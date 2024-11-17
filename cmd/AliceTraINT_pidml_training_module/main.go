package main

import (
	"log"
	"time"

	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/client"
	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/config"
)

func main() {
	cfg := config.LoadConfig()

	// TODO: implement real training loop
	// Mock loop
	for {
		time.Sleep(60 * time.Second)

		ttr, err := client.GetQueuedTask(cfg)
		if err != nil {
			log.Fatal(err.Error())
		}

		if ttr == nil {
			continue
		}

		time.Sleep(60 * time.Second)

		err = client.UpdateTaskStatus(cfg, ttr.ID, client.Benchmarking)
		if err != nil {
			log.Fatal(err.Error())
		}

		time.Sleep(60 * time.Second)

		err = client.UpdateTaskStatus(cfg, ttr.ID, client.Completed)
		if err != nil {
			log.Fatal(err.Error())
		}
	}
}
