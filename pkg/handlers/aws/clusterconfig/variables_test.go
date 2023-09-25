// Copyright 2023 D2iQ, Inc. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package clusterconfig

import (
	"testing"

	"k8s.io/utils/ptr"

	"github.com/d2iq-labs/capi-runtime-extensions/api/v1alpha1"
	"github.com/d2iq-labs/capi-runtime-extensions/common/pkg/testutils/capitest"
)

func TestVariableValidation(t *testing.T) {
	capitest.ValidateDiscoverVariables(
		t,
		MetaVariableName,
		ptr.To(v1alpha1.AWSClusterConfigSpec{}.VariableSchema()),
		true,
		NewVariable,
		capitest.VariableTestDef{
			Name: "specified region",
			Vals: v1alpha1.AWSClusterConfigSpec{
				Region: ptr.To(v1alpha1.Region("a-specified-region")),
			},
		},
	)
}