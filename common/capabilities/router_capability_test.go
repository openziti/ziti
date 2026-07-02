/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package capabilities

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

const (
	capabilitiesPkgPath  = "github.com/openziti/ziti/v2/common/capabilities"
	sdkEdgeClientPkgPath = "github.com/openziti/sdk-golang/v2/pb/edge_client_pb"
	routerCapabilityType = "RouterCapability"
)

// TestRouterCapabilityProvenanceAndUniqueness type-checks this package and
// enforces the router-capability contract on every constant of the
// RouterCapability type:
//
//   - a non-negative (shared) capability must be sourced from the SDK enum: its
//     initializer must reference the edge_client_pb package.
//   - a negative (control-plane-only) capability must NOT reference the SDK enum;
//     it is defined only in this repo.
//   - no two capabilities may resolve to the same bit position (checked through
//     the same translation the Mask uses, so shared and control-only positions
//     can never collide).
//
// Because it discovers the constants by type via go/packages, any capability
// added later is covered automatically with no changes to this test.
func TestRouterCapabilityProvenanceAndUniqueness(t *testing.T) {
	req := require.New(t)

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedSyntax | packages.NeedImports | packages.NeedDeps,
	}
	pkgs, err := packages.Load(cfg, capabilitiesPkgPath)
	req.NoError(err)
	req.Len(pkgs, 1)

	pkg := pkgs[0]
	req.Empty(pkg.Errors, "package %s must type-check cleanly", capabilitiesPkgPath)

	rcTypeObj := pkg.Types.Scope().Lookup(routerCapabilityType)
	req.NotNil(rcTypeObj, "type %s not found in %s", routerCapabilityType, capabilitiesPkgPath)
	rcType := rcTypeObj.Type()

	found := map[string]int{}
	mask := NewMask[int]()

	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.CONST {
				continue
			}
			for _, spec := range genDecl.Specs {
				valueSpec := spec.(*ast.ValueSpec)
				for i, nameIdent := range valueSpec.Names {
					constObj, ok := pkg.TypesInfo.Defs[nameIdent].(*types.Const)
					if !ok || constObj.Type() != rcType {
						continue
					}

					value, ok := constant.Int64Val(constObj.Val())
					req.True(ok, "capability %s must have an integer value", nameIdent.Name)

					var initExpr ast.Expr
					if i < len(valueSpec.Values) {
						initExpr = valueSpec.Values[i]
					}
					req.NotNil(initExpr, "capability %s must have an explicit initializer", nameIdent.Name)
					refsSDK := initializerReferencesPackage(pkg, initExpr, sdkEdgeClientPkgPath)

					if value >= 0 {
						req.Truef(refsSDK, "shared (non-negative) capability %s (bit %d) must be sourced from the SDK enum %s", nameIdent.Name, value, sdkEdgeClientPkgPath)
					} else {
						req.Falsef(refsSDK, "control-plane-only (negative) capability %s (%d) must not reference the SDK enum", nameIdent.Name, value)
					}

					intVal := int(value)
					req.Falsef(mask.IsSet(intVal), "capability %s (%d) collides on a bit position already claimed by another capability", nameIdent.Name, value)
					mask.Set(intVal)

					found[nameIdent.Name] = intVal
				}
			}
		}
	}

	// Guard against a vacuous pass (e.g. a loader misconfiguration finding no
	// constants): the two shared capabilities that exist today must be present.
	req.GreaterOrEqual(len(found), 2, "expected to discover the router capability constants")
	req.Contains(found, "RouterMultiChannel")
	req.Contains(found, "RouterConnectV2")
}

// initializerReferencesPackage reports whether any identifier in expr resolves to
// an object declared in the package with the given import path. Used to decide a
// capability's provenance: a shared capability's initializer references the SDK
// edge_client_pb enum, a control-plane-only one does not.
func initializerReferencesPackage(pkg *packages.Package, expr ast.Expr, pkgPath string) bool {
	referenced := false
	ast.Inspect(expr, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		obj := pkg.TypesInfo.Uses[ident]
		if obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == pkgPath {
			referenced = true
		}
		return true
	})
	return referenced
}
