package generate

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanRoleFilePreservesCompleteUserFile(t *testing.T) {
	name := filepath.Join(t.TempDir(), "reader_conformance_impl_test.go")
	current := []byte("package conformance_test\n\n// Keep this explanation.\nfunc BuildReader() {}\n")
	generated := []byte("package conformance_test\n\nfunc BuildReader() {}\n")
	require.NoError(t, os.WriteFile(name, current, 0o644))

	got, err := planRoleFile(name, generated, "Reader")
	require.NoError(t, err)
	assert.Equal(t, current, got)
}

func TestPlanRoleFileAddsBuildConstraintToCompleteUserFile(t *testing.T) {
	name := filepath.Join(t.TempDir(), "reader_conformance_impl_test.go")
	current := []byte("package conformance_test\n\n// Keep this explanation.\nfunc BuildReader() {}\n")
	generated := []byte("//go:build specguardian\n\npackage conformance_test\n\nfunc BuildReader() {}\n")
	require.NoError(t, os.WriteFile(name, current, 0o644))

	got, err := planRoleFile(name, generated, "Reader")
	require.NoError(t, err)
	assert.Equal(t, "//go:build specguardian\n\n"+string(current), string(got))
}

func TestUpdateRoleFilePreservesExistingAndGeneratedComments(t *testing.T) {
	t.Parallel()
	current := []byte(`package conformance_test

// Existing explains the user implementation.
func Existing() {
	// Keep this comment inside the function.
	println("kept")
}
`)
	generated := []byte(`package conformance_test

func Existing() {}

// Added explains the generated scaffold.
func Added() {}
`)

	got, err := updateRoleFile("role_test.go", current, generated, "Reader")
	require.NoError(t, err)
	const want = `package conformance_test

// Existing explains the user implementation.
func Existing() {
	// Keep this comment inside the function.
	println("kept")
}

// Added explains the generated scaffold.
func Added() {}
`
	assert.Equal(t, want, string(got))
}

func TestUpdateRoleFileAddsAndChangesOnlyRequiredDeclarations(t *testing.T) {
	t.Parallel()
	current := []byte(`package conformance_test

import "testing"

type ReaderReadInput struct { Kept string }
type ReaderReadOutput struct { Kept string }
type ReaderDeleteInput struct{}
type ReaderDeleteOutput struct{}
type ReaderEnvironment struct { Kept string }

func BuildReaderConformanceComponents(t *testing.T, old int) (int, *ReaderEnvironment) { return old, nil }
func (e *ReaderEnvironment) SetupRead(t *testing.T, old string) { println("kept body") }
func (e *ReaderEnvironment) CleanupRead(t *testing.T, old string) {}
func (e *ReaderEnvironment) ToReadInput(t *testing.T, old string) ReaderReadInput { return ReaderReadInput{Kept: old} }
func (e *ReaderEnvironment) CollectReadCurrentState(t *testing.T, old string) int { return len(old) }
func (e *ReaderEnvironment) SetupDelete(t *testing.T, old string) { println("deleted action body") }
func helper() { println("kept helper") }
`)
	generated := []byte(`package conformance_test

import (
    "errors"
    "testing"
)

type ReaderReadInput struct{}
type ReaderReadOutput struct{}
type ReaderCreateInput struct{}
type ReaderCreateOutput struct{}
type ReaderEnvironment struct{}

func BuildReaderConformanceComponents(t *testing.T, next string) (string, *ReaderEnvironment) { panic(errors.New("TODO")) }
func (e *ReaderEnvironment) SetupRead(t *testing.T, next int) { panic(errors.New("TODO")) }
func (e *ReaderEnvironment) CleanupRead(t *testing.T, next int) {}
func (e *ReaderEnvironment) ToReadInput(t *testing.T, next int) ReaderReadInput { return ReaderReadInput{} }
func (e *ReaderEnvironment) CollectReadCurrentState(t *testing.T, next int) string { return "" }
func (e *ReaderEnvironment) SetupCreate(t *testing.T, next int) {}
func (e *ReaderEnvironment) CleanupCreate(t *testing.T, next int) {}
func (e *ReaderEnvironment) ToCreateInput(t *testing.T, next int) ReaderCreateInput { return ReaderCreateInput{} }
func (e *ReaderEnvironment) CollectCreateCurrentState(t *testing.T, next int) string { return "" }
`)

	got, err := updateRoleFile("reader_conformance_impl_test.go", current, generated, "Reader")
	require.NoError(t, err)
	const want = `package conformance_test

import "testing"

type ReaderReadInput struct{ Kept string }

type ReaderReadOutput struct{ Kept string }

type ReaderCreateInput struct{}

type ReaderCreateOutput struct{}

type ReaderDeleteInput struct{}

type ReaderDeleteOutput struct{}

type ReaderEnvironment struct{ Kept string }

func BuildReaderConformanceComponents(t *testing.T, next string) (string, *ReaderEnvironment) {
	return old, nil
}

func (e *ReaderEnvironment) SetupRead(t *testing.T, next int) { println("kept body") }

func (e *ReaderEnvironment) CleanupRead(t *testing.T, next int) {}

func (e *ReaderEnvironment) ToReadInput(t *testing.T, next int) ReaderReadInput {
	return ReaderReadInput{Kept: old}
}

func (e *ReaderEnvironment) CollectReadCurrentState(t *testing.T, next int) string { return len(old) }

func (e *ReaderEnvironment) SetupCreate(t *testing.T, next int) {}

func (e *ReaderEnvironment) CleanupCreate(t *testing.T, next int) {}

func (e *ReaderEnvironment) ToCreateInput(t *testing.T, next int) ReaderCreateInput {
	return ReaderCreateInput{}
}

func (e *ReaderEnvironment) CollectCreateCurrentState(t *testing.T, next int) string { return "" }

func (e *ReaderEnvironment) SetupDelete(t *testing.T, old string) { println("deleted action body") }

func helper() { println("kept helper") }
`
	assert.Equal(t, want, string(got))
	_, err = parser.ParseFile(token.NewFileSet(), "updated.go", got, parser.AllErrors)
	assert.NoError(t, err)
}

