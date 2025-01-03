package scripts

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/client"
	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/config"
)

func PidMLProducer(cfg *config.Config) (string, error) {
	localListPath := filepath.Join(cfg.DataDirPath, "local_list.txt")
	logPath := filepath.Join(cfg.ResultsDirPath, "pidml-producer.log")
	pidMlProducerScriptPath := filepath.Join(cfg.ScriptsDirPath, "run-pidml-producer.sh")
	pidMlProducerConfigPath := filepath.Join(cfg.ScriptsDirPath, "ml-mc-config.json")

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return "", fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	preprocessedRootName := "preprocessed_ao2ds"
	alienvCommand := fmt.Sprintf(
		"alienv setenv O2Physics/latest -c %s %s %s %s %s",
		pidMlProducerScriptPath,
		pidMlProducerConfigPath,
		cfg.DataDirPath,
		localListPath,
		preprocessedRootName,
	)
	pidMlProducerCmd := exec.Command("bash", "-c", alienvCommand)
	pidMlProducerCmd.Stdout = logFile
	pidMlProducerCmd.Stderr = logFile

	log.Printf("Running PID ML producer task, log in %s", logPath)
	err = pidMlProducerCmd.Run()
	if err != nil {
		return "", fmt.Errorf("command execution failed: %w", err)
	}

	return filepath.Join(cfg.DataDirPath, fmt.Sprintf("%s.root", preprocessedRootName)), nil
}

func DownloadFromGrid(cfg *config.Config, aodFiles []client.AODFile) error {
	remoteListPath := filepath.Join(cfg.DataDirPath, "remote_list.txt")
	localListPath := filepath.Join(cfg.DataDirPath, "local_list.txt")
	aodsOutputDir := filepath.Join(cfg.DataDirPath, "raw_ao2ds")
	logPath := filepath.Join(cfg.ResultsDirPath, "download.log")
	scriptPath := filepath.Join(cfg.ScriptsDirPath, "download-from-grid.sh")

	err := prepareFileList(aodFiles, remoteListPath)
	if err != nil {
		return fmt.Errorf("failed to prepare remote list file: %w", err)
	}

	err = os.MkdirAll(aodsOutputDir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	multiWriter := io.MultiWriter(logFile, os.Stdout)

	cmd := exec.Command("alienv", "setenv", "xjalienfs/latest", "-c", scriptPath, remoteListPath, aodsOutputDir)
	cmd.Stdout = multiWriter
	cmd.Stderr = multiWriter

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("command execution failed: %w", err)
	}

	err = os.WriteFile(localListPath, []byte{}, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create local list file: %w", err)
	}

	remoteFile, err := os.Open(remoteListPath)
	if err != nil {
		return fmt.Errorf("failed to open remote file list: %w", err)
	}
	defer remoteFile.Close()

	localList, err := os.OpenFile(localListPath, os.O_WRONLY, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to open local list file for writing: %w", err)
	}
	defer localList.Close()

	scanner := bufio.NewScanner(remoteFile)
	for scanner.Scan() {
		remoteURL := scanner.Text()
		localPath := filepath.Join(aodsOutputDir, strings.ReplaceAll(strings.TrimPrefix(remoteURL, "/"), "/", "-"))

		sourcePath := filepath.Join(aodsOutputDir, remoteURL)
		err = os.Rename(sourcePath, localPath)
		if err != nil {
			return fmt.Errorf("failed to move file %s to %s: %w", sourcePath, localPath, err)
		}

		_, err = localList.WriteString(localPath + "\n")
		if err != nil {
			return fmt.Errorf("failed to write to local list file: %w", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading remote file list: %w", err)
	}

	if err := os.RemoveAll(filepath.Join(aodsOutputDir, "alice")); err != nil {
		return fmt.Errorf("cannnot remove empty remote files directory: %w", err)
	}

	return nil
}

func prepareFileList(aodFiles []client.AODFile, path string) error {
	err := os.MkdirAll(filepath.Dir(path), os.ModeDir|os.ModePerm)
	if err != nil {
		return err
	}

	fo, err := os.Create(path)
	if err != nil {
		return err
	}

	defer func() {
		if err := fo.Close(); err != nil {
			panic(err)
		}
	}()

	for _, aod := range aodFiles {
		_, err := fo.WriteString(fmt.Sprintln(aod.Path))
		if err != nil {
			return err
		}
	}

	return nil
}
