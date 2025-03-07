package main

import (
	"flag"
	"fmt"
	"katanacrawlgo/internal/utils"
	"katanacrawlgo/pkg/crawlergo"
	"katanacrawlgo/pkg/crawlergo/config"
	"katanacrawlgo/pkg/crawlergo/model"
	"katanacrawlgo/pkg/katana/types"
	"log"
	"math"
	neturlparse "net/url"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/ttacon/chalk"
	"github.com/urfave/cli/v2"
)

type Result struct {
	ReqList       []Request `json:"req_list"`
	AllReqList    []Request `json:"all_req_list"`
	AllDomainList []string  `json:"all_domain_list"`
	SubDomainList []string  `json:"sub_domain_list"`
}

type Request struct {
	Url     string                 `json:"url"`
	Method  string                 `json:"method"`
	Headers map[string]interface{} `json:"headers"`
	Data    string                 `json:"data"`
	Source  string                 `json:"source"`
}

type ProxyTask struct {
	req       *model.Request
	pushProxy string
}

func existCheck(filename string) {
	if _, err := os.Stat(filename); err == nil {
		err = os.Remove(filename)
		if err != nil {
			log.Fatal(chalk.Red.Color("error: " + err.Error()))
		}
	}
}
func startCheck(filename string) {
	arr := []string{
		fmt.Sprintf("katana-%s.txt", filename),
		fmt.Sprintf("%s-all.txt", filename),
		fmt.Sprintf("crawlergo-%s.txt", filename),
	}
	for _, s := range arr {
		existCheck(s)
	}
}

var (
	taskConfig              crawlergo.TaskConfig
	postData                string
	signalChan              chan os.Signal
	ignoreKeywords          = cli.NewStringSlice(config.DefaultIgnoreKeywords...)
	customFormTypeValues    = cli.NewStringSlice()
	customFormKeywordValues = cli.NewStringSlice()
	pushAddress             string
	pushProxyPoolMax        int
	pushProxyWG             sync.WaitGroup
	urlScope                []string
)

