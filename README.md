go-test-mysqld
==============

Create real MySQL server instance for testing

```go
import (
    "database/sql"
    "log"
    "mysqltest"
)

mysqld, err := mysqltest.NewMysqld(&MysqldConfig {
    SkipNetworking: true
})
if err != nil {
   log.Fatalf("Failed to start mysqld: %s", err)
}
defer mysqld.Stop()

db, err := sql.Open("mysql", mysqld.Datasource("test", "", "", 0))
// Now use db, which is connected to a mysql db
```

`go-test-mysqld` is a port of [Test::mysqld](https://metacpan.org/release/Test-mysqld)
