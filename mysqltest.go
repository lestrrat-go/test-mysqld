package mysqltest

import (
  "database/sql"
  "errors"
  "fmt"
  "io"
  "io/ioutil"
  "os"
  "os/exec"
  "path/filepath"
  "strconv"
  "syscall"
  "time"
  "github.com/lestrrat/go-tcputil"
  _ "github.com/go-sql-driver/mysql"
)

type MysqldConfig struct {
  BaseDir         string
  BindAddress     string
  CopyDataFrom    string
  DataDir         string
  PidFile         string
  Port            int
  SkipNetworking  bool
  Socket          string
  TmpDir          string

  AutoStart       int
  MysqlInstallDb  string
  Mysqld          string
}

type TestMysqld struct {
  Config        *MysqldConfig
  Command       *exec.Cmd
  DefaultsFile  string
  Guards        []func()
  LogFile       string
}

func NewConfig() (*MysqldConfig) {
  return &MysqldConfig {
    AutoStart: 2,
    SkipNetworking: true,
  }
}

func NewMysqld(config *MysqldConfig) (*TestMysqld, error) {
  guards := []func() {}

  if config == nil {
    config = NewConfig()
  }

  if config.BaseDir != "" {
    // BaseDir provided, make sure it's an absolute path
    abspath, err := filepath.Abs(config.BaseDir)
    if err != nil {
      return nil, err
    }
    config.BaseDir = abspath
  } else {
    preserve, err := strconv.ParseBool(os.Getenv("TEST_MYSQLD_PRESERVE"))
    if err != nil {
      preserve = false // just to make sure
    }

    tempdir, err := ioutil.TempDir("", "mysqltest")
    if err != nil {
      return nil, errors.New(
        fmt.Sprintf("Failed to create temporary directory: %s", err),
      )
    }

    config.BaseDir = tempdir

    if ! preserve {
      guards = append(guards, func() {
        os.RemoveAll(config.BaseDir)
      })
    }
  }

  fi, err := os.Stat(config.BaseDir)
  if err != nil && fi.Mode() & os.ModeSymlink == os.ModeSymlink{
    resolved, err := os.Readlink(config.BaseDir)
    if err != nil {
      return nil, err
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

  if ! config.SkipNetworking {
    if config.BindAddress == "" {
      config.BindAddress = "127.0.0.1"
    }

    if config.Port <= 0 {
      p, err := tcputil.EmptyPort()
      if err != nil {
        return nil, errors.New("Could not find a port to bind to")
      }
      config.Port = p
    }
  }

  if config.PidFile == "" {
    config.PidFile = filepath.Join(config.TmpDir, "mysqld.pid")
  }

  if config.MysqlInstallDb == "" {
    fullpath, err := exec.LookPath("mysql_install_db")
    if err != nil {
      return nil, errors.New(
        fmt.Sprintf("Could not find mysql_install_db: %s", err),
      )
    }
    config.MysqlInstallDb = fullpath
  }

  if config.Mysqld == "" {
    fullpath, err := exec.LookPath("mysqld")
    if err != nil {
      return nil, errors.New(
        fmt.Sprintf("Could not find mysqld: %s", err),
      )
    }
    config.Mysqld = fullpath
  }

  mysqld := &TestMysqld {
    config,
    nil,
    filepath.Join(config.BaseDir, "etc", "my.cnf"),
    guards,
    "",
  }

  if config.AutoStart > 0 {
    if err := mysqld.AssertNotRunning(); err != nil {
      return nil, err
    }

    if config.AutoStart > 1 {
      if err := mysqld.Setup(); err != nil {
        return nil, err
      }
    }

    if err := mysqld.Start(); err != nil {
      return nil, err
    }
  }

  return mysqld, nil
}

func (self *TestMysqld) BaseDir() string {
  return self.Config.BaseDir
}

func (self *TestMysqld) Socket() string {
  return self.Config.Socket
}

func (self *TestMysqld) AssertNotRunning() error {
  if pidfile := self.Config.PidFile; pidfile != "" {
    _, err := os.Stat(pidfile)
    if err == nil {
      return errors.New(
        fmt.Sprintf("mysqld is already running (%s)", pidfile),
      )
    } else {
      if ! os.IsNotExist(err) {
        return err
      }
    }
  }
  return nil
}

func (self *TestMysqld) Setup() error {
  config := self.Config
  if err := os.MkdirAll(config.BaseDir, 0755); err != nil {
    return err
  }

  for _, s := range []string { "etc", "var", "tmp" } {
    subdir := filepath.Join(config.BaseDir, s)
    if err := os.Mkdir(subdir, 0755); err != nil {
      return err
    }
  }

  if config.CopyDataFrom != "" {
    panic("Unimplemented!")
//    filepath.Walk(config.CopyDataFrom, func(path string, info os.FileInfo, err error) error {
//      relpath := filepath.Rel(config.CopyDataFrom, path)
//      dest    := filepath.Join(config.DataDir, relpath)
//    })
  }

  file, err := os.OpenFile(self.DefaultsFile, os.O_CREATE|os.O_WRONLY, 0755)
  if err != nil {
    return err
  }

  // XXX We should probably check for return values here...
  fmt.Fprint(file, "[mysqld]\n")
  fmt.Fprintf(file, "datadir=%s\n", config.DataDir)
  fmt.Fprintf(file, "pid-file=%s\n", config.PidFile)
  if config.SkipNetworking {
    fmt.Fprint(file, "skip-networking\n")
  } else {
    fmt.Fprintf(file, "port=%d\n", config.Port)
  }
  fmt.Fprintf(file, "socket=%s\n", config.Socket)
  fmt.Fprintf(file, "tmpdir=%s\n", config.TmpDir)

  file.Sync()
  file.Close()

  vardir := filepath.Join(config.BaseDir, "var", "mysql")
  _, err  = os.Stat(vardir)
  if err != nil && os.IsNotExist(err) {
    // --basedir is the path to the MYSQL INSTALLATION, not our basedir
    fi, err := os.Lstat(config.MysqlInstallDb)
    if err != nil {
      return err
    }

    var mysqlBaseDir string
    if fi.Mode() & os.ModeSymlink == os.ModeSymlink {
      resolved, err := os.Readlink(config.MysqlInstallDb)
      if err != nil {
        return err
      }

      if ! filepath.IsAbs(resolved) {
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

    cmd := exec.Command(
      config.MysqlInstallDb,
      fmt.Sprintf("--defaults-file=%s", self.DefaultsFile),
      fmt.Sprintf("--basedir=%s", mysqlBaseDir),
    )
    output, err := cmd.CombinedOutput()
    if err != nil {
      return errors.New(
        fmt.Sprintf("*** mysql_install_db failed ***\n%s\n", output),
      )
    }
  }

  return nil
}

func (self *TestMysqld) Start() error {
  if err := self.AssertNotRunning(); err != nil {
    return err
  }

  config := self.Config
  logname := filepath.Join(config.TmpDir, "mysqld.log")
  file, err := os.OpenFile(logname, os.O_CREATE|os.O_WRONLY, 0755)
  if err != nil {
    return err
  }
  self.LogFile = logname

  cmd := exec.Command(
    config.Mysqld,
    fmt.Sprintf("--defaults-file=%s", self.DefaultsFile),
    "--user=root",
  )
  cmd.SysProcAttr = &syscall.SysProcAttr {
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

  self.Command = cmd

  go io.Copy(file, stdoutpipe)
  go io.Copy(file, stderrpipe)

  c := make(chan bool)
  go func() {
    cmd.Run()
    c<-true
  }()

  for {
    if cmd.Process != nil {
      if _, err = os.FindProcess(cmd.Process.Pid); err == nil {
        break
      }
    }

    select {
    case <-c:
      // Fuck, we exited
      return errors.New("Failed to launch mysqld")
    default:
      time.Sleep(100 * time.Millisecond)
    }
  }

  // Wait until we can connect to the database
  timeout := time.Now().Add(30 * time.Second)
  var db *sql.DB
  for time.Now().Before(timeout) {
    dsn := self.Datasource("mysql", "root", "", 0)
    db, err = sql.Open("mysql", dsn)
    if err == nil {
      var id int
      row := db.QueryRow("SELECT 1")
      err = row.Scan(&id)
      if err == nil {
        break
      }
    }
    time.Sleep(1 * time.Second)
  }

  if db == nil {
    return errors.New("Could not connect to database. Server failed to start?")
  }

  return nil
}

func (self *TestMysqld) ReadLog() ([]byte, error) {
  filename := self.LogFile
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

func (self *TestMysqld) ConnectString (port int) string {
  config := self.Config

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

// mysqld.Datasource("test", "user", "pass", 0)
// mysqld.Datasource("test", "user", "pass", 3306)
func (self *TestMysqld) Datasource (dbname string, user string, pass string, port int) string {
  address := self.ConnectString(port)

  if user == "" {
    user = "root"
  }

  if dbname == "" {
    dbname = "test"
  }

  return fmt.Sprintf(
    "%s:%s@%s/%s",
    user,
    pass,
    address,
    dbname,
  )
}

func (self *TestMysqld) Stop() {
  if cmd := self.Command; cmd != nil {
    if process := cmd.Process; process != nil {
      process.Kill()
    }
  }

  // Run any guards that are registered
  for _, g := range self.Guards {
    g()
  }
}