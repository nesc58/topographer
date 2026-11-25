package providers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"sync"

	"github.com/nesc58/topographer/topolib"

	"github.com/oschwald/maxminddb-golang"
)

const maxmindBaseFileName = "database.mmdb"

type maxmindLookupResult struct {
	City struct {
		Names struct {
			En string `maxminddb:"en"`
		} `maxminddb:"names"`
	} `maxminddb:"city"`
	Country struct {
		IsoCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
}

type maxmindBase struct {
	dbReader     *maxminddb.Reader
	dbReaderLock sync.RWMutex
}

func (m *maxmindBase) IsOpened() bool {
	return m.dbReader != nil
}

func (m *maxmindBase) Shutdown() {
	m.dbReaderLock.Lock()
	defer m.dbReaderLock.Unlock()

	if m.dbReader != nil {
		// Shutdown cannot return close errors through the OfflineProvider interface.
		_ = m.dbReader.Close()
		m.dbReader = nil
	}
}

func (m *maxmindBase) Open(path string) error {
	m.dbReaderLock.Lock()
	defer m.dbReaderLock.Unlock()

	reader, err := maxminddb.Open(filepath.Join(path, maxmindBaseFileName))
	if err != nil {
		return fmt.Errorf("cannot initialize a reader of maxminddb: %w", err)
	}

	if m.dbReader != nil {
		if err := m.dbReader.Close(); err != nil {
			return errors.Join(
				fmt.Errorf("cannot close the current maxminddb reader: %w", err),
				reader.Close(),
			)
		}
	}

	m.dbReader = reader

	return nil
}

func (m *maxmindBase) Lookup(ctx context.Context, ip net.IP) (topolib.ProviderLookupResult, error) {
	m.dbReaderLock.RLock()
	defer m.dbReaderLock.RUnlock()

	rv := topolib.ProviderLookupResult{}

	if m.dbReader == nil {
		return rv, ErrDatabaseIsNotReadyYet
	}

	record := maxmindLookupResult{}

	if err := m.dbReader.Lookup(ip, &record); err != nil {
		return rv, fmt.Errorf("cannot lookup this ip address: %w", err)
	}

	rv.CountryCode = topolib.Alpha2ToCountryCode(record.Country.IsoCode)
	rv.City = record.City.Names.En

	return rv, nil
}
