package mysqltest

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
)

func TestOptions(t *testing.T) {
	mysqld, err := NewMysqld(NewConfig())
	if !assert.NoError(t, err, "NewMysqld should succeed") {
		return
	}
	dsn := mysqld.Datasource("mysql", "root", "localhost", 0, WithParseTime(true))
	if !assert.Regexp(t, "parseTime=true", dsn, "dsn matches expected") {
		return
	}
}

func TestBasic(t *testing.T) {
	mysqld, err := NewMysqld(NewConfig())
	if err != nil {
		t.Errorf("Failed to start mysqld: %s", err)
		return
	}
	defer mysqld.Stop()

	dsn := mysqld.Datasource("test", "", "", 0)
	wantdsn := fmt.Sprintf(
		"root:@unix(%s)/test",
		mysqld.Socket(),
	)

	if dsn != wantdsn {
		t.Errorf("DSN does not match expected (got '%s', want '%s')", dsn, wantdsn)
		return
	}

	_, err = sql.Open("mysql", dsn)
	if err != nil {
		t.Errorf("Failed to connect to database: %s", err)
		return
	}

	// Got to wait for a bit till the log gets anything in it
	time.Sleep(2 * time.Second)

	buf, err := mysqld.ReadLog()
	if err != nil {
		t.Errorf("Failed to read log: %s", err)
		return
	}
	if strings.Index(string(buf), "ready for connections") < 0 {
		t.Errorf("Could not find 'ready for connections' in log: %s", buf)
		return
	}

}

func TestCopyDataFrom(t *testing.T) {
	config := NewConfig()
	config.CopyDataFrom = "copy_data_from"

	mysqld, err := NewMysqld(config)
	if err != nil {
		t.Errorf("Failed to start mysqld: %s", err)
		return
	}
	defer mysqld.Stop()

	db, err := sql.Open("mysql", mysqld.Datasource("test", "", "", 0))
	if err != nil {
		t.Errorf("Failed to connect to database: %s", err)
		return
	}

	rows, err := db.Query("select id,str from test.hello order by id")
	if err != nil {
		t.Errorf("Failed to fetch data: %s", err)
		return
	}

	var id int
	var str string

	rows.Next()
	rows.Scan(&id, &str)
	if id != 1 || str != "hello" {
		t.Errorf("Data do not match, got (id:%d str:%s)", id, str)
		return
	}

	rows.Next()
	rows.Scan(&id, &str)
	if id != 2 || str != "ciao" {
		t.Errorf("Data do not match, got (id:%d str:%s)", id, str)
		return
	}
}

func TestDSN(t *testing.T) {
	mysqld, err := NewMysqld(nil)
	if err != nil {
		t.Errorf("Failed to start mysqld: %s", err)
		return
	}
	defer mysqld.Stop()

	dsn := mysqld.DSN()

	re := ":@unix\\(/.*mysql\\.sock\\)/"
	match, _ := regexp.MatchString(re, dsn)

	if !match {
		t.Errorf("DSN %s should match %s", dsn, re)
	}
}

func TestDatasource(t *testing.T) {
	mysqld, err := NewMysqld(nil)
	if err != nil {
		t.Errorf("Failed to start mysqld: %s", err)
		return
	}
	defer mysqld.Stop()

	dsn := mysqld.Datasource("", "", "", 0)

	re := "root:@unix\\(/.*mysql\\.sock\\)/test"
	match, _ := regexp.MatchString(re, dsn)

	if !match {
		t.Errorf("DSN %s should match %s", dsn, re)
	}
}
