package generator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"text/template"

	"github.com/go-openapi/analysis"
	"github.com/go-openapi/loads"
	"github.com/go-openapi/spec"
	"github.com/go-openapi/swag"
)

//go:generate go-bindata -mode 420 -modtime 1482416923 -pkg=generator -ignore=.*\.sw? -ignore=.*\.md ./templates/...

const (
	// default generation targets structure
	defaultModelsTarget = "models"
	defaultServerName   = "swagger"
)

func init() {
	// all initializations for the generator package
	debugOptions()
	initLanguage()
	initTemplateRepo()
	initTypes()
}

// DefaultSectionOpts for a given opts, this is used when no config file is passed
// and uses the embedded templates when no local override can be found
func DefaultSectionOpts(gen *GenOpts) {
	sec := gen.Sections
	if len(sec.Models) == 0 {
		sec.Models = []TemplateOpts{
			{
				Name:     "definition",
				Source:   "asset:model",
				Target:   "{{ joinFilePath .Target (toFilePath .Package) }}",
				FileName: "{{ (snakize (pascalize (.Name))) }}.k",
			},
		}
	}
	gen.Sections = sec
}

// TemplateOpts allows for codegen customization
type TemplateOpts struct {
	Name       string `mapstructure:"name"`
	Source     string `mapstructure:"source"`
	Target     string `mapstructure:"target"`
	FileName   string `mapstructure:"file_name"`
	SkipExists bool   `mapstructure:"skip_exists"`
	SkipFormat bool   `mapstructure:"skip_format"`
}

// SectionOpts allows for specifying options to customize the templates used for generation
type SectionOpts struct {
	Application []TemplateOpts `mapstructure:"application"`
	Models      []TemplateOpts `mapstructure:"models"`
}

// GenOpts the options for the generator
type GenOpts struct {
	IncludeModel               bool
	IncludeValidator           bool
	IncludeParameters          bool
	IncludeResponses           bool
	IncludeURLBuilder          bool
	ExcludeSpec                bool
	DumpData                   bool
	ValidateSpec               bool
	FlattenOpts                *analysis.FlattenOpts
	defaultsEnsured            bool
	KeepOrder                  bool
	StrictAdditionalProperties bool
	AllowTemplateOverride      bool

	Spec              string
	ModelPackage      string
	Principal         string
	Target            string
	Sections          SectionOpts
	LanguageOpts      *LanguageOpts
	TypeMapping       map[string]string
	Imports           map[string]string
	TemplateDir       string
	Template          string
	Models            []string
	Tags              []string
	Name              string
	FlagStrategy      string
	CompatibilityMode string
	ExistingModels    string
	Copyright         string
	SkipTagPackages   bool
	MainPackage       string
	IgnoreOperations  bool
}

// CheckOpts carries out some global consistency checks on options.
func (g *GenOpts) CheckOpts() error {
	if g == nil {
		return errors.New("gen opts are required")
	}

	if !filepath.IsAbs(g.Target) {
		if _, err := filepath.Abs(g.Target); err != nil {
			return fmt.Errorf("could not locate target %s: %v", g.Target, err)
		}
	}

	if strings.HasPrefix(g.Spec, "http://") || strings.HasPrefix(g.Spec, "https://") {
		return nil
	}

	pth, err := findSwaggerSpec(g.Spec)
	if err != nil {
		return err
	}

	// ensure spec path is absolute
	g.Spec, err = filepath.Abs(pth)
	if err != nil {
		return fmt.Errorf("could not locate spec: %s", g.Spec)
	}

	return nil
}

// EnsureDefaults for these gen opts
func (g *GenOpts) EnsureDefaults() error {
	if g.defaultsEnsured {
		return nil
	}

	if g.LanguageOpts == nil {
		g.LanguageOpts = DefaultLanguageFunc()
	}

	DefaultSectionOpts(g)

	// set defaults for flattening options
	if g.FlattenOpts == nil {
		g.FlattenOpts = &analysis.FlattenOpts{
			Minimal:      true,
			Verbose:      true,
			RemoveUnused: false,
			Expand:       false,
		}
	}

	g.defaultsEnsured = true
	return nil
}

