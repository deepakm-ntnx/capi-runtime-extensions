// Copyright 2023 D2iQ, Inc. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	logsv1 "k8s.io/component-base/logs/api/v1"
	"k8s.io/component-base/version/verflag"
	"k8s.io/klog/v2"
	capiv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	crsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/d2iq-labs/capi-runtime-extensions/common/pkg/capi/clustertopology/handlers/mutation"
	"github.com/d2iq-labs/capi-runtime-extensions/common/pkg/server"
	"github.com/d2iq-labs/capi-runtime-extensions/pkg/handlers/auditpolicy"
	"github.com/d2iq-labs/capi-runtime-extensions/pkg/handlers/clusterconfig"
	"github.com/d2iq-labs/capi-runtime-extensions/pkg/handlers/cni/calico"
	"github.com/d2iq-labs/capi-runtime-extensions/pkg/handlers/etcd"
	"github.com/d2iq-labs/capi-runtime-extensions/pkg/handlers/extraapiservercertsans"
	"github.com/d2iq-labs/capi-runtime-extensions/pkg/handlers/httpproxy"
	"github.com/d2iq-labs/capi-runtime-extensions/pkg/handlers/kubernetesimagerepository"
	"github.com/d2iq-labs/capi-runtime-extensions/pkg/handlers/servicelbgc"
)

// Flags.
var logOptions = logs.NewOptions()

// initFlags initializes the flags.
func initFlags(fs *pflag.FlagSet) {
	// Initialize logs flags using Kubernetes component-base machinery.
	logs.AddFlags(fs, logs.SkipLoggingConfigurationFlags())
	logsv1.AddFlags(logOptions, fs)
}

func main() {
	// Creates a logger to be used during the main func.
	setupLog := ctrl.Log.WithName("main")

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(crsv1.AddToScheme(scheme))
	utilruntime.Must(capiv1.AddToScheme(scheme))

	mgrOptions := &ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: ":8080",
		},
		HealthProbeBindAddress: ":8081",
		LeaderElection:         false,
	}

	pflag.CommandLine.StringVar(
		&mgrOptions.Metrics.BindAddress,
		"metrics-bind-address",
		mgrOptions.Metrics.BindAddress,
		"The address the metric endpoint binds to.",
	)

	pflag.CommandLine.StringVar(
		&mgrOptions.HealthProbeBindAddress,
		"health-probe-bind-address",
		mgrOptions.HealthProbeBindAddress,
		"The address the probe endpoint binds to.",
	)

	pflag.CommandLine.StringVar(&mgrOptions.PprofBindAddress, "profiler-address", "",
		"Bind address to expose the pprof profiler (e.g. localhost:6060)")

	calicoCNIConfig := &calico.CalicoCNIConfig{}

	runtimeWebhookServerOpts := server.NewServerOptions()

	// Initialize and parse command line flags.
	initFlags(pflag.CommandLine)
	runtimeWebhookServerOpts.AddFlags(pflag.CommandLine)
	calicoCNIConfig.AddFlags("calicocni", pflag.CommandLine)
	pflag.CommandLine.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	verflag.PrintAndExitIfRequested()

	// Validates logs flags using Kubernetes component-base machinery and applies them
	if err := logsv1.ValidateAndApply(logOptions, nil); err != nil {
		setupLog.Error(err, "unable to start extension")
		os.Exit(1)
	}

	// Add the klog logger in the context.
	ctrl.SetLogger(klog.Background())

	signalCtx := ctrl.SetupSignalHandler()

	mgr, err := newManager(mgrOptions)
	if err != nil {
		setupLog.Error(err, "failed to create a new controller manager")
		os.Exit(1)
	}

	runtimeWebhookServer := server.NewServer(
		runtimeWebhookServerOpts,

		servicelbgc.New(mgr.GetClient()),

		calico.New(mgr.GetClient(), calicoCNIConfig, clusterconfig.VariableName, "cni"),

		httpproxy.NewVariable(),
		httpproxy.NewPatch(mgr.GetClient(), httpproxy.VariableName),

		extraapiservercertsans.NewVariable(),
		extraapiservercertsans.NewPatch(extraapiservercertsans.VariableName),

		auditpolicy.NewPatch(),

		kubernetesimagerepository.NewVariable(),
		kubernetesimagerepository.NewPatch(kubernetesimagerepository.VariableName),

		etcd.NewVariable(),
		etcd.NewPatch(etcd.VariableName),

		clusterconfig.NewVariable(),
		mutation.NewMetaGeneratePatchesHandler(
			"clusterConfigPatch",
			httpproxy.NewPatch(mgr.GetClient(), clusterconfig.VariableName, httpproxy.VariableName),
			extraapiservercertsans.NewPatch(
				clusterconfig.VariableName,
				extraapiservercertsans.VariableName,
			),
			auditpolicy.NewPatch(),
			kubernetesimagerepository.NewPatch(
				clusterconfig.VariableName,
				kubernetesimagerepository.VariableName,
			),
			etcd.NewPatch(clusterconfig.VariableName, etcd.VariableName),
		),
	)
	if err := mgr.Add(runtimeWebhookServer); err != nil {
		setupLog.Error(err, "unable to add runtime webhook server runnable to controller manager")
		os.Exit(1)
	}

	if err := mgr.Start(signalCtx); err != nil {
		setupLog.Error(err, "unable to start controller manager")
		os.Exit(1)
	}
}

func newManager(opts *manager.Options) (ctrl.Manager, error) {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), *opts)
	if err != nil {
		return nil, fmt.Errorf("unable to create manager: %w", err)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("unable to set up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("unable to set up ready check: %w", err)
	}

	return mgr, nil
}
