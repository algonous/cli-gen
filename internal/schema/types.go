package schema

type CliFile struct {
	SchemaVersion string `yaml:"schema_version"`
	Cli           CliDef `yaml:"cli"`
}

type CliDef struct {
	Name        string     `yaml:"name"`
	Description string     `yaml:"description"`
	Runtime     RuntimeDef `yaml:"runtime"`
}

type RuntimeDef struct {
	Env     []EnvEntry `yaml:"env"`
	BaseURL BaseURLDef `yaml:"base_url"`
	Auth    *AuthDef   `yaml:"auth"`
}

type EnvEntry struct {
	Name     string `yaml:"name"`
	Required bool   `yaml:"required"`
	Secret   bool   `yaml:"secret"`
}

type BaseURLDef struct {
	FromEnv string `yaml:"from_env"`
}

type AuthDef struct {
	Header   string `yaml:"header"`
	Template string `yaml:"template"`
}

type ActionFile struct {
	Name          string         `yaml:"name"`
	Impl          string         `yaml:"impl"`
	Description   string         `yaml:"description"`
	References    []string       `yaml:"references"`
	Request       RequestDef     `yaml:"request"`
	Args          []ArgDef       `yaml:"args"`
	Response      ResponseDef    `yaml:"response"`
	ResponseHints *ResponseHints `yaml:"response_hints"`
}

type RequestDef struct {
	Method  string       `yaml:"method"`
	Path    string       `yaml:"path"`
	Query   []QueryParam `yaml:"query"`
	Headers []Header     `yaml:"headers"`
	Body    *BodyDef     `yaml:"body"`
}

type QueryParam struct {
	Name        string `yaml:"name"`
	Value       any    `yaml:"value"`
	ArrayFormat string `yaml:"array_format"`
}

type Header struct {
	Name  string `yaml:"name"`
	Value any    `yaml:"value"`
}

type BodyDef struct {
	Mode     string `yaml:"mode"`
	Arg      string `yaml:"arg"`
	Template any    `yaml:"template"`
}

type ArgDef struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Required bool   `yaml:"required"`
	Help     string `yaml:"help"`
}

type ResponseDef struct {
	SuccessStatus []int `yaml:"success_status"`
}

type ResponseHints struct {
	Fields []HintField `yaml:"fields"`
}

type HintField struct {
	Path string `yaml:"path"`
	Hint string `yaml:"hint"`
}

type SchemaSet struct {
	CLI     *CliFile
	Actions []*ActionFile
}

type ValidationError struct {
	File    string
	Field   string
	Message string
}
