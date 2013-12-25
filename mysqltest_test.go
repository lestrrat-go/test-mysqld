package mysqltest

import (
  "database/sql"
  "fmt"
  "strings"
  "testing"
  "time"
  _ "github.com/go-sql-driver/mysql"
)

func TestBasic (t *testing.T) {
  mysqld, err := NewMysqld(&MysqldConfig {
    AutoStart: 2,
    SkipNetworking: true,
  })

  if err != nil {
    t.Errorf("Failed to start mysqld: %s", err)
  }
  defer mysqld.Stop()

  dsn     := mysqld.Datasource("test", "", "", 0)
  wantdsn := fmt.Sprintf(
    "root:@unix(%s)/test",
    mysqld.Socket(),
  )

  if dsn != wantdsn {
    t.Errorf("DSN does not match expected (got '%s', want '%s')", dsn, wantdsn)
  }

  _, err = sql.Open("mysql", dsn)
  if err != nil {
    t.Errorf("Failed to connect to database: %s", err)
  }

  // Got to wait for a bit till the log gets anything in it
  time.Sleep(2 * time.Second)

  buf, err := mysqld.ReadLog()
  if err != nil {
    t.Errorf("Failed to read log: %s", err)
  }
  if strings.Index(string(buf), "ready for connections") < 0 {
    t.Errorf("Could not find 'ready for connections' in log: %s", buf)
  }

}