package client

import (
	"fmt"
	"net/http"

	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/config"
)

type TaskResultType uint

const (
	Log TaskResultType = iota
	Image
	Onnx
)

type TaskResultPayload struct {
	Name        string
	Type        int
	Description string
	Filename    string
	File        string
}

func UploadTaskResult(cfg *config.Config, ttId uint, ttr *TaskResultPayload) error {
	path := fmt.Sprintf("/training_tasks/%d/training_task_results", ttId)

	resp, _, err := sendRequest(cfg, "POST", path, ttr, nil)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusUnprocessableEntity {
		return fmt.Errorf("invalid task result")
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("internal server error")
	}

	return nil
}