func (g *GenOpts) location(t *TemplateOpts, data interface{}) (string, string, error) {
	v := reflect.Indirect(reflect.ValueOf(data))
	fld := v.FieldByName("Name")
	var name string
	if fld.IsValid() {
		log.Println("name field", fld.String())
		name = fld.String()
	}

	fldpack := v.FieldByName("Package")
	pkg := ""
	if fldpack.IsValid() {
		log.Println("package field", fldpack.String())
		pkg = fldpack.String()
	}
	// concat schema pkg if exist
	dataPkg := v.FieldByName("Pkg")
	if dataPkg.IsValid() {
		log.Println("type pkg field", dataPkg.String())
		pkg += "." + dataPkg.String()
	}

	alias := v.FieldByName("Module")
	if alias.IsValid() && alias.String() != "" {
		log.Println("type pkg alias field", alias.String())
		name = alias.String()
	}

	var tags []string
	tagsF := v.FieldByName("Tags")
	if tagsF.IsValid() {
		tags = tagsF.Interface().([]string)
	}

	var useTags bool
	useTagsF := v.FieldByName("UseTags")
	if useTagsF.IsValid() {
		useTags = useTagsF.Interface().(bool)
	}

	funcMap := FuncMapFunc(g.LanguageOpts)

	pthTpl, err := template.New(t.Name + "-target").Funcs(funcMap).Parse(t.Target)
	if err != nil {
		return "", "", err
	}

	fNameTpl, err := template.New(t.Name + "-filename").Funcs(funcMap).Parse(t.FileName)
	if err != nil {
		return "", "", err
	}

	d := struct {
		Name, Package, ModelPackage, MainPackage, Target string
		Tags                                             []string
		UseTags                                          bool
		Context                                          interface{}
	}{
		Name:         name,
		Package:      pkg,
		ModelPackage: g.ModelPackage,
		MainPackage:  g.MainPackage,
		Target:       g.Target,
		Tags:         tags,
		UseTags:      useTags,
		Context:      data,
	}

	var pthBuf bytes.Buffer
	if e := pthTpl.Execute(&pthBuf, d); e != nil {
		return "", "", e
	}
	var fNameBuf bytes.Buffer
	if e := fNameTpl.Execute(&fNameBuf, d); e != nil {
		return "", "", e
	}
	return pthBuf.String(), fileName(fNameBuf.String()), nil
}

func (g *GenOpts) render(t *TemplateOpts, data interface{}) ([]byte, error) {
	var templ *template.Template

	if strings.HasPrefix(strings.ToLower(t.Source), "asset:") {
		tt, err := templates.Get(strings.TrimPrefix(t.Source, "asset:"))
		if err != nil {
			return nil, err
		}
		templ = tt
	}

	if templ == nil {
		// try to load from repository (and enable dependencies)
		name := swag.ToJSONName(strings.TrimSuffix(t.Source, ".gotmpl"))
		tt, err := templates.Get(name)
		if err == nil {
			templ = tt
		}
	}

	if templ == nil {
		// try to load template from disk, in TemplateDir if specified
		// (dependencies resolution is limited to preloaded assets)
		var templateFile string
		if g.TemplateDir != "" {
			templateFile = filepath.Join(g.TemplateDir, t.Source)
		} else {
			templateFile = t.Source
		}
		content, err := ioutil.ReadFile(templateFile)
		if err != nil {
			return nil, fmt.Errorf("error while opening %s template file: %v", templateFile, err)
		}
		tt, err := template.New(t.Source).Funcs(FuncMapFunc(g.LanguageOpts)).Parse(string(content))
		if err != nil {
			return nil, fmt.Errorf("template parsing failed on template %s: %v", t.Name, err)
		}
		templ = tt
	}

	if templ == nil {
		return nil, fmt.Errorf("template %q not found", t.Source)
	}

	var tBuf bytes.Buffer
	if err := templ.Execute(&tBuf, data); err != nil {
		return nil, fmt.Errorf("template execution failed for template %s: %v", t.Name, err)
	}
	log.Printf("executed template %s", t.Source)

	return tBuf.Bytes(), nil
}

