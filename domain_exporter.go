package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/mhansen/domain"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	phttp "github.com/travelaudience/go-promhttp"
)

var (
	addr   = flag.String("listen", ":10550", "Address to listen on")
	apiKey = flag.String("api_key", "", "API key")
	index  = template.Must(template.New("index").Parse(
		`<!doctype html>
<title>Domain Exporter</title>
<h1>Domain Exporter</h1>
<a href="/metrics">Metrics</a>`))
)

func main() {
	flag.Parse()
	if *apiKey == "" {
		log.Fatalf("--api_key flag required")
	}
	log.Printf("Exporter starting on addr %s", *addr)
	reg := prometheus.NewPedanticRegistry()
	phttpClient := &phttp.Client{
		Client:     http.DefaultClient,
		Registerer: reg,
	}
	c, err := phttpClient.ForRecipient("domain")
	if err != nil {
		log.Fatalf("could not create http client: %v\n", err)
	}

	dc := domainCollector{domain.NewClient(c, *apiKey)}
	reg.MustRegister(
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		prometheus.NewGoCollector(),
	)

	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	http.HandleFunc("/listings", dc.domainHandler)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		err := index.Execute(w, nil)
		if err != nil {
			log.Println(err)
		}
	})
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatal(err)
	}
}

type domainCollector struct {
	*domain.Client
}

func (dc domainCollector) domainHandler(w http.ResponseWriter, r *http.Request) {
	reg := prometheus.NewPedanticRegistry()
	listingCount := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "domain_listing_count",
		},
		[]string{"propertytype", "suburb", "postcode", "bedrooms", "bathrooms", "carspaces"},
	)
	reg.MustRegister(listingCount)
	rsr := domain.ResidentialSearchRequest{
		ListingType:  "Rent",
		MinBathrooms: 0,
		MinBedrooms:  0,
		MinCarspaces: 0,
		Locations: []domain.LocationFilter{
			{
				State:                     r.URL.Query().Get("state"),
				Area:                      "",
				Region:                    "",
				Suburb:                    r.URL.Query().Get("suburb"),
				PostCode:                  r.URL.Query().Get("postCode"),
				IncludeSurroundingSuburbs: false,
			},
		},
	}
	listings, err := dc.SearchResidential(rsr)
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "error searching domain: %v", err)
		log.Printf("error searching domain for %+v: %v\n", rsr, err)
		return
	}
	for _, l := range listings {
		listingCount.WithLabelValues(
			l.Listing.PropertyDetails.PropertyType,
			l.Listing.PropertyDetails.Suburb,
			l.Listing.PropertyDetails.Postcode,
			fmt.Sprintf("%.1f", l.Listing.PropertyDetails.Bedrooms),
			fmt.Sprintf("%.1f", l.Listing.PropertyDetails.Bathrooms),
			fmt.Sprintf("%v", l.Listing.PropertyDetails.CarSpaces),
		).Inc()
	}

	h := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
}
