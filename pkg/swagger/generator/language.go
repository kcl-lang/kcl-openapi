package generator

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-openapi/swag"
)

var (
	// DefaultLanguageFunc defines the default generation language
	DefaultLanguageFunc func() *LanguageOpts
)

func initLanguage() {
	DefaultLanguageFunc = KclLangOpts
}

// LanguageOpts to describe a language to the code generator
type LanguageOpts struct {
	ReservedWords        []string
	BaseImportFunc       func(string) string               `json:"-"`
	ImportsFunc          func(map[string]string) string    `json:"-"`
	ArrayInitializerFunc func(interface{}) (string, error) `json:"-"`
	reservedWordsSet     map[string]struct{}
	initialized          bool
	formatFunc           func(string, []byte) ([]byte, error)
	fileNameFunc         func(string) string // language specific source file naming rules
	dirNameFunc          func(string) string // language specific directory naming rules
}

// Init the language option
func (l *LanguageOpts) Init() {
	if l.initialized {
		return
	}
	l.initialized = true
	l.reservedWordsSet = make(map[string]struct{})
	for _, rw := range l.ReservedWords {
		l.reservedWordsSet[rw] = struct{}{}
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

// FormatContent formats a file with a language specific formatter
func (l *LanguageOpts) FormatContent(name string, content []byte) ([]byte, error) {
	if l.formatFunc != nil {
		return l.formatFunc(name, content)
	}
	return content, nil
}

// imports generate the code to import some external packages, possibly aliased
func (l *LanguageOpts) imports(imports map[string]string) string {
	if l.ImportsFunc != nil {
		return l.ImportsFunc(imports)
	}
	return ""
}

// arrayInitializer builds a litteral array
func (l *LanguageOpts) arrayInitializer(data interface{}) (string, error) {
	if l.ArrayInitializerFunc != nil {
		return l.ArrayInitializerFunc(data)
	}
	return "", nil
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
		"check", "False", "else", "import", "pass",
		"None", "in", "True", "is", "return",
		"and", "for", "def", "assert", "not",
		"elif", "if", "if", "or", "schema",
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

	opts.ArrayInitializerFunc = func(data interface{}) (string, error) {
		return "", nil
	}

	opts.BaseImportFunc = func(tgt string) string {
		// todo
		return tgt
	}
	opts.Init()
	return opts
}
