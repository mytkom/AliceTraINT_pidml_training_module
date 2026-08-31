package scripts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/client"
	"github.com/mytkom/AliceTraINT_pidml_training_module/internal/config"
)

const (
	maxAODBatchSize           = 10
	downloadMultipleScript    = "download-multiple-grid-data.sh"
	producerScriptName        = "run-pidml-mc-producer.sh"
	subsampleScriptName       = "subsample.sh"
	localAo2dsListName        = "local_ao2ds_list.txt"
	datasetWorkSubdir         = "dataset_work"
	batchOutputsSubdir        = "batch_outputs"
	rawAo2dsSubdir            = "raw_ao2ds"
)

type DatasetRunner struct {
	*config.Config
	AODFiles            []client.AODFile
	IsONe               bool
	IsData              bool
	SubsampleEventCount uint
	TrainingConfigPath  string
	Checksum            string
	CachedDatasetPath   string
	LogOutPath          string
	LogErrPath          string
}

type datasetCacheMeta struct {
	Checksum            string   `json:"checksum"`
	AODPaths            []string `json:"aod_paths"`
	IsONe               bool     `json:"is_o_ne"`
	IsData              bool     `json:"is_data"`
	SubsampleEventCount uint     `json:"subsample_event_count"`
	DatasetPath         string   `json:"dataset_path"`
}

func NewDatasetRunner(
	cfg *config.Config,
	aodFiles []client.AODFile,
	isONe bool,
	isData bool,
	subsampleEventCount uint,
	trainingConfigPath string,
) *DatasetRunner {
	return &DatasetRunner{
		Config:              cfg,
		AODFiles:            aodFiles,
		IsONe:               isONe,
		IsData:              isData,
		SubsampleEventCount: subsampleEventCount,
		TrainingConfigPath:  trainingConfigPath,
		LogOutPath:          filepath.Join(cfg.ResultsDirPath, "dataset_out.log"),
		LogErrPath:          filepath.Join(cfg.ResultsDirPath, "dataset_err.log"),
	}
}

func DatasetCacheChecksum(aodFiles []client.AODFile, isONe, isData bool, subsampleEventCount uint) (string, []string, error) {
	paths := make([]string, 0, len(aodFiles))
	for _, aod := range aodFiles {
		path := strings.TrimSpace(aod.Path)
		if path == "" {
			return "", nil, fmt.Errorf("empty AOD path in task payload")
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return "", nil, fmt.Errorf("no AOD files in task payload")
	}
	sort.Strings(paths)

	var b strings.Builder
	for _, path := range paths {
		b.WriteString(path)
		b.WriteByte('\n')
	}
	b.WriteString(fmt.Sprintf("is_o_ne=%t\n", isONe))
	b.WriteString(fmt.Sprintf("is_data=%t\n", isData))
	b.WriteString(fmt.Sprintf("subsample_event_count=%d\n", subsampleEventCount))

	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:]), paths, nil
}

func (r *DatasetRunner) Run() error {
	checksum, sortedPaths, err := DatasetCacheChecksum(r.AODFiles, r.IsONe, r.IsData, r.SubsampleEventCount)
	if err != nil {
		return err
	}
	r.Checksum = checksum

	if err := os.MkdirAll(r.DatasetCacheDirPath, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create dataset cache dir: %w", err)
	}
	if err := os.MkdirAll(r.ResultsDirPath, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create results dir: %w", err)
	}

	r.CachedDatasetPath = filepath.Join(r.DatasetCacheDirPath, checksum+".root")
	metaPath := filepath.Join(r.DatasetCacheDirPath, checksum+".meta.json")

	logOut, err := os.OpenFile(r.LogOutPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to open dataset out log: %w", err)
	}
	defer logOut.Close()
	logErr, err := os.OpenFile(r.LogErrPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to open dataset err log: %w", err)
	}
	defer logErr.Close()

	outW := io.MultiWriter(logOut, os.Stdout)
	errW := io.MultiWriter(logErr, os.Stderr)

	if st, statErr := os.Stat(r.CachedDatasetPath); statErr == nil && !st.IsDir() {
		fmt.Fprintf(outW, "Dataset cache hit: %s\n", r.CachedDatasetPath)
	} else {
		fmt.Fprintf(outW, "Dataset cache miss: building %s\n", r.CachedDatasetPath)
		if err := r.buildDataset(sortedPaths, outW, errW); err != nil {
			return err
		}
		if err := r.writeMeta(metaPath, sortedPaths); err != nil {
			return err
		}
	}

	return r.patchTrainingConfig()
}

