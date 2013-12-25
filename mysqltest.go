package mysqltest

import (
  "errors"
  "fmt"
  "io"
  "io/ioutil"
  "os"
  "os/exec"
  "path/filepath"
  "strconv"
  "time"
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
      config.Port = 3306
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

  stdout, err := cmd.StdoutPipe()
  if err != nil {
    return err
  }
  stderr, err := cmd.StderrPipe()
  if err != nil {
    return err
  }

  cmd.Start()
  self.Command = cmd

  out_c := make(chan bool, 1)
  err_c := make(chan bool, 1)
  list := []struct {
    Name string
    Dest io.Reader
    CloseChan chan bool
  } {
    { "stdout", stdout, out_c },
    { "stderr", stderr, err_c },
  }

  for _, x := range list {
    go func (name string, in io.Reader, c chan bool) {
      for loop := true; loop; {
        _, err := io.Copy(file, in)
        if err != nil {
          fmt.Fprintf(os.Stderr, "%s pipe error = %s\n", name, err)
        }

        select {
        case <-c:
          loop = false
          break
        }
      }
    }(x.Name, x.Dest, x.CloseChan)
  }

  c := make(chan bool, 1)
  go func() {
    defer func () { c     <- true }()
    defer func () { out_c <- true }()
    defer func () { err_c <- true }()
    cmd.Wait()
    fmt.Fprintf(os.Stderr, "mysqld exiting\n")
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

  // Create 'test' database

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
   , err := io.ReadFull(file, buf)
  if err != nil {
    return nil, err
  }
  return buf, nil
}

// mysqld.Datasource("test", "user", "pass", 0)
// mysqld.Datasource("test", "user", "pass", 3306)
func (self *TestMysqld) Datasource (dbname string, user string, pass string, port int) string {
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