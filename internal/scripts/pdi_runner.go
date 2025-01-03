package scripts

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/client"
	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/config"
)

type PdiCommand string

const (
	PdiCommandTrain           PdiCommand = "train"
	PdiCommandProcess         PdiCommand = "process"
	PdiCommandDataExploration PdiCommand = "data-exploration"
	PdiCommandBenchmark       PdiCommand = "benchmark"
)

type PdiRunner struct {
	*config.Config
	Command    PdiCommand
	Args       []string
	LogOutPath string
	LogErrPath string
}

func NewPdiRunner(command PdiCommand, cfg *config.Config, args ...string) *PdiRunner {
	return &PdiRunner{
		Command:    command,
		Config:     cfg,
		Args:       args,
		LogOutPath: filepath.Join(cfg.ResultsDirPath, fmt.Sprintf("pdi_%s_out.log", string(command))),
		LogErrPath: filepath.Join(cfg.ResultsDirPath, fmt.Sprintf("pdi_%s_err.log", string(command))),
	}
}

func (p *PdiRunner) Run() error {
	os.Setenv("PDI_DIR", p.PdiDirPath)
	os.Setenv("DATA_DIR", p.DataDirPath)
	os.Setenv("RESULTS_DIR", p.ResultsDirPath)

	logFileOut, err := os.OpenFile(p.LogOutPath, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFileOut.Close()
	multiWriterOut := io.MultiWriter(logFileOut, os.Stdout)

	logFileErr, err := os.OpenFile(p.LogErrPath, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFileErr.Close()
	multiWriterErr := io.MultiWriter(logFileErr, os.Stderr)

	pythonVenvBin := filepath.Join(p.VenvDirPath, "bin/python3")
	scriptPath := filepath.Join(p.ScriptsDirPath, "pdi_scripts.py")

	cmdArgs := append([]string{scriptPath, string(p.Command)}, p.Args...)

	cmd := exec.Command(pythonVenvBin, cmdArgs...)
	cmd.Stdout = multiWriterOut
	cmd.Stderr = multiWriterErr

	fmt.Printf("Executing: %s %s\n", pythonVenvBin, strings.Join(cmdArgs, " "))
	return cmd.Run()
}

func (p *PdiRunner) UploadLogs(ttId uint) error {
	err := client.UploadTaskResult(p.Config, ttId, &client.TaskResultPayload{
		Name:        filepath.Base(p.LogOutPath),
		Description: fmt.Sprintf("Log file of %s pdi's command", string(p.Command)),
		Type:        client.Log,
		FilePath:    p.LogOutPath,
	})
	if err != nil {
		return err
	}

	err = client.UploadTaskResult(p.Config, ttId, &client.TaskResultPayload{
		Name:        filepath.Base(p.LogErrPath),
		Description: fmt.Sprintf("Log file of %s pdi's command", string(p.Command)),
		Type:        client.Log,
		FilePath:    p.LogErrPath,
	})
	if err != nil {
		return err
	}

	return nil
}

func uploadWalkDir(cfg *config.Config, rootDir string, resType client.TaskResultType, ttId uint, descFunc func(name string) string) error {
	return filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			fmt.Println("Error accessing path:", err)
			return err
		}

		if !d.IsDir() {
			if strings.HasSuffix(d.Name(), client.GetExtensionFromResultType(resType)) {
				fmt.Println("Found file:", path)
				client.UploadTaskResult(cfg, ttId, &client.TaskResultPayload{
					Name:        d.Name(),
					Description: descFunc(d.Name()),
					Type:        resType,
					FilePath:    path,
				})
			}
		}
		return nil
	})
}

func (p *PdiRunner) UploadResults(ttId uint) error {
	switch p.Command {
	case PdiCommandProcess:
	case PdiCommandDataExploration:
		return uploadWalkDir(
			p.Config,
			filepath.Join(p.ResultsDirPath, "data-exploration"),
			client.Image,
			ttId,
			func(name string) string {
				return "Part of data exploration graphs"
			},
		)
	case PdiCommandTrain:
		return uploadWalkDir(
			p.Config,
			filepath.Join(p.ResultsDirPath, "models"),
			client.Onnx,
			ttId,
			func(name string) string {
				particle := strings.TrimSuffix(name, filepath.Ext(name))
				return fmt.Sprintf("ONNX exported neural network for %s", particle)
			},
		)
	case PdiCommandBenchmark:
		err := uploadWalkDir(
			p.Config,
			filepath.Join(p.ResultsDirPath, "benchmark"),
			client.Image,
			ttId,
			func(name string) string {
				return "Benchmark data of trained neural network"
			},
		)
		if err != nil {
			return err
		}
		return uploadWalkDir(
			p.Config,
			filepath.Join(p.ResultsDirPath, "feature_importance"),
			client.Image,
			ttId,
			func(name string) string {
				return "Feature importance of neural network"
			},
		)
	}

	return nil
}
