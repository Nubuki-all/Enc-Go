package argparser

import (
	"bytes"
	"strconv"

	"github.com/spf13/pflag"
)

// ParseResult holds the outcome of parsing.
type ParseResult struct {
	Values      map[string]interface{} // parsed flag values keyed by long-name
	UnknownArgs []string               // remaining (positional / unknown) args
	Output      string                 // captured help / usage / error text
	Err         error                  // parse error (including pflag.ErrHelp)
}

// ParseFlags parses tokens according to provided flag definitions.
// - cmdName: name to give the FlagSet (used in usage output).
// - helpMsg: string to print when user requests --help / -h (will be captured).
// - tokens: the tokenized args (use your SplitQuotedFields helper).
// - defs: list of flag definitions each as []string.
//
// Definition slice format (indices optional after 0):
//
//	[0] long name (required) e.g. "name"
//	[1] short (optional) e.g. "n" (single char without dash)
//	[2] type (optional) "string"|"bool"|"int"  (default "string")
//	[3] default value (optional) as string
//	[4] usage (optional) description
//
// Example:
//
//	ParseFlags("mycmd", "mycmd usage...", tokens,
//	  []string{"name", "n", "string", "unknown", "user name"},
//	  []string{"force", "f", "bool", "false", "force action"},
//	  []string{"count", "c", "int", "1", "how many"},
//	)
func ParseFlags(cmdName, helpMsg string, tokens []string, defs ...[]string) ParseResult {
	fs := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)

	// Capture help/errors into buffer instead of printing to stderr
	var buf bytes.Buffer
	fs.SetOutput(&buf)

	// Custom usage to output provided helpMsg
	fs.Usage = func() {
		buf.WriteString(helpMsg)
		//if len(helpMsg) == 0 || helpMsg[len(helpMsg)-1] != '\n' {
			//buf.WriteByte('\n')
		//}
	}

	// Keep registration order so we can fetch typed values later
	type rec struct {
		name string
		typ  string
	}
	recs := make([]rec, 0, len(defs))

	// Register flags
	for _, d := range defs {
		if len(d) == 0 {
			continue
		}
		name := d[0]
		short := ""
		typ := "string"
		defVal := ""
		usage := ""

		if len(d) > 1 {
			short = d[1]
		}
		if len(d) > 2 && d[2] != "" {
			typ = d[2]
		}
		if len(d) > 3 {
			defVal = d[3]
		}
		if len(d) > 4 {
			usage = d[4]
		}

		switch typ {
		case "bool":
			defBool := false
			if defVal != "" {
				if v, err := strconv.ParseBool(defVal); err == nil {
					defBool = v
				}
			}
			if short != "" {
				fs.BoolP(name, short, defBool, usage)
			} else {
				fs.Bool(name, defBool, usage)
			}
		case "int":
			defInt := 0
			if defVal != "" {
				if v, err := strconv.Atoi(defVal); err == nil {
					defInt = v
				}
			}
			if short != "" {
				fs.IntP(name, short, defInt, usage)
			} else {
				fs.Int(name, defInt, usage)
			}
		default: // string
			if short != "" {
				fs.StringP(name, short, defVal, usage)
			} else {
				fs.String(name, defVal, usage)
			}
		}

		recs = append(recs, rec{name: name, typ: typ})
	}

	// Parse tokens
	err := fs.Parse(tokens)

	// Collect typed values
	values := make(map[string]interface{}, len(recs))
	for _, r := range recs {
		switch r.typ {
		case "bool":
			v, _ := fs.GetBool(r.name)
			values[r.name] = v
		case "int":
			v, _ := fs.GetInt(r.name)
			values[r.name] = v
		default:
			v, _ := fs.GetString(r.name)
			values[r.name] = v
		}
	}

	// Remaining args (positional / unknown)
	unknown := fs.Args()

	return ParseResult{
		Values:      values,
		UnknownArgs: unknown,
		Output:      buf.String(),
		Err:         err,
	}
}

func CreateDBoolFlag(name, sName, helpMsg string) []string {
	return []string{name, sName, "bool", "false", helpMsg}
}
