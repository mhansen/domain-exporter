package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	addr   = flag.String("listen", ":10550", "Address to listen on")
	apiKey = flag.String("api_key", "", "API key")
)

func main() {
	flag.Parse()
	if *apiKey == "" {
		log.Fatalf("--api_key flag required")
	}
	log.Printf("Exporter starting on addr %s", *addr)
	reg := prometheus.NewPedanticRegistry()
	c := &http.Client{}
	dc := domainCollector{dc: &domainClient{c: c}}
	reg.MustRegister(
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		prometheus.NewGoCollector(),
		dc,
	)

	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatal(err)
	}
}

type domainCollector struct {
	dc *domainClient
}

func (kc domainCollector) Describe(ch chan<- *prometheus.Desc) {
	// Don't describe at startup, the API calls are too expensive.
	// prometheus.DescribeByCollect(kc, ch)
}

type domainClient struct {
	c *http.Client
}

func (dc domainClient) SearchResidentialPage(rsr ResidentialSearchRequest) ([]SearchResult, error) {
	rsrJSON, err := json.Marshal(rsr)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", "https://api.domain.com.au/v1/listings/residential/_search", bytes.NewBuffer(rsrJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Add("X-Api-Key", *apiKey)
	req.Header.Add("accept", "application/json")
	log.Printf("making request for page #%v: %v", rsr.PageNumber, req.URL)
	resp, err := dc.c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to %v failed: %v", req.URL.String(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		log.Print(string(b))
		return nil, fmt.Errorf("got non-200 code: %v, %v", resp.StatusCode, resp.Status)
	}

	listingsPage := []SearchResult{}
	err = json.NewDecoder(resp.Body).Decode(&listingsPage)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse json: %v", err)
	}
	log.Printf("got %v listings", len(listingsPage))
	return listingsPage, nil
}

func (dc domainClient) SearchResidential(rsr ResidentialSearchRequest) ([]SearchResult, error) {
	rsr.PageSize = 200
	rsr.PageNumber = 0
	listings := []SearchResult{}
	for {
		listingsPage, err := dc.SearchResidentialPage(rsr)
		if err != nil {
			return nil, err
		}
		if len(listingsPage) == 0 {
			break
		}
		listings = append(listings, listingsPage...)
		rsr.PageNumber++
	}
	return listings, nil
}

func (kc domainCollector) Collect(ch chan<- prometheus.Metric) {
	matches, err := filepath.Glob("searches/*.json")
	if err != nil {
		log.Printf("failed to glob for json: %v\n", err)
		// No way to effectively return errors from Collect.
		return
	}

	listingCount := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "domain_listing_count",
		},
		[]string{"propertytype", "suburb", "postcode", "bedrooms", "bathrooms", "carspaces"},
	)

	for _, m := range matches {
		f, err := os.Open(m)
		if err != nil {
			log.Printf("couldn't open search json: %v\n", err)
			// No need to quit, just try the next one
			continue
		}
		var rsr ResidentialSearchRequest
		json.NewDecoder(f).Decode(&rsr)

		listings, err := kc.dc.SearchResidential(rsr)
		if err != nil {
			log.Printf("error searching domain for %+v: %v\n", rsr, err)
			// No need to quit, just try the next one.
			continue
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
	}

	listingCount.Collect(ch)
}

type LocationFilter struct {
	State                     string `json:"state"`
	Region                    string `json:"region"`
	Area                      string `json:"area"`
	Suburb                    string `json:"suburb"`
	PostCode                  string `json:"postCode"`
	IncludeSurroundingSuburbs bool   `json:"includeSurroundingSuburbs"`
}

type ResidentialSearchRequest struct {
	ListingType  string `json:"listingType"`
	MinBedrooms  int    `json:"minBedrooms"`
	MinBathrooms int    `json:"minBathrooms"`
	MinCarspaces int    `json:"minCarspaces"`
	PageSize     int    `json:"pageSize"`
	PageNumber   int    `json:"pageNumber"`
	Locations    []LocationFilter
}

// Listing https://developer.domain.com.au/docs/latest/apis/pkg_agents_listings/references/listings_detailedresidentialsearch
type SearchResult struct {
	Listing PropertyListing `json:"listing"`
}

type PropertyListing struct {
	PropertyDetails PropertyDetails `json:"propertyDetails"`
}

type PropertyDetails struct {
	State        string  `json:"state"`
	PropertyType string  `json:"propertyType"`
	Bathrooms    float32 `json:"bathrooms"`
	Bedrooms     float32 `json:"bedrooms"`
	CarSpaces    int32   `json:"carspaces"`
	Suburb       string  `json:"suburb"`
	Postcode     string  `json:"postcode"`
}
