package client

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/config"
)

type AODFile struct {
	Path string
}

type TrainingTaskResponse struct {
	ID            uint
	AODFiles      []AODFile
	Configuration interface{}
}

func GetQueuedTask(cfg *config.Config) (*TrainingTaskResponse, error) {
	path := fmt.Sprintf("/training_machines/%d/training_task", cfg.MachineID)

	resp, body, err := sendRequest(cfg, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("internal server error")
	}

	var ttr TrainingTaskResponse
	err = json.Unmarshal(body, &ttr)
	if err != nil {
		return nil, err
	}

	return &ttr, nil
}
