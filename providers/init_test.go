package providers_test

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/nesc58/topographer/topolib"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/suite"
)

type TmpDirTestSuite struct {
	suite.Suite

	tmpDir string
}

func (suite *TmpDirTestSuite) SetupTest() {
	dir, err := os.MkdirTemp("", "test_")
	if err != nil {
		panic(err)
	}

	suite.tmpDir = dir
}

func (suite *TmpDirTestSuite) TearDownTest() {
	os.Chmod(suite.tmpDir, 0777) // nolint: errcheck
	suite.NoError(os.RemoveAll(suite.tmpDir))
}

type ProviderTestSuite struct {
	suite.Suite

	http topolib.HTTPClient
}

func (suite *ProviderTestSuite) GetTestdataPath() string {
	absPath, err := filepath.Abs("testdata")
	if err != nil {
		panic(err)
	}

	return absPath
}

func (suite *ProviderTestSuite) SetupTest() {
	suite.http = topolib.NewHTTPClient(&http.Client{},
		"test-agent",
		time.Millisecond,
		100,
		5,
		time.Minute,
		time.Minute)
}

type OnlineProviderTestSuite struct {
	ProviderTestSuite

	prov topolib.Provider
}

type OfflineProviderTestSuite struct {
	ProviderTestSuite

	prov topolib.OfflineProvider
}

func (suite *OfflineProviderTestSuite) TearDownTest() {
	if suite.prov != nil {
		suite.prov.Shutdown()
		suite.prov = nil
	}
}

type HTTPMockMixin struct{}

func (suite *HTTPMockMixin) SetupSuite() {
	httpmock.Activate()
}

func (suite *HTTPMockMixin) TearDownSuite() {
	httpmock.DeactivateAndReset()
}

func (suite *HTTPMockMixin) TearDownTest() {
	httpmock.Reset()
}
