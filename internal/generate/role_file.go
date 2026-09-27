package generate

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
)

func updateRoleFile(name string, currentSource, generated []byte, role string) ([]byte, error) {
	currentFileset := token.NewFileSet()
	current, err := parser.ParseFile(currentFileset, name, currentSource, parser.ParseComments)
	if err != nil {
		return nil, errors.Wrapf(err, "parse user-owned role file %s", name)
	}
	generatedFileset := token.NewFileSet()
	generatedFile, err := parser.ParseFile(generatedFileset, name+".generated", generated, parser.ParseComments)
	if err != nil {
		return nil, errors.Wrapf(err, "parse generated role scaffold for %s", role)
	}

	currentByName := namedDeclarations(current.Decls)
	updatedSource, err := replaceFunctionSignatures(currentSource, generated, currentFileset, generatedFileset, currentByName, generatedFile.Decls)
	if err != nil {
		return nil, err
	}
	fileset := token.NewFileSet()
	current, err = parser.ParseFile(fileset, name, updatedSource, parser.ParseComments)
	if err != nil {
		return nil, errors.Wrapf(err, "parse updated user-owned role file %s", name)
	}
	currentByName = namedDeclarations(current.Decls)
	requiredImports := make(map[string]struct{})
	for _, generatedDeclaration := range generatedFile.Decls {
		declarationName := namedDeclarationName(generatedDeclaration)
		if declarationName == "" {
			continue
		}
		_, ok := currentByName[declarationName]
		if !ok {
			collectReferencedNames(generatedDeclaration, requiredImports)
			continue
		}
		if generatedFunction, ok := generatedDeclaration.(*ast.FuncDecl); ok {
			collectReferencedNames(generatedFunction.Type, requiredImports)
		}
	}
	mergeImports(current, generatedFile, requiredImports)
	output, err := formatOrderedRoleFile(current, generatedFile, updatedSource, generated, fileset, generatedFileset)
	if err != nil {
		return nil, errors.Wrapf(err, "format updated role file %s", name)
	}
	return output, nil
}

func roleFileComplete(name string, currentSource, generated []byte) (bool, string) {
	currentSet := token.NewFileSet()
	current, err := parser.ParseFile(currentSet, name, currentSource, parser.ParseComments)
	if err != nil {
		return false, ""
	}
	generatedSet := token.NewFileSet()
	generatedFile, err := parser.ParseFile(generatedSet, name+".generated", generated, parser.ParseComments)
	if err != nil {
		return false, ""
	}
	constraint := fileBuildConstraint(generatedFile)
	currentConstraint := fileBuildConstraint(current)
	if constraint != "" && currentConstraint != "" && currentConstraint != constraint {
		return false, ""
	}
	currentByName := namedDeclarations(current.Decls)
	for _, declaration := range generatedFile.Decls {
		name := namedDeclarationName(declaration)
		if name == "" {
			continue
		}
		existing, ok := currentByName[name]
		if !ok {
			return false, ""
		}
		generatedFunction, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		currentFunction, ok := existing.(*ast.FuncDecl)
		if !ok {
			return false, ""
		}
		var currentType, generatedType bytes.Buffer
		if format.Node(&currentType, currentSet, currentFunction.Type) != nil || format.Node(&generatedType, generatedSet, generatedFunction.Type) != nil || currentType.String() != generatedType.String() {
			return false, ""
		}
	}
	if constraint != currentConstraint {
		return true, constraint
	}
	return true, ""
}

