package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/config"
	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/logger"
)

func sendRequest(cfg *config.Config, method, path string, body interface{}, headers map[string]string) (*http.Response, []byte, error) {
	log := logger.NewLogger()

	url := fmt.Sprintf("%s%s", cfg.AlicetrainBaseUrl, path)

	var requestBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			log.Printf("Error marshaling request body: %v", err)
			return nil, nil, fmt.Errorf("failed to marshal request body: %v", err)
		}
		requestBody = bytes.NewBuffer(data)
	}

	req, err := http.NewRequest(method, url, requestBody)
	if err != nil {
		log.Printf("Error creating request: %v", err)
		return nil, nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Secret-Id", cfg.MachineSecretKey)

	for key, value := range headers {
		req.Header.Add(key, value)
	}

	log.Printf("Sending %s request to %s with headers: %v", method, url, req.Header)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error sending request: %v", err)
		return nil, nil, fmt.Errorf("failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	bodyResp, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		return nil, nil, fmt.Errorf("failed to read response body: %v", err)
	}

	log.Printf("Response Status: %s", resp.Status)
	log.Printf("Response Body: %s", string(bodyResp))

	return resp, bodyResp, nil
}
