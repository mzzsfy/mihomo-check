package check

import (
	"context"
	"fmt"
	"github.com/metacubex/mihomo/component/dialer"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bestruirui/mihomo-check/check/platfrom"
	"github.com/bestruirui/mihomo-check/config"
	proxyutils "github.com/bestruirui/mihomo-check/proxy"
	"github.com/bestruirui/mihomo-check/proxy/ipinfo"
	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"
	"gopkg.in/yaml.v3"
)

// Result 存储节点检测结果
type Result struct {
	Proxy      map[string]any
	Openai     bool
	Youtube    bool
	Netflix    bool
	Google     bool
	Cloudflare bool
	Disney     bool
}

// ProxyChecker 处理代理检测的主要结构体
type ProxyChecker struct {
	results     []Result
	proxyCount  int
	threadCount int
	progress    int32
	available   int32
	mu          sync.Mutex
	resultChan  chan Result
	tasks       chan map[string]any
}

// NewProxyChecker 创建新的检测器实例
func NewProxyChecker(proxies []map[string]any) *ProxyChecker {
	proxyCount := len(proxies)
	threadCount := config.GlobalConfig.Concurrent
	if proxyCount < threadCount {
		threadCount = proxyCount
	}

	return &ProxyChecker{
		results:     make([]Result, 0),
		proxyCount:  proxyCount,
		threadCount: threadCount,
		resultChan:  make(chan Result),
		tasks:       make(chan map[string]any, proxyCount),
	}
}

// Check 执行代理检测的主函数
func Check() ([]Result, error) {
	proxyutils.ResetRenameCounter()

	proxies, err := GetProxyFromSubs()
	if err != nil {
		return nil, fmt.Errorf("获取节点失败: %w", err)
	}

	log.Infoln("共获取到%d个节点", len(proxies))
	proxies = proxyutils.DeduplicateProxies(proxies)
	log.Infoln("去重后共%d个节点", len(proxies))

	checker := NewProxyChecker(proxies)
	return checker.run(proxies)
}

// Run 运行检测流程
func (pc *ProxyChecker) run(proxies []map[string]any) ([]Result, error) {
	done := make(chan bool)
	if config.GlobalConfig.PrintProgress {
		go pc.showProgress(done)
	}

	var wg sync.WaitGroup
	// 启动工作线程
	for i := 0; i < pc.threadCount; i++ {
		wg.Add(1)
		go pc.worker(&wg)
	}

	// 发送任务
	go pc.distributeProxies(proxies)

	// 收集结果
	go pc.collectResults()

	wg.Wait()
	close(pc.resultChan)

	if config.GlobalConfig.PrintProgress {
		done <- true
	}

	log.Infoln("共%d个可用节点", len(pc.results))
	return pc.results, nil
}

// worker 处理单个代理检测的工作线程
func (pc *ProxyChecker) worker(wg *sync.WaitGroup) {
	defer wg.Done()
	for proxy := range pc.tasks {
		if result := pc.checkProxy(proxy); result != nil {
			pc.resultChan <- *result
		}
		pc.incrementProgress()
	}
}

// checkProxy 检测单个代理
func (pc *ProxyChecker) checkProxy(proxy map[string]any) *Result {
	httpClient := CreateClient(proxy)
	if httpClient == nil {
		if config.GlobalConfig.DebugMode {
			log.Infoln("%s 创建客户端失败", proxy["name"])
		}
		return nil
	}
	if config.GlobalConfig.Warmup {
		platfrom.CheckCloudflare(httpClient)
	}

	cloudflare, err := platfrom.CheckCloudflare(httpClient)
	if err != nil || !cloudflare {
		if config.GlobalConfig.DebugMode {
			log.Infoln("%s 检查代理结果:无法访问Cloudflare", proxy["name"])
		}
		return nil
	}

	google, err := platfrom.CheckGoogle(httpClient)
	if err != nil || !google {
		if config.GlobalConfig.DebugMode {
			log.Infoln("%s 检查代理结果:无法访问Google", proxy["name"])
		}
		return nil
	}
	for _, url := range config.GlobalConfig.ExtraCheckUrls {
		if url == "" {
			continue
		}
		if ok, err := platfrom.CheckCustomize(url, httpClient); err != nil || !ok {
			if config.GlobalConfig.DebugMode {
				log.Infoln("%s 检查代理结果:无法访问自定义url (%s)", proxy["name"], url)
			}
			return nil
		}
	}

	// 执行其他平台检测
	openai, _ := platfrom.CheckOpenai(httpClient)
	youtube, _ := platfrom.CheckYoutube(httpClient)
	netflix, _ := platfrom.CheckNetflix(httpClient)
	disney, _ := platfrom.CheckDisney(httpClient)

	// 更新代理名称
	pc.updateProxyName(proxy, httpClient)
	pc.incrementAvailable()
	result := Result{
		Proxy:      proxy,
		Cloudflare: cloudflare,
		Google:     google,
		Openai:     openai,
		Youtube:    youtube,
		Netflix:    netflix,
		Disney:     disney,
	}
	if config.GlobalConfig.DebugMode {
		log.Infoln("%s 检查代理结果: %+v", proxy["name"], result)
	}
	return &result
}

