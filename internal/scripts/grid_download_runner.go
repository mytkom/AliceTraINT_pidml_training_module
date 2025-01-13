package scripts

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/client"
	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/config"
)

const (
	RemoteListName     = "remote_list.txt"
	LocalListName      = "local_list.txt"
	RawAodsSUbdir      = "raw_ao2ds"
	DownloadScriptName = "download-from-grid.sh"
	GenerateRunScriptName = "generate-run-pidml-producer-script.py"
)

type GridDownloadRunner struct {
	*config.Config
	AODFiles       []client.AODFile
	LogErrPath     string
	LogOutPath     string
	RemoteListPath string
	LocalListPath  string
	AodsOutputDir  string
	ScriptPath     string
	PIDMLProducerGenerateScript string
}

func NewGridDownloadRunner(cfg *config.Config, aodFiles []client.AODFile) *GridDownloadRunner {
	return &GridDownloadRunner{
		Config:         cfg,
		AODFiles:       aodFiles,
		LogErrPath:     filepath.Join(cfg.DataDirPath, "grid_download_err.log"),
		LogOutPath:     filepath.Join(cfg.DataDirPath, "grid_download_out.log"),
		RemoteListPath: filepath.Join(cfg.DataDirPath, RemoteListName),
		LocalListPath:  filepath.Join(cfg.DataDirPath, LocalListName),
		AodsOutputDir:  filepath.Join(cfg.DataDirPath, RawAodsSUbdir),
		ScriptPath:     filepath.Join(cfg.ScriptsDirPath, DownloadScriptName),
		PIDMLProducerGenerateScript: filepath.Join(cfg.ScriptsDirPath, GenerateRunScriptName),
	}
}

func (r *GridDownloadRunner) Run() error {
	err := r.prepareFileList()
	if err != nil {
		return fmt.Errorf("failed to prepare remote list file: %w", err)
	}

	err = os.MkdirAll(r.AodsOutputDir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	logErr, err := os.OpenFile(r.LogErrPath, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logErr.Close()
	logOut, err := os.OpenFile(r.LogOutPath, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logOut.Close()

	multiWriterOut := io.MultiWriter(logOut, os.Stdout)
	multiWriterErr := io.MultiWriter(logOut, os.Stderr)

	cmd := exec.Command("alienv", "setenv", "xjalienfs/latest", "-c", r.ScriptPath, r.RemoteListPath, r.AodsOutputDir)
	cmd.Stdout = multiWriterOut
	cmd.Stderr = multiWriterErr

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("command execution failed: %w", err)
	}

	err = os.WriteFile(r.LocalListPath, []byte{}, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create local list file: %w", err)
	}

	remoteFile, err := os.Open(r.RemoteListPath)
	if err != nil {
		return fmt.Errorf("failed to open remote file list: %w", err)
	}
	defer remoteFile.Close()

	localList, err := os.OpenFile(r.LocalListPath, os.O_WRONLY, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to open local list file for writing: %w", err)
	}
	defer localList.Close()

	scanner := bufio.NewScanner(remoteFile)
	lastLocalPath := ""
	for scanner.Scan() {
		remoteURL := scanner.Text()
		localPath := filepath.Join(r.AodsOutputDir, strings.ReplaceAll(strings.TrimPrefix(remoteURL, "/"), "/", "-"))
		lastLocalPath = localPath

		sourcePath := filepath.Join(r.AodsOutputDir, remoteURL)
		err = os.Rename(sourcePath, localPath)
		if err != nil {
			return fmt.Errorf("failed to move file %s to %s: %w", sourcePath, localPath, err)
		}

		_, err = localList.WriteString(localPath + "\n")
		if err != nil {
			return fmt.Errorf("failed to write to local list file: %w", err)
		}
	}

	pythonVenvBin := filepath.Join(r.VenvDirPath, "bin/python3")
	pidMlProducerSubscriptPath := filepath.Join(r.DataDirPath, ProducerRunSubscriptName)
	cmd = exec.Command(pythonVenvBin, r.PIDMLProducerGenerateScript, lastLocalPath, pidMlProducerSubscriptPath)
	cmd.Stdout = multiWriterOut
	cmd.Stderr = multiWriterErr

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("command execution failed: %w", err)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading remote file list: %w", err)
	}

	if err := os.RemoveAll(filepath.Join(r.AodsOutputDir, "alice")); err != nil {
		return fmt.Errorf("cannnot remove empty remote files directory: %w", err)
	}

	return nil
}

func (r *GridDownloadRunner) prepareFileList() error {
	err := os.MkdirAll(filepath.Dir(r.RemoteListPath), os.ModeDir|os.ModePerm)
	if err != nil {
		return err
	}

	fo, err := os.Create(r.RemoteListPath)
	if err != nil {
		return err
	}

	defer func() {
		if err := fo.Close(); err != nil {
			panic(err)
		}
	}()

	for _, aod := range r.AODFiles {
		_, err := fo.WriteString(fmt.Sprintln(aod.Path))
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *GridDownloadRunner) UploadLogs(ttId uint) error {
	err := client.UploadTaskResult(r.Config, ttId, &client.TaskResultPayload{
		Name:        filepath.Base(r.LogOutPath),
		Description: "Stdout log file of GRID downloader script.",
		Type:        client.Log,
		FilePath:    r.LogOutPath,
	})
	if err != nil {
		return err
	}

	err = client.UploadTaskResult(r.Config, ttId, &client.TaskResultPayload{
		Name:        filepath.Base(r.LogErrPath),
		Description: "Stderr log file of GRID downloader script.",
		Type:        client.Log,
		FilePath:    r.LogErrPath,
	})
	if err != nil {
		return err
	}

	return nil
}

func (r *GridDownloadRunner) UploadResults(ttId uint) error {
	return nil
}
