package mysqltest

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql" // for mysql
	"github.com/lestrrat-go/tcputil"
	"github.com/pkg/errors"
)

// NewConfig creates a new MysqldConfig struct with default values
func NewConfig() *MysqldConfig {
	return &MysqldConfig{
		AutoStart:      2,
		SkipNetworking: true,
	}
}

// NewMysqld creates a new TestMysqld instance
func NewMysqld(config *MysqldConfig) (*TestMysqld, error) {
	guards := []func(){}

	if config == nil {
		config = NewConfig()
	}

	if config.BaseDir != "" {
		// BaseDir provided, make sure it's an absolute path
		abspath, err := filepath.Abs(config.BaseDir)
		if err != nil {
			return nil, errors.Wrap(err, `failed to normalize config.BaseDir`)
		}
		config.BaseDir = abspath
	} else {
		preserve, err := strconv.ParseBool(os.Getenv("TEST_MYSQLD_PRESERVE"))
		if err != nil {
			preserve = false // just to make sure
		}

		tempdir, err := ioutil.TempDir("", "mysqltest")
		if err != nil {
			return nil, errors.Wrap(err, `failed to create temporary directory`)
		}

		config.BaseDir = tempdir

		if !preserve {
			guards = append(guards, func() {
				os.RemoveAll(config.BaseDir)
			})
		}
	}

	fi, err := os.Stat(config.BaseDir)
	if err != nil && fi.Mode()&os.ModeSymlink == os.ModeSymlink {
		resolved, err := os.Readlink(config.BaseDir)
		if err != nil {
			return nil, errors.Wrap(err, `failed to readlink for config.BaseDir`)
		}
		config.BaseDir = resolved
	}

	if config.TmpDir == "" {
		config.TmpDir = filepath.Join(config.BaseDir, "tmp")
	}

	if config.Socket == "" {
		config.Socket = filepath.Join(config.TmpDir, "mysql.sock")
	}

	if config.DataDir == "" {
		config.DataDir = filepath.Join(config.BaseDir, "var")
	}

	if !config.SkipNetworking {
		if config.BindAddress == "" {
			config.BindAddress = "127.0.0.1"
		}

		if config.Port <= 0 {
			p, err := tcputil.EmptyPort()
			if err != nil {
				return nil, errors.Wrap(err, `could not find a temporary port to bind to`)
			}
			config.Port = p
		}
	}

	if config.PidFile == "" {
		config.PidFile = filepath.Join(config.TmpDir, "mysqld.pid")
	}

	if config.Mysqld == "" {
		fullpath, err := lookMysqldPath()
		if err != nil {
			return nil, errors.Wrap(err, `could not find mysqld in pat`)
		}
		config.Mysqld = fullpath
	}

	// Detecting if the mysqld supports `--initialize-insecure` option or not from the
	// output of `mysqld --help --verbose`.
	// `mysql_install_db` command is obsoleted MySQL 5.7.6 or later and
	// `mysqld --initialize-insecure` should be used.
	out, err := exec.Command(config.Mysqld, "--help", "--verbose").Output()
	if err != nil {
		return nil, errors.Wrap(err, `failed to execute 'mysqld --help --verbose'`)
	}
	if !strings.Contains(string(out), "--initialize-insecure") && config.MysqlInstallDb == "" {
		fullpath, err := exec.LookPath("mysql_install_db")
		if err != nil {
			return nil, errors.Wrap(err, `could not find mysql_install_db in path`)
		}
		config.MysqlInstallDb = fullpath
	}

	mysqld := &TestMysqld{
		config,
		nil,
		filepath.Join(config.BaseDir, "etc", "my.cnf"),
		guards,
		"",
	}

	if config.AutoStart > 0 {
		if err := mysqld.AssertNotRunning(); err != nil {
			return nil, errors.Wrap(err, `could not detect mysqld to be running`)
		}

		if config.AutoStart > 1 {
			if err := mysqld.Setup(); err != nil {
				return nil, errors.Wrap(err, `failed to setup mysqld`)
			}
		}

		if err := mysqld.Start(); err != nil {
			return nil, errors.Wrap(err, `failed to start mysqld`)
		}
	}

	return mysqld, nil
}

// BaseDir returns the base dir for mysqld
func (m *TestMysqld) BaseDir() string {
	return m.Config.BaseDir
}

// Socket returns the unix socket location
func (m *TestMysqld) Socket() string {
	return m.Config.Socket
}

