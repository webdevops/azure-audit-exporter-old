Azure Audit Exporter
====================

[![license](https://img.shields.io/github/license/mblaschke/azure-audit-exporter.svg)](https://github.com/mblaschke/azure-audit-exporter/blob/master/LICENSE)
[![Docker](https://img.shields.io/badge/docker-mblaschke%2Fazure--audit--exporter-blue.svg?longCache=true&style=flat&logo=docker)](https://hub.docker.com/r/mblaschke/azure-audit-exporter/)
[![Docker Build Status](https://img.shields.io/docker/build/mblaschke/azure-audit-exporter.svg)](https://hub.docker.com/r/mblaschke/azure-audit-exporter/)

Prometheus exporter for Azure Audit informations.

Configuration
-------------

Normally no configuration is needed but can be customized using environment variables.

| Environment variable     | DefaultValue                | Description                                                       |
|--------------------------|-----------------------------|-------------------------------------------------------------------|
| `AZURE_SUBSCRIPTION_ID`  | `empty`                     | Azure Subscription IDs (empty for auto lookup)                    |
| `AZURE_LOCATION`         | `westeurope`, `northeurope` | Azure location for usage statitics                                |
| `SCRAPE_TIME`            | `120`                       | Time between API calls                                            |
| `SERVER_BIND`            | `:8080`                     | IP/Port binding                                                   |

for Azure API authentication (using ENV vars) see https://github.com/Azure/azure-sdk-for-go#authentication
