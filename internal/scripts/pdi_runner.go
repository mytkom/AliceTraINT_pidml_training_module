package scripts

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

	// Cross-platform python path detection
	pythonExePath := "bin/python3"
	if runtime.GOOS == "windows" {
		pythonExePath = "Scripts/python.exe"
	}
	pythonVenvBin := filepath.Join(p.VenvDirPath, pythonExePath)

	scriptPath := filepath.Join(p.ScriptsDirPath, "pdi_scripts.py")
	cmdArgs := append([]string{scriptPath, string(p.Command)}, p.Args...)

	cmd := exec.Command(pythonVenvBin, cmdArgs...)
	cmd.Stdout = multiWriterOut
	cmd.Stderr = multiWriterErr

	fmt.Printf("Executing: %s %s\n", pythonVenvBin, strings.Join(cmdArgs, " "))
	return cmd.Run()
}

func (p *PdiRunner) UploadLogs(ttId uint) error {
	client.UploadTaskResult(p.Config, ttId, &client.TaskResultPayload{
		Name:        filepath.Base(p.LogOutPath),
		Description: fmt.Sprintf("Log file of %s pdi's command", string(p.Command)),
		Type:        client.Log,
		FilePath:    p.LogOutPath,
	})
	client.UploadTaskResult(p.Config, ttId, &client.TaskResultPayload{
		Name:        filepath.Base(p.LogErrPath),
		Description: fmt.Sprintf("Log file of %s pdi's command", string(p.Command)),
		Type:        client.Log,
		FilePath:    p.LogErrPath,
	})
	return nil
}

// uploadWalkDir searches RECURSIVELY for files with specific extensions
func uploadWalkDir(cfg *config.Config, rootDir string, resType client.TaskResultType, ttId uint, descFunc func(name string) string) error {
	return filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip inaccessible files
		}
		if !d.IsDir() {
			ext := client.GetExtensionFromResultType(resType)
			if strings.HasSuffix(strings.ToLower(d.Name()), strings.ToLower(ext)) {
				fmt.Println("Found and uploading:", path)
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
		return nil
	case PdiCommandDataExploration, PdiCommandBenchmark:
		// Upload all images (.png) found anywhere in results/
		return uploadWalkDir(p.Config, p.ResultsDirPath, client.Image, ttId, func(name string) string {
			return "Generated plot/graph"
		})
	case PdiCommandTrain:
		// Upload all ONNX models found anywhere in results/
		return uploadWalkDir(p.Config, p.ResultsDirPath, client.Onnx, ttId, func(name string) string {
			particle := strings.TrimSuffix(name, filepath.Ext(name))
			return fmt.Sprintf("ONNX model for %s", particle)
		})
	}
	return nil
}