// AssertNotRunning returns nil if mysqld is not running
func (m *TestMysqld) AssertNotRunning() error {
	if pidfile := m.Config.PidFile; pidfile != "" {
		_, err := os.Stat(pidfile)
		if err == nil {
			return errors.Errorf("mysqld is already running (%s)", pidfile)
		}
		if !os.IsNotExist(err) {
			return errors.Wrap(err, `invalid error while checking for mysqld pid file`)
		}
	}
	return nil
}

// Setup sets up all the files and directories needed to start mysqld
func (m *TestMysqld) Setup() error {
	config := m.Config
	if err := os.MkdirAll(config.BaseDir, 0755); err != nil {
		return errors.Wrap(err, `failed to create config.BaseDir`)
	}

	for _, s := range []string{"etc", "var", "tmp"} {
		subdir := filepath.Join(config.BaseDir, s)
		if err := os.Mkdir(subdir, 0755); err != nil {
			return errors.Wrapf(err, `failed to create subdirectory %s`, subdir)
		}
	}

	// When using `mysql_install_db`, copy the data before setup db for quick bootstrap.
	// But `mysqld --initialize-insecure` doesn't work while the data dir exists,
	// so don't copy here and do after setup db.
	if config.MysqlInstallDb != "" && config.CopyDataFrom != "" {
		if err := Dircopy(config.CopyDataFrom, config.DataDir); err != nil {
			return errors.Wrap(err, `failed to copy data from config.CopyDataFrom`)
		}
	}

	// XXX We should probably check for return values here...
	var buf bytes.Buffer
	buf.WriteString("[mysqld]\n")
	fmt.Fprintf(&buf, "datadir=%s\n", config.DataDir)
	fmt.Fprintf(&buf, "pid-file=%s\n", config.PidFile)
	if config.SkipNetworking {
		buf.WriteString("skip-networking\n")
	} else {
		fmt.Fprintf(&buf, "port=%d\n", config.Port)
	}
	fmt.Fprintf(&buf, "socket=%s\n", config.Socket)
	fmt.Fprintf(&buf, "tmpdir=%s\n", config.TmpDir)

	file, err := os.OpenFile(m.DefaultsFile, os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		return errors.Wrap(err, `failed to create defaults file`)
	}
	file.Write(buf.Bytes())
	file.Sync()
	file.Close()

	vardir := filepath.Join(config.BaseDir, "var", "mysql")
	_, err = os.Stat(vardir)
	if err != nil && os.IsNotExist(err) {
		setupArgs := []string{fmt.Sprintf("--defaults-file=%s", m.DefaultsFile)}
		setupCmd := config.MysqlInstallDb
		if setupCmd != "" {
			// --basedir is the path to the MYSQL INSTALLATION, not our basedir
			fi, err := os.Lstat(config.MysqlInstallDb)
			if err != nil {
				return errors.Wrap(err, `failed to stat config.MysqlInstallDb`)
			}

			var mysqlBaseDir string
			if fi.Mode()&os.ModeSymlink == os.ModeSymlink {
				resolved, err := os.Readlink(config.MysqlInstallDb)
				if err != nil {
					return errors.Wrap(err, `failed to readlink config.MysqlInstallDb`)
				}

				if !filepath.IsAbs(resolved) {
					resolved, err = filepath.Abs(
						filepath.Join(
							filepath.Dir(config.MysqlInstallDb),
							resolved,
						),
					)
					if err != nil {
						return err
					}
				}

				mysqlBaseDir = resolved
			} else {
				mysqlBaseDir = config.MysqlInstallDb
			}

			mysqlBaseDir = filepath.Dir(filepath.Dir(mysqlBaseDir))
			setupArgs = append(setupArgs, fmt.Sprintf("--basedir=%s", mysqlBaseDir))
		} else {
			setupCmd = config.Mysqld
			setupArgs = append(setupArgs, "--initialize-insecure")
		}

		cmd := exec.Command(setupCmd, setupArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			cmdName := setupCmd + " " + strings.Join(setupArgs, " ")
			return fmt.Errorf("error: *** [%s] failed ***\n%s\n", cmdName, output)
		}
	}

	if config.MysqlInstallDb == "" && config.CopyDataFrom != "" {
		if err := Dircopy(config.CopyDataFrom, config.DataDir); err != nil {
			return err
		}
	}

	return nil
}