// Render template and write generated source code
// generated code is reformatted ("linted"), which gives an
// additional level of checking. If this step fails, the generated
// code is still dumped, for template debugging purposes.
func (g *GenOpts) write(t *TemplateOpts, data interface{}) error {
	dir, fname, err := g.location(t, data)
	if err != nil {
		return fmt.Errorf("failed to resolve template location for template %s: %v", t.Name, err)
	}

	if t.SkipExists && fileExists(dir, fname) {
		debugLog("skipping generation of %s because it already exists and skip_exist directive is set for %s",
			filepath.Join(dir, fname), t.Name)
		return nil
	}

	log.Printf("creating generated file %q in %q as %s", fname, dir, t.Name)
	content, err := g.render(t, data)
	if err != nil {
		return fmt.Errorf("failed rendering template data for %s: %v", t.Name, err)
	}

	if dir != "" {
		_, exists := os.Stat(dir)
		if os.IsNotExist(exists) {
			debugLog("creating directory %q for \"%s\"", dir, t.Name)
			// Directory settings consistent with file privileges.
			// Environment's umask may alter this setup
			if e := os.MkdirAll(dir, 0755); e != nil {
				return e
			}
		}
	}

	// Conditionally format the code, unless the user wants to skip
	formatted := content
	var writeerr error

	if !t.SkipFormat {
		formatted, err = g.LanguageOpts.FormatContent(filepath.Join(dir, fname), content)
		if err != nil {
			log.Printf("source formatting failed on template-generated source (%q for %s). Check that your template produces valid code", filepath.Join(dir, fname), t.Name)
			writeerr = ioutil.WriteFile(filepath.Join(dir, fname), content, 0644)
			if writeerr != nil {
				return fmt.Errorf("failed to write (unformatted) file %q in %q: %v", fname, dir, writeerr)
			}
			log.Printf("unformatted generated source %q has been dumped for template debugging purposes. DO NOT build on this source!", fname)
			return fmt.Errorf("source formatting on generated source %q failed: %v", t.Name, err)
		}
	}

	writeerr = ioutil.WriteFile(filepath.Join(dir, fname), formatted, 0644)
	if writeerr != nil {
		return fmt.Errorf("failed to write file %q in %q: %v", fname, dir, writeerr)
	}
	return err
}

func fileName(in string) string {
	ext := filepath.Ext(in)
	return swag.ToFileName(strings.TrimSuffix(in, ext)) + ext
}

func (g *GenOpts) shouldRenderOperations() bool {
	return g.IncludeParameters || g.IncludeResponses
}

func (g *GenOpts) renderApplication(app *GenApp) error {
	log.Printf("rendering %d templates for application %s", len(g.Sections.Application), app.Name)
	for _, templ := range g.Sections.Application {
		if err := g.write(&templ, app); err != nil {
			return err
		}
	}
	return nil
}

func (g *GenOpts) renderDefinition(gg *GenDefinition) error {
	log.Printf("rendering %d templates for model %s", len(g.Sections.Models), gg.Name)
	for _, templ := range g.Sections.Models {
		if !g.IncludeModel {
			continue
		}
		if err := g.write(&templ, gg); err != nil {
			return err
		}
	}
	return nil
}

func (g *GenOpts) setTemplates() error {
	templates.LoadDefaults()

	if g.Template != "" {
		// set contrib templates
		if err := templates.LoadContrib(g.Template); err != nil {
			return err
		}
	}

	templates.SetAllowOverride(g.AllowTemplateOverride)

	if g.TemplateDir != "" {
		// set custom templates
		if err := templates.LoadDir(g.TemplateDir); err != nil {
			return err
		}
	}
	return nil
}

func fileExists(target, name string) bool {
	_, err := os.Stat(filepath.Join(target, name))
	return !os.IsNotExist(err)
}

