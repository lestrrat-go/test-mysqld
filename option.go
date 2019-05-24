package mysqltest

type optionWithValue struct {
	name  string
	value interface{}
}

func (o *optionWithValue) Name() string {
	return o.name
}

func (o *optionWithValue) Value() interface{} {
	return o.value
}

// WithProto specifies the connection protocol ("unix" or "tcp")
func WithProto(s string) DatasourceOption {
	return &optionWithValue{name: "proto", value: s}
}

// WithProto specifies the path to the unix socket
// This is only respected if connection protocol is "unix"
func WithSocket(s string) DatasourceOption {
	return &optionWithValue{name: "socket", value: s}
}

// WithHost specifies the host name to connect.
// This is only respected if connection protocol is "tcp"
func WithHost(s string) DatasourceOption {
	return &optionWithValue{name: "host", value: s}
}

// WithDbname specifies the database name to connect.
func WithDbname(s string) DatasourceOption {
	return &optionWithValue{name: "dbname", value: s}
}

// WithUser specifies the user name to use when authenticating.
func WithUser(s string) DatasourceOption {
	return &optionWithValue{name: "user", value: s}
}

// WithPassword specifies the password to use when authenticating.
func WithPassword(s string) DatasourceOption {
	return &optionWithValue{name: "password", value: s}
}

// WithPort specifies the port number to connect.
// This is only respected if connection protocol is "tcp"
func WithPort(p int) DatasourceOption {
	return &optionWithValue{name: "port", value: p}
}

// WithParseTime specifies whethere the `parseTime` parameter should
// be appended to the DSN
func WithParseTime(t bool) DatasourceOption {
	return &optionWithValue{name: "parseTime", value: t}
}

// WithMultiStatements specifies whethere the `multiStatements` parameter should
// be appended to the DSN
func WithMultiStatements(t bool) DatasourceOption {
	return &optionWithValue{name: "multiStatements", value: t}
}
