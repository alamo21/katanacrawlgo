package crawlergo

import (
	"encoding/json"
	"katanacrawlgo/pkg/crawlergo/config"
	"katanacrawlgo/pkg/crawlergo/engine"
	filter3 "katanacrawlgo/pkg/crawlergo/filter"
	"katanacrawlgo/pkg/crawlergo/model"
	"log"
	"sync"
	"time"

	"github.com/ttacon/chalk"

	"github.com/panjf2000/ants/v2"
)

type CrawlerTask struct {
	Browser       *engine.Browser       //
	RootDomain    string                // 当前爬取根域名 用于子域名收集
	Targets       []*model.Request      // 输入目标
	Result        *Result               // 最终结果
	Config        *TaskConfig           // 配置信息
	filter        filter3.FilterHandler // 过滤对象
	Pool          *ants.Pool            // 协程池
	taskWG        sync.WaitGroup        // 等待协程池所有任务结束
	crawledCount  int                   // 爬取过的数量
	taskCountLock sync.Mutex            // 已爬取的任务总数锁
	Start         time.Time             //开始时间

}

type Result struct {
	ReqList       []*model.Request // 返回的同域名结果
	AllReqList    []*model.Request // 所有域名的请求
	AllDomainList []string         // 所有域名列表
	SubDomainList []string         // 子域名列表
	resultLock    sync.Mutex       // 合并结果时加锁
}

type tabTask struct {
	crawlerTask *CrawlerTask
	browser     *engine.Browser
	req         *model.Request
}

/*
*
新建爬虫任务
*/
func NewCrawlerTask(targets []*model.Request, taskConf TaskConfig) (*CrawlerTask, error) {
	crawlerTask := CrawlerTask{
		Result: &Result{},
		Config: &taskConf,
	}

	baseFilter := filter3.NewSimpleFilter(targets[0].URL.Host)

	if taskConf.FilterMode == config.SmartFilterMode {
		crawlerTask.filter = filter3.NewSmartFilter(baseFilter, false)

	} else if taskConf.FilterMode == config.StrictFilterMode {
		crawlerTask.filter = filter3.NewSmartFilter(baseFilter, true)

	} else {
		crawlerTask.filter = baseFilter
	}

	if len(targets) == 1 {
		_newReq := *targets[0]
		newReq := &_newReq
		_newURL := *_newReq.URL
		newReq.URL = &_newURL
		if targets[0].URL.Scheme == "http" {
			newReq.URL.Scheme = "https"
		} else {
			newReq.URL.Scheme = "http"
		}
		targets = append(targets, newReq)
	}
	crawlerTask.Targets = targets[:]

	for _, req := range targets {
		req.Source = config.FromTarget
	}

	// 业务代码与数据代码分离, 初始化一些默认配置
	// 使用 funtion option 和一个代理来初始化 taskConf 的配置
	for _, fn := range []TaskConfigOptFunc{
		WithTabRunTimeout(config.TabRunTimeout),
		WithMaxTabsCount(config.MaxTabsCount),
		WithMaxCrawlCount(config.MaxCrawlCount),
		WithDomContentLoadedTimeout(config.DomContentLoadedTimeout),
		WithEventTriggerInterval(config.EventTriggerInterval),
		WithBeforeExitDelay(config.BeforeExitDelay),
		WithEventTriggerMode(config.DefaultEventTriggerMode),
		WithIgnoreKeywords(config.DefaultIgnoreKeywords),
	} {
		fn(&taskConf)
	}

	if taskConf.ExtraHeadersString != "" {
		err := json.Unmarshal([]byte(taskConf.ExtraHeadersString), &taskConf.ExtraHeaders)
		if err != nil {
			log.Println(chalk.Red.Color("error: 自定义请求头不能被序列化"))
			return nil, err
		}
	}

	if len(taskConf.ChromiumWSUrl) > 0 {
		crawlerTask.Browser = engine.ConnectBrowser(taskConf.ChromiumWSUrl, taskConf.ExtraHeaders)
	} else {
		crawlerTask.Browser = engine.InitBrowser(taskConf.ChromiumPath, taskConf.ExtraHeaders, taskConf.Proxy, taskConf.NoHeadless)
	}
	crawlerTask.RootDomain = targets[0].URL.RootDomain()

	// 创建协程池
	p, _ := ants.NewPool(taskConf.MaxTabsCount)
	crawlerTask.Pool = p

	return &crawlerTask, nil
}

/*
*
根据请求列表生成tabTask协程任务列表
*/
func (t *CrawlerTask) generateTabTask(req *model.Request) *tabTask {
	task := tabTask{
		crawlerTask: t,
		browser:     t.Browser,
		req:         req,
	}
	return &task
}

