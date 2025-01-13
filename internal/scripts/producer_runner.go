package scripts

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/client"
	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/config"
)

const (
	PreprocessedAodFileName = "preprocessed_ao2ds"
	ProducerRunScriptName   = "run-pidml-producer.sh"
	ProducerRunSubscriptName  = "run-pidml-producer.sh"
	ProducerConfigFileName  = "ml-mc-config.json"
)

type ProducerRunner struct {
	*config.Config
	LocalListPath string
	LogErrPath    string
	LogOutPath    string
}

func NewProducerRunner(cfg *config.Config) *ProducerRunner {
	return &ProducerRunner{
		Config:        cfg,
		LocalListPath: filepath.Join(cfg.DataDirPath, LocalListName),
		LogErrPath:    filepath.Join(cfg.ResultsDirPath, "pidml_producer_err.log"),
		LogOutPath:    filepath.Join(cfg.ResultsDirPath, "pidml_producer_out.log"),
	}
}

func (p *ProducerRunner) Run() error {
	localListPath := filepath.Join(p.DataDirPath, "local_list.txt")
	pidMlProducerScriptPath := filepath.Join(p.ScriptsDirPath, ProducerRunScriptName)
	pidMlProducerSubscriptPath := filepath.Join(p.DataDirPath, ProducerRunSubscriptName)
	pidMlProducerConfigPath := filepath.Join(p.ScriptsDirPath, ProducerConfigFileName)

	logErr, err := os.OpenFile(p.LogErrPath, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logErr.Close()
	logOut, err := os.OpenFile(p.LogOutPath, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logOut.Close()

	preprocessedRootName := "preprocessed_ao2ds"
	alienvCommand := fmt.Sprintf(
		"alienv setenv O2Physics/latest -c %s %s %s %s %s %s",
		pidMlProducerScriptPath,
		pidMlProducerConfigPath,
		p.DataDirPath,
		localListPath,
		preprocessedRootName,
		pidMlProducerSubscriptPath,
	)
	pidMlProducerCmd := exec.Command("bash", "-c", alienvCommand)
	pidMlProducerCmd.Stdout = logOut
	pidMlProducerCmd.Stderr = logErr

	log.Printf("Running PID ML producer task, logs in err: %s, out: %s", p.LogErrPath, p.LogOutPath)
	err = pidMlProducerCmd.Run()
	if err != nil {
		return fmt.Errorf("command execution failed: %w", err)
	}

	return nil
}

func (p *ProducerRunner) UploadLogs(ttId uint) error {
	err := client.UploadTaskResult(p.Config, ttId, &client.TaskResultPayload{
		Name:        filepath.Base(p.LogOutPath),
		Description: "Stdout log file of PID ML Producer run.",
		Type:        client.Log,
		FilePath:    p.LogOutPath,
	})
	if err != nil {
		return err
	}

	err = client.UploadTaskResult(p.Config, ttId, &client.TaskResultPayload{
		Name:        filepath.Base(p.LogErrPath),
		Description: "Stderr log file of PID ML Producer run.",
		Type:        client.Log,
		FilePath:    p.LogErrPath,
	})
	if err != nil {
		return err
	}

	return nil
}

func (p *ProducerRunner) UploadResults(ttId uint) error {
	return nil
}
