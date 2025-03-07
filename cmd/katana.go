package main

import (
	"katanacrawlgo/internal/runner"
	"katanacrawlgo/pkg/katana/types"
	"log"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"

	"github.com/ttacon/chalk"
)

func katanaRun(options *types.Options) {
	katanaRunner, err := runner.New(options)
	if err != nil || katanaRunner == nil {
		log.Println(chalk.Green.Color("error: katana不能创建执行器, " + err.Error()))
	}
	defer katanaRunner.Close()

	// close handler
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		for range c {
			log.Println(chalk.Yellow.Color("- Ctrl + C 在终端被按下"))
			katanaRunner.Close()
			os.Exit(0)
		}
	}()

	if err := katanaRunner.ExecuteCrawling(); err != nil {
		log.Println(chalk.Red.Color("error: katana爬行器不能被执行, " + err.Error()))
	}
}

func parseUrl(_url string) string {
	u, err := url.Parse(_url)
	if err != nil {
		log.Println(chalk.Red.Color("error: " + _url + "不能被正常解析"))
	}
	baseURL := u.Scheme + "://" + u.Host
	return baseURL
}

func dealUrlScope(urls []string) []string {
	var newUrls []string
	for _, _url := range urls {
		newUrl := parseUrl(_url)
		if newUrl != "" {
			newUrls = append(newUrls, newUrl)
		}
	}
	return newUrls
}

func newparseUrl(_url string) string {
	u, err := url.Parse(_url)
	if err != nil {
		log.Println(chalk.Red.Color("error: " + _url + "不能被正常解析"))
	}
	baseURL := u.Scheme + "://" + u.Host
	ExtensionFilters := []string{"css", "jpg", "jpeg", "png", "ico", "gif", "webp", "mp3", "mp4", "ttf", "tif", "tiff", "woff", "woff2", "'+", "+'", "/+", "vue"}
	for _, filter := range ExtensionFilters {
		if strings.Contains(_url, filter) {
			return baseURL
		}
	}
	return _url
}

// 新增辅助函数：获取URL的根部分
func getRootURL(rawUrl string) string {
	u, err := url.Parse(rawUrl)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func newdealUrlScope(urls []string) []string {
	unique := make(map[string]bool)
	var list []string

	// 路径相似度过滤
	for _, _url := range urls {
		current := newparseUrl(_url)
		if current == "" || unique[current] {
			continue
		}

		// 检查路径相似性
		exists := false
		path1 := getPathSegments(current)
		for existUrl := range unique {
			path2 := getPathSegments(existUrl)
			if isPathSimilar(path1, path2) {
				exists = true
				break
			}
		}
		if !exists {
			unique[current] = true
			list = append(list, current)
		}
	}

	// 结果数量控制

	if len(list) > 25 {
		sort.Slice(list, func(i, j int) bool {
			return len(list[i]) < len(list[j])
		})
		list = list[:25]
	}
	// 新增根URL校验
	rootMap := make(map[string]string) // 存储根URL到完整URL的映射
	existing := make(map[string]bool)  // 当前存在的URL集合

	// 收集所有根URL和现有URL
	for _, u := range list {
		existing[u] = true
		if root := getRootURL(u); root != "" {
			rootMap[root] = u // 记录最后出现的该根URL下的子URL
		}
	}

	// 补充缺失的根URL
	for root := range rootMap {
		if !existing[root] {
			list = append(list, root)
			existing[root] = true
		}
	}
	return list
}

// 获取URL的路径分段
func getPathSegments(rawUrl string) []string {
	u, err := url.Parse(rawUrl)
	if err != nil {
		return []string{}
	}
	return strings.Split(strings.Trim(u.Path, "/"), "/")
}

// 路径分层相似度判断
func isPathSimilar(path1, path2 []string) bool {
	// 层级差异超过2层直接排除
	if diff := len(path1) - len(path2); diff < -2 || diff > 2 {
		return false
	}

	// 计算公共前缀
	minLen := len(path1)
	if len(path2) < minLen {
		minLen = len(path2)
	}
	common := 0
	for i := 0; i < minLen; i++ {
		if path1[i] == path2[i] {
			common++
		} else {
			break
		}
	}

	// 相似度判断标准：
	// 1. 公共前缀超过3层
	// 2. 或公共前缀占比超过60%
	return common >= 3 || float64(common)/float64(max(len(path1), len(path2))) > 0.8
}