/*
*
开始当前任务
*/
func (t *CrawlerTask) Run() {
	defer t.Pool.Release()  // 释放协程池
	defer t.Browser.Close() // 关闭浏览器

	t.Start = time.Now()
	if t.Config.PathFromRobots {
		reqsFromRobots := GetPathsFromRobots(*t.Targets[0])
		t.Targets = append(t.Targets, reqsFromRobots...)
	}

	if t.Config.FuzzDictPath != "" {
		if t.Config.PathByFuzz {
		}
		reqsByFuzz := GetPathsByFuzzDict(*t.Targets[0], t.Config.FuzzDictPath)
		t.Targets = append(t.Targets, reqsByFuzz...)
	} else if t.Config.PathByFuzz {
		reqsByFuzz := GetPathsByFuzz(*t.Targets[0])
		t.Targets = append(t.Targets, reqsByFuzz...)
	}

	t.Result.AllReqList = t.Targets[:]

	var initTasks []*model.Request
	for _, req := range t.Targets {
		if t.filter.DoFilter(req) {
			continue
		}
		initTasks = append(initTasks, req)
		t.Result.ReqList = append(t.Result.ReqList, req)
	}

	for _, req := range initTasks {
		if !engine.IsIgnoredByKeywordMatch(*req, t.Config.IgnoreKeywords) {
			t.addTask2Pool(req)
		}
	}

	t.taskWG.Wait()

	// 对全部请求进行唯一去重
	todoFilterAll := make([]*model.Request, len(t.Result.AllReqList))
	copy(todoFilterAll, t.Result.AllReqList)

	t.Result.AllReqList = []*model.Request{}
	var simpleFilter filter3.SimpleFilter
	for _, req := range todoFilterAll {
		if !simpleFilter.UniqueFilter(req) {
			t.Result.AllReqList = append(t.Result.AllReqList, req)
		}
	}

	// 全部域名
	t.Result.AllDomainList = AllDomainCollect(t.Result.AllReqList)
	// 子域名
	t.Result.SubDomainList = SubDomainCollect(t.Result.AllReqList, t.RootDomain)
}

/*
*
添加任务到协程池
添加之前实时过滤
*/
func (t *CrawlerTask) addTask2Pool(req *model.Request) {
	t.taskCountLock.Lock()
	if t.crawledCount >= t.Config.MaxCrawlCount {
		t.taskCountLock.Unlock()
		return
	} else {
		t.crawledCount += 1
	}

	if t.Start.Add(time.Second * time.Duration(t.Config.MaxRunTime)).Before(time.Now()) {
		t.taskCountLock.Unlock()
		return
	}
	t.taskCountLock.Unlock()

	t.taskWG.Add(1)
	task := t.generateTabTask(req)
	go func() {
		err := t.Pool.Submit(task.Task)
		if err != nil {
			t.taskWG.Done()
			log.Print(chalk.Red.Color("error: 加入任务2队列池失败, " + err.Error()))
		}
	}()
}

/*
*
单个运行的tab标签任务，实现了workpool的接口
*/
func (t *tabTask) Task() {
	defer t.crawlerTask.taskWG.Done()

	// 设置tab超时时间，若设置了程序最大运行时间， tab超时时间和程序剩余时间取小
	timeremaining := t.crawlerTask.Start.Add(time.Duration(t.crawlerTask.Config.MaxRunTime) * time.Second).Sub(time.Now())
	tabTime := t.crawlerTask.Config.TabRunTimeout
	if t.crawlerTask.Config.TabRunTimeout > timeremaining {
		tabTime = timeremaining
	}

	if tabTime <= 0 {
		return
	}

	tab := engine.NewTab(t.browser, *t.req, engine.TabConfig{
		TabRunTimeout:           tabTime,
		DomContentLoadedTimeout: t.crawlerTask.Config.DomContentLoadedTimeout,
		EventTriggerMode:        t.crawlerTask.Config.EventTriggerMode,
		EventTriggerInterval:    t.crawlerTask.Config.EventTriggerInterval,
		BeforeExitDelay:         t.crawlerTask.Config.BeforeExitDelay,
		EncodeURLWithCharset:    t.crawlerTask.Config.EncodeURLWithCharset,
		IgnoreKeywords:          t.crawlerTask.Config.IgnoreKeywords,
		CustomFormValues:        t.crawlerTask.Config.CustomFormValues,
		CustomFormKeywordValues: t.crawlerTask.Config.CustomFormKeywordValues,
	})
	tab.Start()

	// 收集结果
	t.crawlerTask.Result.resultLock.Lock()
	t.crawlerTask.Result.AllReqList = append(t.crawlerTask.Result.AllReqList, tab.ResultList...)
	t.crawlerTask.Result.resultLock.Unlock()

	for _, req := range tab.ResultList {
		if !t.crawlerTask.filter.DoFilter(req) {
			t.crawlerTask.Result.resultLock.Lock()
			t.crawlerTask.Result.ReqList = append(t.crawlerTask.Result.ReqList, req)
			t.crawlerTask.Result.resultLock.Unlock()
			if !engine.IsIgnoredByKeywordMatch(*req, t.crawlerTask.Config.IgnoreKeywords) {
				t.crawlerTask.addTask2Pool(req)
			}
		}
	}
}
