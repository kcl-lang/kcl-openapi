package generator

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
)

var (
	// Debug when the env var DEBUG or SWAGGER_DEBUG is not empty
	// the generators will be very noisy about what they are doing
	Debug = os.Getenv("DEBUG") != "" || os.Getenv("SWAGGER_DEBUG") != ""
	// generatorLogger is a debug logger for this package
	generatorLogger *log.Logger
)

func debugOptions() {
	generatorLogger = log.New(os.Stdout, "generator:", log.LstdFlags)
}

// debugLog wraps log.Printf with a debug-specific logger
func debugLog(frmt string, args ...interface{}) {
	if Debug {
		_, file, pos, _ := runtime.Caller(1)
		generatorLogger.Printf("%s:%d: %s", filepath.Base(file), pos,
			fmt.Sprintf(frmt, args...))
	}
}

// debugLogAsJSON unmarshals its last arg as pretty JSON
func debugLogAsJSON(frmt string, args ...interface{}) {
	if Debug {
		var dfrmt string
		_, file, pos, _ := runtime.Caller(1)
		dargs := make([]interface{}, 0, len(args)+2)
		dargs = append(dargs, filepath.Base(file), pos)
		if len(args) > 0 {
			dfrmt = "%s:%d: " + frmt + "\n%s"
			bbb, _ := json.MarshalIndent(args[len(args)-1], "", " ")
			dargs = append(dargs, args[0:len(args)-1]...)
			dargs = append(dargs, string(bbb))
		} else {
			dfrmt = "%s:%d: " + frmt
		}
		generatorLogger.Printf(dfrmt, dargs...)
	}
}
