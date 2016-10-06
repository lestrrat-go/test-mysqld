go-test-mysqld
==============

[![Build Status](https://travis-ci.org/lestrrat/go-test-mysqld.png?branch=master)](https://travis-ci.org/lestrrat/go-test-mysqld)

Create real MySQL server instance for testing

To install, simply issue a `go get`:

```
go get github.com/lestrrat/go-test-mysqld
```

By default importing `github.com/lestrrat/go-test-mysqld` will import package
`mysqltest`

```go
import (
    "database/sql"
    "log"
    "github.com/lestrrat/go-test-mysqld"
)

mysqld, err := mysqltest.NewMysqld(nil)
if err != nil {
   log.Fatalf("Failed to start mysqld: %s", err)
}
defer mysqld.Stop()

db, err := sql.Open("mysql", mysqld.Datasource("test", "", "", 0))
// Now use db, which is connected to a mysql db
```

`go-test-mysqld` is a port of [Test::mysqld](https://metacpan.org/release/Test-mysqld)

When you create a new struct via `NewMysqld()` a new mysqld instance is
automatically setup and launched. Don't forget to call `Stop()` on this
struct to stop the launched mysqld

If you want to customize the configuration, create a new config and set each
field on the struct:

```go

config := mysqltest.NewConfig()
config.SkipNetworking = false
config.Port = 13306

// Starts mysqld listening on port 13306
mysqld, _ := mysqltest.NewMysqld(config)
```
