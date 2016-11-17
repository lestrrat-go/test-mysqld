package mysqltest

import "os/exec"

// DatasourceOption is an object that can be passed to the
// various methods that generate datasource names
type DatasourceOption interface {
	Name() string
	Value() interface{}
}

// MysqldConfig is used to configure the new mysql instance
type MysqldConfig struct {
	BaseDir        string
	BindAddress    string
	CopyDataFrom   string
	DataDir        string
	PidFile        string
	Port           int
	SkipNetworking bool
	Socket         string
	TmpDir         string

	AutoStart      int
	MysqlInstallDb string
	Mysqld         string
}

// TestMysqld is the main struct that handles the execution of mysqld
type TestMysqld struct {
	Config       *MysqldConfig
	Command      *exec.Cmd
	DefaultsFile string
	Guards       []func()
	LogFile      string
}
