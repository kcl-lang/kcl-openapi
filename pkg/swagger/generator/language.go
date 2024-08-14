// Copyright 2015 go-swagger maintainers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package generator

import (
	"encoding/json"
	"fmt"
	"log"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/go-openapi/swag"
)

var (
	// DefaultLanguageFunc defines the default generation language
	DefaultLanguageFunc func() *LanguageOpts
	validNameRegexp     = regexp.MustCompile(`\$?^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

const (
	RegexPkgPath = "regex"
)

func initLanguage() {
	DefaultLanguageFunc = KclLangOpts
}

// LanguageOpts to describe a language to the code generator
type LanguageOpts struct {
	ReservedWords    []string
	SystemModules    []string
	BaseImportFunc   func(string) string            `json:"-"`
	ImportsFunc      func(map[string]string) string `json:"-"`
	reservedWordsSet map[string]struct{}
	systemModuleSet  map[string]struct{}
	initialized      bool
	formatFunc       func(string, []byte) ([]byte, error)
	fileNameFunc     func(string) string // language specific source file naming rules
	dirNameFunc      func(string) string // language specific directory naming rules
}

// Init the language option
func (l *LanguageOpts) Init() {
	if l.initialized {
		return
	}
	l.initialized = true
	l.reservedWordsSet = make(map[string]struct{})
	l.systemModuleSet = make(map[string]struct{})
	for _, rw := range l.ReservedWords {
		l.reservedWordsSet[rw] = struct{}{}
	}
	for _, rw := range l.SystemModules {
		l.systemModuleSet[rw] = struct{}{}
	}
}

// MangleName makes sure a reserved word gets a safe name
func (l *LanguageOpts) MangleName(name, suffix string) string {
	if _, ok := l.reservedWordsSet[swag.ToFileName(name)]; !ok {
		return name
	}
	return strings.Join([]string{name, suffix}, "_")
}

// MangleVarName makes sure a reserved word gets a safe name
func (l *LanguageOpts) MangleVarName(name string) string {
	nm := swag.ToVarName(name)
	if _, ok := l.reservedWordsSet[nm]; !ok {
		return nm
	}
	return nm + "Var"
}

// MangleModelName adds "$" prefix to name if it is conflict with KCL keyword
func (l *LanguageOpts) MangleModelName(modelName string) string {
	lastDotIndex := strings.LastIndex(modelName, ".")
	shortName := modelName[lastDotIndex+1:]
	// Replace all the "-" to "_" in the model name
	if strings.Contains(shortName, "-") || strings.Contains(shortName, ".") {
		log.Printf("[WARN] the modelName %s contains symbols '-' or '.' which is forbidden in KCL. Will be replaced by '_'", shortName)
		modelName = modelName[:lastDotIndex+1] + strings.Replace(strings.Replace(shortName, "-", "_", -1), ".", "_", -1)
	}
	for _, kw := range l.ReservedWords {
		if modelName == kw {
			return fmt.Sprintf("$%s", modelName)
		}
	}
	return modelName
}

// ManglePropertyName adds "$" prefix to name if it is conflict with KCL keyword or adds quotes "
func (l *LanguageOpts) ManglePropertyName(name string) string {
	if !validNameRegexp.MatchString(name) {
		name = fmt.Sprintf(`"%s"`, name)
	}
	for _, kw := range l.ReservedWords {
		if name == kw {
			return fmt.Sprintf("$%s", name)
		}
	}
	return name
}

// MangleFileName makes sure a file name gets a safe name
func (l *LanguageOpts) MangleFileName(name string) string {
	if l.fileNameFunc != nil {
		return l.fileNameFunc(name)
	}
	return swag.ToFileName(name)
}

// ManglePackageName makes sure a package gets a safe name.
// In case of a file system path (e.g. name contains "/" or "\" on Windows), this return only the last element.
func (l *LanguageOpts) ManglePackageName(name, suffix string) string {
	if name == "" {
		return suffix
	}
	if l.dirNameFunc != nil {
		name = l.dirNameFunc(name)
	}
	pth := filepath.ToSlash(filepath.Clean(name)) // preserve path
	pkg := importAlias(pth)                       // drop path
	return l.MangleName(swag.ToFileName(prefixForName(pkg)+pkg), suffix)
}

// ManglePackagePath makes sure a full package path gets a safe name.
// Only the last part of the path is altered.
func (l *LanguageOpts) ManglePackagePath(name string, suffix string) string {
	if name == "" {
		return suffix
	}
	target := filepath.ToSlash(filepath.Clean(name)) // preserve path
	parts := strings.Split(target, "/")
	parts[len(parts)-1] = l.ManglePackageName(parts[len(parts)-1], suffix)
	return strings.Join(parts, "/")
}

func (l *LanguageOpts) ToKclValue(data interface{}) string {
	if data == nil {
		return "None"
	}
	value := reflect.ValueOf(data)
	switch value.Kind() {
	case reflect.Map:
		var mapContents []string
		iter := value.MapRange()
		for iter.Next() {
			mapContents = append(mapContents, fmt.Sprintf("%s: %s", l.ToKclValue(iter.Key()), l.ToKclValue(iter.Value())))
		}
		content := strings.Join(mapContents, ", ")
		return fmt.Sprintf("{%s}", content)
	case reflect.Slice:
		// if is a MapSlice
		if dataSlice, ok := data.(yaml.MapSlice); ok {
			var dictContents []string
			for _, v := range dataSlice {
				k := v.Key
				v := v.Value
				dictContents = append(dictContents, fmt.Sprintf("%s: %s", l.ToKclValue(k), l.ToKclValue(v)))
			}
			content := strings.Join(dictContents, ", ")
			return fmt.Sprintf("{%s}", content)
		}
		// if is a normal slice
		var sliceContents []string
		for i := 0; i < value.Len(); i++ {
			sliceContents = append(sliceContents, l.ToKclValue(value.Index(i).Interface()))
		}
		content := strings.Join(sliceContents, ", ")
		return fmt.Sprintf("[%s]", content)
	case reflect.String:
		return fmt.Sprintf("\"%s\"", data)
	case reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64:
		return fmt.Sprintf("%v", data)
	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%v", data)
	case reflect.Bool:
		if data.(bool) {
			return "True"
		}
		return "False"
	default:
		// Reflect value
		if dataValue, ok := data.(reflect.Value); ok {
			return l.ToKclValue(dataValue.Interface())
		} else if dataSlice, ok := data.(yaml.MapSlice); ok {
			// If is a MapSlice
			var dictContents []string
			for _, v := range dataSlice {
				k := v.Key
				v := v.Value
				dictContents = append(dictContents, fmt.Sprintf("%s: %s", l.ToKclValue(k), l.ToKclValue(v)))
			}
			content := strings.Join(dictContents, ", ")
			return fmt.Sprintf("{%s}", content)
		} else {
			// User defined struct
			valueString, err := ToKCLValueString(data)
			if err != nil {
				log.Fatal(err)
				return "None"
			}
			return valueString
		}
	}
}

// FormatContent formats a file with a language specific formatter
func (l *LanguageOpts) FormatContent(name string, content []byte) ([]byte, error) {
	if l.formatFunc != nil {
		return l.formatFunc(name, content)
	}
	return content, nil
}

// NonEmptyValue checks if a value is non-empty
func (l *LanguageOpts) NonEmptyValue(data interface{}) bool {
	return data != nil
}

// baseImport figures out the base path to generate import statements
func (l *LanguageOpts) baseImport(tgt string) string {
	if l.BaseImportFunc != nil {
		return l.BaseImportFunc(tgt)
	}
	debugLog("base import func is nil")
	return ""
}

// KclLangOpts for rendering items as kcl code
func KclLangOpts() *LanguageOpts {
	var kclOtherReservedSuffixes = map[string]bool{
		"aix":       true,
		"android":   true,
		"darwin":    true,
		"dragonfly": true,
		"freebsd":   true,
		"hurd":      true,
		"illumos":   true,
		"js":        true,
		"linux":     true,
		"nacl":      true,
		"netbsd":    true,
		"openbsd":   true,
		"plan9":     true,
		"solaris":   true,
		"windows":   true,
		"zos":       true,

		// arch
		"386":         true,
		"amd64":       true,
		"amd64p32":    true,
		"arm":         true,
		"armbe":       true,
		"arm64":       true,
		"arm64be":     true,
		"mips":        true,
		"mipsle":      true,
		"mips64":      true,
		"mips64le":    true,
		"mips64p32":   true,
		"mips64p32le": true,
		"ppc":         true,
		"ppc64":       true,
		"ppc64le":     true,
		"riscv":       true,
		"riscv64":     true,
		"s390":        true,
		"s390x":       true,
		"sparc":       true,
		"sparc64":     true,
		"wasm":        true,

		// other reserved suffixes
		"test": true,
	}

	opts := new(LanguageOpts)
	opts.ReservedWords = []string{
		"import",
		"as",
		"rule",
		"schema",
		"mixin",
		"protocol",
		"relaxed",
		"check",
		"for",
		"assert",
		"if",
		"elif",
		"else",
		"or",
		"and",
		"not",
		"in",
		"is",
		"final",
		"lambda",
		"all",
		"filter",
		"map",
		"type",
	}
	opts.SystemModules = []string{
		"collection",
		"net",
		"manifests",
		"math",
		"datetime",
		"regex",
		"yaml",
		"json",
		"crypto",
		"base64",
		"units",
		"file",
	}

	opts.formatFunc = func(ffn string, content []byte) ([]byte, error) {
		// todo: support kcl code format
		return content, nil
	}

	opts.fileNameFunc = func(name string) string {
		// whenever a generated file name ends with a suffix
		// that is meaningful to go build, adds a "swagger"
		// suffix
		parts := strings.Split(swag.ToFileName(name), "_")
		if kclOtherReservedSuffixes[parts[len(parts)-1]] {
			// file name ending with a reserved arch or os name
			// are appended an innocuous suffix "swagger"
			parts = append(parts, "swagger")
		}
		return strings.Join(parts, "_")
	}

	opts.dirNameFunc = func(name string) string {
		// whenever a generated directory name is a special
		// golang directory, append an innocuous suffix
		switch name {
		case "vendor", "internal":
			return strings.Join([]string{name, "swagger"}, "_")
		}
		return name
	}

	opts.ImportsFunc = func(imports map[string]string) string {
		if len(imports) == 0 {
			return ""
		}
		result := make([]string, 0, len(imports))
		for k, v := range imports {
			_, name := path.Split(v)
			if name != k {
				result = append(result, fmt.Sprintf("\t%s %q", k, v))
			} else {
				result = append(result, fmt.Sprintf("\t%q", v))
			}
		}
		sort.Strings(result)
		return strings.Join(result, "\n")
	}

	opts.BaseImportFunc = func(tgt string) string {
		// todo
		return tgt
	}
	opts.Init()
	return opts
}

func ToKCLValueString(value interface{}) (string, error) {
	jsonString, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	// In KCL, `true`, `false` and `null` are denoted by `True`, `False` and `None`.
	result := strings.Replace(string(jsonString), ": true", ": True", -1)
	result = strings.Replace(result, ": false", ": False", -1)
	result = strings.Replace(result, ": null", ": None", -1)
	return result, nil
}
