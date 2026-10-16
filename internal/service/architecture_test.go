// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// goaPackagePrefix is the import path of the generated Goa code. The service layer
// works in domain types, and cmd/project-api converts to and from the Goa types.
const goaPackagePrefix = "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen"

func TestServiceLayerDoesNotImportGoaTypes(t *testing.T) {
	fset := token.NewFileSet()
	checked := 0

	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return walkErr
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		checked++
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			assert.Falsef(t, strings.HasPrefix(importPath, goaPackagePrefix),
				"%s imports %s; convert Goa types in cmd/project-api/converters.go instead", path, importPath)
		}
		return nil
	})

	require.NoError(t, err)
	assert.Positive(t, checked, "the walk should find the package's Go files")
}
