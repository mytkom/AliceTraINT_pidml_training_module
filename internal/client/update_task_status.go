package client

import (
	"fmt"
	"net/http"

	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/config"
)

type TrainingTaskStatus uint

const (
	Failed TrainingTaskStatus = iota
	Queued
	Training
	Benchmarking
	Completed
)

type UpdateTaskStatusPayload struct {
	Status TrainingTaskStatus
}

func UpdateTaskStatus(cfg *config.Config, ttId uint, status TrainingTaskStatus) error {
	path := fmt.Sprintf("/training-tasks/%d/status", ttId)

	statusPayload := UpdateTaskStatusPayload{
		Status: status,
	}

	resp, _, err := sendRequest(cfg, "POST", path, statusPayload, nil)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusUnprocessableEntity {
		return fmt.Errorf("invalid task status")
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("internal server error")
	}

	return nil
}
