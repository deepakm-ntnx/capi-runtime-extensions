// Copyright 2023 D2iQ, Inc. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package machinedetails

import (
	"context"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	capxv1 "github.com/d2iq-labs/capi-runtime-extensions/api/external/github.com/nutanix-cloud-native/cluster-api-provider-nutanix/api/v1beta1"
	"github.com/d2iq-labs/capi-runtime-extensions/api/v1alpha1"
	"github.com/d2iq-labs/capi-runtime-extensions/common/pkg/capi/clustertopology/patches"
	"github.com/d2iq-labs/capi-runtime-extensions/common/pkg/capi/clustertopology/patches/selectors"
	"github.com/d2iq-labs/capi-runtime-extensions/common/pkg/capi/clustertopology/variables"
	"github.com/d2iq-labs/capi-runtime-extensions/pkg/handlers/generic/clusterconfig"
)

const (
	// VariableName is the external patch variable name.
	VariableName = "machineDetails"
)

type nutanixMachineDetailsControlPlanePatchHandler struct {
	variableName      string
	variableFieldPath []string
}

func NewControlPlanePatch() *nutanixMachineDetailsControlPlanePatchHandler {
	return newNutanixMachineDetailsControlPlanePatchHandler(
		clusterconfig.MetaVariableName,
		clusterconfig.MetaControlPlaneConfigName,
		v1alpha1.NutanixVariableName,
		VariableName,
	)
}

func newNutanixMachineDetailsControlPlanePatchHandler(
	variableName string,
	variableFieldPath ...string,
) *nutanixMachineDetailsControlPlanePatchHandler {
	return &nutanixMachineDetailsControlPlanePatchHandler{
		variableName:      variableName,
		variableFieldPath: variableFieldPath,
	}
}

func (h *nutanixMachineDetailsControlPlanePatchHandler) Mutate(
	ctx context.Context,
	obj *unstructured.Unstructured,
	vars map[string]apiextensionsv1.JSON,
	holderRef runtimehooksv1.HolderReference,
	_ client.ObjectKey,
) error {
	log := ctrl.LoggerFrom(ctx).WithValues(
		"holderRef", holderRef,
	)

	nutanixMachineDetailsVar, found, err := variables.Get[v1alpha1.NutanixMachineDetails](
		vars,
		h.variableName,
		h.variableFieldPath...,
	)
	if err != nil {
		return err
	}
	if !found {
		log.V(5).Info("Nutanix machine details variable for control-plane not defined")
		return nil
	}

	log = log.WithValues(
		"variableName",
		h.variableName,
		"variableFieldPath",
		h.variableFieldPath,
		"variableValue",
		nutanixMachineDetailsVar,
	)

	return patches.MutateIfApplicable(
		obj,
		vars,
		&holderRef,
		selectors.InfrastructureControlPlaneMachines(
			"v1beta1",
			"NutanixMachineTemplate",
		),
		log,
		func(obj *capxv1.NutanixMachineTemplate) error {
			log.WithValues(
				"patchedObjectKind", obj.GetObjectKind().GroupVersionKind().String(),
				"patchedObjectName", client.ObjectKeyFromObject(obj),
			).Info("setting Nutanix machine details in control plane NutanixMachineTemplate spec")

			obj.Spec.Template.Spec.BootType = capxv1.NutanixBootType(
				nutanixMachineDetailsVar.BootType,
			)
			obj.Spec.Template.Spec.Cluster = capxv1.NutanixResourceIdentifier{
				Type: capxv1.NutanixIdentifierType(nutanixMachineDetailsVar.Cluster.Type),
			}
			if nutanixMachineDetailsVar.Cluster.Type == v1alpha1.NutanixIdentifierName {
				obj.Spec.Template.Spec.Cluster.Name = nutanixMachineDetailsVar.Cluster.Name
			} else {
				obj.Spec.Template.Spec.Cluster.UUID = nutanixMachineDetailsVar.Cluster.UUID
			}

			obj.Spec.Template.Spec.Image = capxv1.NutanixResourceIdentifier{
				Type: capxv1.NutanixIdentifierType(nutanixMachineDetailsVar.Image.Type),
			}
			if nutanixMachineDetailsVar.Image.Type == v1alpha1.NutanixIdentifierName {
				obj.Spec.Template.Spec.Image.Name = nutanixMachineDetailsVar.Image.Name
			} else {
				obj.Spec.Template.Spec.Image.UUID = nutanixMachineDetailsVar.Image.UUID
			}

			obj.Spec.Template.Spec.VCPUSockets = nutanixMachineDetailsVar.VCPUSockets
			obj.Spec.Template.Spec.VCPUsPerSocket = nutanixMachineDetailsVar.VCPUsPerSocket
			obj.Spec.Template.Spec.MemorySize = resource.MustParse(
				nutanixMachineDetailsVar.MemorySize,
			)
			obj.Spec.Template.Spec.SystemDiskSize = resource.MustParse(
				nutanixMachineDetailsVar.SystemDiskSize,
			)
			// TODO
			// obj.Spec.Template.Spec.Subnets = nutanixMachineDetailsVar.Subnets
			// obj.Spec.Template.Spec.Project = nutanixMachineDetailsVar.Project
			// obj.Spec.Template.Spec.AdditionalCategories = nutanixMachineDetailsVar.AdditionalCategories
			// obj.Spec.Template.Spec.GPUs = nutanixMachineDetailsVar.GPUs
			return nil
		},
	)
}