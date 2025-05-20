package save

import (
	"fmt"

	"github.com/bestruirui/mihomo-check/check"
	"github.com/bestruirui/mihomo-check/config"
	"github.com/bestruirui/mihomo-check/save/method"
	"github.com/metacubex/mihomo/log"
	"gopkg.in/yaml.v3"
)

// ProxyCategory 定义代理分类
type ProxyCategory struct {
	Name    string
	Proxies []map[string]any
	Filter  func(result check.Result) bool
}

// ConfigSaver 处理配置保存的结构体
type ConfigSaver struct {
	results    []check.Result
	categories []ProxyCategory
	saveMethod func([]byte, string) error
}

// NewConfigSaver 创建新的配置保存器
func NewConfigSaver(results []check.Result) *ConfigSaver {
	conf := []ProxyCategory{
		{
			Name:    "noCheck.yaml",
			Proxies: make([]map[string]any, 0),
			Filter:  func(result check.Result) bool { return true },
		},
		{
			Name:    "all.yaml",
			Proxies: make([]map[string]any, 0),
			Filter:  func(result check.Result) bool { return result.Any },
		},
		{
			Name:    "openai.yaml",
			Proxies: make([]map[string]any, 0),
			Filter:  func(result check.Result) bool { return result.Openai },
		},
		{
			Name:    "youtube.yaml",
			Proxies: make([]map[string]any, 0),
			Filter:  func(result check.Result) bool { return result.Youtube },
		},
		{
			Name:    "netflix.yaml",
			Proxies: make([]map[string]any, 0),
			Filter:  func(result check.Result) bool { return result.Netflix },
		},
		{
			Name:    "disney.yaml",
			Proxies: make([]map[string]any, 0),
			Filter:  func(result check.Result) bool { return result.Disney },
		},
	}
	if len(config.GlobalConfig.CheckUrls) > 0 {
		conf = append(conf, ProxyCategory{Name: "customize.yaml",
			Proxies: make([]map[string]any, 0),
			Filter:  func(result check.Result) bool { return result.Customize },
		})
	}
	return &ConfigSaver{
		results:    results,
		saveMethod: chooseSaveMethod(),
		categories: conf,
	}
}

// SaveConfig 保存配置的入口函数
func SaveConfig(results []check.Result) {
	saver := NewConfigSaver(results)
	if err := saver.Save(); err != nil {
		log.Errorln("保存配置失败: %v", err)
	}
}

// Save 执行保存操作
func (cs *ConfigSaver) Save() error {
	// 分类处理代理
	cs.categorizeProxies()

	// 保存各个类别的代理
	for _, category := range cs.categories {
		if err := cs.saveCategory(category); err != nil {
			log.Errorln("保存 %s 类别失败: %v", category.Name, err)
			continue
		}
	}

	return nil
}

// categorizeProxies 将代理按类别分类
func (cs *ConfigSaver) categorizeProxies() {
	for _, result := range cs.results {
		for i := range cs.categories {
			if cs.categories[i].Filter(result) {
				cs.categories[i].Proxies = append(cs.categories[i].Proxies, result.Proxy)
			}
		}
	}
}

// saveCategory 保存单个类别的代理
func (cs *ConfigSaver) saveCategory(category ProxyCategory) error {
	if len(category.Proxies) == 0 {
		log.Warnln("%s 节点为空，跳过", category.Name)
		return nil
	}
	yamlData, err := yaml.Marshal(map[string]any{
		"proxies": category.Proxies,
	})
	if err != nil {
		return fmt.Errorf("序列化 %s 失败: %w", category.Name, err)
	}
	if err := cs.saveMethod(yamlData, category.Name); err != nil {
		return fmt.Errorf("保存 %s 失败: %w", category.Name, err)
	}

	return nil
}

// chooseSaveMethod 根据配置选择保存方法
func chooseSaveMethod() func([]byte, string) error {
	switch config.GlobalConfig.SaveMethod {
	case "r2":
		if err := method.ValiR2Config(); err != nil {
			log.Errorln("R2配置不完整: %v ,使用本地保存", err)
			return method.SaveToLocal
		}
		return method.UploadToR2Storage
	case "gist":
		if err := method.ValiGistConfig(); err != nil {
			log.Errorln("Gist配置不完整: %v ,使用本地保存", err)
			return method.SaveToLocal
		}
		return method.UploadToGist
	case "local":
		return method.SaveToLocal
	default:
		log.Errorln("未知的保存方法: %s，使用本地保存", config.GlobalConfig.SaveMethod)
		return method.SaveToLocal
	}
}