// Start starts the mysqld process
func (m *TestMysqld) Start() error {
	if err := m.AssertNotRunning(); err != nil {
		return err
	}

	config := m.Config
	logname := filepath.Join(config.TmpDir, "mysqld.log")
	file, err := os.OpenFile(logname, os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	m.LogFile = logname

	cmd := exec.Command(
		config.Mysqld,
		fmt.Sprintf("--defaults-file=%s", m.DefaultsFile),
		"--user=root",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	stdoutpipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderrpipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	go io.Copy(file, stdoutpipe)
	go io.Copy(file, stderrpipe)

	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "error: Failed to launch mysqld")
	}

	checktimeout := time.NewTimer(20 * time.Second)
	checktick := time.NewTicker(time.Second)
	defer checktimeout.Stop()
	defer checktick.Stop()

	for cmd.Process == nil {
		select {
		case <-checktimeout.C:
			if proc := cmd.Process; proc != nil {
				proc.Kill()
			}
			return errors.New("error: Failed to launch mysqld (timeout)")
		case <-checktick.C:
			// will force `for cmd.Process != nil` to be
			// evaluated by bailing out of this select
		}
	}

	// Wait until we can connect to the database
	conntimeout := time.NewTimer(30 * time.Second)
	conntick := time.NewTicker(time.Second)
	defer conntimeout.Stop()
	defer conntick.Stop()

	dsn := m.DSN(WithDbname("mysql"), WithUser("root"))
	for {
		select {
		case <-conntimeout.C:
			if proc := cmd.Process; proc != nil {
				proc.Kill()
			}
			return errors.New("error: timeout reached before we could connect to database")
		case <-conntick.C:
			db, err := sql.Open("mysql", dsn)
			if err != nil {
				continue
			}

			var id int
			row := db.QueryRow("SELECT 1")
			if err = row.Scan(&id); err != nil {
				continue
			}
			m.Command = cmd

			if config.CopyDataFrom == "" {
				// Check if we have a database named "test". if not, create one
				dsn := m.DSN(WithDbname("mysql"), WithUser("root"))
				db, err := sql.Open("mysql", dsn)
				if err != nil {
					return errors.Wrap(err, `failed to connect to database`)
				}

				if _, err := db.Exec("CREATE DATABASE IF NOT EXISTS test"); err != nil {
					return errors.Wrap(err, `failed to create database 'test'`)
				}
			}
			return nil
		}
	}
	return errors.New("error: Could not connect to database. Server failed to start?")
}

// ReadLog reads the output log file specified by LogFile and returns its content
func (m *TestMysqld) ReadLog() ([]byte, error) {
	filename := m.LogFile
	fi, err := os.Lstat(filename)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, fi.Size())
	_, err = io.ReadFull(file, buf)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// ConnectString returns the connect string `tcp(...)` or `unix(...)`
// This method is deprecated, and will be removed in a future version.
func (m *TestMysqld) ConnectString(port int) string {
	config := m.Config

	var address string

	if config.SkipNetworking {
		address = fmt.Sprintf("unix(%s)", config.Socket)
	} else {
		if port <= 0 {
			port = config.Port
		}
		address = fmt.Sprintf("tcp(%s:%d)", config.BindAddress, port)
	}
	return address
}

// Datasource is a utility function to format the DSN that can be passed
// to mysqld driver.
func Datasource(options ...DatasourceOption) string {
	host := "localhost"
	socket := ""
	dbname := "test"
	user := "root"
	pass := ""
	port := 3306
	proto := "tcp"
	q := url.Values{}

	for _, o := range options {
		name := o.Name()
		switch name {
		case "host":
			host = o.Value().(string)
		case "dbname":
			dbname = o.Value().(string)
		case "user":
			user = o.Value().(string)
		case "password":
			pass = o.Value().(string)
		case "proto":
			proto = o.Value().(string)
		case "socket":
			socket = o.Value().(string)
		case "port":
			port = o.Value().(int)
		case "parseTime":
			q.Add(name, fmt.Sprintf("%t", o.Value().(bool)))
		case "multiStatements":
			q.Add(name, fmt.Sprintf("%t", o.Value().(bool)))
		}
	}

	var address string
	switch proto {
	case "unix":
		address = fmt.Sprintf(`unix(%s)`, socket)
	default: // Ah, ignore cases where proto != "unix" and != "tcp"
		address = fmt.Sprintf(`tcp(%s:%d)`, host, port)
	}

	s := fmt.Sprintf(
		"%s:%s@%s/%s",
		user,
		pass,
		address,
		dbname,
	)

	if qs := q.Encode(); qs != "" {
		s = s + "?" + qs
	}
	return s
}

