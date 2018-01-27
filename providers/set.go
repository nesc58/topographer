package providers

import (
	"net"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/9seconds/topographer/config"
)

const updateAttempts = 3

type ProviderSet struct {
	providers map[string]GeoProvider
	weights   map[string]float64
}

func (ps *ProviderSet) Update(force bool) {
	for k := range ps.providers {
		go ps.updateProvider(force, k, 0)
	}
}

func (ps *ProviderSet) updateProvider(force bool, name string, attempt int) {
	provider := ps.providers[name]

	log.WithFields(log.Fields{
		"provider": name,
		"attempt":  attempt,
	}).Info("Update provider.")

	needToUpdate, err := provider.Update()
	if err != nil {
		log.WithFields(log.Fields{
			"provider": name,
			"attempt":  attempt,
			"error":    err.Error(),
		}).Warn("Cannot update provider")
	} else {
		if needToUpdate || force {
			log.WithFields(log.Fields{
				"provider": name,
				"attempt":  attempt,
			}).Info("Reopening database")

			if err = provider.Reopen(time.Now()); err != nil {
				log.WithFields(log.Fields{
					"provider": name,
					"attempt":  attempt,
					"error":    err.Error(),
				}).Error("Cannot reopen provider database!")
			} else {
				log.WithFields(log.Fields{
					"provider": name,
					"attempt":  attempt,
				}).Info("Database refreshed.")
				return
			}
		} else {
			return
		}
	}

	if attempt < updateAttempts {
		time.Sleep(time.Duration(attempt+1) * time.Minute)
		ps.updateProvider(force, name, attempt+1)
	}
}

func (ps *ProviderSet) Resolve(ips []net.IP) []ResolveResult {
	var wg sync.WaitGroup
	results := make([]ResolveResult, 0, len(ps.providers))
	channel := make(chan ResolveResult, len(ps.providers))
	defer close(channel)

	resultsCount := 0
	for _, v := range ps.providers {
		wg.Add(1)
		resultsCount += 1

		go func(provider GeoProvider) {
			defer wg.Done()

			result := provider.Resolve(ips)
			result.Weight = ps.weights[result.Provider]
			channel <- result
		}(v)
	}

	wg.Wait()
	for i := 0; i < resultsCount; i++ {
		results = append(results, <-channel)
	}

	return results
}

func NewProviderSet(conf *config.Config) ProviderSet {
	set := ProviderSet{
		providers: make(map[string]GeoProvider),
		weights:   make(map[string]float64),
	}

	for k, v := range conf.Databases {
		if v.Enabled {
			switch k {
			case "maxmind":
				set.providers["maxmind"] = NewMaxMind(conf)
			case "dbip":
				set.providers["dbip"] = NewDBIP(conf)
			case "sypex":
				set.providers["sypex"] = NewSypex(conf)
			case "ip2location":
				set.providers["ip2location"] = NewIP2Location(conf)
			}
			set.weights[k] = v.Weight
		}
	}

	return set
}
