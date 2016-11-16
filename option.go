package mysqltest

type optionWithValue struct {
	name string
	value interface {}
}

func (o *optionWithValue) Name() string {
	return o.name
}

func (o *optionWithValue) Value() interface{} {
	return o.value
}

func WithParseTime(t bool) DatasourceOption {
	return &optionWithValue{name: "parseTime", value: t}
}