// DSN creates a datasource name string that is appropriate for
// connecting to the database instance started by TestMysqld.
//
// This method respects networking settings and host:port/socket
// settings, and provide sane defaults for those parameters.
// If you want to forcefully override them, you still can do so
// by providing explicit DatasourceOption values
func (m *TestMysqld) DSN(options ...DatasourceOption) string {
	var hasSocket bool
	var hasHost bool
	var hasPort bool
	var hasProto bool
	var proto string
	for _, o := range options {
		switch o.Name() {
		case "proto":
			hasProto = true
			proto = o.Value().(string)
		case "socket":
			hasSocket = true
		case "host":
			hasHost = true
		case "port":
			hasPort = true
		}
	}

	if !hasProto {
		if m.Config.SkipNetworking {
			proto = "unix"
		} else {
			proto = "tcp"
		}
		options = append(options, WithProto(proto))
	}

	if proto == "unix" {
		if !hasSocket {
			options = append(options, WithSocket(m.Config.Socket))
		}
	} else {

		if !hasHost {
			options = append(options, WithHost(m.Config.BindAddress))
		}

		if !hasPort {
			options = append(options, WithPort(m.Config.Port))
		}
	}

	return Datasource(options...)
}

// Datasource is a DEPRECATED method to create a datasource string
// that can be passed to sql.Open(). Please consider using `DSN` instead
func (m *TestMysqld) Datasource(dbname string, user string, pass string, port int, options ...DatasourceOption) string {
	if user != "" {
		options = append(options, WithUser(user))
	}
	if dbname != "" {
		options = append(options, WithDbname(dbname))
	}
	if pass != "" {
		options = append(options, WithPassword(pass))
	}
	if port != 0 {
		options = append(options, WithPort(port))
	}

	return m.DSN(options...)
}

// Stop explicitly stops the execution of mysqld
func (m *TestMysqld) Stop() {
	if cmd := m.Command; cmd != nil {
		if process := cmd.Process; process != nil {
			process.Kill()
		}
	}

	// Run any guards that are registered
	for _, g := range m.Guards {
		g()
	}
}

// Dircopy recursively copies directories and files
func Dircopy(from string, to string) error {
	return filepath.Walk(from, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relpath, err := filepath.Rel(from, path)
		if relpath == "." {
			return nil
		}

		destpath := filepath.Join(to, relpath)

		if info.IsDir() {
			return os.Mkdir(destpath, info.Mode())
		}

		var src, dest *os.File
		src, err = os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()

		flags := os.O_WRONLY | os.O_CREATE
		dest, err = os.OpenFile(destpath, flags, info.Mode())
		if err != nil {
			return err
		}
		defer dest.Close()

		_, err = io.Copy(dest, src)
		if err != nil {
			return err
		}

		return nil
	})
}

var MysqlSearchPaths = []string{
	".",
	filepath.FromSlash("/usr/local/mysql/bin"),
}
var MysqldSearchDirs = []string{
	"bin", "libexec", "sbin",
}

// Find executable path in search paths under the base directory
func lookExecutablePath(name, base string, search []string) (string, error) {
	err := errors.New("error: No search path")
	for _, dir := range search {
		fullpath, err := exec.LookPath(filepath.Join(base, dir, name))
		if err == nil {
			return fullpath, nil
		}
	}
	return "", err
}

// Find mysqld executable path
func lookMysqldPath() (string, error) {
	const mysqld = "mysqld"
	fullpath, err := exec.LookPath(mysqld)
	if err == nil {
		return fullpath, nil
	}

	// Let's guess from mysql binary path

	mysqlPath, err := lookExecutablePath("mysql", "", MysqlSearchPaths)
	if err != nil { // no mysql binary; give up
		return "", err
	}

	// Strip "/bin/mysql" part
	mysqlBin := filepath.FromSlash("/bin/mysql")
	if !strings.HasSuffix(mysqlPath, mysqlBin) {
		return "", errors.New("error: Unsupported mysql path")
	}
	base := mysqlPath[:len(mysqlPath)-len(mysqlBin)]

	return lookExecutablePath(mysqld, base, MysqldSearchDirs)
}
