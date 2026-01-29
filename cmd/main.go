package main

import (
	"flag"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	watcherv1 "github.com/alex.osumi/watcher/api/v1"
	"github.com/alex.osumi/watcher/internal/controller"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(watcherv1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var leaderElectionID string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":9090", "The address the metric endpoint binds to. Use '0' to disable.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager.")
	flag.StringVar(&leaderElectionID, "leader-election-id", "watcher-controller", "The name of the leader election ID.")
	
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Get log level from environment
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel != "" {
		setupLog.Info("Log level set from environment", "level", logLevel)
		switch logLevel {
		case "debug":
			opts.Development = true
			opts.Level = nil // Show all logs including V(1)
		case "info":
			opts.Development = false
			// Set level to hide V(1) debug logs
		case "error":
			opts.Development = false
		}
		ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	}

	// Check Kubernetes version
	config := ctrl.GetConfigOrDie()
	discoveryClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		setupLog.Error(err, "unable to create discovery client")
		os.Exit(1)
	}
	
	serverVersion, err := discoveryClient.Discovery().ServerVersion()
	if err != nil {
		setupLog.Error(err, "unable to get server version")
		os.Exit(1)
	}
	
	// Parse version (format: v1.33.0)
	var major, minor int
	if _, err := fmt.Sscanf(serverVersion.GitVersion, "v%d.%d", &major, &minor); err != nil {
		setupLog.Info("Warning: unable to parse Kubernetes version, skipping version check", "version", serverVersion.GitVersion)
	} else if major < 1 || (major == 1 && minor < 33) {
		setupLog.Error(fmt.Errorf("kubernetes version too old"), "Watcher operator requires Kubernetes 1.33 or greater", "current", serverVersion.GitVersion)
		os.Exit(1)
	}
	
	setupLog.Info("Kubernetes version check passed", "version", serverVersion.GitVersion)

	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme:                 scheme,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       leaderElectionID,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	k8sClient, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		setupLog.Error(err, "unable to create kubernetes client")
		os.Exit(1)
	}

	metricsClient, err := metricsclientset.NewForConfig(mgr.GetConfig())
	if err != nil {
		setupLog.Error(err, "unable to create metrics client")
		os.Exit(1)
	}

	if err = (&controller.WatcherReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		K8sClient:     k8sClient,
		MetricsClient: metricsClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Watcher")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}