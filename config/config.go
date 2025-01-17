package config

import (
	"github.com/metacubex/mihomo/component/resolver"
)

type IPInfo struct {
	APIURL  []string `yaml:"api-url"`
	IPDBURL string   `yaml:"ipdb-url"`
}

type Config struct {
	PrintProgress   bool     `yaml:"print-progress"`
	Concurrent      int      `yaml:"concurrent"`
	CheckInterval   int      `yaml:"check-interval"`
	DownloadSize    int      `yaml:"download-size"`
	UploadSize      int      `yaml:"upload-size"`
	Timeout         int      `yaml:"timeout"`
	FilterRegex     string   `yaml:"filter-regex"`
	SaveMethod      string   `yaml:"save-method"`
	GithubToken     string   `yaml:"github-token"`
	GithubGistID    string   `yaml:"github-gist-id"`
	GithubAPIMirror string   `yaml:"github-api-mirror"`
	WorkerURL       string   `yaml:"worker-url"`
	WorkerToken     string   `yaml:"worker-token"`
	SubUrlsReTry    int      `yaml:"sub-urls-retry"`
	SubUrls         []string `yaml:"sub-urls"`
	IPInfo          IPInfo   `yaml:"ip-info"`
	MihomoApiUrl    string   `yaml:"mihomo-api-url"`
	MihomoApiSecret string   `yaml:"mihomo-api-secret"`
	MateDns         []string `yaml:"mate-dns"`
	Warmup          bool     `yaml:"warmup"`
}

var GlobalConfig Config
var ProxyResolver resolver.Resolver
