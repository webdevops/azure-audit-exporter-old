package main

import (
	"os"
	"fmt"
	"context"
	"github.com/jessevdk/go-flags"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/subscriptions"
	"time"
)

const (
	Author  = "webdevops.io"
	Version = "0.3.0"
)

var (
	argparser          *flags.Parser
	args               []string
	Logger             *DaemonLogger
	ErrorLogger        *DaemonLogger
	AzureAuthorizer    autorest.Authorizer
	AzureSubscriptions []subscriptions.Subscription
)

var opts struct {
	// general settings
	Verbose     []bool `         long:"verbose" short:"v"      env:"VERBOSE"                              description:"Verbose mode"`

	// server settings
	ServerBind  string `         long:"bind"                   env:"SERVER_BIND"                           description:"Server address"                                   default:":8080"`
	ScrapeTime  time.Duration `  long:"scrape-time"            env:"SCRAPE_TIME"                           description:"Scrape time (time.duration)"                      default:"5m"`

	// azure settings
	AzureSubscription []string ` long:"azure-subscription"     env:"AZURE_SUBSCRIPTION_ID"   env-delim:" " description:"Azure subscription ID"`
	AzureLocation []string `     long:"azure-location"         env:"AZURE_LOCATION"          env-delim:" " description:"Azure locations" default:"westeurope" default:"northeurope"`

	CollectSubscription bool `   long:"collect-subscription"   env:"COLLECT_SUBSCRIPTION"                  description:"Collect subscription metrics (standalone if azure_resourcemanager_exporter is not used)"`
	CollectResourceGroup bool `  long:"collect-resourcegroup"  env:"COLLECT_RESOURCEGROUP"                 description:"Collect resourcegroup metrics (standalone if azure_resourcemanager_exporter is not used)"`}

func main() {
	initArgparser()

	// Init logger
	Logger = CreateDaemonLogger(0)
	ErrorLogger = CreateDaemonErrorLogger(0)

	// set verbosity
	Verbose = len(opts.Verbose) >= 1

	Logger.Messsage("Init Azure Audit exporter v%s (written by %v)", Version, Author)

	Logger.Messsage("Init Azure connection")
	initAzureConnection()

	Logger.Messsage("Starting metrics collection")
	Logger.Messsage("  scape time: %v", opts.ScrapeTime)
	initMetrics()
	startMetricsCollection()

	Logger.Messsage("Starting http server on %s", opts.ServerBind)
	startHttpServer()
}

func initArgparser() {
	argparser = flags.NewParser(&opts, flags.Default)
	_, err := argparser.Parse()

	// check if there is an parse error
	if err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		} else {
			fmt.Println(err)
			fmt.Println()
			argparser.WriteHelp(os.Stdout)
			os.Exit(1)
		}
	}
}

func initAzureConnection() {
	var err error
	ctx := context.Background()

	// azure authorizer
	AzureAuthorizer, err = auth.NewAuthorizerFromEnvironment()
	if err != nil {
		panic(err)
	}
	subscriptionsClient := subscriptions.NewClient()
	subscriptionsClient.Authorizer = AzureAuthorizer

	if len(opts.AzureSubscription) == 0 {
		listResult, err := subscriptionsClient.List(ctx)
		if err != nil {
			panic(err)
		}
		AzureSubscriptions = listResult.Values()
	} else {
		AzureSubscriptions = []subscriptions.Subscription{}
		for _, subId := range opts.AzureSubscription {
			result, err := subscriptionsClient.Get(ctx, subId)
			if err != nil {
				panic(err)
			}
			AzureSubscriptions = append(AzureSubscriptions, result)
		}
	}
}