func cmd() {
	isHeadless := flag.Bool("headless", false, chalk.Green.Color("浏览器是否可见"))
	chromium := flag.String("chromium", "", chalk.Green.Color("无头浏览器chromium路径配置"))
	customHeaders := flag.String("headers", "{\"User-Agent\": \""+config.DefaultUA+"\"}", chalk.Green.Color("自定义请求头参数，要以json格式被序列化"))
	maxCrawler := flag.Int("maxCrawler", config.MaxCrawlCount, chalk.Green.Color("URL启动的任务最大的爬行个数"))
	mode := flag.String("mode", "smart", chalk.Green.Color("爬行模式，simple/smart/strict,默认smart"))
	proxy := flag.String("proxy", "", chalk.Green.Color("请求的代理，针对访问URL在墙外的情况，默认直连为空"))
	blackKey := flag.String("blackKey", "", chalk.Green.Color("黑名单关键词，用于避免被爬虫执行危险操作，用,分割，如：logout,delete,update"))
	url := flag.String("url", "", chalk.Green.Color("执行爬行的单个URL"))
	urlTxt := flag.String("urlTxtPath", "", chalk.Green.Color("如果需求是批量爬行URL，那需要将URL写入txt，然后将路径放入"))
	resultTxt := flag.String("resultTxtPath", "", chalk.Green.Color("结果文件"))
	encode := flag.Bool("encodeUrlWithCharset", false, chalk.Green.Color("是否对URL进行编码"))
	depth := flag.Int("depth", 2, chalk.Green.Color("最大爬行深度，默认是2"))
	flag.Parse()
	startCheck(*resultTxt)
	options := &types.Options{}
	if *urlTxt == "" && *url == "" {
		log.Println(chalk.Red.Color("URL文件和URL必须有一个！！！"))
		os.Exit(0)
	}
	var urls []string
	if *urlTxt != "" {
		urlList := utils.GetUrlListFromTxt(*urlTxt)
		if len(urlList) > 0 {
			urlList = utils.UniqueUrls(urlList)
			urls = urlList
			options.URLs = urlList
			urlScope = dealUrlScope(urlList)
		}
	}

	options.MaxDepth = *depth
	options.Headless = false

	if *mode == "simple" {
		options.ScrapeJSResponses = false
		options.AutomaticFormFill = false
	} else {
		options.ScrapeJSResponses = true
		options.AutomaticFormFill = true
	}
	options.KnownFiles = ""
	options.BodyReadSize = math.MaxInt
	options.Timeout = 15
	options.Retries = 1
	options.AutomaticFormFill = true
	options.Proxy = *proxy

	// 请求头要单独将json处理为键值对,目前不设置
	options.Strategy = "depth-first"
	options.ShowBrowser = *isHeadless
	if *chromium != "" {
		options.SystemChromePath = *chromium
	}
	options.FieldScope = "fqdn"
	options.OutputFile = fmt.Sprintf("katana-%s.txt", *resultTxt)
	if *url != "" {
		newUrl := parseUrl(*url)
		if newUrl != "" {
			urlScope = append(urlScope, newUrl)
		}
		options.URLs = append(urls, *url)
	}
	options.Scope = utils.UniqueUrls(urlScope)
	options.Concurrency = 5
	options.Parallelism = 5
	options.RateLimit = 20
	options.ExtensionFilter = []string{"css", "jpg", "jpeg", "png", "ico", "gif", "webp", "mp3", "mp4", "ttf", "tif", "tiff", "woff", "woff2", "'+", "+'", "/+"}
	katanaRun(options)

	// 执行crawlergo之前将结果文件读取

	urls = utils.GetUrlListFromTxt(options.OutputFile)
	if len(urls) > 0 {
		taskConfig.URLList = newdealUrlScope(urls)
	}

	// Crawlergo配置
	ignoreList := make([]string, 0)
	taskConfig.NoHeadless = *isHeadless
	taskConfig.ChromiumPath = *chromium
	taskConfig.Proxy = *proxy
	taskConfig.EncodeURLWithCharset = *encode
	taskConfig.FilterMode = *mode
	taskConfig.MaxCrawlCount = *maxCrawler
	taskConfig.ExtraHeadersString = *customHeaders
	taskConfig.MaxTabsCount = config.MaxTabsCount
	taskConfig.PathFromRobots = true
	taskConfig.TabRunTimeout = config.TabRunTimeout
	taskConfig.DomContentLoadedTimeout = config.DomContentLoadedTimeout
	taskConfig.EventTriggerMode = config.EventTriggerAsync
	taskConfig.EventTriggerInterval = config.EventTriggerInterval
	taskConfig.BeforeExitDelay = config.BeforeExitDelay
	taskConfig.MaxRunTime = config.MaxRunTime
	taskConfig.CustomFormValues = map[string]string{}
	taskConfig.ResultFile = fmt.Sprintf("crawlergo-%s.txt", *resultTxt)
	if *blackKey != "" {
		ignoreList = strings.Split(*blackKey, ",")
	}
	taskConfig.IgnoreKeywords = ignoreList
	crawlergoRun()

	//全部程序执行完之后将三个文件进行合并，这里暂时只有两个
	finalResult := make([]string, 0)
	arr := []string{
		fmt.Sprintf("katana-%s.txt", *resultTxt),
		fmt.Sprintf("crawlergo-%s.txt", *resultTxt),
	}
	for _, filename := range arr {
		fromTxt := utils.GetUrlListFromTxt(filename)
		finalResult = append(finalResult, fromTxt...)
	}

	// 新增URL处理逻辑
	var cleanUrls []string

	// 过滤特殊字符和路径处理
	ExtensionFilters := []string{"css", "jpg", "jpeg", "png", "ico", "gif", "webp", "mp3", "mp4", "ttf", "tif", "tiff", "woff", "woff2", "vue", "YYYY", "MM", "DD", "HH"}
	for _, filter := range ExtensionFilters {
		if strings.Contains(*resultTxt, filter) {
			log.Fatal(chalk.Red.Color("error: 结果文件名中不能包含特殊字符"))
		}
	}
	var filterRegex = regexp.MustCompile(`(` + strings.Join(ExtensionFilters, "|") + `)`)
	for _, u := range finalResult {
		// 路径层级处理
		if parsed, err := neturlparse.Parse(u); err == nil {
			// 检查路径是否包含特殊字符
			if strings.ContainsAny(parsed.Path, "+'\"-") || filterRegex.MatchString(parsed.Path) {
				continue
			}
			pathSegments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
			if len(pathSegments) > 3 && !strings.Contains(u, "?") && !strings.Contains(pathSegments[len(pathSegments)-1], ".") {
				parsed.Path = "/" + strings.Join(pathSegments[:3], "/") + "/"
			}
			parsed.Path = strings.TrimSuffix(parsed.Path, "/")
			u = parsed.String()
		}
		cleanUrls = append(cleanUrls, u)
	}

	// 应用路径相似度算法
	finalUrls := newdealUrlScope(cleanUrls)

	for _, _url := range utils.UniqueUrls(finalUrls) {
		utils.AppendToFile(fmt.Sprintf("%s-all.txt", *resultTxt), _url)
	}

	/*for _, filename := range result_arr {
		if _, err := os.Stat(filename); err == nil {
			if err := os.Remove(filename); err != nil {
				log.Println(chalk.Yellow.Color("临时文件删除失败: " + filename))
			}
		}
	}*/
}

func main() {
	cmd()
}