func TestUpdateRoleFileLeavesRemovedRoleDeclarationsAlone(t *testing.T) {
	t.Parallel()
	current := []byte(`package conformance_test

type GoneRunInput struct{}
type GoneRunOutput struct{}
type GoneEnvironment struct{ Kept int }
func (e *GoneEnvironment) SetupRun() { println("kept") }
func helper() {}
`)
	generated := []byte("package conformance_test\n")

	got, err := updateRoleFile("gone_conformance_impl_test.go", current, generated, "Gone")
	require.NoError(t, err)
	const want = `package conformance_test

type GoneRunInput struct{}

type GoneRunOutput struct{}

type GoneEnvironment struct{ Kept int }

func (e *GoneEnvironment) SetupRun() { println("kept") }

func helper() {}
`
	assert.Equal(t, want, string(got))
}

func TestUpdateRoleFileRejectsInvalidCurrentSource(t *testing.T) {
	t.Parallel()
	_, err := updateRoleFile("reader_conformance_impl_test.go", []byte("not Go"), []byte("package conformance_test\n"), "Reader")
	assert.Error(t, err)
}

func TestUpdateRoleFileRejectsInvalidGeneratedSource(t *testing.T) {
	t.Parallel()
	_, err := updateRoleFile("reader_conformance_impl_test.go", []byte("package conformance_test\n"), []byte("not Go"), "Reader")
	assert.Error(t, err)
}

func TestUpdateRoleFileUsesGeneratedBuildConstraint(t *testing.T) {
	t.Parallel()
	current := []byte("package conformance_test\n\nfunc helper() {}\n")
	generated := []byte("//go:build specguardian\n\npackage conformance_test\n")

	got, err := updateRoleFile("reader_conformance_impl_test.go", current, generated, "Reader")
	require.NoError(t, err)
	const want = `//go:build specguardian

package conformance_test

func helper() {}
`
	assert.Equal(t, want, string(got))
}

func TestUpdateRoleFileAddsDocumentationWithMissingFunction(t *testing.T) {
	t.Parallel()
	current := []byte(`package conformance_test

type WriterEnvironment struct{}
`)
	generated := []byte(`package conformance_test

type WriterEnvironment struct{}

// CollectWriteCurrentState observes the state after Writer.Write and maps it for comparison with FizzBee states.
func (e *WriterEnvironment) CollectWriteCurrentState(next int) { panic("TODO") }
`)

	got, err := updateRoleFile("writer_conformance_impl_test.go", current, generated, "Writer")
	require.NoError(t, err)
	const want = `package conformance_test

type WriterEnvironment struct{}

// CollectWriteCurrentState observes the state after Writer.Write and maps it for comparison with FizzBee states.
func (e *WriterEnvironment) CollectWriteCurrentState(next int) { panic("TODO") }
`
	assert.Equal(t, want, string(got))
}

func TestUpdateRoleFileIsIdempotent(t *testing.T) {
	t.Parallel()
	current := []byte(`package conformance_test

import "testing"

type ReaderEnvironment struct{ Kept string }

// SetupRead is user documentation.
func (e *ReaderEnvironment) SetupRead(t *testing.T, old string) { println("kept body", old) }

func helper() { println("kept helper") }
`)
	generated := []byte(`package conformance_test

import (
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

type ReaderEnvironment struct{}

// SetupRead is generated documentation.
func (e *ReaderEnvironment) SetupRead(t *testing.T, input int) { panic("TODO") }

// CleanupRead is generated documentation.
func (e *ReaderEnvironment) CleanupRead(t *testing.T, input int) {
	t.Helper()
	require.NoError(t, errors.New("TODO"))
}
`)

	first, err := updateRoleFile("reader_conformance_impl_test.go", current, generated, "Reader")
	require.NoError(t, err)
	second, err := updateRoleFile("reader_conformance_impl_test.go", first, generated, "Reader")
	require.NoError(t, err)
	const want = `package conformance_test

import (
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

type ReaderEnvironment struct{ Kept string }

// SetupRead is user documentation.
func (e *ReaderEnvironment) SetupRead(t *testing.T, input int) { println("kept body", old) }

// CleanupRead is generated documentation.
func (e *ReaderEnvironment) CleanupRead(t *testing.T, input int) {
	t.Helper()
	require.NoError(t, errors.New("TODO"))
}

func helper() { println("kept helper") }
`
	assert.Equal(t, want, string(first))
	assert.Equal(t, want, string(second))
}

func TestUpdateRoleFileDoesNotAddUnusedGeneratedImports(t *testing.T) {
	t.Parallel()
	current := []byte(`package conformance_test

type ReaderEnvironment struct{}

func (e *ReaderEnvironment) SetupRead(input int) { println("kept", input) }
`)
	generated := []byte(`package conformance_test

import (
	"errors"
	"github.com/stretchr/testify/require"
)

type ReaderEnvironment struct{}
func (e *ReaderEnvironment) SetupRead(input int) { require.NoError(nil, errors.New("TODO")) }
`)

	got, err := updateRoleFile("reader_conformance_impl_test.go", current, generated, "Reader")
	require.NoError(t, err)
	const want = `package conformance_test

type ReaderEnvironment struct{}

func (e *ReaderEnvironment) SetupRead(input int) { println("kept", input) }
`
	assert.Equal(t, want, string(got))
}
