package main

import (
	"context"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/advisor/mgmt/advisor"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/subscriptions"
	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/security/mgmt/security"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net/http"
	"regexp"
	"sync"
	"time"
)

var (
	prometheusSubscriptionInfo *prometheus.GaugeVec
	prometheusResourceGroupInfo *prometheus.GaugeVec
	prometheusSecuritycenterCompliance *prometheus.GaugeVec
	prometheusAdvisorRecommendations *prometheus.GaugeVec
	resourceGroupRegexp = regexp.MustCompile(`resourceGroups/(?P<resourceGroup>[^/]+)/?`)
)

// Create and setup metrics and collection
func initMetrics() {
	prometheusSubscriptionInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "azurerm_subscription_info",
			Help: "Azure Audit Subscription info",
		},
		[]string{"subscriptionID", "subscriptionName", "spendingLimit", "quotaID", "locationPlacementID"},
	)

	prometheusResourceGroupInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "azurerm_resourcegroup_info",
			Help: "Azure Audit ResourceGroup info",
		},
		[]string{"subscriptionID", "resourceGroup", "location"},
	)

	prometheusSecuritycenterCompliance = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "azurerm_securitycenter_compliance",
			Help: "Azure Audit SecurityCenter compliance status",
		},
		[]string{"subscriptionID", "assessmentType"},
	)

	prometheusAdvisorRecommendations = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "azurerm_advisor_recommendation",
			Help: "Azure Audit Advisor recommendation",
		},
		[]string{"subscriptionID", "category", "resourceType", "resourceName", "resourceGroup", "impact", "risk"},
	)

	prometheus.MustRegister(prometheusSubscriptionInfo)
	prometheus.MustRegister(prometheusResourceGroupInfo)
	prometheus.MustRegister(prometheusSecuritycenterCompliance)
	prometheus.MustRegister(prometheusAdvisorRecommendations)
}

func startMetricsCollection() {
	go func() {
		for {
			go func() {
				probeCollect()
			}()
			time.Sleep(opts.ScrapeTime)
		}
	}()
}

func startHttpServer() {
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(opts.ServerBind, nil))
}

func probeCollect() {
	var wg sync.WaitGroup
	context := context.Background()

	callbackChannel := make(chan func())

	for _, subscription := range AzureSubscriptions {
		// Subscription
		if opts.CollectSubscription {
			wg.Add(1)
			go func(subscriptionId string) {
				defer wg.Done()
				collectAzureSubscription(context, subscriptionId, callbackChannel)
				Logger.Verbose("subscription[%v]: finished Azure Subscription collection", subscriptionId)
			}(*subscription.SubscriptionID)
		}

		// ResourceGroups
		if opts.CollectResourceGroup {
			wg.Add(1)
			go func(subscriptionId string) {
				defer wg.Done()
				collectAzureResourceGroup(context, subscriptionId, callbackChannel)
				Logger.Verbose("subscription[%v]: finished Azure ResourceGroups collection", subscriptionId)
			}(*subscription.SubscriptionID)
		}

		// SecurityCompliance
		for _, location := range opts.AzureLocation {
			wg.Add(1)
			go func(subscriptionId, location string) {
				defer wg.Done()
				collectAzureSecurityCompliance(context, subscriptionId, location, callbackChannel)
				Logger.Verbose("subscription[%v]: finished Azure SecurityCompliance collection (%v)", subscriptionId, location)
			}(*subscription.SubscriptionID, location)
		}


		// AdvisorRecommendations
		wg.Add(1)
		go func(subscriptionId string) {
			defer wg.Done()
			collectAzureAdvisorRecommendations(context, subscriptionId, callbackChannel)
			Logger.Verbose("subscription[%v]: finished Azure AdvisorRecommendations collection", subscriptionId)
		}(*subscription.SubscriptionID)
	}

	// collect metrics (callbacks) and proceses them
	go func() {
		var callbackList []func()
		for callback := range callbackChannel {
			callbackList = append(callbackList, callback)
		}

		prometheusSubscriptionInfo.Reset()
		prometheusResourceGroupInfo.Reset()
		prometheusSecuritycenterCompliance.Reset()
		prometheusAdvisorRecommendations.Reset()
		for _, callback := range callbackList {
			callback()
		}

		Logger.Messsage("run: finished")
	}()

	// wait for all funcs
	wg.Wait()
	close(callbackChannel)
}

