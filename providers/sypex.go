package providers

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	sypex "github.com/night-codes/go-sypexgeo"

	"github.com/nesc58/topographer/topolib"
)

const (
	sypexDBURL    = "https://sypexgeo.net/files/SxGeoCity_utf8.zip"
	sypexFileName = "SxGeoCity.dat"
)

type sypexProvider struct {
	db            *sypex.SxGEO
	dbMutex       sync.RWMutex
	baseDirectory string
	updateEvery   time.Duration
	httpClient    topolib.HTTPClient
}

func (s *sypexProvider) Name() string {
	return NameSypex
}

func (s *sypexProvider) UpdateEvery() time.Duration {
	return s.updateEvery
}

func (s *sypexProvider) BaseDirectory() string {
	return s.baseDirectory
}

func (s *sypexProvider) IsOpened() bool {
	return s.db != nil
}

func (sx *sypexProvider) Lookup(ctx context.Context, ip net.IP) (topolib.ProviderLookupResult, error) {
	result := topolib.ProviderLookupResult{}

	sx.dbMutex.RLock()
	defer sx.dbMutex.RUnlock()

	if sx.db == nil {
		return result, ErrDatabaseIsNotReadyYet
	}

	res, err := sx.db.GetCityFull(ip.String())
	if err != nil {
		return result, fmt.Errorf("cannot lookup: %w", err)
	}

	result.CountryCode = topolib.Alpha2ToCountryCode(sx.extractCountry(res))
	result.City = sx.extractCity(res)

	return result, nil
}

func (sx *sypexProvider) extractCountry(info map[string]interface{}) string {
	if countryData, ok := info["country"]; ok {
		if countryMap, ok := countryData.(map[string]interface{}); ok {
			if isoCode, ok := countryMap["iso"]; ok {
				if isoCodeString, ok := isoCode.(string); ok {
					return strings.ToUpper(isoCodeString)
				}
			}
		}
	}

	return ""
}

func (sx *sypexProvider) extractCity(info map[string]interface{}) string {
	if cityData, ok := info["city"]; ok {
		if cityMap, ok := cityData.(map[string]interface{}); ok {
			if cityName, ok := cityMap["name_en"]; ok {
				if cityNameString, ok := cityName.(string); ok {
					return cityNameString
				}
			}
		}
	}

	return ""
}

func (s *sypexProvider) Open(rootDir string) error {
	db := sypex.New(filepath.Join(rootDir, sypexFileName))

	s.dbMutex.Lock()
	defer s.dbMutex.Unlock()

	s.db = &db

	return nil
}

func (s *sypexProvider) Shutdown() {
	s.dbMutex.Lock()
	defer s.dbMutex.Unlock()

	s.db = nil
}

func (s *sypexProvider) Download(ctx context.Context, rootDir string) (retErr error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, sypexDBURL, nil)

	fileResp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("cannot download a file: %w", err)
	}

	defer func() {
		retErr = errors.Join(retErr, flushResponse(fileResp.Body))
	}()

	buff := bytes.NewBuffer([]byte{})
	size, err := io.Copy(buff, fileResp.Body)
	if err != nil {
		return err
	}
	reader := bytes.NewReader(buff.Bytes())

	zipReader, err := zip.NewReader(reader, size)
	if err != nil {
		return fmt.Errorf("cannot open zip archive: %w", err)
	}

	for _, zfile := range zipReader.File {
		if zfile.FileInfo().IsDir() {
			continue
		}

		extension := filepath.Ext(zfile.Name)
		if strings.ToLower(extension) == ".dat" {
			opened, err := zfile.Open()
			if err != nil {
				return fmt.Errorf("cannot extract file from archive: %w", err)
			}
			outFile, err := os.Create(filepath.Join(rootDir, sypexFileName))
			if err != nil {
				if closeErr := opened.Close(); closeErr != nil {
					err = errors.Join(err, closeErr)
				}

				return fmt.Errorf("cannot create a target filename: %w", err)
			}

			_, copyErr := io.Copy(outFile, opened)
			closeErr := errors.Join(opened.Close(), outFile.Close())
			if err := errors.Join(copyErr, closeErr); err != nil {
				return fmt.Errorf("cannot write a target filename: %w", err)
			}

			return nil
		}
	}

	return fmt.Errorf("cannot find required file")
}

// NewSypex returns a new instance which works with
// databases from sypex.
//
//	Identifier: sypex
//	Provider type: offline
//	Website: https://sypex.com

func NewSypex(client topolib.HTTPClient,
	updateEvery time.Duration,
	baseDirectory string) topolib.OfflineProvider {
	return &sypexProvider{
		baseDirectory: baseDirectory,
		updateEvery:   updateEvery,
		httpClient:    client,
	}
}
