package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"

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

	dc := domainClient{c: c}
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
	log.Printf("making request for page #%v: %v, %+v", rsr.PageNumber, req.URL, rsr)
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

	// Domain returns an error: "Cannot page beyond 1000 records" if you try to.
	for len(listings) < 1000 {
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

func (dc domainClient) domainHandler(w http.ResponseWriter, r *http.Request) {
	reg := prometheus.NewPedanticRegistry()
	listingCount := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "domain_listing_count",
		},
		[]string{"propertytype", "suburb", "postcode", "bedrooms", "bathrooms", "carspaces"},
	)
	reg.MustRegister(listingCount)
	rsr := ResidentialSearchRequest{
		ListingType:  "Rent",
		MinBathrooms: 0,
		MinBedrooms:  0,
		MinCarspaces: 0,
		Locations: []LocationFilter{
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