func gatherModels(specDoc *loads.Document, modelNames []string) (map[string]spec.Schema, error) {
	models, mnc := make(map[string]spec.Schema), len(modelNames)
	defs := specDoc.Spec().Definitions

	if mnc > 0 {
		var unknownModels []string
		for _, m := range modelNames {
			_, ok := defs[m]
			if !ok {
				unknownModels = append(unknownModels, m)
			}
		}
		if len(unknownModels) != 0 {
			return nil, fmt.Errorf("unknown models: %s", strings.Join(unknownModels, ", "))
		}
	}
	for k, v := range defs {
		if mnc == 0 {
			models[k] = v
		}
		for _, nm := range modelNames {
			if k == nm {
				models[k] = v
			}
		}
	}
	return models, nil
}

// titleOrDefault infers a name for the app from the title of the spec
func titleOrDefault(specDoc *loads.Document, name, defaultName string) string {
	if strings.TrimSpace(name) == "" {
		if specDoc.Spec().Info != nil && strings.TrimSpace(specDoc.Spec().Info.Title) != "" {
			name = specDoc.Spec().Info.Title
		} else {
			name = defaultName
		}
	}
	return swag.ToGoName(name)
}

func mainNameOrDefault(specDoc *loads.Document, name, defaultName string) string {
	// _test won't do as main server name
	return strings.TrimSuffix(titleOrDefault(specDoc, name, defaultName), "Test")
}

func appNameOrDefault(specDoc *loads.Document, name, defaultName string) string {
	// _test_api, _api_test, _test, _api won't do as app names
	return strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(titleOrDefault(specDoc, name, defaultName), "Test"), "API"), "Test")
}

func trimBOM(in string) string {
	return strings.Trim(in, "\xef\xbb\xbf")
}

// gatherSecuritySchemes produces a sorted representation from a map of spec security schemes
func gatherSecuritySchemes(securitySchemes map[string]spec.SecurityScheme, appName, principal, receiver string) (security GenSecuritySchemes) {
	for scheme, req := range securitySchemes {
		isOAuth2 := strings.ToLower(req.Type) == "oauth2"
		var scopes []string
		if isOAuth2 {
			for k := range req.Scopes {
				scopes = append(scopes, k)
			}
		}
		sort.Strings(scopes)

		security = append(security, GenSecurityScheme{
			AppName:      appName,
			ID:           scheme,
			ReceiverName: receiver,
			Name:         req.Name,
			IsBasicAuth:  strings.ToLower(req.Type) == "basic",
			IsAPIKeyAuth: strings.ToLower(req.Type) == "apikey",
			IsOAuth2:     isOAuth2,
			Scopes:       scopes,
			Principal:    principal,
			Source:       req.In,
			// from original spec
			Description:      req.Description,
			Type:             strings.ToLower(req.Type),
			In:               req.In,
			Flow:             req.Flow,
			AuthorizationURL: req.AuthorizationURL,
			TokenURL:         req.TokenURL,
			Extensions:       req.Extensions,
		})
	}
	sort.Sort(security)
	return
}

// gatherExtraSchemas produces a sorted list of extra schemas.
//
// ExtraSchemas are inlined types rendered in the same model file.
func gatherExtraSchemas(extraMap map[string]GenSchema) (extras GenSchemaList) {
	var extraKeys []string
	for k := range extraMap {
		extraKeys = append(extraKeys, k)
	}
	sort.Strings(extraKeys)
	for _, k := range extraKeys {
		// figure out if top level validations are needed
		p := extraMap[k]
		extras = append(extras, p)
	}
	return
}

func sharedValidationsFromSchema(v spec.Schema, sg schemaGenContext) (sh sharedValidations) {
	sh = sharedValidations{
		Maximum:          v.Maximum,
		ExclusiveMaximum: v.ExclusiveMaximum,
		Minimum:          v.Minimum,
		ExclusiveMinimum: v.ExclusiveMinimum,
		MaxLength:        v.MaxLength,
		MinLength:        v.MinLength,
		Pattern:          v.Pattern,
		MaxItems:         v.MaxItems,
		MinItems:         v.MinItems,
		UniqueItems:      v.UniqueItems,
		MultipleOf:       v.MultipleOf,
		Enum:             v.Enum,
	}
	sh.pruneEnums(sg)
	return
}

func dumpData(data interface{}) error {
	bb, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, string(bb))
	return nil
}

func importAlias(pkg string) string {
	_, k := path.Split(pkg)
	return k
}
