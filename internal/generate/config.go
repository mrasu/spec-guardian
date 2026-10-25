package generate

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/cockroachdb/errors"
	"github.com/mrasu/spec-guardian/internal/configschema"
	"github.com/mrasu/spec-guardian/internal/generate/renderer"
	"go.yaml.in/yaml/v3"
)

type resolvedSource struct {
	ImportPath   string
	Alias        string
	Capabilities []string
}

var builtinSources = map[string]resolvedSource{
	"error_as_crashed": {
		ImportPath:   "github.com/mrasu/spec-guardian/guardian/provider/errorascrashed",
		Alias:        "errorascrashedprovider",
		Capabilities: []string{"observe_result"},
	},
	"sql": {
		ImportPath:   "github.com/mrasu/spec-guardian/guardian/provider/sql",
		Alias:        "sqlprovider",
		Capabilities: []string{"seed", "observe"},
	},
	"valkey": {
		ImportPath:   "github.com/mrasu/spec-guardian/guardian/provider/valkey",
		Alias:        "valkeyprovider",
		Capabilities: []string{"seed", "observe"},
	},
	"dynamodb": {
		ImportPath:   "github.com/mrasu/spec-guardian/guardian/provider/dynamodb",
		Alias:        "dynamodbprovider",
		Capabilities: []string{"seed", "observe"},
	},
}

// FileConfig holds the declarations read from a specguardian.yml file.
type FileConfig struct {
	Version      string              `yaml:"version"`
	FizzbeeFiles []fizzbeeFileConfig `yaml:"fizzbee_files"`
	Development  bool                `yaml:"dev"`
	Providers    []providerConfig    `yaml:"providers"`
}

type fizzbeeFileConfig struct {
	Hooks      []renderer.Hook  `yaml:"hooks"`
	OutputDir  string           `yaml:"output_dir"`
	Roles      []roleConfig     `yaml:"roles"`
	SgMetadata sgMetadataConfig `yaml:"sg_metadata"`
}

type sgMetadataConfig struct {
	ActionCrashed actionCrashedConfig `yaml:"action_crashed"`
}

type actionCrashedConfig struct {
	Compare  *bool                `yaml:"compare"`
	Provider *fieldProviderConfig `yaml:"provider"`
}

type roleConfig struct {
	Name    *string       `yaml:"name"`
	Compare *bool         `yaml:"compare"`
	Reason  string        `yaml:"reason"`
	Fields  []fieldConfig `yaml:"fields"`
}

type fieldConfig struct {
	Name     string               `yaml:"name"`
	Compare  *bool                `yaml:"compare"`
	Reason   string               `yaml:"reason"`
	Provider *fieldProviderConfig `yaml:"provider"`
	Type     typeConfig           `yaml:"type"`
}

type typeConfig struct {
	Kind         renderer.Kind
	Nullable     bool
	Compare      *bool
	Reason       string
	CompareOrder *bool
	Values       *typeConfig
	Fields       map[string]typeConfig
}

type typeCommonConfig struct {
	Kind     renderer.Kind `yaml:"kind"`
	Nullable bool          `yaml:"nullable"`
	Compare  *bool         `yaml:"compare"`
	Reason   string        `yaml:"reason"`
}

type scalarTypeConfig struct {
	typeCommonConfig `yaml:",inline"`
}
type mapTypeConfig struct {
	typeCommonConfig `yaml:",inline"`
	Values           *typeConfig `yaml:"values"`
}
type listTypeConfig struct {
	typeCommonConfig `yaml:",inline"`
	Values           *typeConfig `yaml:"values"`
	CompareOrder     *bool       `yaml:"compare_order"`
}
type messageTypeConfig struct {
	typeCommonConfig `yaml:",inline"`
	Fields           map[string]typeConfig `yaml:"fields"`
}

