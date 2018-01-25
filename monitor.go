package monitor

import (
	"os"
	"encoding/json"
)

type (
	// 监控告警类型枚举
	AlertType uint8
	// 队列任务类型枚举
	TaskType uint8
)

const (
	// 健康状态
	NONE AlertType = iota
	// 访问成功率告警
	FAIL
	// 时间达标率告警
	SLOW
)

const (
	_ TaskType = iota
	// 服务端数据上报类型的统计
	SERVER
	// 清理任务，通常用于一个阶段的分析完成并清空旧数据
	CLEAR
)


// 一个客户端实例所拥有的方法
type ReportClient interface {
	// 上报
	Report(name string, ms uint32, code int)
	// 添加自定义条目配置，包括条目对应的耗时达标标准以及时延分布等数据
	AddEntryConfig(name string, entryConfig EntryConfig)
}

// 客户端的全局配置，一个客户端可能会上报若干个接口
type ReportClientConfig struct {
	// 标识本上报的名称
	Name string
	// 默认高速访问时间，根据调用系统的不同特性调整，优先匹配FastLessThan，剩余的则以此值作为依据，默认50ms
	DefaultFastTime uint32
	// 统计周期，默认为1分钟，不超过10分钟（避免周期过长，存储统计数据的变量溢出），单位ms
	StatisticalCycle int
	// 成功率连续不达标多少个统计周期发出告警，默认3
	AlertForBadSuccessRateReachedTimes int
	// 耗时连续不达标多少个统计周期发出告警，默认3
	AlertForBadFastRateReachedTimes	int
	// 成功率连续达标多少个统计周期发出恢复报告，默认3
	AlertForGreatSuccessRateReachedTimes int
	// 耗时连续达标多少个统计周期发出恢复报告，默认3
	AlertForGreatFastRateReachedTimes	int
	// 成功率多少以上算通过，1表示100%，默认0.95
	SuccessRate	float64
	// 高效访问率多少以上算通过，1表示100%，默认0.8
	FastRate float64
	// 上报管道的缓存个数，默认为100
	ChannelCacheCount int
	// 判定code是否成功的依据，默认为 {200: { Success: true }}，取白名单机制，除此处定义的以外，统统认为失败。当然，如果有必要自定义Name属性，也可以定义一些失败的code
	CodeFeatureMap map[int]CodeFeature
	// 自定义获取code属性的方式，优先于CodeFeatureMap
	GetCodeFeature func(code int) (success bool, name string)
	// 失败分布统计出报表时，如果没有在DefaultSuccessStatus定义过该状态的Name属性，将默认将DefaultFailDistributionFormat中的%code转化为对应的code并作为报表项，该值默认为"code[%code]"
	DefaultFailDistributionFormat string
	// 接受数据输出定制，默认输出到控制台
	OutputCaller func(o *OutPutData)
	// 告警处理方式定制，默认输出到控制台，目前alertType取值为FAIL代表成功率告警，SLOW代表耗时告警
	AlertCaller func(clientName string, interfaceName string, alertType AlertType, recentOutputData []OutPutData)
	// 恢复通知处理方式定制，同AlertCaller
	RecoverCaller func(clientName string, interfaceName string, alertType AlertType, recentOutputData []OutPutData)

	// 自定义url或命名关于耗时达标，分布区间等属性。为了维持内部key的一致性，需要调用方法来设置这个属性
	entryConfigMap map[string]EntryConfig
	// 存储成功率告警以及恢复相关的数据
	recentSuccessRateStatus map[string]*alertStatus
	// 存储时延达标率以及恢复相关的数据
	recentFastRateStatus map[string]*alertStatus
	// 上报通道，channel有利于解决资源竞争和缓存计算问题
	taskChannel chan *taskQueue
	// 收集累计每个条目的上报数据，用于统计分析
	collectDataMap map[string]*reportData
	// 分析通道
	statisticsChannel chan reportData
}

// 状态码定制
type CodeFeature struct {
	Success bool			// 是否计为成功，默认为false
	Name string				// 命名，用于出报表数据
}

// 使用上报必须先注册，得到一个唯一的客户端再进行上报
func Register(c ReportClientConfig) ReportClient {
	if c.Name == "" {
		panic("必须为该上报类型注册一个名称")
	}
	// 最大允许5分钟一个统计周期
	if c.StatisticalCycle <= 0 || c.StatisticalCycle > 300000 {
		c.StatisticalCycle = 60000
	}
	if c.AlertForBadFastRateReachedTimes < 3 {
		c.AlertForBadFastRateReachedTimes = 3
	}
	if c.AlertForGreatFastRateReachedTimes < 3 {
		c.AlertForGreatFastRateReachedTimes = 3
	}
	if c.AlertForBadSuccessRateReachedTimes < 3 {
		c.AlertForBadSuccessRateReachedTimes = 3
	}
	if c.AlertForGreatSuccessRateReachedTimes < 3 {
		c.AlertForGreatSuccessRateReachedTimes = 3
	}
	if c.SuccessRate == 0 {
		c.SuccessRate = 0.95
	}
	if c.FastRate == 0 {
		c.FastRate = 0.8
	}
	if c.ChannelCacheCount <= 0 {
		c.ChannelCacheCount = 100
	}
	if c.DefaultFastTime > 0 {
		defaultEntryConfig.FastLessThan = c.DefaultFastTime
	}
	if c.DefaultFailDistributionFormat == "" {
		c.DefaultFailDistributionFormat = "code[%code]"
	}
	c.entryConfigMap = map[string]EntryConfig {}
	c.recentFastRateStatus = map[string]*alertStatus {}
	c.recentSuccessRateStatus = map[string]*alertStatus {}
	// 如果没有指定自定义code特征识别函数，且状态码映射为空，则启用默认的机制
	if c.GetCodeFeature == nil && c.CodeFeatureMap == nil {
		c.CodeFeatureMap = map[int]CodeFeature {
			200: {
				Success: true,
			},
		}
	}
	client := &c
	// 建立一条带缓存的channel信道，上报的数据流经通道以支持串行处理（避免并发锁）
	client.taskChannel = make(chan *taskQueue, c.ChannelCacheCount)
	// 建立一条统计分析的channel通道
	client.statisticsChannel = make(chan reportData, c.ChannelCacheCount)
	client.collectDataMap = map[string]*reportData {}
	// 启动收集模块
	client.collect()
	// 启动定时器任务
	client.scheduleTask()
	// 启动统计分析模块
	client.statistics()
	return client
}

// 默认输出回调函数，将直接打印到控制台
func defaultOutputCaller(o *OutPutData) {
	b, err := json.Marshal(*o)
	if err != nil {
		os.Stderr.WriteString(err.Error())
	} else {
		os.Stdout.Write(b)
		os.Stdout.WriteString("\n")
	}
}