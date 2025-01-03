package scripts

type Command interface {
	Run() error
	UploadLogs(ttId uint) error
	UploadResults(ttId uint) error
}