// updateProxyName 更新代理名称
func (pc *ProxyChecker) updateProxyName(proxy map[string]any, client *http.Client) {
	ipAddr := ipinfo.GetIPaddrFromAPI(client)
	country := ipinfo.GetIPCountrynameFromdb(ipAddr)
	if country == "" {
		country = "未识别"
	}
	proxy["name"] = proxyutils.Rename(country)
}

// showProgress 显示进度条
func (pc *ProxyChecker) showProgress(done chan bool) {
	for {
		select {
		case <-done:
			return
		default:
			pc.mu.Lock()
			current := pc.progress
			available := pc.available
			pc.mu.Unlock()

			percent := float64(current) / float64(pc.proxyCount) * 100
			fmt.Printf("\r进度: [%-50s] %.1f%% (%d/%d) 可用: %d",
				strings.Repeat("=", int(percent/2))+">",
				percent,
				current,
				pc.proxyCount,
				available)
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// 辅助方法
func (pc *ProxyChecker) incrementProgress() {
	pc.mu.Lock()
	pc.progress++
	pc.mu.Unlock()
}

func (pc *ProxyChecker) incrementAvailable() {
	pc.mu.Lock()
	pc.available++
	pc.mu.Unlock()
}

// distributeProxies 分发代理任务
func (pc *ProxyChecker) distributeProxies(proxies []map[string]any) {
	for _, proxy := range proxies {
		pc.tasks <- proxy
	}
	close(pc.tasks)
}

// collectResults 收集检测结果
func (pc *ProxyChecker) collectResults() {
	for result := range pc.resultChan {
		pc.results = append(pc.results, result)
	}
}

func GetProxyFromSubs() ([]map[string]any, error) {

	if len(config.GlobalConfig.SubUrls) == 0 {
		log.Errorln("未设置订阅链接")
		os.Exit(1)
	}

	log.Infoln("共设置%d个订阅链接", len(config.GlobalConfig.SubUrls))

	proxies := make([]map[string]any, 0)

	urls := config.GlobalConfig.SubUrls[:]
	retries := 0
	for i := 0; i < len(urls); i++ {
		subUrl := urls[i]
		// 添加重试逻辑
		var resp *http.Response
		var err error
		resp, err = http.Get(subUrl)
		if err != nil || resp.StatusCode != 200 {
			retries++
			time.Sleep(time.Second * time.Duration(retries+1))
			if retries < config.GlobalConfig.SubUrlsReTry {
				i--
				code := 0
				if resp != nil {
					code = resp.StatusCode
				}
				log.Errorln("获取订阅链接失败(%d): %v,httpCode: %v,重试: %d", i+1, err, code, retries)
			} else {
				retries = 0
				log.Errorln("获取订阅链接失败(%d): %v", i, err)
			}
			continue
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			retries++
			if retries < config.GlobalConfig.SubUrlsReTry {
				i--
				log.Errorln("读取配置文件失败(%d): %v,重试: %d", i+1, err, retries)
			} else {
				retries = 0
				log.Errorln("读取配置文件失败(%d): %v", i, err)
			}
			continue
		}

		var conf map[string]any
		if err := yaml.Unmarshal(data, &conf); err != nil {
			retries++
			if retries < config.GlobalConfig.SubUrlsReTry {
				i--
				log.Errorln("解析订阅链接失败(%d): %v,重试: %d", i+1, err, retries)
			} else {
				retries = 0
				log.Errorln("解析订阅链接失败(%d): %v", i, err)
				log.Errorln("订阅链接: %s", subUrl)
			}
			continue
		}
		retries = 0
		// 添加空值检查
		proxyInterface, ok := conf["proxies"]
		if !ok || proxyInterface == nil {
			continue
		}

		proxyList, ok := proxyInterface.([]any)
		if !ok {
			continue
		}

		for _, proxy := range proxyList {
			proxyMap, ok := proxy.(map[string]any)
			if !ok {
				continue
			}
			proxies = append(proxies, proxyMap)
		}
	}

	if len(proxies) == 0 {
		return nil, fmt.Errorf("未找到任何可用节点")
	}

	return proxies, nil
}

func CreateClient(mapping map[string]any) *http.Client {
	proxy, err := adapter.ParseProxy(mapping)
	if err != nil {
		return nil
	}

	return &http.Client{
		Timeout: time.Duration(config.GlobalConfig.Timeout) * time.Millisecond,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				var u16Port uint16
				if port, err := strconv.ParseUint(port, 10, 16); err == nil {
					u16Port = uint16(port)
				}
				return proxy.DialContext(ctx, &constant.Metadata{
					Host:    host,
					DstPort: u16Port,
				}, dialer.WithResolver(config.ProxyResolver))
			},
			// 设置连接超时
			IdleConnTimeout: time.Duration(config.GlobalConfig.Timeout) * time.Millisecond,
			// 关闭keepalive
			DisableKeepAlives: true,
		},
	}
}