func (r *DatasetRunner) buildDataset(sortedPaths []string, outW, errW io.Writer) error {
	workRoot := filepath.Join(r.DataDirPath, datasetWorkSubdir)
	rawDir := filepath.Join(workRoot, rawAo2dsSubdir)
	batchOutDir := filepath.Join(workRoot, batchOutputsSubdir)

	if err := os.RemoveAll(workRoot); err != nil {
		return fmt.Errorf("failed to reset dataset work dir: %w", err)
	}
	if err := os.MkdirAll(rawDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create raw AO2D dir: %w", err)
	}
	if err := os.MkdirAll(batchOutDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create batch output dir: %w", err)
	}

	batches := chunkStrings(sortedPaths, maxAODBatchSize)
	batchRoots := make([]string, 0, len(batches))

	for i, batch := range batches {
		batchName := fmt.Sprintf("batch_%03d", i+1)
		fmt.Fprintf(outW, "Processing %s (%d AODs)\n", batchName, len(batch))

		if err := clearDirContents(rawDir); err != nil {
			return fmt.Errorf("failed to clear raw AO2D dir before %s: %w", batchName, err)
		}

		remoteListPath := filepath.Join(workRoot, batchName+"_remote.txt")
		if err := writeLines(remoteListPath, batch); err != nil {
			return fmt.Errorf("failed to write remote list for %s: %w", batchName, err)
		}

		if err := r.runDownload(remoteListPath, rawDir, outW, errW); err != nil {
			return fmt.Errorf("download failed for %s: %w", batchName, err)
		}

		localListPath := filepath.Join(rawDir, localAo2dsListName)
		if _, err := os.Stat(localListPath); err != nil {
			return fmt.Errorf("expected local AO2D list missing after download: %w", err)
		}
		aodListArg := "@" + localListPath

		if err := r.runProducer(aodListArg, batchName, batchOutDir, rawDir, outW, errW); err != nil {
			return fmt.Errorf("producer failed for %s: %w", batchName, err)
		}

		batchRoot := filepath.Join(batchOutDir, batchName+".root")
		if _, err := os.Stat(batchRoot); err != nil {
			return fmt.Errorf("expected batch ROOT missing: %s: %w", batchRoot, err)
		}
		batchRoots = append(batchRoots, batchRoot)
	}

	// SubsampleEventCount == 0 means full dataset - batches are only merged, not subsampled.
	tmpRoot := r.CachedDatasetPath + ".tmp.root"
	_ = os.Remove(tmpRoot)
	if err := r.runSubsample(tmpRoot, batchRoots, outW, errW); err != nil {
		_ = os.Remove(tmpRoot)
		return fmt.Errorf("subsample failed: %w", err)
	}

	if err := os.Rename(tmpRoot, r.CachedDatasetPath); err != nil {
		_ = os.Remove(tmpRoot)
		return fmt.Errorf("failed to promote cached dataset: %w", err)
	}

	fmt.Fprintf(outW, "Cached dataset written: %s\n", r.CachedDatasetPath)
	return nil
}

func (r *DatasetRunner) runDownload(remoteListPath, dataDir string, outW, errW io.Writer) error {
	script := filepath.Join(r.ScriptsDirPath, downloadMultipleScript)
	cmd := exec.Command(r.AlienvBin, "setenv", r.XjalienfsTag, "-c", script, remoteListPath)
	cmd.Stdout = outW
	cmd.Stderr = errW
	cmd.Env = r.scriptEnv(dataDir, r.DataDirPath)
	log.Printf("Running GRID download: %v", cmd.Args)
	return cmd.Run()
}