func (t *typeConfig) UnmarshalYAML(node *yaml.Node) error {
	var header struct {
		Kind renderer.Kind `yaml:"kind"`
	}
	if err := node.Decode(&header); err != nil {
		return errors.Wrap(err, "decode type kind")
	}
	var common typeCommonConfig
	switch header.Kind {
	case renderer.KindString, renderer.KindInt64, renderer.KindBool, renderer.KindDouble, renderer.KindBytes:
		var value scalarTypeConfig
		if err := decodeConfigNode(node, &value); err != nil {
			return err
		}
		common = value.typeCommonConfig
	case renderer.KindMap:
		var value mapTypeConfig
		if err := decodeConfigNode(node, &value); err != nil {
			return err
		}
		common = value.typeCommonConfig
		t.Values = value.Values
	case renderer.KindList:
		var value listTypeConfig
		if err := decodeConfigNode(node, &value); err != nil {
			return err
		}
		common = value.typeCommonConfig
		t.Values, t.CompareOrder = value.Values, value.CompareOrder
	case renderer.KindMessage:
		var value messageTypeConfig
		if err := decodeConfigNode(node, &value); err != nil {
			return err
		}
		common = value.typeCommonConfig
		t.Fields = value.Fields
	default:
		return fmt.Errorf("unsupported kind %q", header.Kind)
	}
	t.Kind, t.Nullable, t.Compare, t.Reason = common.Kind, common.Nullable, common.Compare, common.Reason
	return nil
}

func decodeConfigNode(node *yaml.Node, value any) error {
	if err := node.Decode(value); err != nil {
		return errors.Wrap(err, "decode configuration node")
	}
	return nil
}

type fieldProviderConfig struct {
	ID     string         `yaml:"id"`
	Config map[string]any `yaml:"config"`
}

type providerConfig struct {
	ID     string         `yaml:"id"`
	Source sourceConfig   `yaml:"source"`
	Config map[string]any `yaml:"config"`
}

type sourceConfig struct {
	Builtin      string   `yaml:"builtin"`
	GoPackage    string   `yaml:"go_package"`
	Capabilities []string `yaml:"capabilities"`
}

// Options configures optional generation behavior.
type Options struct {
	ConfigPath string
}

// Config holds the resolved inputs used by a generation run.
type Config struct {
	configPath       string
	projectDir       string
	fizzbeeOutputDir string
	development      bool
	declarations     renderer.Declarations
}

// NewConfig reads and resolves the configuration file.
func NewConfig(options Options) (*Config, error) {
	if options.ConfigPath == "" {
		return nil, fmt.Errorf("configuration file path is empty")
	}
	return configFromFile(options.ConfigPath)
}

func configFromFile(path string) (*Config, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, errors.Wrap(err, "resolve configuration file")
	}
	fileConfig, err := readFileConfig(abs)
	if err != nil {
		return nil, err
	}
	declarations, err := parseDeclarations(fileConfig)
	if err != nil {
		return nil, err
	}
	projectDir := filepath.Dir(abs)
	return &Config{
		configPath:       abs,
		projectDir:       projectDir,
		fizzbeeOutputDir: filepath.Join(projectDir, fileConfig.FizzbeeFiles[0].OutputDir),
		development:      fileConfig.Development,
		declarations:     declarations,
	}, nil
}

func readFileConfig(path string) (*FileConfig, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "read configuration file %s", path)
	}
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	if err := decoder.Decode(&document); err != nil {
		return nil, errors.Wrapf(err, "parse configuration file %s", path)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, errors.Wrapf(err, "parse configuration file %s", path)
		}
		return nil, fmt.Errorf("configuration file %s must contain one YAML document", path)
	}
	if err := configschema.Validate(&document); err != nil {
		return nil, err
	}
	var config FileConfig
	if err := document.Decode(&config); err != nil {
		return nil, errors.Wrapf(err, "decode configuration file %s", path)
	}
	return &config, nil
}

func parseDeclarations(config *FileConfig) (renderer.Declarations, error) {
	providers, err := validateProviderConfigs(config.Providers)
	if err != nil {
		return renderer.Declarations{}, err
	}
	return buildDeclarations(config.FizzbeeFiles[0], providers)
}

type resolvedProvider struct {
	Config providerConfig
	Source resolvedSource
}