func formatOrderedRoleFile(current, generated *ast.File, currentSource, generatedSource []byte, currentFileset, generatedFileset *token.FileSet) ([]byte, error) {
	type declarationSource struct {
		declaration ast.Decl
		fileset     *token.FileSet
		source      []byte
	}
	currentByName := namedDeclarations(current.Decls)
	generatedNames := make(map[string]struct{})
	ordered := make([]declarationSource, 0, len(current.Decls)+len(generated.Decls))
	for _, declaration := range current.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if ok && general.Tok == token.IMPORT {
			ordered = append(ordered, declarationSource{declaration: declaration, fileset: currentFileset})
		}
	}
	for _, declaration := range generated.Decls {
		if name := namedDeclarationName(declaration); name != "" {
			generatedNames[name] = struct{}{}
		}
	}
	extrasBefore := make(map[string][]declarationSource)
	var trailingExtras []declarationSource
	for index, declaration := range current.Decls {
		if general, ok := declaration.(*ast.GenDecl); ok && general.Tok == token.IMPORT {
			continue
		}
		if _, managed := generatedNames[namedDeclarationName(declaration)]; managed {
			continue
		}
		extra := declarationSource{declaration: declaration, fileset: currentFileset, source: currentSource}
		nextManaged := ""
		for _, candidate := range current.Decls[index+1:] {
			name := namedDeclarationName(candidate)
			if _, managed := generatedNames[name]; managed {
				nextManaged = name
				break
			}
		}
		if nextManaged == "" {
			trailingExtras = append(trailingExtras, extra)
		} else {
			extrasBefore[nextManaged] = append(extrasBefore[nextManaged], extra)
		}
	}
	for _, declaration := range generated.Decls {
		name := namedDeclarationName(declaration)
		if name == "" {
			continue
		}
		ordered = append(ordered, extrasBefore[name]...)
		if existing, ok := currentByName[name]; ok {
			ordered = append(ordered, declarationSource{declaration: existing, fileset: currentFileset, source: currentSource})
		} else {
			ordered = append(ordered, declarationSource{declaration: declaration, fileset: generatedFileset, source: generatedSource})
		}
	}
	ordered = append(ordered, trailingExtras...)

	var output bytes.Buffer
	if buildConstraint := fileBuildConstraint(current, generated); buildConstraint != "" {
		fmt.Fprintf(&output, "%s\n\n", buildConstraint)
	}
	fmt.Fprintf(&output, "package %s\n", current.Name.Name)
	for _, source := range ordered {
		output.WriteByte('\n')
		if source.source == nil {
			if err := format.Node(&output, source.fileset, source.declaration); err != nil {
				return nil, err
			}
		} else {
			output.Write(declarationText(source.source, source.fileset, source.declaration))
		}
		output.WriteByte('\n')
	}
	return format.Source(output.Bytes())
}

func declarationText(source []byte, fileset *token.FileSet, declaration ast.Decl) []byte {
	start := declaration.Pos()
	switch declaration := declaration.(type) {
	case *ast.FuncDecl:
		if declaration.Doc != nil {
			start = declaration.Doc.Pos()
		}
	case *ast.GenDecl:
		if declaration.Doc != nil {
			start = declaration.Doc.Pos()
		}
	}
	file := fileset.File(declaration.Pos())
	return source[file.Offset(start):file.Offset(declaration.End())]
}

func fileBuildConstraint(files ...*ast.File) string {
	for _, file := range files {
		for _, commentGroup := range file.Comments {
			if commentGroup.Pos() >= file.Package {
				break
			}
			for _, comment := range commentGroup.List {
				if strings.HasPrefix(comment.Text, "//go:build ") {
					return comment.Text
				}
			}
		}
	}
	return ""
}