// Collect Azure Subscription metrics
func collectAzureSubscription(context context.Context, subscriptionId string, callback chan<- func()) {
	subscriptionClient := subscriptions.NewClient()
	subscriptionClient.Authorizer = AzureAuthorizer

	sub, err := subscriptionClient.Get(context, subscriptionId)
	if err != nil {
		panic(err)
	}

	infoLabels := prometheus.Labels{
		"subscriptionID": *sub.SubscriptionID,
		"subscriptionName": *sub.DisplayName,
		"spendingLimit": string(sub.SubscriptionPolicies.SpendingLimit),
		"quotaID": *sub.SubscriptionPolicies.QuotaID,
		"locationPlacementID": *sub.SubscriptionPolicies.LocationPlacementID,
	}

	callback <- func() {
		prometheusSubscriptionInfo.With(infoLabels).Set(1)
	}
}

// Collect Azure ResourceGroup metrics
func collectAzureResourceGroup(context context.Context, subscriptionId string, callback chan<- func()) {
	resourceGroupClient := resources.NewGroupsClient(subscriptionId)
	resourceGroupClient.Authorizer = AzureAuthorizer

	resourceGroupResult, err := resourceGroupClient.ListComplete(context, "", nil)
	if err != nil {
		panic(err)
	}

	for _, item := range *resourceGroupResult.Response().Value {
		infoLabels := prometheus.Labels{
			"subscriptionID": subscriptionId,
			"resourceGroup": *item.Name,
			"location": *item.Location,
		}

		callback <- func() {
			prometheusResourceGroupInfo.With(infoLabels).Set(1)
		}
	}
}

func collectAzureSecurityCompliance(context context.Context, subscriptionId, location string, callback chan<- func()) {
	subscriptionResourceId := fmt.Sprintf("/subscriptions/%v", subscriptionId)
	complianceClient := security.NewCompliancesClient(subscriptionResourceId, location)
	complianceClient.Authorizer = AzureAuthorizer

	complienceResult, err := complianceClient.Get(context, subscriptionResourceId, time.Now().Format("2006-01-02Z"))
	if err != nil {
		ErrorLogger.Error(fmt.Sprintf("subscription[%v]", subscriptionId), err)
		return
	}

	if complienceResult.AssessmentResult != nil {
		for _, result := range *complienceResult.AssessmentResult {
			segmentType := ""
			if result.SegmentType != nil {
				segmentType = *result.SegmentType
			}

			infoLabels := prometheus.Labels{
				"subscriptionID": subscriptionId,
				"assessmentType": segmentType,
			}
			infoValue := *result.Percentage

			callback <- func() {
				prometheusSecuritycenterCompliance.With(infoLabels).Add(infoValue)
			}
		}
	}
}

func collectAzureAdvisorRecommendations(context context.Context, subscriptionId string, callback chan<- func()) {
	advisorRecommendationsClient := advisor.NewRecommendationsClient(subscriptionId)
	advisorRecommendationsClient.Authorizer = AzureAuthorizer

	recommendationResult, err := advisorRecommendationsClient.ListComplete(context, "", nil, "")
	if err != nil {
		panic(err)
	}

	for _, item := range *recommendationResult.Response().Value {

		infoLabels := prometheus.Labels{
			"subscriptionID": subscriptionId,
			"category":       string(item.RecommendationProperties.Category),
			"resourceType":   *item.RecommendationProperties.ImpactedField,
			"resourceName":   *item.RecommendationProperties.ImpactedValue,
			"resourceGroup":  extractResourceGroupFromAzureId(*item.ID),
			"impact":         string(item.Impact),
			"risk":           string(item.Risk),
		}

		callback <- func() {
			prometheusAdvisorRecommendations.With(infoLabels).Add(1)
		}
	}
}
