package main

import (
	"log"
	"time"
	"regexp"
	"context"
	"net/http"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/advisor/mgmt/advisor"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/subscriptions"
	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/security/mgmt/security"
)

var (
	prometheusSubscriptionInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "azureaudit_subscription_info",
			Help: "Azure Audit Subscription info",
		},
		[]string{"subscriptionID", "subscriptionName", "spendingLimit", "quotaID", "locationPlacementID"},
	)

	prometheusResourceGroupInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "azureaudit_resourcegroup_info",
			Help: "Azure Audit ResourceGroup info",
		},
		[]string{"subscriptionID", "resourceGroup", "location"},
	)

	prometheusSecuritycenterCompliance = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "azureaudit_securitycenter_compliance",
			Help: "Azure Audit SecurityCenter compliance status",
		},
		[]string{"subscriptionID", "assessmentType"},
	)

	prometheusAdvisorRecommendations = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "azureaudit_advisor_recommendation",
			Help: "Azure Audit Advisor recommendation",
		},
		[]string{"subscriptionID", "category", "resourceType", "resourceName", "resourceGroup", "impact", "risk"},
	)

	resourceGroupRegexp = regexp.MustCompile(`resourceGroups/(?P<resourceGroup>[^/]+)/?`)
)

func initMetrics() {
	prometheus.MustRegister(prometheusSubscriptionInfo)
	prometheus.MustRegister(prometheusResourceGroupInfo)
	prometheus.MustRegister(prometheusSecuritycenterCompliance)
	prometheus.MustRegister(prometheusAdvisorRecommendations)

	go func() {
		for {
			go func() {
				probeCollect()
			}()
			time.Sleep(time.Duration(opts.ScrapeTime) * time.Second)
		}
	}()
}

func startHttpServer() {
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(opts.ServerBind, nil))
}

func probeCollect() {
	context := context.Background()

	prometheusSubscriptionInfo.Reset()
	prometheusResourceGroupInfo.Reset()
	prometheusSecuritycenterCompliance.Reset()
	prometheusAdvisorRecommendations.Reset()

	for _, subscription := range AzureSubscriptions {

		//---------------------------------------
		// Subscription
		//---------------------------------------

		subscriptionClient := subscriptions.NewClient()
		subscriptionClient.Authorizer = AzureAuthorizer

		sub, err := subscriptionClient.Get(context, *subscription.SubscriptionID)
		if err != nil {
			panic(err)
		}

		prometheusSubscriptionInfo.With(
			prometheus.Labels{
				"subscriptionID": *sub.SubscriptionID,
				"subscriptionName": *sub.DisplayName,
				"spendingLimit": string(sub.SubscriptionPolicies.SpendingLimit),
				"quotaID": *sub.SubscriptionPolicies.QuotaID,
				"locationPlacementID": *sub.SubscriptionPolicies.LocationPlacementID,
			},
		).Set(1)

		//---------------------------------------
		// ResourceGroup
		//---------------------------------------


		resourceGroupClient := resources.NewGroupsClient(*subscription.SubscriptionID)
		resourceGroupClient.Authorizer = AzureAuthorizer

		resourceGroupResult, err := resourceGroupClient.ListComplete(context, "", nil)
		if err != nil {
			panic(err)
		}


		for _, item := range *resourceGroupResult.Response().Value {
			prometheusResourceGroupInfo.With(prometheus.Labels{
				"subscriptionID": *sub.SubscriptionID,
				"resourceGroup": *item.Name,
				"location": *item.Location,
			}).Set(1)
		}

		//---------------------------------------
		// Security Complience
		//---------------------------------------

		complianceClient := security.NewCompliancesClient(*subscription.SubscriptionID, "westeurope")
		complianceClient.Authorizer = AzureAuthorizer

		complienceResult, err := complianceClient.Get(context, *subscription.ID, time.Now().Format("2006-01-02Z"))
		if err != nil {
			panic(err)
		}

		if complienceResult.AssessmentResult != nil {
			for _, itm := range *complienceResult.AssessmentResult {

				segmentType := ""
				if itm.SegmentType != nil {
					segmentType = *itm.SegmentType
				}

				prometheusSecuritycenterCompliance.With(prometheus.Labels{
					"subscriptionID": *sub.SubscriptionID,
					"assessmentType": segmentType,
				}).Add(*itm.Percentage)
			}
		}

		//---------------------------------------
		// Advisor Recommendations
		//---------------------------------------

		advisorRecommendationsClient := advisor.NewRecommendationsClient(*subscription.SubscriptionID)
		advisorRecommendationsClient.Authorizer = AzureAuthorizer

		recommendationResult, err := advisorRecommendationsClient.ListComplete(context, "", nil, "")
		if err != nil {
			panic(err)
		}

		for _, item := range *recommendationResult.Response().Value {
			resourceGroupName := ""
			rgMatch := resourceGroupRegexp.FindStringSubmatch(*item.ID)
			if len(rgMatch) > 0 {
				resourceGroupName = rgMatch[1]
			}

			prometheusAdvisorRecommendations.With(prometheus.Labels{
				"subscriptionID": *sub.SubscriptionID,
				"category": string(item.RecommendationProperties.Category),
				"resourceType": *item.RecommendationProperties.ImpactedField,
				"resourceName": *item.RecommendationProperties.ImpactedValue,
				"resourceGroup": resourceGroupName,
				"impact": string(item.Impact),
				"risk": string(item.Risk),
			}).Add(1)
		}

	}
}