func replaceFunctionSignatures(currentSource, generatedSource []byte, currentFileset, generatedFileset *token.FileSet, currentByName map[string]ast.Decl, generatedDeclarations []ast.Decl) ([]byte, error) {
	type sourceReplacement struct {
		start, end int
		text       []byte
	}
	var replacements []sourceReplacement
	for _, declaration := range generatedDeclarations {
		generatedFunction, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		currentDeclaration, ok := currentByName[functionDeclarationName(generatedFunction)]
		if !ok {
			continue
		}
		currentFunction, ok := currentDeclaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		currentFile := currentFileset.File(currentFunction.Pos())
		generatedFile := generatedFileset.File(generatedFunction.Pos())
		if currentFile == nil || generatedFile == nil || currentFunction.Body == nil || generatedFunction.Body == nil {
			return nil, fmt.Errorf("locate function signature")
		}
		replacements = append(replacements, sourceReplacement{
			start: currentFile.Offset(currentFunction.Type.Func),
			end:   currentFile.Offset(currentFunction.Body.Lbrace),
			text:  generatedSource[generatedFile.Offset(generatedFunction.Type.Func):generatedFile.Offset(generatedFunction.Body.Lbrace)],
		})
	}
	result := bytes.Clone(currentSource)
	sort.Slice(replacements, func(i, j int) bool { return replacements[i].start > replacements[j].start })
	for _, replacement := range replacements {
		updated := make([]byte, 0, len(result)-(replacement.end-replacement.start)+len(replacement.text))
		updated = append(updated, result[:replacement.start]...)
		updated = append(updated, replacement.text...)
		updated = append(updated, result[replacement.end:]...)
		result = updated
	}
	return result, nil
}

func collectReferencedNames(node ast.Node, names map[string]struct{}) {
	ast.Inspect(node, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if ok {
			names[identifier.Name] = struct{}{}
		}
		return true
	})
}

func namedDeclarations(declarations []ast.Decl) map[string]ast.Decl {
	result := make(map[string]ast.Decl)
	for _, declaration := range declarations {
		if name := namedDeclarationName(declaration); name != "" {
			result[name] = declaration
		}
	}
	return result
}

func namedDeclarationName(declaration ast.Decl) string {
	switch declaration := declaration.(type) {
	case *ast.FuncDecl:
		return functionDeclarationName(declaration)
	case *ast.GenDecl:
		if declaration.Tok != token.TYPE || len(declaration.Specs) != 1 {
			return ""
		}
		typeSpec, ok := declaration.Specs[0].(*ast.TypeSpec)
		if ok {
			return "type " + typeSpec.Name.Name
		}
	}
	return ""
}

func functionDeclarationName(function *ast.FuncDecl) string {
	receiver := ""
	if function.Recv != nil && len(function.Recv.List) != 0 {
		receiver = receiverTypeName(function.Recv.List[0].Type) + "."
	}
	return "func " + receiver + function.Name.Name
}

func receiverTypeName(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.StarExpr:
		return receiverTypeName(expression.X)
	default:
		return ""
	}
}

func mergeImports(current, generated *ast.File, requiredNames map[string]struct{}) {
	existing := make(map[string]struct{})
	for _, importSpec := range current.Imports {
		existing[importSpec.Path.Value] = struct{}{}
	}
	var missing []ast.Spec
	for _, importSpec := range generated.Imports {
		if _, ok := requiredNames[importName(importSpec)]; !ok {
			continue
		}
		if _, ok := existing[importSpec.Path.Value]; ok {
			continue
		}
		missing = append(missing, cloneImportSpec(importSpec))
		existing[importSpec.Path.Value] = struct{}{}
	}
	if len(missing) == 0 {
		return
	}
	for _, declaration := range current.Decls {
		imports, ok := declaration.(*ast.GenDecl)
		if ok && imports.Tok == token.IMPORT {
			imports.Specs = append(imports.Specs, missing...)
			return
		}
	}
	current.Decls = append([]ast.Decl{&ast.GenDecl{Tok: token.IMPORT, Specs: missing}}, current.Decls...)
}

func cloneImportSpec(spec *ast.ImportSpec) *ast.ImportSpec {
	cloned := &ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: spec.Path.Value}}
	if spec.Name != nil {
		cloned.Name = ast.NewIdent(spec.Name.Name)
	}
	return cloned
}

func importName(spec *ast.ImportSpec) string {
	if spec.Name != nil {
		return spec.Name.Name
	}
	importPath, err := strconv.Unquote(spec.Path.Value)
	if err != nil {
		return ""
	}
	return path.Base(importPath)
}