func (r *DatasetRunner) runProducer(aodListArg, outputName, trainingDir, dataDir string, outW, errW io.Writer) error {
	producer := filepath.Join(r.ScriptsDirPath, producerScriptName)
	isONe := strconv.FormatBool(r.IsONe)
	isData := strconv.FormatBool(r.IsData)

	inner := shellJoin(
		r.AlienvBin, "setenv", r.O2PhysicsTag, "-c",
		producer, aodListArg, outputName, isONe, isData,
	)
	cmd := exec.Command("script", "-q", "-c", inner, "/dev/null")
	cmd.Stdout = outW
	cmd.Stderr = errW
	cmd.Env = r.scriptEnv(dataDir, trainingDir)
	cmd.Dir = dataDir
	log.Printf("Running PIDML producer: %v", cmd.Args)
	return cmd.Run()
}

func shellJoin(args ...string) string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(out, " ")
}

func (r *DatasetRunner) runSubsample(outputPath string, inputRoots []string, outW, errW io.Writer) error {
	script := filepath.Join(r.ScriptsDirPath, subsampleScriptName)
	isData := strconv.FormatBool(r.IsData)
	args := []string{
		r.AlienvBin, "setenv", r.O2PhysicsTag, "-c",
		script,
		strconv.FormatUint(uint64(r.SubsampleEventCount), 10),
		outputPath,
		isData,
	}
	args = append(args, inputRoots...)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = outW
	cmd.Stderr = errW
	cmd.Env = r.scriptEnv(r.DataDirPath, r.DataDirPath)
	log.Printf("Running subsample: %v", cmd.Args)
	return cmd.Run()
}

func (r *DatasetRunner) scriptEnv(dataDir, trainingDir string) []string {
	env := os.Environ()
	env = append(env,
		"DATA_DIR="+dataDir,
		"PIDML_TRAINING_DIR="+trainingDir,
	)
	return env
}

func (r *DatasetRunner) writeMeta(metaPath string, sortedPaths []string) error {
	meta := datasetCacheMeta{
		Checksum:            r.Checksum,
		AODPaths:            sortedPaths,
		IsONe:               r.IsONe,
		IsData:              r.IsData,
		SubsampleEventCount: r.SubsampleEventCount,
		DatasetPath:         r.CachedDatasetPath,
	}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath, raw, os.ModePerm)
}

func (r *DatasetRunner) patchTrainingConfig() error {
	raw, err := os.ReadFile(r.TrainingConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read training config: %w", err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("failed to parse training config: %w", err)
	}

	paths := []interface{}{r.CachedDatasetPath}
	if r.IsData {
		cfg["exp_dataset_paths"] = paths
	} else {
		cfg["sim_dataset_paths"] = paths
	}

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal patched training config: %w", err)
	}
	if err := os.WriteFile(r.TrainingConfigPath, out, os.ModePerm); err != nil {
		return fmt.Errorf("failed to write patched training config: %w", err)
	}
	return nil
}

func (r *DatasetRunner) UploadLogs(ttId uint) error {
	err := client.UploadTaskResult(r.Config, ttId, &client.TaskResultPayload{
		Name:        filepath.Base(r.LogOutPath),
		Description: "Stdout log of dataset download/produce/subsample.",
		Type:        client.Log,
		FilePath:    r.LogOutPath,
	})
	if err != nil {
		return err
	}
	return client.UploadTaskResult(r.Config, ttId, &client.TaskResultPayload{
		Name:        filepath.Base(r.LogErrPath),
		Description: "Stderr log of dataset download/produce/subsample.",
		Type:        client.Log,
		FilePath:    r.LogErrPath,
	})
}

func (r *DatasetRunner) UploadResults(ttId uint) error {
	return nil
}

func chunkStrings(items []string, size int) [][]string {
	if size <= 0 {
		size = 1
	}
	var chunks [][]string
	for i := 0; i < len(items); i += size {
		end := i + size
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, items[i:end])
	}
	return chunks
}

func writeLines(path string, lines []string) error {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(b.String()), os.ModePerm)
}

func clearDirContents(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return os.MkdirAll(dir, os.ModePerm)
		}
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}
