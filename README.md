test-mysqld
==============

Create real MySQL server instance for testing

[![Build Status](https://travis-ci.org/lestrrat-go/test-mysqld.png?branch=master)](https://travis-ci.org/lestrrat-go/test-mysqld)

[![GoDoc](https://godoc.org/github.com/lestrrat-go/test-mysqld?status.svg)](https://godoc.org/github.com/lestrrat-go/test-mysqld)

# DESCRIPTION

By default importing `github.com/lestrrat-go/test-mysqld` will import package
`mysqltest`

```go
import (
    "database/sql"
    "log"
    "github.com/lestrrat-go/test-mysqld"
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

# Generating DSN

DSN strings can be generated using the `DSN` method:

```go
// Use default
dsn := mysqld.DSN()

// Pass explicit parameters
dsn := mysqld.DSN(mysqltest.WithUser("foo"), mysqltest.WithPassword("passw0rd!"))

// Tell the mysql driver to parse time values
dsn := mysqld.DSN(mysqltest.WithParseTime(true))

// ...And pass the dsn to sql.Open
db, err := sql.Open("mysql", dsn)
```

Following is a list of possible parameters to `DSN`. I

| Option | Description | Default |
|:-------|:------------|:--------|
| mysqltest.WithProto(string)    | Specifies the protocol ("unix" or "tcp")                          | Depends on value of `config.SkipNetworking` |
| mysqltest.WithSocket(string)   | Specifies the path to the unix socket                             | value of `config.Socket` |
| mysqltest.WithHost(string)     | Specifies the hostname                                            | value of `config.BindAddress` |
| mysqltest.WithPort(int)        | Specifies the port number                                         | value of `config.Port` |
| mysqltest.WithUser(string)     | Specifies the username                                            | `"root"` |
| mysqltest.WithPassword(string) | Specifies the password                                            | `""` |
| mysqltest.WithDbname(string)   | Specifies the database name to connect                            | `"test"` |
| mysqltest.WithParseTime(bool)  | Specifies if mysql driver should parse time values to `time.Time` | `false` |
