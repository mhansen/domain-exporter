# Domain exporter

This allows monitoring domain.com.au listings with Prometheus.

## API Key

Grab an API key from https://developer.domain.com.au/. Make a project and add
an API key. The free API keys only support 500 queries per day, so don't query
often!

## Building and running

Edit the searches in `searches/*.json` to be searches that look like the domain
[`/v1/listings/residential/_search`](https://developer.domain.com.au/docs/latest/apis/pkg_agents_listings/references/listings_detailedresidentialsearch)
request schema.

```bash
$ go build .
$ ./domain_exporter --api_key=<domain api key>
```

Then navigate to http://localhost:10550/

## Building with docker

```shell
$ docker build .
```

## Querying with Prometheus

Example Prometheus config for querying:

```yaml
scrape_configs:
  - job_name: 'domain_exporter'
    # Only 500 requests per day allowed, so don't query too often!
    scrape_interval: 1h
    static_configs:
      - targets:
        - 'localhost:10550'
```
