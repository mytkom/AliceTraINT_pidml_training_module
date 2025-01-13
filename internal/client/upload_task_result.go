package client

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/config"
)

type TaskResultType uint

const (
	Log TaskResultType = iota
	Image
	Onnx
)

func GetExtensionFromResultType(resType TaskResultType) string {
	switch resType {
	case Log:
		return ".log"
	case Image:
		return ".png"
	case Onnx:
		return ".onnx"
	}

	return ""
}

type TaskResultPayload struct {
	Name        string
	Type        TaskResultType
	Description string
	FilePath    string
}

func intToReader(value int) io.Reader {
	str := fmt.Sprintf("%d", value)

	return bytes.NewBufferString(str)
}

func UploadTaskResult(cfg *config.Config, ttId uint, ttr *TaskResultPayload) error {
	path := fmt.Sprintf("/training-tasks/%d/training-task-results", ttId)

	file, err := os.Open(ttr.FilePath)
	if err != nil {
		return fmt.Errorf("error opening file: %s", err.Error())
	}
	defer file.Close()

	formData := map[string]io.Reader{
		"file":        file,
		"file-type":   intToReader(int(ttr.Type)),
		"name":        bytes.NewBufferString(ttr.Name),
		"description": bytes.NewBufferString(ttr.Description),
	}

	resp, _, err := sendMultipartRequest(cfg, "POST", path, formData, nil)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusUnprocessableEntity {
		return fmt.Errorf("invalid task result")
	}

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("internal server error")
	}

	return nil
}
