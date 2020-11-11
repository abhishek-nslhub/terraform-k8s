package workspace

import (
	"context"
	"fmt"
	"os"
	"sync"

	flag "github.com/spf13/pflag"

	tfc "github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-k8s/pkg/apis"
	"github.com/mitchellh/cli"
	"github.com/operator-framework/operator-lib/leader"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

var (
	metricsHost               = "0.0.0.0"
	metricsPort         int32 = 8383
	operatorMetricsPort int32 = 8686
	log                       = ctrl.Log.Logger.WithName("operator")
)

// Command is the command for syncing the K8S and Terraform
// Cloud workspaces.
type Command struct {
	UI cli.Ui

	flags                 *flag.FlagSet
	flagLogLevel          string
	flagK8sWatchNamespace string

	tfcClient *tfc.Client

	once  sync.Once
	sigCh chan os.Signal
	help  string
}

func (c *Command) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.StringVar(&c.flagK8sWatchNamespace, "k8s-watch-namespace", metav1.NamespaceAll,
		"The Kubernetes namespace to watch for service changes and sync to Terraform Cloud. "+
			"If this is not set then it will default to all namespaces.")

	c.help = fmt.Sprintf("%s\n%s", help, c.flags.FlagUsages())

	flag.CommandLine.AddFlagSet(c.flags)
	flag.Parse()

	ctrl.SetLogger(zap.New())
}

// Run starts the operator to synchronize workspaces.
func (c *Command) Run(args []string) int {
	c.init()

	namespace := c.flagK8sWatchNamespace

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "")
		return 1
	}

	ctx := context.TODO()
	// Become the leader before proceeding
	err = leader.Become(ctx, "workspace-lock")
	if err != nil {
		log.Error(err, "")
		return 1
	}

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(cfg, manager.Options{
		Namespace:          namespace,
		MapperProvider:     apiutil.NewDiscoveryRESTMapper,
		MetricsBindAddress: fmt.Sprintf("%s:%d", metricsHost, metricsPort),
	})
	if err != nil {
		log.Error(err, "")
		return 1
	}

	log.Info("Registering Components.")

	// Setup Scheme for all resources
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "")
		return 1
	}

	log.Info("Starting the Cmd.")

	// Start the Cmd
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "Manager exited non-zero")
		return 1
	}

	return 0
}

func (c *Command) Synopsis() string { return synopsis }
func (c *Command) Help() string {
	c.once.Do(c.init)
	return c.help
}

const synopsis = "Sync Workspace and Terraform Cloud."
const help = `
Usage: terraform-k8s sync-workspace [options]

  Sync K8s TFC Workspace resource with Terraform Cloud.
	This enables Workspaces in Kubernetes to manage infrastructure resources
	created by Terraform Cloud.
`
