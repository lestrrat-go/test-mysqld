package mysqltest

import (
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"strings"
	"testing"
	"time"
)

func TestBasic(t *testing.T) {
	mysqld, err := NewMysqld(NewConfig())
	if err != nil {
		t.Errorf("Failed to start mysqld: %s", err)
	}
	defer mysqld.Stop()

	dsn := mysqld.Datasource("test", "", "", 0)
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

func TestCopyDataFrom(t *testing.T) {
	config := NewConfig()
	config.CopyDataFrom = "copy_data_from"

	mysqld, err := NewMysqld(config)
	if err != nil {
		t.Errorf("Failed to start mysqld: %s", err)
	}
	defer mysqld.Stop()

	db, err := sql.Open("mysql", mysqld.Datasource("test", "", "", 0))
	if err != nil {
		t.Errorf("Failed to connect to database: %s", err)
	}

	rows, err := db.Query("select id,str from test.hello order by id")
	if err != nil {
		t.Errorf("Failed to fetch data: %s", err)
	}

	var id int
	var str string

	rows.Next()
	rows.Scan(&id, &str)
	if id != 1 || str != "hello" {
		t.Errorf("Data do not match, got (id:%d str:%s)", id, str)
	}

	rows.Next()
	rows.Scan(&id, &str)
	if id != 2 || str != "ciao" {
		t.Errorf("Data do not match, got (id:%d str:%s)", id, str)
	}
}