func validateProviderConfigs(configs []providerConfig) (map[string]resolvedProvider, error) {
	providers := make(map[string]resolvedProvider, len(configs))
	aliases := make(map[string]string, len(configs)+len(builtinSources))
	for _, source := range builtinSources {
		aliases[source.ImportPath] = source.Alias
	}
	usedAliases := make(map[string]bool, len(configs)+len(builtinSources))
	for _, alias := range aliases {
		usedAliases[alias] = true
	}
	for _, config := range configs {
		if _, exists := providers[config.ID]; exists {
			return nil, fmt.Errorf("duplicate provider %q", config.ID)
		}
		source, err := resolveSource(config.Source)
		if err != nil {
			return nil, fmt.Errorf("provider %q: %w", config.ID, err)
		}
		if source.Alias == "" {
			alias, ok := aliases[source.ImportPath]
			if !ok {
				base := toSourceAlias(source.ImportPath)
				alias = base
				for suffix := 1; usedAliases[alias]; suffix++ {
					alias = fmt.Sprintf("%s%d", base, suffix)
				}
				usedAliases[alias] = true
				aliases[source.ImportPath] = alias
			}
			source.Alias = alias
		}
		providers[config.ID] = resolvedProvider{Config: config, Source: source}
	}
	return providers, nil
}

