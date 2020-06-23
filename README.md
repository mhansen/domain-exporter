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

Then navigate to http://localhost:10550/listings?suburb=Pyrmont

## Building with docker

```shell
$ docker build .
```

## Querying with Prometheus

Example Prometheus config for querying:

```yaml
scrape_configs:
  - job_name: 'domain_exporter'
    static_configs:
      - targets:
        - 'domain_exporter:10550'
  - job_name: 'domain_exporter_listings'
    # Only 500 requests per day allowed, so don't query too often!
    scrape_interval: 2h
    metrics_path: "/listings"
    params:
      state: ["NSW"]
    # https://github.com/prometheus/prometheus/issues/4885#issuecomment-515144405
    static_configs:
      - targets:
        - Alexandria
        - Pyrmont
        - Glebe
        - Redfern
        - Newtown
        - Surry Hills
        - Ultimo
        - Waterloo
    relabel_configs:
      # Convert the 'targets' list given above to a ?suburb=Pyrmont URL param
      - source_labels: [__address__]
        target_label: __param_suburb
      # Send requests to the real hostname:port
      - target_label: __address__
        replacement: domain_exporter:10550
```

## Caveats

* Domain API will only return a max of 1000 results per search. If you want
  more than that, consider splitting your search into multiple JSON files.
