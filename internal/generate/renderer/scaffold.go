// Package renderer generates the source files used by SpecGuardian conformance tests.
package renderer

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"go/format"
	"path/filepath"
	"sort"
	"text/template"

	"github.com/cockroachdb/errors"
	"github.com/mrasu/spec-guardian/internal/fizzbee"
)

//go:embed templates/*.tmpl
var templateFiles embed.FS

type templateName uint8

const (
	templateTypes templateName = iota
	templateConformance
	templateRole
	templateCommon
)

func (name templateName) path() string {
	switch name {
	case templateTypes:
		return "templates/types_test.go.tmpl"
	case templateConformance:
		return "templates/conformance_test.go.tmpl"
	case templateRole:
		return "templates/role_test.go.tmpl"
	case templateCommon:
		return "templates/common_test.go.tmpl"
	default:
		panic(fmt.Sprintf("unsupported template name %d", name))
	}
}

// Scaffold contains the source for every file managed by the generator.
type Scaffold struct {
	Common           []byte
	RoleCases        map[string][]byte
	RoleConformances map[string][]byte
	RoleScaffolds    map[string][]byte
	CaseData         map[string][]byte
}

// GenerateScaffold creates deterministic source files from extracted Action cases.
// modulePath is the module declared by the project being generated into.
func GenerateScaffold(modulePath string, model fizzbee.Model, declarations Declarations) (*Scaffold, error) {
	data, err := BuildTemplateData(modulePath, "", model, declarations)
	if err != nil {
		return nil, err
	}
	return generateScaffold(data)
}

func generateScaffold(data *TemplateData) (*Scaffold, error) {
	var common []byte
	if len(data.Observers) > 0 || len(data.DeclaredTypes) > 0 {
		var err error
		common, err = generateTemplateScaffold(templateCommon, data)
		if err != nil {
			return nil, err
		}
	}
	roleCases, err := generateRoleSources(data, templateTypes)
	if err != nil {
		return nil, err
	}
	roleConformances, err := generateRoleConformances(data)
	if err != nil {
		return nil, err
	}
	roleScaffolds, err := generateRoleScaffolds(data)
	if err != nil {
		return nil, err
	}
	caseData, err := generateCaseData(data)
	if err != nil {
		return nil, err
	}
	return &Scaffold{Common: common, RoleCases: roleCases, RoleConformances: roleConformances, RoleScaffolds: roleScaffolds, CaseData: caseData}, nil
}

type caseInputJSON struct {
	Fields map[string]any `json:"fields"`
	Params map[string]any `json:"params"`
}

type caseOutputJSON struct {
	Fields     map[string]any     `json:"fields"`
	Params     map[string]any     `json:"params"`
	SgMetadata fizzbee.SgMetadata `json:"sg_metadata"`
}

func generateCaseData(data *TemplateData) (map[string][]byte, error) {
	type generatedCase struct {
		Name    string           `json:"name"`
		Input   caseInputJSON    `json:"input"`
		Allowed []caseOutputJSON `json:"allowed"`
	}

	files := make(map[string][]byte, len(data.Actions))
	for _, action := range data.Actions {
		cases := make([]generatedCase, 0, len(action.Cases))
		for caseIndex, actionCase := range action.Cases {
			cases = append(cases, generatedCase{
				Name:    fmt.Sprintf("case%d", caseIndex+1),
				Input:   caseStateJSON(actionCase.Input),
				Allowed: caseStatesJSON(actionCase.Outputs),
			})
		}
		contents, err := json.MarshalIndent(cases, "", "  ")
		if err != nil {
			return nil, errors.Wrapf(err, "generate %s cases", action.Role+"."+action.Name)
		}
		files[action.CaseFile] = contents
	}
	return files, nil
}

func caseStatesJSON(states []roleState) []caseOutputJSON {
	values := make([]caseOutputJSON, 0, len(states))
	for _, state := range states {
		value := caseStateJSON(state)
		values = append(values, caseOutputJSON{Params: value.Params, Fields: value.Fields, SgMetadata: state.SgMetadata})
	}
	return values
}

func caseStateJSON(state roleState) caseInputJSON {
	params := make(map[string]any)
	fields := make(map[string]any, len(state.Roles))
	for _, role := range state.Roles {
		if len(role.Params) != 0 {
			params[role.Name] = fieldValuesJSON(role.Params)
		}
		fields[role.Name] = fieldValuesJSON(role.Fields)
	}
	return caseInputJSON{Params: params, Fields: fields}
}

func fieldValuesJSON(fields []fieldValue) map[string]any {
	values := make(map[string]any, len(fields))
	for _, field := range fields {
		values[field.JSONName] = field.Value
	}
	return values
}

func generateTemplateScaffold(name templateName, data *TemplateData) ([]byte, error) {
	templatePath := name.path()
	templates, err := template.New(templatePath).Funcs(template.FuncMap{
		"goLiteral": goLiteral,
	}).ParseFS(templateFiles, templatePath)
	if err != nil {
		return nil, errors.Wrapf(err, "parse %s template", filepath.Base(templatePath))
	}
	var output bytes.Buffer
	if err := templates.ExecuteTemplate(&output, filepath.Base(templatePath), data); err != nil {
		return nil, errors.Wrapf(err, "generate %s", filepath.Base(templatePath))
	}
	source, err := format.Source(output.Bytes())
	if err != nil {
		return nil, errors.Wrapf(err, "format %s", filepath.Base(templatePath))
	}
	return source, nil
}

func generateRoleScaffolds(data *TemplateData) (map[string][]byte, error) {
	return generateRoleSources(data, templateRole)
}

func generateRoleConformances(data *TemplateData) (map[string][]byte, error) {
	return generateRoleSources(data, templateConformance)
}

func generateRoleSources(data *TemplateData, name templateName) (map[string][]byte, error) {
	byRole := make(map[string][]*action)
	for _, action := range data.Actions {
		byRole[action.Role] = append(byRole[action.Role], action)
	}
	roleNames := make([]string, 0, len(byRole))
	for role := range byRole {
		roleNames = append(roleNames, role)
	}
	sort.Strings(roleNames)
	scaffolds := make(map[string][]byte, len(roleNames))
	for _, role := range roleNames {
		roleData := newTemplateData(data.Module, role, byRole[role])
		roleData.FetchedActionCrashed = data.FetchedActionCrashed
		roleData.ManualActionCrashed = data.ManualActionCrashed
		roleData.Observers = data.Observers
		roleData.ObserverGroups = data.ObserverGroups
		roleData.PluginImports = data.PluginImports
		source, err := generateTemplateScaffold(name, roleData)
		if err != nil {
			return nil, err
		}
		scaffolds[role] = source
	}
	return scaffolds, nil
}