func toSourceAlias(importPath string) string {
	words := strings.FieldsFunc(path.Base(importPath), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	var name strings.Builder
	name.WriteString("_sg")
	for _, word := range words {
		letters := []rune(word)
		letters[0] = unicode.ToUpper(letters[0])
		name.WriteString(string(letters))
	}
	return name.String()
}

func buildDeclarations(file fizzbeeFileConfig, providers map[string]resolvedProvider) (renderer.Declarations, error) {
	result := renderer.Declarations{Hooks: renderer.ResolveHooks(file.Hooks)}
	result.CompareActionCrashed = file.SgMetadata.ActionCrashed.Compare
	metadataProvider, err := buildActionCrashedProvider(file.SgMetadata.ActionCrashed, providers)
	if err != nil {
		return renderer.Declarations{}, err
	}
	result.ActionCrashedProvider = metadataProvider
	roles := make(map[string]bool)
	seen := make(map[string]bool)
	for _, role := range file.Roles {
		if role.Name == nil {
			return renderer.Declarations{}, fmt.Errorf("role name: null is reserved for future globals support")
		}
		name := *role.Name
		if strings.TrimSpace(name) == "" || strings.Contains(name, ".") {
			return renderer.Declarations{}, fmt.Errorf("invalid role name %q", name)
		}
		if roles[name] {
			return renderer.Declarations{}, fmt.Errorf("duplicate role %q", name)
		}
		roles[name] = true
		if role.Compare != nil && !*role.Compare {
			result.ExcludedRoles = append(result.ExcludedRoles, renderer.ExcludedRole{Name: name, Reason: role.Reason})
		}
		for _, entry := range role.Fields {
			if err := addFieldDeclaration(&result, name, entry, role.Compare != nil && !*role.Compare, providers, seen); err != nil {
				return renderer.Declarations{}, err
			}
		}
	}
	return result, nil
}

func buildActionCrashedProvider(config actionCrashedConfig, providers map[string]resolvedProvider) (*renderer.Provider, error) {
	if config.Provider == nil || config.Compare != nil && !*config.Compare {
		return nil, nil
	}
	declared, ok := providers[config.Provider.ID]
	if !ok {
		return nil, fmt.Errorf("sg_metadata.action_crashed: unknown provider %q", config.Provider.ID)
	}
	if !slices.Contains(declared.Source.Capabilities, "observe_result") {
		return nil, fmt.Errorf("sg_metadata.action_crashed: provider %q must have observe_result capability", config.Provider.ID)
	}
	field, err := renderer.NewRoleField("sg_metadata", "action_crashed")
	if err != nil {
		return nil, err
	}
	declaration, err := buildProviderDeclaration(field, fieldConfig{Provider: config.Provider}, renderer.Type{Kind: renderer.KindBool}, providers)
	if err != nil {
		return nil, err
	}
	declaration.Metadata = true
	return &declaration, nil
}

func addFieldDeclaration(result *renderer.Declarations, role string, entry fieldConfig, excludedRole bool, providers map[string]resolvedProvider, seen map[string]bool) error {
	field, err := renderer.NewRoleField(role, entry.Name)
	if err != nil {
		return err
	}
	if seen[field.String()] {
		return fmt.Errorf("duplicate field %q", field.String())
	}
	seen[field.String()] = true
	if excludedRole {
		return nil
	}
	if entry.Compare != nil && !*entry.Compare {
		result.Unobservable = append(result.Unobservable, renderer.UnobservableField{Field: field, Reason: entry.Reason})
		return nil
	}
	if entry.Type.Compare != nil && !*entry.Type.Compare {
		result.Unobservable = append(result.Unobservable, renderer.UnobservableField{Field: field, Reason: entry.Type.Reason})
		return nil
	}
	spec, err := toType(entry.Type)
	if err != nil {
		return fmt.Errorf("field %s: %w", field, err)
	}
	if entry.Provider == nil {
		result.TypedFields = append(result.TypedFields, renderer.TypedField{Field: field, Type: *spec})
		return nil
	}
	declaration, err := buildProviderDeclaration(field, entry, *spec, providers)
	if err != nil {
		return err
	}
	result.Providers = append(result.Providers, declaration)
	return nil
}

func buildProviderDeclaration(field *renderer.Field, entry fieldConfig, spec renderer.Type, providers map[string]resolvedProvider) (renderer.Provider, error) {
	request := *entry.Provider
	resolvedProvider, ok := providers[request.ID]
	if !ok {
		return renderer.Provider{}, fmt.Errorf("field %s: unknown provider %q", field, request.ID)
	}
	if !slices.Contains(resolvedProvider.Source.Capabilities, "observe") && !slices.Contains(resolvedProvider.Source.Capabilities, "observe_result") {
		return renderer.Provider{}, fmt.Errorf("field %s: provider %q requires observe or observe_result capability", field, request.ID)
	}
	declaration := renderer.Provider{
		Field:            field,
		Result:           slices.Contains(resolvedProvider.Source.Capabilities, "observe_result"),
		ProviderID:       resolvedProvider.Config.ID,
		SourceImportPath: resolvedProvider.Source.ImportPath,
		SourceAlias:      resolvedProvider.Source.Alias,
		Capabilities:     slices.Clone(resolvedProvider.Source.Capabilities),
		Type:             spec,
		ProviderConfig:   resolvedProvider.Config.Config,
		FieldConfig:      request.Config,
	}
	return declaration, nil
}

func toType(config typeConfig) (*renderer.Type, error) {
	return toTypeWithParent(config, false)
}

func toTypeWithParent(config typeConfig, ignored bool) (*renderer.Type, error) {
	ignored = ignored || config.Compare != nil && !*config.Compare
	var values *renderer.Type
	if config.Values != nil {
		var err error
		values, err = toTypeWithParent(*config.Values, ignored)
		if err != nil {
			return nil, err
		}
	}

	var fields map[string]renderer.Type
	if config.Fields != nil {
		fields = make(map[string]renderer.Type, len(config.Fields))
		for name, fieldConfig := range config.Fields {
			field, err := toTypeWithParent(fieldConfig, ignored)
			if err != nil {
				return nil, fmt.Errorf("message field %s: %w", name, err)
			}
			fields[name] = *field
		}
	}

	spec, err := renderer.NewType(config.Kind, config.Nullable, values, fields)
	if err != nil {
		return nil, err
	}
	spec.SkipComparison = ignored
	if config.CompareOrder != nil && !*config.CompareOrder {
		spec.IgnoreOrder = true
	}
	return spec, nil
}

func resolveSource(config sourceConfig) (resolvedSource, error) {
	if config.GoPackage != "" {
		return resolvedSource{ImportPath: config.GoPackage, Capabilities: config.Capabilities}, nil
	}

	source, ok := builtinSources[config.Builtin]
	if !ok {
		return resolvedSource{}, fmt.Errorf("unknown source.builtin %q", config.Builtin)
	}
	return source, nil
}
