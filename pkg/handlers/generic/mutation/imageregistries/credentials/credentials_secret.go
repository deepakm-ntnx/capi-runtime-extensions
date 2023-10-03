// Copyright 2023 D2iQ, Inc. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package credentials

import (
	"bytes"
	_ "embed"
	"fmt"
	"net/url"
	"strings"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	cabpkv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
)

const (
	secretKeyForStaticCredentialProviderConfig = "static-credential-provider" //nolint:gosec // Not a credential.
)

//go:embed templates/static-credential-provider.json.gotmpl
var staticCredentialProviderConfigPatch []byte

func generateCredentialsSecretFile(config providerConfig, ownerName string) []cabpkv1.File {
	if config.isCredentialsEmpty() {
		return nil
	}
	return []cabpkv1.File{
		{
			Path: kubeletStaticCredentialProviderCredentialsOnRemote,
			ContentFrom: &cabpkv1.FileSource{
				Secret: cabpkv1.SecretFileSource{
					Name: credentialSecretName(ownerName),
					Key:  secretKeyForStaticCredentialProviderConfig,
				},
			},
			Permissions: "0600",
		},
	}
}

// generateCredentialsSecret generates a Secret containing the config for the image registry.
// The function needs the cluster name to add the required move and cluster name labels.
func generateCredentialsSecret(
	config providerConfig, clusterName, ownerName, namespace string,
) (*corev1.Secret, error) {
	if config.isCredentialsEmpty() {
		return nil, nil
	}

	staticCredentialProviderSecretContents, err := kubeletStaticCredentialProviderSecretContents(
		config,
	)
	if err != nil {
		return nil, err
	}
	secretData := map[string]string{
		secretKeyForStaticCredentialProviderConfig: staticCredentialProviderSecretContents,
	}

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      credentialSecretName(ownerName),
			Namespace: namespace,
			Labels:    newLabels(withMove(), withClusterName(clusterName)),
		},
		StringData: secretData,
		Type:       corev1.SecretTypeOpaque,
	}, nil
}

func kubeletStaticCredentialProviderSecretContents(config providerConfig) (string, error) {
	registryURL, err := url.ParseRequestURI(config.URL)
	if err != nil {
		return "", fmt.Errorf("failed parsing registry URL: %w", err)
	}

	templateInput := struct {
		RegistryHost string
		Username     string
		Password     string
	}{
		RegistryHost: registryURL.Host,
		Username:     config.Username,
		Password:     config.Password,
	}
	t, err := template.New("").Parse(string(staticCredentialProviderConfigPatch))
	if err != nil {
		return "", fmt.Errorf("failed to parse go template: %w", err)
	}

	var b bytes.Buffer
	err = t.Execute(&b, templateInput)

	if err != nil {
		return "", fmt.Errorf("failed executing template: %w", err)
	}
	return strings.TrimSpace(b.String()), nil
}

func credentialSecretName(ownerName string) string {
	return fmt.Sprintf("%s-registry-config", ownerName)
}

type labelFn func(labels map[string]string)

func newLabels(fs ...labelFn) map[string]string {
	labels := map[string]string{}
	for _, f := range fs {
		f(labels)
	}
	return labels
}

func withClusterName(clusterName string) labelFn {
	return func(labels map[string]string) {
		labels[clusterv1.ClusterNameLabel] = clusterName
	}
}

func withMove() labelFn {
	return func(labels map[string]string) {
		labels[clusterctlv1.ClusterctlMoveLabel] = ""
	}
}